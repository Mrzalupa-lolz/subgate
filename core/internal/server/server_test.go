package server

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/config"
	"github.com/subgate/subgate/core/internal/db"
	"github.com/subgate/subgate/pkg/wire"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	cfg := config.Defaults()
	cfg.Demo = true
	cfg.Database.URL = ":memory:"
	cfg.Database.BackupPath = t.TempDir()
	cfg.Auth.AdminPassword = "secure-test-password"
	cfg.Auth.LoginRateLimit = 100
	database, err := db.New(cfg.Database.URL)
	if err != nil {
		t.Fatal(err)
	}
	if err = db.RunMigrations(database); err != nil {
		database.Close()
		t.Fatal(err)
	}
	s, err := New(cfg, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Shutdown(context.Background()); database.Close() })
	return s
}
func perform(s *Server, method, path string, input any, session *http.Cookie, bearer string, csrf bool) *httptest.ResponseRecorder {
	var body io.Reader
	if input != nil {
		raw, _ := json.Marshal(input)
		body = bytes.NewReader(raw)
	}
	r := httptest.NewRequest(method, "http://console.test"+path, body)
	r.Header.Set("Content-Type", "application/json")
	if session != nil {
		r.AddCookie(session)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	if csrf {
		r.Header.Set("X-SubGate-CSRF", "1")
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func loginTest(t *testing.T, s *Server) *http.Cookie {
	t.Helper()
	w := perform(s, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "secure-test-password"}, nil, "", false)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	var p map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p["username"] != "admin" || p["role"] != nil || p["id"] == nil {
		t.Fatalf("principal response fields: %v", p)
	}
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == "subgate_session" {
			if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
				t.Fatalf("cookie security: %+v", cookie)
			}
			return cookie
		}
	}
	t.Fatal("session cookie missing")
	return nil
}

func TestAuthenticationCSRFAndOwnerEnforcement(t *testing.T) {
	s := newTestServer(t)
	if w := perform(s, "GET", "/api/v1/users", nil, nil, "", false); w.Code != 401 {
		t.Fatalf("unauthenticated read: %d", w.Code)
	}
	if w := perform(s, "POST", "/api/v1/auth/login", map[string]string{"username": "admin", "password": "wrong"}, nil, "", false); w.Code != 401 {
		t.Fatalf("incorrect login: %d", w.Code)
	}
	session := loginTest(t, s)
	if w := perform(s, "POST", "/api/v1/nodes/demo-node-1/actions/disable", nil, session, "", false); w.Code != 403 {
		t.Fatalf("missing CSRF accepted: %d", w.Code)
	}
	for _, origin := range []string{"http://evil.test", "https://console.test", "null"} {
		r := httptest.NewRequest("POST", "http://console.test/api/v1/nodes/demo-node-1/actions/disable", nil)
		r.AddCookie(session)
		r.Header.Set("Origin", origin)
		r.Header.Set("X-SubGate-CSRF", "1")
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("cross origin %s accepted: %d", origin, w.Code)
		}
	}
	if w := perform(s, "POST", "/api/v1/nodes/demo-node-1/actions/disable", nil, session, "", true); w.Code != 200 {
		t.Fatalf("authorized mutation: %d %s", w.Code, w.Body)
	}
	if _, err := s.db.Exec("UPDATE admin_users SET role='viewer' WHERE username='admin'"); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "GET", "/api/v1/users", nil, session, "", false); w.Code != 200 {
		t.Fatalf("owner read: %d", w.Code)
	}
	if w := perform(s, "GET", "/api/v1/activity", nil, session, "", false); w.Code != 200 {
		t.Fatalf("owner activity blocked by legacy role: %d", w.Code)
	}
	if w := perform(s, "POST", "/api/v1/nodes/demo-node-1/actions/enable", nil, nil, session.Value, false); w.Code != 200 {
		t.Fatalf("owner bearer write blocked by legacy role: %d", w.Code)
	}
	if w := perform(s, "POST", "/api/v1/auth/logout", nil, session, "", true); w.Code != 200 {
		t.Fatalf("owner cannot logout: %d %s", w.Code, w.Body)
	}
	if w := perform(s, "GET", "/api/v1/auth/me", nil, session, "", false); w.Code != 401 {
		t.Fatalf("logout did not invalidate cookie: %d", w.Code)
	}
	var failures int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='auth.login' AND status='failure'").Scan(&failures); err != nil || failures != 1 {
		t.Fatalf("failed login audit: %d %v", failures, err)
	}
}

