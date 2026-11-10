package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
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
	s.syncMu.Unlock()
	nodes, err := s.panel.ListNodesContext(ctx)
	users := []panel.User{}
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
		s.syncMu.Lock()
		defer s.syncMu.Unlock()
		if revision != s.syncRevision {
			return fmt.Errorf("panel snapshot superseded by another sync")
		}
		tx, e := s.db.BeginTx()
		if e != nil {
			err = e
		} else {
			defer s.db.TxUnlock()
			defer tx.Rollback()
			// A complete snapshot is committed atomically; failed fetches keep the last-good view.
			_, err = tx.Exec("DELETE FROM sg_objects WHERE kind IN ('nodes','users')")
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
			if err == nil {
				// Drop the watermark left by the removed panel webhook receiver.
				_, err = tx.Exec("DELETE FROM sg_objects WHERE kind='webhook-sync-clock'")
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
func (s *Server) startBackground() {
	s.wg.Add(1)
	go s.background()
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
			s.pruneStorage(time.Now())
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
	writeJSON(w, 200, map[string]bool{"ok": true})
}

// auditRetention keeps owner actions and journal reads for half a year.
const auditRetention = 180 * 24 * time.Hour

// pruneStorage applies retention to every table that grows with traffic or
// time. Raw per-second speed rows (sg_speed, sg_speed_meta, sg_usage) back the
// 24-hour live and traffic views only.
func (s *Server) pruneStorage(now time.Time) {
	ttl := s.config.Storage.ActivityJournal.HotTTL
	if ttl <= 0 {
		ttl = 14 * 24 * time.Hour
	}
	speedCutoff := now.Add(-24 * time.Hour).UnixMilli()
	statements := []struct {
		query string
		arg   any
	}{
		{"DELETE FROM sg_sessions WHERE expires_at<?", now.Unix()},
		{"DELETE FROM sg_enrollment WHERE expires_at<?", now.Unix()},
		{"DELETE FROM sg_webhooks WHERE timestamp<?", now.Add(-24 * time.Hour).Unix()},
		{"DELETE FROM sg_speed WHERE timestamp<?", speedCutoff},
		{"DELETE FROM sg_speed_meta WHERE timestamp<?", speedCutoff},
		{"DELETE FROM sg_usage WHERE timestamp<?", speedCutoff},
		{"DELETE FROM sg_analysis_windows WHERE minute<?", now.Add(-3*time.Minute).Unix() / 60},
		{"DELETE FROM sg_analysis_seen WHERE minute<?", now.Add(-3*time.Minute).Unix() / 60},
		{"DELETE FROM sg_incidents WHERE updated_at<?", now.Add(-14 * 24 * time.Hour).Unix()},
		{"DELETE FROM sg_shaper_actions WHERE expires_at<?", now.Add(-30 * 24 * time.Hour).Unix()},
		{"DELETE FROM sg_activity WHERE timestamp<?", now.Add(-ttl).UnixMilli()},
		{"DELETE FROM audit_log WHERE timestamp<?", now.Add(-auditRetention).UTC().Format("2006-01-02 15:04:05")},
	}
	for _, statement := range statements {
		if _, err := s.db.Exec(statement.query, statement.arg); err != nil {
			s.logger.Warn("storage retention failed", "query", statement.query, "error", err)
		}
	}
}
