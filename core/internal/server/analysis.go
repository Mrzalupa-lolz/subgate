package server

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/subgate/subgate/core/internal/analysis"
	"github.com/subgate/subgate/pkg/wire"
)

type incident struct {
	ID         string     `json:"id"`
	NodeID     string     `json:"node_id"`
	UserID     string     `json:"user_id"`
	Status     string     `json:"status"`
	Note       string     `json:"note,omitempty"`
	ReviewedAt *time.Time `json:"reviewed_at,omitempty"`
	analysis.Signal
}

func analysisID(parts ...any) string {
	raw, _ := json.Marshal(parts)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:16])
}

func (s *Server) analysisPolicyTx(tx *sql.Tx) (analysis.Policy, error) {
	p := analysis.DefaultPolicy()
	var raw string
	err := tx.QueryRow("SELECT data FROM sg_objects WHERE kind='analysis-policy' AND id='default'").Scan(&raw)
	if err == sql.ErrNoRows {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return p, err
	}
	if !p.Valid() {
		return p, fmt.Errorf("invalid stored analysis policy")
	}
	return p, nil
}

// Analysis and telemetry commit together: retrying a failed batch neither loses
// evidence nor increments it twice. Minute windows survive Core restarts.
func (s *Server) analyzeTelemetry(tx *sql.Tx, input wire.Telemetry, now time.Time) error {
	if len(input.Events) == 0 {
		return nil
	}
	p, err := s.analysisPolicyTx(tx)
	if err != nil {
		return err
	}
	type key struct {
		user   string
		minute int64
	}
	groups := map[key][]wire.Activity{}
	for _, event := range input.Events {
		// Late offline batches remain in the journal but cannot trigger a current response.
		if event.Timestamp.Before(now.Add(-2*time.Minute)) || event.Timestamp.After(now.Add(5*time.Second)) {
			continue
		}
		k := key{event.UserID, event.Timestamp.Unix() / 60}
		groups[k] = append(groups[k], event)
	}
	for k, events := range groups {
		id := analysisID(input.NodeID, k.user, k.minute)
		var w analysis.Window
		var encrypted []byte
		err := tx.QueryRow("SELECT payload FROM sg_analysis_windows WHERE id=?", id).Scan(&encrypted)
		if err != nil && err != sql.ErrNoRows {
			return err
		}
		if err == nil {
			raw, err := s.decrypt(encrypted, "analysis:"+id)
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &w); err != nil {
				return err
			}
		}
		for _, event := range events {
			if w.Count >= analysis.MaxEvents {
				w.Truncated = true
				continue
			}
			fingerprint := analysisID(id, analysis.Fingerprint(event))
			result, err := tx.Exec("INSERT OR IGNORE INTO sg_analysis_seen(id,minute) VALUES(?,?)", fingerprint, k.minute)
			if err != nil {
				return err
			}
			added, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if added == 0 {
				continue
			}
			w.Add(event)
		}
		raw, err := json.Marshal(w)
		if err != nil {
			return err
		}
		encrypted, err = s.encrypt(raw, "analysis:"+id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO sg_analysis_windows(id,minute,payload) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload", id, k.minute, encrypted); err != nil {
			return err
		}
		for _, signal := range w.Signals(p) {
			item := incident{ID: analysisID(id, signal.Kind), NodeID: input.NodeID, UserID: k.user, Status: "new", Signal: signal}
			raw, err := json.Marshal(item)
			if err != nil {
				return err
			}
			encrypted, err := s.encrypt(raw, "incident:"+item.ID)
			if err != nil {
				return err
			}
			// Review state is stored separately and survives later evidence updates.
			if _, err := tx.Exec("INSERT INTO sg_incidents(id,updated_at,status,payload) VALUES(?,?,'new',?) ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at,payload=excluded.payload", item.ID, signal.Last.Unix(), encrypted); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec("DELETE FROM sg_analysis_windows WHERE minute<?", now.Add(-3*time.Minute).Unix()/60); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM sg_analysis_seen WHERE minute<?", now.Add(-3*time.Minute).Unix()/60); err != nil {
		return err
	}
	_, err = tx.Exec("DELETE FROM sg_incidents WHERE updated_at<?", now.Add(-14*24*time.Hour).Unix())
	return err
}