func TestSecureCookieTracksPublicEndpoint(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.URL = "https://panel.example.com"
	if cookie := loginTest(t, s); cookie.Secure {
		t.Fatal("upstream panel HTTPS incorrectly secures local HTTP cookie")
	}
	s.config.HTTP.PublicURL = "https://console.example.com"
	if cookie := loginTest(t, s); !cookie.Secure {
		t.Fatal("public HTTPS origin requires Secure cookie")
	}
	r := httptest.NewRequest("POST", "http://console.test/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"secure-test-password"}`))
	r.Header.Set("Origin", "https://console.example.com")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("public proxy origin rejected: %d %s", w.Code, w.Body)
	}
	r = httptest.NewRequest("POST", "http://console.test/api/v1/auth/login", strings.NewReader(`{"username":"admin","password":"secure-test-password"}`))
	r.Header.Set("Origin", "https://attacker.example.com")
	w = httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross origin login accepted: %d", w.Code)
	}
	if cookie := loginTest(t, s); perform(s, "POST", "/api/v1/auth/logout", nil, nil, cookie.Value, false).Code != 200 {
		t.Fatal("bearer logout failed")
	} else if w := perform(s, "GET", "/api/v1/auth/me", nil, nil, cookie.Value, false); w.Code != 401 {
		t.Fatalf("bearer survives logout: %d", w.Code)
	}
}

func enrollTest(t *testing.T, s *Server, session *http.Cookie, nodeID string) (string, string) {
	t.Helper()
	w := perform(s, "POST", "/api/v1/agents/enrollment-tokens", map[string]string{"node_id": nodeID}, session, "", true)
	if w.Code != 201 {
		t.Fatalf("enrollment token: %d %s", w.Code, w.Body)
	}
	var issued struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	w = perform(s, "POST", "/api/v1/agents/enroll", map[string]string{"node_id": nodeID, "token": issued.Token}, nil, "", false)
	if w.Code != 201 {
		t.Fatalf("agent enroll: %d %s", w.Code, w.Body)
	}
	var credential struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &credential); err != nil {
		t.Fatal(err)
	}
	if credential.Token == "" || credential.Token == issued.Token {
		t.Fatal("invalid issued agent token")
	}
	return credential.Token, issued.Token
}

func TestEnrollmentOneTimeIdentityRevocation(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, issued := enrollTest(t, s, session, "demo-node-1")
	if w := perform(s, "POST", "/api/v1/agents/enroll", map[string]string{"node_id": "demo-node-1", "token": issued}, nil, "", false); w.Code != 401 {
		t.Fatalf("enrollment token reused: %d", w.Code)
	}
	if w := perform(s, "POST", "/api/v1/agents/heartbeat", wire.Heartbeat{NodeID: "demo-node-2"}, nil, credential, false); w.Code != 403 {
		t.Fatalf("node identity forgery: %d", w.Code)
	}
	hb := wire.Heartbeat{NodeID: "demo-node-1", Version: "test", MemoryMB: 25, UptimeSeconds: 100, Timestamp: time.Now()}
	if w := perform(s, "POST", "/api/v1/agents/heartbeat", hb, nil, credential, false); w.Code != 200 {
		t.Fatalf("heartbeat: %d %s", w.Code, w.Body)
	}
	var stored string
	if err := s.db.QueryRow("SELECT token_hash FROM agents WHERE node_id='demo-node-1'").Scan(&stored); err != nil || stored != hash(credential) || stored == credential {
		t.Fatalf("credential storage: %s %v", stored, err)
	}
	if w := perform(s, "DELETE", "/api/v1/agents/demo-node-1", nil, session, "", true); w.Code != 200 {
		t.Fatalf("revocation: %d %s", w.Code, w.Body)
	}
	if w := perform(s, "POST", "/api/v1/agents/heartbeat", hb, nil, credential, false); w.Code != 401 {
		t.Fatalf("revoked credential accepted: %d", w.Code)
	}
	newCredential, _ := enrollTest(t, s, session, "demo-node-1")
	if newCredential == credential {
		t.Fatal("reenrollment reused credential")
	}
	if w := perform(s, "POST", "/api/v1/agents/heartbeat", hb, nil, credential, false); w.Code != 401 {
		t.Fatalf("old credential survives reenrollment: %d", w.Code)
	}
}

