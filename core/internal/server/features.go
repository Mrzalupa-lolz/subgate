package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/subgate/subgate/core/internal/blocks"
	"github.com/subgate/subgate/core/internal/notify"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/core/internal/quota"
	"github.com/subgate/subgate/core/internal/youtube"
)

func (s *Server) loadPersistedQuotas() error {
	items, err := s.objects("quotas")
	if err != nil {
		return err
	}
	for _, raw := range items {
		var rule quota.Rule
		if err := json.Unmarshal(raw, &rule); err != nil {
			return fmt.Errorf("decode quota: %w", err)
		}
		if err := s.quota.Upsert(rule); err != nil {
			return fmt.Errorf("load quota %q: %w", rule.ID, err)
		}
	}
	return nil
}

func (s *Server) listQuotas(w http.ResponseWriter, r *http.Request) { s.listObjects(w, r, "quotas") }

func (s *Server) putQuota(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" || len(id) > 128 {
		fail(w, 400, "invalid quota id")
		return
	}
	var input quota.Rule
	if !decode(w, r, &input) {
		return
	}
	input.ID = id
	input, err := quota.Normalize(input)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	input.UpdatedAt = time.Now().UTC()
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	if err := s.mutateFeature(r, "quotas", id, input, "quota.upsert"); err != nil {
		fail(w, 500, "quota save failed")
		return
	}
	// Validation above guarantees this cannot fail after the commit.
	_ = s.quota.Upsert(input)
	writeJSON(w, 200, input)
}

func (s *Server) deleteQuota(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	s.quotaMu.Lock()
	defer s.quotaMu.Unlock()
	if err := s.mutateFeature(r, "quotas", id, nil, "quota.delete"); err != nil {
		if err == errFeatureNotFound {
			fail(w, 404, "quota not found")
			return
		}
		fail(w, 500, "quota delete failed")
		return
	}
	s.quota.Delete(id)
	writeJSON(w, 200, map[string]bool{"ok": true})
}

var errFeatureNotFound = fmt.Errorf("feature not found")

