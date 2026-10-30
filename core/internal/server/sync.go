package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
)

func (s *Server) objects(kind string) ([]json.RawMessage, error) {
	rows, err := s.db.Query("SELECT data FROM sg_objects WHERE kind=? ORDER BY id", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []json.RawMessage{}
	for rows.Next() {
		var raw string
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		result = append(result, json.RawMessage(raw))
	}
	return result, rows.Err()
}
func (s *Server) object(kind, id string, out any) error {
	var raw string
	err := s.db.QueryRow("SELECT data FROM sg_objects WHERE kind=? AND id=?", kind, id).Scan(&raw)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), out)
}
func (s *Server) saveObject(kind, id string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES(?,?,?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", kind, id, string(raw), time.Now().Unix())
	return err
}
func (s *Server) syncPanel(ctx context.Context) error {
	return s.syncPanelWithAudit(ctx, nil)
}

func (s *Server) syncPanelWithAudit(ctx context.Context, audit func(*sql.Tx) error) error {
	if s.config.Demo {
		if audit == nil {
			return nil
		}
		return s.enrollmentTransaction(audit)
	}
	s.syncMu.Lock()
	revision := s.syncRevision
	fetchStarted := time.Now()
	s.syncMu.Unlock()
	nodes, err := s.panel.ListNodesContext(ctx)
	users := []panel.User{}
	var hosts []panel.Host
	if err == nil {
		for offset := 0; ; {
			page, total, e := s.panel.ListUsersContext(ctx, offset, 1000)
			if e != nil {
				err = e
				break
			}
			if total < 0 || total > 1000000 || offset+len(page) > total {
				err = fmt.Errorf("panel pagination total is invalid or exceeds the 1000000-user limit")
				break
			}
			users = append(users, page...)
			offset += len(page)
			if offset >= total {
				break
			}
			if len(page) == 0 {
				err = fmt.Errorf("incomplete panel pagination")
				break
			}
		}
	}
	if err == nil {
		if result, e := s.panel.ListHostsContext(ctx); e == nil {
			hosts = result
		} else {
			s.logger.Warn("host mapping unavailable; node address matching only", "error", e)
		}
	}
	if err == nil {
		s.syncMu.Lock()
		defer s.syncMu.Unlock()
		if revision != s.syncRevision {
			return fmt.Errorf("panel snapshot superseded by a webhook or another sync")
		}
		tx, e := s.db.BeginTx()
		if e != nil {
			err = e
		} else {
			defer s.db.TxUnlock()
			defer tx.Rollback()
			// A complete snapshot is committed atomically; failed fetches keep the last-good view.
			_, err = tx.Exec("DELETE FROM sg_objects WHERE kind IN ('nodes','users','hosts')")
			for _, n := range nodes {
				if err != nil {
					break
				}
				raw, _ := json.Marshal(n)
				_, err = tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('nodes',?,?,?)", n.ID, string(raw), time.Now().Unix())
				if err == nil {
					_, err = tx.Exec("INSERT INTO nodes(id,name,ip_address,status,last_sync) VALUES(?,?,?,?,CURRENT_TIMESTAMP) ON CONFLICT(id) DO UPDATE SET name=excluded.name,ip_address=excluded.ip_address,status=excluded.status,last_sync=CURRENT_TIMESTAMP", n.ID, n.Name, n.IPAddress, n.Status)
				}
			}
			for _, u := range users {
				if err != nil {
					break
				}
				raw, _ := json.Marshal(u)
				_, err = tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('users',?,?,?)", u.ID, string(raw), time.Now().Unix())
			}
			for _, h := range hosts {
				if err != nil {
					break
				}
				raw, _ := json.Marshal(h)
				_, err = tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('hosts',?,?,?)", h.ID, string(raw), time.Now().Unix())
			}
			if err == nil {
				// Global watermark also covers users removed or absent in the full
				// snapshot, preventing delayed events from resurrecting them.
				_, err = tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('webhook-sync-clock','users',?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", strconv.FormatInt(fetchStarted.UnixNano(), 10), time.Now().Unix())
			}
			if err == nil && audit != nil {
				err = audit(tx)
			}
			if err == nil {
				err = tx.Commit()
				if err == nil {
					s.syncRevision++
				}
			}
		}
	}
	s.stateMu.Lock()
	if err != nil {
		s.syncError = err.Error()
	} else {
		s.lastSync = time.Now()
		s.syncError = ""
	}
	s.stateMu.Unlock()
	return err
}
func (s *Server) background() {
	defer s.wg.Done()
	if !s.config.Demo {
		versionCtx, versionCancel := context.WithTimeout(s.ctx, 3*time.Second)
		s.checkPanelVersion(versionCtx)
		versionCancel()
		ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
		if err := s.syncPanel(ctx); err != nil {
			s.logger.Warn("initial panel sync failed", "error", err)
		}
		cancel()
	}
	ticker := time.NewTicker(s.config.Panel.SyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if !s.config.Demo {
				ctx, cancel := context.WithTimeout(s.ctx, 2*time.Minute)
				if err := s.syncPanel(ctx); err != nil {
					s.logger.Warn("panel sync failed", "error", err)
				}
				cancel()
			}
			_, _ = s.db.Exec("DELETE FROM sg_sessions WHERE expires_at<?", time.Now().Unix())
			_, _ = s.db.Exec("DELETE FROM sg_enrollment WHERE expires_at<?", time.Now().Unix())
			_, _ = s.db.Exec("DELETE FROM sg_webhooks WHERE timestamp<?", time.Now().Add(-24*time.Hour).Unix())
			_, _ = s.db.Exec("DELETE FROM sg_snapshots WHERE stored_at<?", time.Now().Add(-5*time.Minute).Unix())
			_, _ = s.db.Exec("DELETE FROM sg_speed WHERE timestamp<?", time.Now().Add(-24*time.Hour).UnixMilli())
			ttl := s.config.Storage.ActivityJournal.HotTTL
			if ttl <= 0 {
				ttl = 14 * 24 * time.Hour
			}
			_, _ = s.db.Exec("DELETE FROM sg_activity WHERE timestamp<?", time.Now().Add(-ttl).UnixMilli())
		}
	}
}
func (s *Server) syncHandler(w http.ResponseWriter, r *http.Request) {
	p := admin(r)
	if err := s.syncPanelWithAudit(r.Context(), func(tx *sql.Tx) error { return s.auditTx(tx, r, &p, "panel.sync", "panel", "", "success") }); err != nil {
		if auditErr := s.recordAudit(r, &p, "panel.sync", "panel", "", "failure"); auditErr != nil {
			fail(w, 500, "audit unavailable")
			return
		}
		fail(w, 502, "panel sync failed")
		return
	}
	if err := s.gateway.Clear(); err != nil {
		// The snapshot and success outcome are committed. Surface a separate
		// cache failure instead of claiming the snapshot transaction failed.
		if auditErr := s.recordAudit(r, &p, "panel.sync.cache", "panel", "", "failure"); auditErr != nil {
			fail(w, 500, "audit unavailable")
			return
		}
		fail(w, 500, "cache invalidation failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) webhook(w http.ResponseWriter, r *http.Request) {
	if s.config.Panel.WebhookKey == "" {
		fail(w, 503, "webhook secret is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		fail(w, 413, "webhook exceeds limit")
		return
	}
	signature := strings.TrimPrefix(r.Header.Get("X-Remnawave-Signature"), "sha256=")
	if signature == "" {
		signature = strings.TrimPrefix(r.Header.Get("X-Webhook-Signature"), "sha256=")
	}
	provided, e := hex.DecodeString(signature)
	mac := hmac.New(sha256.New, []byte(s.config.Panel.WebhookKey))
	mac.Write(body)
	if e != nil || !hmac.Equal(provided, mac.Sum(nil)) {
		fail(w, 401, "invalid signature")
		return
	}
	var event struct {
		Event     string          `json:"event"`
		Type      string          `json:"type"`
		Timestamp json.RawMessage `json:"timestamp"`
		Data      json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(body, &event); err != nil || len(event.Data) == 0 || (event.Event == "" && event.Type == "") {
		fail(w, 400, "invalid webhook event")
		return
	}
	var timestamp time.Time
	var value string
	if json.Unmarshal(event.Timestamp, &value) == nil {
		timestamp, _ = time.Parse(time.RFC3339, value)
	} else {
		var n int64
		if json.Unmarshal(event.Timestamp, &n) == nil {
			if n > 1e12 {
				timestamp = time.UnixMilli(n)
			} else {
				timestamp = time.Unix(n, 0)
			}
		}
	}
	if timestamp.IsZero() || time.Since(timestamp) > 24*time.Hour || time.Until(timestamp) > 5*time.Minute {
		fail(w, 400, "webhook timestamp outside replay window")
		return
	}
	eventType := event.Event
	if eventType == "" {
		eventType = event.Type
	}
	var user *panel.User
	if eventType == "user.updated" {
		user, err = panel.DecodeWebhookUser(event.Data)
		if err != nil {
			fail(w, 400, "invalid webhook user")
			return
		}
	}
	// A full sync must not overwrite a webhook with an older snapshot fetched
	// before delivery. Serialize both paths through syncMu.
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	key := hash(string(body))
	var identity struct {
		ID json.Number `json:"id"`
	}
	if user != nil {
		_ = json.Unmarshal(event.Data, &identity)
	}
	duplicate, err := s.persistWebhook(key, user, string(identity.ID), timestamp)
	if err != nil {
		fail(w, 500, "webhook persistence failed")
		return
	}
	if duplicate {
		writeJSON(w, 200, map[string]bool{"duplicate": true})
		return
	}
	s.syncRevision++
	if err = s.gateway.Clear(); err != nil {
		// The user upsert is idempotent; allow panel retry if persistent cache
		// invalidation failed after committing it.
		_, _ = s.db.Exec("DELETE FROM sg_webhooks WHERE key=?", key)
		fail(w, 500, "cache invalidation failed")
		return
	}
	// Unimplemented event types only invalidate cache, retaining reconciliation
	// as the fallback. No user fields are inferred from their payloads.
	writeJSON(w, 202, map[string]bool{"accepted": true})
}

func (s *Server) persistWebhook(key string, user *panel.User, numericID string, timestamp time.Time) (bool, error) {
	tx, err := s.db.BeginTx()
	if err != nil {
		return false, err
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	result, err := tx.Exec("INSERT OR IGNORE INTO sg_webhooks(key,timestamp) VALUES(?,?)", key, time.Now().Unix())
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return true, nil
	}
	if user != nil {
		var watermark string
		err = tx.QueryRow("SELECT data FROM sg_objects WHERE kind='webhook-sync-clock' AND id='users'").Scan(&watermark)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if err == nil {
			previous, parseErr := strconv.ParseInt(watermark, 10, 64)
			if parseErr != nil {
				return false, parseErr
			}
			if timestamp.UnixNano() <= previous {
				return false, tx.Commit()
			}
		}
		// Preserve the canonical numeric identity when the REST schema omitted
		// uuid, but only if both native numeric id and shortUuid agree. Ambiguous
		// or conflicting identities are rejected instead of creating aliases.
		rows, queryErr := tx.Query("SELECT id FROM sg_objects WHERE kind='users' AND json_extract(data,'$.short_uuid')=?", user.ShortUUID)
		if queryErr != nil {
			return false, queryErr
		}
		var canonical string
		count := 0
		for rows.Next() {
			if err := rows.Scan(&canonical); err != nil {
				rows.Close()
				return false, err
			}
			count++
		}
		queryErr = rows.Err()
		rows.Close()
		if queryErr != nil {
			return false, queryErr
		}
		if count > 1 {
			return false, errors.New("ambiguous webhook user identity")
		}
		if count == 1 && canonical != user.ID {
			if canonical != numericID {
				return false, errors.New("conflicting webhook user identity")
			}
			user.ID = canonical
		}
		var last string
		err = tx.QueryRow("SELECT data FROM sg_objects WHERE kind='webhook-user-clocks' AND id=?", user.ID).Scan(&last)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		if err == nil {
			previous, parseErr := strconv.ParseInt(last, 10, 64)
			if parseErr != nil {
				return false, parseErr
			}
			if timestamp.UnixNano() <= previous {
				return false, tx.Commit()
			}
		}
		_, err = tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('webhook-user-clocks',?,?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", user.ID, strconv.FormatInt(timestamp.UnixNano(), 10), time.Now().Unix())
		if err != nil {
			return false, err
		}
		raw, marshalErr := json.Marshal(user)
		if marshalErr != nil {
			return false, marshalErr
		}
		_, err = tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('users',?,?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", user.ID, string(raw), time.Now().Unix())
		if err != nil {
			return false, err
		}
	}
	return false, tx.Commit()
}
