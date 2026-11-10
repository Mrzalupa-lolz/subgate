package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/subgate/subgate/pkg/wire"
)

func (s *Server) createEnrollment(w http.ResponseWriter, r *http.Request) {
	var input struct {
		NodeID string `json:"node_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	var node map[string]any
	if s.object("nodes", input.NodeID, &node) != nil {
		fail(w, 404, "node not found")
		return
	}
	raw := token()
	expiry := time.Now().Add(15 * time.Minute)
	err := s.enrollmentTransaction(func(tx *sql.Tx) error {
		if _, err := tx.Exec("INSERT INTO sg_enrollment(token_hash,node_id,expires_at) VALUES(?,?,?)", hash(raw), input.NodeID, expiry.Unix()); err != nil {
			return err
		}
		p := admin(r)
		return s.auditTx(tx, r, &p, "agent.enrollment.create", "nodes", input.NodeID, "success")
	})
	if err != nil {
		p := admin(r)
		if auditErr := s.recordAudit(r, &p, "agent.enrollment.create", "nodes", input.NodeID, "failure"); auditErr != nil {
			fail(w, 500, "audit unavailable")
			return
		}
		fail(w, 500, "token creation failed")
		return
	}
	writeJSON(w, 201, map[string]any{"token": raw, "node_id": input.NodeID, "expires_at": expiry})
}

// enrollAttemptsPerMinute bounds unauthenticated enrollment attempts per IP;
// each attempt takes the database write lock.
const enrollAttemptsPerMinute = 10

func (s *Server) enroll(w http.ResponseWriter, r *http.Request) {
	if !s.allowAttempt("enroll:"+s.clientIP(r), enrollAttemptsPerMinute) {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "too many enrollment attempts")
		return
	}
	var input struct {
		NodeID string `json:"node_id"`
		Token  string `json:"token"`
	}
	if !decode(w, r, &input) {
		return
	}
	raw := token()
	valid := false
	err := s.enrollmentTransaction(func(tx *sql.Tx) error {
		result, err := tx.Exec("UPDATE sg_enrollment SET used=1 WHERE token_hash=? AND node_id=? AND expires_at>? AND used=0", hash(input.Token), input.NodeID, time.Now().Unix())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n != 1 {
			return nil
		}
		valid = true
		if _, err = tx.Exec("INSERT INTO agents(node_id,token_hash,status) VALUES(?,?,'unknown') ON CONFLICT(node_id) DO UPDATE SET token_hash=excluded.token_hash,status='unknown'", input.NodeID, hash(raw)); err != nil {
			return err
		}
		return s.auditTx(tx, r, nil, "agent.enroll", "nodes", input.NodeID, "success")
	})
	if err != nil {
		if valid { // Only a valid enrollment credential authorizes an audit row.
			if auditErr := s.recordAudit(r, nil, "agent.enroll", "nodes", input.NodeID, "failure"); auditErr != nil {
				fail(w, 500, "audit unavailable")
				return
			}
		}
		fail(w, 500, "enrollment failed")
		return
	}
	if !valid {
		fail(w, 401, "invalid or expired enrollment token")
		return
	}
	writeJSON(w, 201, map[string]string{"token": raw, "node_id": input.NodeID})
}
func (s *Server) agentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			fail(w, 401, "agent authentication required")
			return
		}
		var node string
		if err := s.db.QueryRow("SELECT node_id FROM agents WHERE token_hash=? AND status!='revoked'", hash(strings.TrimPrefix(h, "Bearer "))).Scan(&node); err != nil {
			fail(w, 401, "invalid agent token")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), nodeKey, node)))
	})
}
func node(r *http.Request) string { id, _ := r.Context().Value(nodeKey).(string); return id }
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request) {
	var input wire.Heartbeat
	if !decode(w, r, &input) {
		return
	}
	if input.NodeID != node(r) {
		fail(w, 403, "node identity mismatch")
		return
	}
	if input.MemoryMB < 0 || input.UptimeSeconds < 0 || len(input.StatsError) > 1024 || len(input.LogsError) > 1024 {
		fail(w, 400, "invalid heartbeat")
		return
	}
	status := "online"
	if input.StatsError != "" || input.LogsError != "" {
		status = "error"
	}
	if _, err := s.db.Exec("UPDATE agents SET version=?,status=?,last_heartbeat=CURRENT_TIMESTAMP,memory_mb=? WHERE node_id=?", input.Version, status, input.MemoryMB, input.NodeID); err != nil {
		fail(w, 500, "heartbeat persistence failed")
		return
	}
	if err := s.saveObject("agent-diagnostics", input.NodeID, input); err != nil {
		fail(w, 500, "diagnostics persistence failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) telemetry(w http.ResponseWriter, r *http.Request) {
	var input wire.Telemetry
	if !decode(w, r, &input) {
		return
	}
	if input.NodeID != node(r) {
		fail(w, 403, "node identity mismatch")
		return
	}
	if input.BatchID == "" || len(input.BatchID) > 128 || len(input.Samples) > 10000 || len(input.Events) > 1000 {
		fail(w, 400, "invalid telemetry batch")
		return
	}
	now := time.Now()
	validTime := func(t time.Time) bool {
		return !t.IsZero() && t.After(now.Add(-24*time.Hour)) && t.Before(now.Add(5*time.Minute))
	}
	for _, sample := range input.Samples {
		if sample.UserID == "" || len(sample.UserID) > 256 || sample.UplinkBps < 0 || sample.DownlinkBps < 0 || sample.UplinkBps > 1e15 || sample.DownlinkBps > 1e15 || !validTime(sample.Timestamp) || sample.IntervalMS < 0 || sample.IntervalMS > 86400000 || sample.UplinkDelta < 0 || sample.DownlinkDelta < 0 || sample.UplinkDelta > 1e15 || sample.DownlinkDelta > 1e15 || (sample.Scope != "" && sample.Scope != "user" && sample.Scope != "inbound") {
			fail(w, 400, "invalid speed sample")
			return
		}
	}
	// SentAt is fixed when the agent queues a batch, not on delivery. Retain
	// observation timestamps so offline replay and repacking address the same
	// SQLite rows and quota period.
	if !input.SentAt.IsZero() && !validTime(input.SentAt) {
		fail(w, 400, "invalid telemetry send time")
		return
	}
	for _, event := range input.Events {
		if event.UserID == "" || len(event.UserID) > 256 || len(event.DstHost) > 1024 || len(event.SrcIP) > 128 || event.DstPort < 0 || event.DstPort > 65535 || !validTime(event.Timestamp) {
			fail(w, 400, "invalid activity event")
			return
		}
	}
	tx, err := s.db.BeginTx()
	if err != nil {
		fail(w, 500, "ingest transaction failed")
		return
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	result, err := tx.Exec("INSERT OR IGNORE INTO sg_webhooks(key,timestamp) VALUES(?,?)", "agent:"+input.NodeID+":"+input.BatchID, now.Unix())
	if err != nil {
		fail(w, 500, "batch persistence failed")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		writeJSON(w, 200, map[string]bool{"duplicate": true})
		return
	}
	for _, sample := range input.Samples {
		_, err = tx.Exec("INSERT OR REPLACE INTO sg_speed(node_id,user_id,timestamp,uplink,downlink) VALUES(?,?,?,?,?)", input.NodeID, sample.UserID, sample.Timestamp.UnixMilli(), sample.UplinkBps, sample.DownlinkBps)
		if err != nil {
			fail(w, 500, "speed persistence failed")
			return
		}
		interval := sample.IntervalMS
		if interval == 0 {
			interval = 1000
		}
		scope := sample.Scope
		if scope == "" {
			scope = "user"
		}
		_, err = tx.Exec("INSERT OR REPLACE INTO sg_speed_meta(node_id,user_id,timestamp,interval_ms,scope,gap,warming,uplink_delta,downlink_delta,received_at) VALUES(?,?,?,?,?,?,?,?,?,?)", input.NodeID, sample.UserID, sample.Timestamp.UnixMilli(), interval, scope, sample.Gap, sample.Warming, sample.UplinkDelta, sample.DownlinkDelta, now.UnixMilli())
		if err != nil {
			fail(w, 500, "speed metadata persistence failed")
			return
		}
		if scope == "user" && (sample.UplinkDelta > 0 || sample.DownlinkDelta > 0) {
			_, err = tx.Exec("INSERT OR REPLACE INTO sg_usage(node_id,user_id,timestamp,uplink,downlink) VALUES(?,?,?,?,?)", input.NodeID, sample.UserID, sample.Timestamp.UnixMilli(), sample.UplinkDelta, sample.DownlinkDelta)
		} else {
			// A correction to zero or inbound scope replaces the previous user
			// observation just as sg_speed and sg_speed_meta do.
			_, err = tx.Exec("DELETE FROM sg_usage WHERE node_id=? AND user_id=? AND timestamp=?", input.NodeID, sample.UserID, sample.Timestamp.UnixMilli())
		}
		if err != nil {
			fail(w, 500, "usage persistence failed")
			return
		}
	}
	for _, event := range input.Events {
		raw, _ := json.Marshal(event)
		raw, err = s.encrypt(raw, input.NodeID+":"+event.UserID)
		if err != nil {
			fail(w, 500, "activity encryption failed")
			return
		}
		_, err = tx.Exec("INSERT INTO sg_activity(node_id,user_id,timestamp,payload) VALUES(?,?,?,?)", input.NodeID, event.UserID, event.Timestamp.UnixMilli(), raw)
		if err != nil {
			fail(w, 500, "activity persistence failed")
			return
		}
	}
	if err = s.analyzeTelemetry(tx, input, now); err != nil {
		fail(w, 500, "analysis persistence failed")
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, 500, "ingest commit failed")
		return
	}
	s.liveStore.Add(input.NodeID, input.Samples, now)
	writeJSON(w, 200, map[string]any{"ok": true, "samples": len(input.Samples), "events": len(input.Events)})
}
func (s *Server) agents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT node_id,COALESCE(version,''),status,COALESCE(last_heartbeat,''),COALESCE(memory_mb,0),CASE WHEN last_heartbeat>datetime('now','-90 seconds') AND status!='revoked' THEN 1 ELSE 0 END FROM agents ORDER BY node_id")
	if err != nil {
		fail(w, 500, "query failed")
		return
	}
	data := []map[string]any{}
	for rows.Next() {
		var id, version, status, last string
		var memory, online int
		if err = rows.Scan(&id, &version, &status, &last, &memory, &online); err != nil {
			rows.Close()
			fail(w, 500, "invalid agent row")
			return
		}
		data = append(data, map[string]any{"node_id": id, "version": version, "status": status, "last_heartbeat": last, "memory_mb": memory, "online": online == 1})
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		fail(w, 500, "query failed")
		return
	}
	for _, item := range data {
		var diagnostics wire.Heartbeat
		if s.object("agent-diagnostics", item["node_id"].(string), &diagnostics) == nil {
			item["diagnostics"] = diagnostics
		}
	}
	writeJSON(w, 200, map[string]any{"data": data})
}
func (s *Server) revokeAgent(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := s.auditAction(r, "agent.revoke", "agents", id); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	_, err := s.db.Exec("UPDATE agents SET token_hash=NULL,status='revoked' WHERE node_id=?", id)
	if err != nil {
		fail(w, 500, "revocation failed")
		return
	}
	s.liveStore.RemoveNode(id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

type activityRow struct {
	ID     int64  `json:"id"`
	NodeID string `json:"node_id"`
	wire.Activity
}

// activityDomainScanLimit bounds how many journal rows one domain search
// decrypts.
const activityDomainScanLimit = 100000

func (s *Server) queryActivity(f activityFilters) ([]activityRow, error) {
	query := "SELECT id,node_id,user_id,payload FROM sg_activity WHERE 1=1"
	args := []any{}
	if user := f.User; user != "" {
		identities, err := s.activityIdentities(user)
		if err != nil {
			return nil, err
		}
		predicate, values := identityPredicate(identities)
		query += " AND " + predicate
		args = append(args, values...)
	}
	if n := f.Node; n != "" {
		query += " AND node_id=?"
		args = append(args, n)
	}
	for _, bound := range []struct{ name, op string }{{"from", ">="}, {"to", "<="}} {
		if v := map[string]string{"from": f.From, "to": f.To}[bound.name]; v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				return nil, fmt.Errorf("invalid %s timestamp", bound.name)
			}
			query += " AND timestamp" + bound.op + "?"
			args = append(args, t.UnixMilli())
		}
	}
	// Destinations are encrypted, so a domain filter is applied after
	// decryption. Scan newest rows until Limit matches are found instead of
	// filtering only the newest Limit rows.
	scan := f.Limit
	if f.Domain != "" {
		scan = activityDomainScanLimit
	}
	query += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, scan)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	data := []activityRow{}
	for rows.Next() {
		var item activityRow
		var user string
		var encrypted []byte
		if err = rows.Scan(&item.ID, &item.NodeID, &user, &encrypted); err != nil {
			return nil, err
		}
		raw, e := s.decrypt(encrypted, item.NodeID+":"+user)
		if e != nil {
			return nil, fmt.Errorf("activity integrity check failed")
		}
		if err = json.Unmarshal(raw, &item.Activity); err != nil {
			return nil, err
		}
		domain := f.Domain
		if domain == "" || strings.Contains(strings.ToLower(item.DstHost), domain) {
			data = append(data, item)
			if len(data) >= f.Limit {
				break
			}
		}
	}
	return data, rows.Err()
}
func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	data, f, ok := s.readActivity(w, r, 1000, "activity.read")
	if !ok {
		return
	}
	writeJSON(w, 200, map[string]any{"data": data, "limit": f.Limit})
}
func (s *Server) exportActivity(w http.ResponseWriter, r *http.Request) {
	data, f, ok := s.readActivity(w, r, 10000, "activity.export")
	if !ok {
		return
	}
	if f.Format == "csv" {
		rows := [][]string{{"timestamp", "user", "node", "source_ip", "destination", "port", "network", "result"}}
		for _, e := range data {
			rows = append(rows, []string{e.Timestamp.Format(time.RFC3339Nano), e.UserID, e.NodeID, e.SrcIP, e.DstHost, strconv.Itoa(e.DstPort), e.Network, e.Result})
		}
		csvRows(w, rows)
		return
	}
	w.Header().Set("Content-Disposition", "attachment; filename=activity.json")
	writeJSON(w, 200, data)
}
func (s *Server) purgeActivity(w http.ResponseWriter, r *http.Request) {
	user := chi.URLParam(r, "user")
	if err := s.auditAction(r, "activity.purge", "activity", user); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	identities, err := s.activityIdentities(user)
	if err != nil {
		fail(w, 500, "identity lookup failed")
		return
	}
	predicate, args := identityPredicate(identities)
	_, err = s.db.Exec("DELETE FROM sg_activity WHERE "+predicate, args...)
	if err != nil {
		fail(w, 500, "purge failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) enrollmentTransaction(mutate func(*sql.Tx) error) error {
	tx, err := s.db.BeginTx()
	if err != nil {
		return err
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	if err = mutate(tx); err != nil {
		return err
	}
	return tx.Commit()
}