// mutateFeature commits the object and its audit entry together. A nil value
// deletes an existing object; callers update any memory state after commit.
func (s *Server) mutateFeature(r *http.Request, kind, id string, value any, action string) error {
	var raw []byte
	var err error
	if value != nil {
		raw, err = json.Marshal(value)
		if err != nil {
			return err
		}
	}
	tx, err := s.db.BeginTx()
	if err != nil {
		return err
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	if value == nil {
		result, e := tx.Exec("DELETE FROM sg_objects WHERE kind=? AND id=?", kind, id)
		if e != nil {
			return e
		}
		count, e := result.RowsAffected()
		if e != nil {
			return e
		}
		if count == 0 {
			return errFeatureNotFound
		}
	} else {
		_, err = tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES(?,?,?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", kind, id, string(raw), time.Now().Unix())
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec("INSERT INTO audit_log(admin_id,action,resource,resource_id,status,ip_address) VALUES(?,?,?,?,?,?)", admin(r).ID, action, kind, id, "success", s.clientIP(r))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) quotaUsage(rule quota.Rule, start, end time.Time) (int64, error) {
	query := "SELECT uplink,downlink FROM sg_usage WHERE node_id=? AND timestamp>=? AND timestamp<?"
	args := []any{rule.NodeID, start.UnixMilli(), end.UnixMilli()}
	if rule.Scope == "per_user" {
		identities, err := s.activityIdentities(rule.UserID)
		if err != nil || len(identities) == 0 {
			return 0, fmt.Errorf("quota user identity unavailable")
		}
		query += " AND user_id IN (" + strings.TrimRight(strings.Repeat("?,", len(identities)), ",") + ")"
		for _, identity := range identities {
			args = append(args, identity)
		}
	} else {
		// Agent falls back to inbound counters when user counters are absent.
		// Prefer user observations over overlapping inbound intervals to avoid
		// charging the same traffic twice, even across repacked batches. Partial
		// overlaps cannot be disaggregated, so bypass the whole inbound interval.
		query += ` UNION ALL SELECT m.uplink_delta,m.downlink_delta FROM sg_speed_meta m
		 WHERE m.node_id=? AND m.timestamp>=? AND m.timestamp<? AND m.scope='inbound'
		 AND NOT EXISTS (SELECT 1 FROM sg_speed_meta u
		 WHERE u.node_id=m.node_id AND u.scope='user'
		   AND u.timestamp < m.timestamp+86400000
		   AND u.timestamp-COALESCE(u.interval_ms,1000) < m.timestamp
		   AND m.timestamp-COALESCE(m.interval_ms,1000) < u.timestamp)`
		args = append(args, rule.NodeID, start.UnixMilli(), end.UnixMilli())
	}
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var total int64
	for rows.Next() {
		var up, down int64
		if err := rows.Scan(&up, &down); err != nil {
			return 0, err
		}
		for _, bytes := range []int64{up, down} {
			if bytes < 0 {
				return 0, fmt.Errorf("invalid persisted quota usage")
			}
			if bytes > math.MaxInt64-total {
				total = math.MaxInt64
			} else {
				total += bytes
			}
		}
	}
	return total, rows.Err()
}

func (s *Server) blockCheck(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var input struct {
		Address    string `json:"address"`
		Port       int    `json:"port"`
		TLS        bool   `json:"tls"`
		ServerName string `json:"server_name"`
	}
	if !decode(w, r, &input) {
		return
	}
	var node panel.Node
	if s.object("nodes", id, &node) != nil {
		fail(w, 404, "node not found")
		return
	}
	if node.IPAddress == "" || len(node.Inbounds) == 0 {
		fail(w, 409, "no inbound endpoints are configured; sync the panel first")
		return
	}
	if input.Address != "" && !strings.EqualFold(input.Address, node.IPAddress) {
		fail(w, 400, "address must match the synced node")
		return
	}
	if input.Port == 0 {
		input.Port = 443
	}
	if input.Port < 1 || input.Port > 65535 {
		fail(w, 400, "invalid port")
		return
	}
	configured := false
	for _, inbound := range node.Inbounds {
		if inbound.Port == input.Port {
			configured = true
			break
		}
	}
	if !configured {
		fail(w, 400, "port must match a synced inbound")
		return
	}
	if err := s.auditAction(r, "node.block-check", "nodes", id); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	address := net.JoinHostPort(node.IPAddress, strconv.Itoa(input.Port))
	result, err := blocks.Probe(r.Context(), blocks.Config{Address: address, TLS: input.TLS, ServerName: input.ServerName})
	if err != nil && result.Error == "" {
		result.Error = err.Error()
	}
	result.Address = address
	key := id + ":" + time.Now().UTC().Format("20060102T150405.000000000")
	_ = s.saveObject("block-checks", key, map[string]any{"id": key, "node_id": id, "result": result, "created_at": time.Now().UTC()})
	writeJSON(w, 200, result)
}

func (s *Server) listBlocks(w http.ResponseWriter, r *http.Request) {
	s.listObjects(w, r, "block-checks")
}

func (s *Server) youtubeStatus(w http.ResponseWriter, r *http.Request) {
	s.listObjects(w, r, "youtube-checks")
}

func (s *Server) youtubeCheck(w http.ResponseWriter, r *http.Request) {
	var input struct {
		NodeID   string `json:"node_id"`
		Endpoint string `json:"endpoint"`
		VideoID  string `json:"video_id"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Endpoint != "" {
		endpoint, err := url.Parse(input.Endpoint)
		if err != nil || validateYouTubeURL(endpoint) != nil {
			fail(w, 400, "endpoint must be an HTTPS YouTube URL on port 443")
			return
		}
	}
	if len(input.VideoID) > 128 || strings.ContainsAny(input.VideoID, "&?#/\\\r\n") {
		fail(w, 400, "invalid video_id")
		return
	}
	if err := s.auditAction(r, "youtube.check", "youtube", input.NodeID); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many YouTube redirects")
		}
		return validateYouTubeURL(request.URL)
	}}
	result := youtube.Check(r.Context(), youtube.Config{Endpoint: input.Endpoint, VideoID: input.VideoID, Client: client})
	key := input.NodeID + ":" + time.Now().UTC().Format("20060102T150405.000000000")
	_ = s.saveObject("youtube-checks", key, map[string]any{"id": key, "node_id": input.NodeID, "result": result, "created_at": time.Now().UTC()})
	writeJSON(w, 200, result)
}

func validateYouTubeURL(endpoint *url.URL) error {
	if endpoint == nil || endpoint.Scheme != "https" || endpoint.User != nil || endpoint.Fragment != "" || (endpoint.Port() != "" && endpoint.Port() != "443") {
		return fmt.Errorf("untrusted YouTube endpoint")
	}
	switch strings.ToLower(endpoint.Hostname()) {
	case "youtube.com", "www.youtube.com", "m.youtube.com", "youtu.be":
		return nil
	default:
		return fmt.Errorf("untrusted YouTube endpoint")
	}
}

func (s *Server) notificationTest(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Category string          `json:"category"`
		Severity notify.Severity `json:"severity"`
		Title    string          `json:"title"`
		Message  string          `json:"message"`
	}
	if !decode(w, r, &input) {
		return
	}
	if input.Category == "" {
		input.Category = "system"
	}
	if input.Title == "" {
		input.Title = "SubGate test notification"
	}
	err := s.notify.Send(context.WithoutCancel(r.Context()), notify.Alert{Category: input.Category, Severity: input.Severity, Title: input.Title, Message: input.Message, Dedupe: "manual-test-" + input.Category})
	if err != nil {
		fail(w, 502, err.Error())
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}