func TestTelemetryIdempotencyEncryptionAndReadAudit(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	now := time.Now().UTC()
	batch := wire.Telemetry{BatchID: "batch-1", NodeID: "demo-node-1", Samples: []wire.Sample{{UserID: "1", Timestamp: now, UplinkBps: 1000, DownlinkBps: 2000}}, Events: []wire.Activity{{UserID: "1", Timestamp: now, SrcIP: "198.51.100.1", DstHost: "private-destination.example.com", DstPort: 443, Network: "tcp", Result: "accepted"}}}
	for i := 0; i < 2; i++ {
		w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false)
		if w.Code != 200 {
			t.Fatalf("telemetry: %d %s", w.Code, w.Body)
		}
		if i == 1 && !strings.Contains(w.Body.String(), `"duplicate":true`) {
			t.Fatalf("repeated batch not duplicate: %s", w.Body)
		}
	}
	var activities, samples int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM sg_activity").Scan(&activities)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM sg_speed").Scan(&samples)
	if activities != 1 || samples != 1 {
		t.Fatalf("duplicate rows: activity=%d samples=%d", activities, samples)
	}
	var encrypted []byte
	if err := s.db.QueryRow("SELECT payload FROM sg_activity").Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encrypted, []byte("private-destination")) || json.Valid(encrypted) {
		t.Fatal("activity stored as plaintext")
	}
	if _, err := s.decrypt(encrypted, "demo-node-2:1"); err == nil {
		t.Fatal("ciphertext not bound to node identity")
	}
	w := perform(s, "GET", "/api/v1/activity?user=1&domain=private-destination", nil, session, "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), "private-destination.example.com") {
		t.Fatalf("activity read: %d %s", w.Code, w.Body)
	}
	var audits int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='activity.read' AND resource_id='1'").Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("activity read unaudited: %d %v", audits, err)
	}
	w = perform(s, "GET", "/api/v1/live", nil, session, "", false)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"uplink_bps":1000`) || !strings.Contains(w.Body.String(), `"downlink_bps":2000`) {
		t.Fatalf("live persisted sample: %d %s", w.Code, w.Body)
	}
	batch.BatchID = "invalid"
	batch.Samples[0].UplinkBps = -1
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 400 {
		t.Fatalf("negative speed accepted: %d", w.Code)
	}
	batch.Samples[0].UplinkBps = 1
	batch.NodeID = "demo-node-2"
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 403 {
		t.Fatalf("telemetry node forgery: %d", w.Code)
	}
	if w := perform(s, "GET", "/api/v1/activity?from=bad-date", nil, session, "", false); w.Code != 400 {
		t.Fatalf("invalid date accepted: %d", w.Code)
	}
	if _, err := s.db.Exec("DROP TABLE audit_log"); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "GET", "/api/v1/activity", nil, session, "", false); w.Code != 500 {
		t.Fatalf("activity read with unavailable audit: %d", w.Code)
	}
}

func TestEncryptedBackupRestoresRealDatabase(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	batch := wire.Telemetry{BatchID: "backup-batch", NodeID: "demo-node-1", Events: []wire.Activity{{UserID: "1", Timestamp: time.Now().UTC(), DstHost: "backup-private.example.com", DstPort: 443, Network: "tcp"}}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	w := perform(s, "POST", "/api/v1/backups", nil, session, "", true)
	if w.Code != 201 {
		t.Fatalf("create backup: %d %s", w.Code, w.Body)
	}
	var result struct {
		Name      string `json:"name"`
		Encrypted bool   `json:"encrypted"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || !result.Encrypted {
		t.Fatalf("backup response: %s %v", w.Body, err)
	}
	w = perform(s, "GET", "/api/v1/backups/"+result.Name, nil, session, "", false)
	if w.Code != 200 {
		t.Fatalf("download backup: %d %s", w.Code, w.Body)
	}
	data := w.Body.Bytes()
	if !bytes.HasPrefix(data, []byte("SUBGATE1")) || bytes.Contains(data, []byte("SQLite format")) {
		t.Fatal("backup missing encrypted format")
	}
	raw, err := s.decrypt(data[8:], "subgate-backup-v1")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "restored.sqlite")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	restored, err := db.New("file:" + filepath.ToSlash(path))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var payload []byte
	if err = restored.QueryRow("SELECT payload FROM sg_activity WHERE user_id='1'").Scan(&payload); err != nil {
		t.Fatal(err)
	}
	plain, err := s.decrypt(payload, "demo-node-1:1")
	if err != nil || !bytes.Contains(plain, []byte("backup-private.example.com")) {
		t.Fatalf("backup lost original activity: %s %v", plain, err)
	}
	var count int
	if err = restored.QueryRow("SELECT COUNT(*) FROM agents WHERE node_id='demo-node-1'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("backup lost enrolled agent: %d %v", count, err)
	}
	data[len(data)-1] ^= 1
	if _, err = s.decrypt(data[8:], "subgate-backup-v1"); err == nil {
		t.Fatal("corrupted backup accepted")
	}
}

func TestCSVExportEscapesSpreadsheetFormula(t *testing.T) {
	w := httptest.NewRecorder()
	csvRows(w, [][]string{{"user", "destination"}, {"=CMD()", "@hidden"}, {"normal", "safe.example.com"}})
	rows, err := csv.NewReader(w.Body).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if rows[1][0] != "'=CMD()" || rows[1][1] != "'@hidden" || rows[2][0] != "normal" {
		t.Fatalf("unsafe CSV: %v", rows)
	}
}
