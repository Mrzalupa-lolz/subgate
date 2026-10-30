package server

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type principal struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
}
type contextKey string

const adminKey contextKey = "admin"
const nodeKey contextKey = "node"

func passwordHash(password string) string {
	salt := token()
	value, _ := pbkdf2.Key(sha256.New, password, []byte(salt), 210000, 32)
	return "pbkdf2-sha256$210000$" + salt + "$" + hex.EncodeToString(value)
}
func verifyPassword(password, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 4 || parts[0] != "pbkdf2-sha256" || parts[1] != "210000" {
		return false
	}
	value, err := pbkdf2.Key(sha256.New, password, []byte(parts[2]), 210000, 32)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(hex.EncodeToString(value)), []byte(parts[3])) == 1
}
func (s *Server) bootstrapAdmin() error {
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM admin_users").Scan(&count); err != nil {
		return err
	}
	if count > 1 {
		return fmt.Errorf("single-owner installation requires exactly one administrator; resolve legacy extra accounts before starting")
	}
	if count == 1 {
		return nil
	}
	password := s.config.Auth.AdminPassword
	if password == "" && s.config.Demo {
		password = "subgate-demo"
	}
	if len(password) < 12 {
		return fmt.Errorf("set SUBGATE_ADMIN_PASSWORD (at least 12 characters) to create the first administrator")
	}
	_, err := s.db.Exec("INSERT INTO admin_users(username,password_hash,role) VALUES(?,?,'superadmin')", s.config.Auth.AdminUsername, passwordHash(password))
	return err
}
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	if !s.validOrigin(r) {
		fail(w, 403, "invalid request origin")
		return
	}
	ip := s.clientIP(r)
	s.loginMu.Lock()
	a := s.attempts[ip]
	if time.Since(a.start) > time.Minute {
		a = attempt{start: time.Now()}
	}
	a.count++
	s.attempts[ip] = a
	if len(s.attempts) > 10000 {
		for k, v := range s.attempts {
			if time.Since(v.start) > time.Minute {
				delete(s.attempts, k)
			}
		}
	}
	s.loginMu.Unlock()
	if a.count > s.config.Auth.LoginRateLimit {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "too many login attempts")
		return
	}
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &input) {
		return
	}
	if len(input.Password) > 1024 || len(input.Username) > 128 {
		fail(w, 400, "credentials exceed limits")
		return
	}
	var p principal
	var stored string
	var enabled int
	err := s.db.QueryRow("SELECT id,username,password_hash,enabled FROM admin_users WHERE username=? AND (SELECT COUNT(*) FROM admin_users)=1", input.Username).Scan(&p.ID, &p.Username, &stored, &enabled)
	if err != nil {
		stored = "pbkdf2-sha256$210000$" + strings.Repeat("0", 64) + "$" + strings.Repeat("0", 64)
	}
	valid := verifyPassword(input.Password, stored)
	if err != nil || !valid || enabled != 1 {
		s.recordAudit(r, nil, "auth.login", "admin", "", "failure")
		fail(w, 401, "invalid credentials")
		return
	}
	raw := token()
	expiry := time.Now().Add(s.config.Auth.SessionTimeout)
	if _, err = s.db.Exec("INSERT INTO sg_sessions(token_hash,admin_id,expires_at) VALUES(?,?,?)", hash(raw), p.ID, expiry.Unix()); err != nil {
		fail(w, 500, "session could not be created")
		return
	}
	_, _ = s.db.Exec("UPDATE admin_users SET last_login=CURRENT_TIMESTAMP WHERE id=?", p.ID)
	http.SetCookie(w, &http.Cookie{Name: "subgate_session", Value: raw, Path: "/", HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteStrictMode, Expires: expiry})
	s.recordAudit(r, &p, "auth.login", "admin", p.Username, "success")
	writeJSON(w, 200, p)
}
func sessionToken(r *http.Request) (string, bool) {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer "), true
	}
	if cookie, err := r.Cookie("subgate_session"); err == nil {
		return cookie.Value, false
	}
	return "", false
}
func (s *Server) secureCookie(r *http.Request) bool {
	return r.TLS != nil || strings.HasPrefix(s.config.HTTP.PublicURL, "https://")
}
func (s *Server) validOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	supplied, err := url.Parse(origin)
	if err != nil || supplied.User != nil || supplied.RawQuery != "" || supplied.Fragment != "" || supplied.Path != "" || supplied.Host == "" {
		return false
	}
	expectedScheme := "http"
	expectedHost := r.Host
	if r.TLS != nil {
		expectedScheme = "https"
	}
	if s.config.HTTP.PublicURL != "" {
		expected, err := url.Parse(s.config.HTTP.PublicURL)
		if err != nil {
			return false
		}
		expectedScheme, expectedHost = expected.Scheme, expected.Host
	}
	return supplied.Scheme == expectedScheme && strings.EqualFold(supplied.Host, expectedHost)
}
func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, bearer := sessionToken(r)
		if raw == "" {
			fail(w, 401, "authentication required")
			return
		}
		var p principal
		err := s.db.QueryRow("SELECT a.id,a.username FROM sg_sessions s JOIN admin_users a ON a.id=s.admin_id WHERE s.token_hash=? AND s.expires_at>? AND a.enabled=1 AND (SELECT COUNT(*) FROM admin_users)=1", hash(raw), time.Now().Unix()).Scan(&p.ID, &p.Username)
		if err != nil {
			fail(w, 401, "session expired")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if !bearer {
				if !s.validOrigin(r) {
					fail(w, 403, "invalid request origin")
					return
				}
				if r.Header.Get("X-SubGate-CSRF") != "1" {
					fail(w, 403, "CSRF header required")
					return
				}
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminKey, p)))
	})
}
func admin(r *http.Request) principal                       { p, _ := r.Context().Value(adminKey).(principal); return p }
func (s *Server) me(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, admin(r)) }
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	raw, _ := sessionToken(r)
	if _, err := s.db.Exec("DELETE FROM sg_sessions WHERE token_hash=?", hash(raw)); err != nil {
		fail(w, 500, "logout failed")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "subgate_session", Path: "/", HttpOnly: true, Secure: s.secureCookie(r), SameSite: http.SameSiteStrictMode, MaxAge: -1})
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) recordAudit(r *http.Request, p *principal, action, resource, id, status string) error {
	var adminID any
	if p != nil {
		adminID = p.ID
	}
	_, err := s.db.Exec("INSERT INTO audit_log(admin_id,action,resource,resource_id,status,ip_address) VALUES(?,?,?,?,?,?)", adminID, action, resource, id, status, s.clientIP(r))
	if err != nil {
		s.logger.Error("audit write failed", "error", err)
	}
	return err
}
func (s *Server) auditAction(r *http.Request, action, kind, id string) error {
	p := admin(r)
	return s.recordAudit(r, &p, action, kind, id, "success")
}

// auditTx commits a final outcome with its local persisted mutation. Callers
// must roll back when this write fails, and never include credentials in fields.
func (s *Server) auditTx(tx *sql.Tx, r *http.Request, p *principal, action, resource, id, status string) error {
	var adminID any
	if p != nil {
		adminID = p.ID
	}
	_, err := tx.Exec("INSERT INTO audit_log(admin_id,action,resource,resource_id,status,ip_address) VALUES(?,?,?,?,?,?)", adminID, action, resource, id, status, s.clientIP(r))
	return err
}