func (s *Server) incidents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id,status,payload,review FROM sg_incidents WHERE updated_at>=? ORDER BY updated_at DESC,id LIMIT ?", time.Now().Add(-14*24*time.Hour).Unix(), limit(r, 200, 500))
	if err != nil {
		fail(w, 500, "incident query failed")
		return
	}
	items := []incident{}
	for rows.Next() {
		var id, status string
		var payload, review []byte
		if err = rows.Scan(&id, &status, &payload, &review); err != nil {
			break
		}
		var raw []byte
		raw, err = s.decrypt(payload, "incident:"+id)
		if err != nil {
			break
		}
		var item incident
		if err = json.Unmarshal(raw, &item); err != nil {
			break
		}
		item.Status = status
		if len(review) > 0 {
			raw, err = s.decrypt(review, "review:"+id)
			if err != nil {
				break
			}
			var record struct {
				Note string
				At   time.Time
			}
			if err = json.Unmarshal(raw, &record); err != nil {
				break
			}
			item.Note, item.ReviewedAt = record.Note, &record.At
		}
		items = append(items, item)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "incident integrity check failed")
		return
	}
	// Reading evidence is audited just like reading the Xray journal.
	if err := s.auditAction(r, "incidents.read", "incidents", ""); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"data": items, "window_seconds": 60, "retention_days": 14})
}

func (s *Server) reviewIncident(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Status string `json:"status"`
		Note   string `json:"note"`
	}
	if !decode(w, r, &input) {
		return
	}
	if len(input.Note) > 2000 || (input.Status != "reviewing" && input.Status != "confirmed" && input.Status != "false_positive" && input.Status != "resolved") {
		fail(w, 400, "invalid incident review")
		return
	}
	id := chi.URLParam(r, "id")
	raw, _ := json.Marshal(struct {
		Note string
		At   time.Time
	}{input.Note, time.Now().UTC()})
	encrypted, err := s.encrypt(raw, "review:"+id)
	if err != nil {
		fail(w, 500, "review encryption failed")
		return
	}
	tx, err := s.db.BeginTx()
	if err != nil {
		fail(w, 500, "review transaction failed")
		return
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	result, err := tx.Exec("UPDATE sg_incidents SET status=?,review=? WHERE id=?", input.Status, encrypted, id)
	if err != nil {
		fail(w, 500, "review update failed")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		fail(w, 404, "incident not found")
		return
	}
	p := admin(r)
	if err := s.auditTx(tx, r, &p, "incident.review."+input.Status, "incidents", id, "success"); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "review commit failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) getAnalysisPolicy(w http.ResponseWriter, r *http.Request) {
	p := analysis.DefaultPolicy()
	if err := s.object("analysis-policy", "default", &p); err != nil && err != sql.ErrNoRows {
		fail(w, 500, "analysis policy unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"policy": p.Normalized(), "shaper_available": true, "dpi_available": true, "automatic_actions": p.AutomaticShaper})
}

func (s *Server) putAnalysisPolicy(w http.ResponseWriter, r *http.Request) {
	var input analysis.Policy
	if !decode(w, r, &input) {
		return
	}
	input = input.Normalized()
	if !input.Valid() {
		fail(w, 400, "invalid analysis thresholds or shaper draft")
		return
	}
	raw, _ := json.Marshal(input)
	tx, err := s.db.BeginTx()
	if err != nil {
		fail(w, 500, "policy transaction failed")
		return
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	if _, err := tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('analysis-policy','default',?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", string(raw), time.Now().Unix()); err != nil {
		fail(w, 500, "policy persistence failed")
		return
	}
	p := admin(r)
	if err := s.auditTx(tx, r, &p, "analysis.policy.save", "analysis-policy", "default", "success"); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	if err := tx.Commit(); err != nil {
		fail(w, 500, "policy commit failed")
		return
	}
	writeJSON(w, 200, input)
}
