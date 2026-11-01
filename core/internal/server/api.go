package server

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/subgate/subgate/core/internal/panel"
)

func (s *Server) systemStatus(w http.ResponseWriter, r *http.Request) {
	s.stateMu.RLock()
	last, problem, version := s.lastSync, s.syncError, s.panelVersion
	s.stateMu.RUnlock()
	var nodes, users, agents, activity int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM sg_objects WHERE kind='nodes'").Scan(&nodes)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM sg_objects WHERE kind='users'").Scan(&users)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM agents WHERE token_hash IS NOT NULL AND last_heartbeat>datetime('now','-90 seconds')").Scan(&agents)
	_ = s.db.QueryRow("SELECT COUNT(*) FROM sg_activity").Scan(&activity)
	writeJSON(w, 200, map[string]any{"version": "0.2.0", "demo": s.config.Demo, "uptime_seconds": int(time.Since(s.started).Seconds()), "nodes": nodes, "users": users, "agents_online": agents, "activity_events": activity, "last_sync": last, "sync_error": problem, "panel_version": version, "gateway": s.gateway.Statistics(), "database": s.db.Statistics(), "ipintel": s.intel.Health()})
}
func limit(r *http.Request, def, max int) int {
	n, e := strconv.Atoi(r.URL.Query().Get("limit"))
	if e != nil || n < 1 {
		return def
	}
	if n > max {
		return max
	}
	return n
}
func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) { s.listObjects(w, r, "nodes") }
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) { s.listObjects(w, r, "users") }
func (s *Server) listObjects(w http.ResponseWriter, r *http.Request, kind string) {
	data, err := s.objects(kind)
	if err != nil {
		fail(w, 500, "query failed")
		return
	}
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	filtered := []json.RawMessage{}
	for _, raw := range data {
		if q == "" || strings.Contains(strings.ToLower(string(raw)), q) {
			filtered = append(filtered, raw)
		}
	}
	total := len(filtered)
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := offset + limit(r, 500, 1000)
	if end > total {
		end = total
	}
	writeJSON(w, 200, map[string]any{"data": filtered[offset:end], "total": total, "offset": offset})
}
func (s *Server) getNode(w http.ResponseWriter, r *http.Request) {
	var n panel.Node
	if s.object("nodes", chi.URLParam(r, "id"), &n) != nil {
		fail(w, 404, "node not found")
		return
	}
	writeJSON(w, 200, n)
}
func (s *Server) getUser(w http.ResponseWriter, r *http.Request) {
	var u panel.User
	if s.object("users", chi.URLParam(r, "id"), &u) != nil {
		fail(w, 404, "user not found")
		return
	}
	writeJSON(w, 200, u)
}

// User mutations are owned by remnawave-subscription-middleware.
func (s *Server) nodeAction(w http.ResponseWriter, r *http.Request) {
	const kind = "nodes"
	id, action := chi.URLParam(r, "id"), chi.URLParam(r, "action")
	allowed := action == "enable" || action == "disable" || action == "reset-traffic" || action == "restart"
	if !allowed {
		fail(w, 400, "unsupported action")
		return
	}
	var raw map[string]any
	if s.object(kind, id, &raw) != nil {
		fail(w, 404, "resource not found")
		return
	}
	if err := s.auditAction(r, kind+"."+action, kind, id); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	if s.config.Demo {
		if action == "enable" {
			raw["status"] = "online"
		}
		if action == "disable" {
			raw["status"] = "disabled"
		}
		if action == "reset-traffic" {
			raw["used_traffic_bytes"] = 0
			raw["traffic_used_bytes"] = 0
		}
		if err := s.saveObject(kind, id, raw); err != nil {
			fail(w, 500, "update failed")
			return
		}
	} else {
		if err := s.panel.Action(r.Context(), kind, id, action); err != nil {
			p := admin(r)
			_ = s.recordAudit(r, &p, kind+"."+action, kind, id, "failure")
			fail(w, 502, err.Error())
			return
		}
	}
	_ = s.gateway.Clear()
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) settings(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"demo": s.config.Demo, "listen": s.config.HTTP.Listen, "panel_url": s.config.Panel.URL, "subscription_url": s.config.Panel.SubscriptionURL, "sync_interval_seconds": s.config.Panel.SyncInterval.Seconds(), "session_timeout_hours": s.config.Auth.SessionTimeout.Hours(), "activity_encrypted": true, "activity_hot_days": 14, "backup_path": s.config.Database.BackupPath, "ipintel_data_path": s.config.IPIntel.DataPath, "protocol_mode": "opaque passthrough", "trusted_proxies": s.config.HTTP.TrustedProxies})
}
func (s *Server) auditList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id,COALESCE(admin_id,0),action,COALESCE(resource,''),COALESCE(resource_id,''),status,COALESCE(ip_address,''),timestamp,COALESCE(details,'') FROM audit_log ORDER BY id DESC LIMIT ?", limit(r, 100, 1000))
	if err != nil {
		fail(w, 500, "query failed")
		return
	}
	defer rows.Close()
	data := []map[string]any{}
	for rows.Next() {
		var id, a int
		var action, kind, res, status, ip, ts, details string
		if rows.Scan(&id, &a, &action, &kind, &res, &status, &ip, &ts, &details) != nil {
			fail(w, 500, "invalid audit row")
			return
		}
		decoded, err := s.decodeActivityAudit(details, action, a)
		if err != nil {
			fail(w, 500, "audit integrity check failed")
			return
		}
		data = append(data, map[string]any{"id": id, "admin_id": a, "action": action, "resource": kind, "resource_id": res, "status": status, "ip": ip, "timestamp": ts, "details": decoded})
	}
	writeJSON(w, 200, map[string]any{"data": data})
}
func (s *Server) lookup(w http.ResponseWriter, r *http.Request) {
	result, err := s.intel.Lookup(r.URL.Query().Get("ip"))
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	writeJSON(w, 200, result)
}
func (s *Server) putObject(w http.ResponseWriter, r *http.Request, kind string) {
	id := chi.URLParam(r, "id")
	if len(id) > 128 || id == "" {
		fail(w, 400, "invalid id")
		return
	}
	var input map[string]any
	if !decode(w, r, &input) {
		return
	}
	input["id"] = id
	if err := s.validateObject(kind, input); err != nil {
		fail(w, 400, err.Error())
		return
	}
	if err := s.auditAction(r, kind+".save", kind, id); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	if err := s.saveObject(kind, id, input); err != nil {
		fail(w, 500, "save failed")
		return
	}
	_ = s.gateway.Clear()
	writeJSON(w, 200, input)
}
func (s *Server) validateObject(kind string, input map[string]any) error {
	if enabled, ok := input["enabled"]; ok {
		if _, valid := enabled.(bool); !valid {
			return fmt.Errorf("enabled must be a boolean")
		}
	}
	if kind != "alerts" {
		allowed := map[string]bool{"id": true, "name": true, "enabled": true}
		for _, key := range map[string][]string{"tiers": {"strategy", "node_ids", "min_healthy", "fallback_tier_id"}, "policies": {"tier_id", "conditions", "priority"}, "pins": {"user_id", "node_id"}, "geo-rules": {"rule_type", "action", "values", "priority"}}[kind] {
			allowed[key] = true
		}
		for key := range input {
			if !allowed[key] {
				return fmt.Errorf("unsupported field: %s", key)
			}
		}
	}
	if kind == "alerts" {
		return nil
	}
	if name, _ := input["name"].(string); kind != "pins" && (name == "" || len(name) > 200) {
		return fmt.Errorf("name is required (maximum 200 characters)")
	}
	if kind == "geo-rules" {
		rule, _ := input["rule_type"].(string)
		action, _ := input["action"].(string)
		if action != "allow" && action != "deny" {
			return fmt.Errorf("action must be allow or deny")
		}
		if rule != "country" && rule != "asn" && rule != "cidr" && rule != "network_type" {
			return fmt.Errorf("unsupported rule type")
		}
		values, ok := input["values"].([]any)
		if !ok || len(values) == 0 || len(values) > 1000 {
			return fmt.Errorf("values must be a nonempty array (maximum 1000)")
		}
		for _, v := range values {
			value, ok := v.(string)
			if !ok {
				return fmt.Errorf("rule values must be strings")
			}
			if rule == "cidr" {
				if _, err := netip.ParsePrefix(value); err != nil {
					return fmt.Errorf("invalid CIDR")
				}
			}
			if rule == "asn" {
				if n, err := strconv.Atoi(value); err != nil || n <= 0 {
					return fmt.Errorf("invalid ASN")
				}
			}
		}
		return nil
	}
	if kind == "tiers" {
		if raw, ok := input["node_ids"]; ok {
			nodes, valid := raw.([]any)
			if !valid || len(nodes) > 1000 {
				return fmt.Errorf("node_ids must be an array (maximum 1000)")
			}
			seen := map[string]bool{}
			for _, raw := range nodes {
				node, valid := raw.(string)
				if !valid || seen[node] {
					return fmt.Errorf("node_ids must contain unique node IDs")
				}
				seen[node] = true
				var n panel.Node
				if s.object("nodes", node, &n) != nil {
					return fmt.Errorf("unknown node: %s", node)
				}
			}
		}
		if raw, ok := input["min_healthy"]; ok {
			n, valid := raw.(float64)
			if !valid || n < 1 || n > 1000 || n != float64(int(n)) {
				return fmt.Errorf("min_healthy must be an integer between 1 and 1000")
			}
		}
		strategy, _ := input["strategy"].(string)
		if strategy != "random" && strategy != "roundRobin" && strategy != "leastPing" && strategy != "leastLoad" {
			return fmt.Errorf("unsupported balancing strategy")
		}
		if fallback, _ := input["fallback_tier_id"].(string); fallback != "" {
			if fallback == input["id"] {
				return fmt.Errorf("tier cannot fall back to itself")
			}
			visited := map[string]bool{input["id"].(string): true}
			for fallback != "" {
				if visited[fallback] {
					return fmt.Errorf("tier fallback cycle")
				}
				visited[fallback] = true
				var next map[string]any
				if err := s.object("tiers", fallback, &next); err != nil {
					return fmt.Errorf("fallback tier does not exist")
				}
				fallback, _ = next["fallback_tier_id"].(string)
			}
		}
	}
	if kind == "policies" {
		if raw, ok := input["priority"]; ok {
			n, valid := raw.(float64)
			if !valid || n < 0 || n > 1000000 || n != float64(int(n)) {
				return fmt.Errorf("priority must be an integer between 0 and 1000000")
			}
		}
		if raw, ok := input["conditions"]; ok {
			conditions, valid := raw.(map[string]any)
			if !valid {
				return fmt.Errorf("conditions must be an object")
			}
			for key, value := range conditions {
				switch key {
				case "countries", "asns":
					list, ok := value.([]any)
					if !ok || len(list) > 1000 {
						return fmt.Errorf("%s must be an array", key)
					}
					for _, item := range list {
						if v, ok := item.(string); !ok || v == "" {
							return fmt.Errorf("%s entries must be strings", key)
						}
					}
				case "user_agent":
					if _, ok := value.(string); !ok {
						return fmt.Errorf("user_agent must be a string")
					}
				default:
					return fmt.Errorf("unsupported condition: %s", key)
				}
			}
		}
		tierID, _ := input["tier_id"].(string)
		var tier map[string]any
		if tierID == "" || s.object("tiers", tierID, &tier) != nil {
			return fmt.Errorf("tier_id must reference an existing tier")
		}
	}
	if kind == "pins" {
		user, _ := input["user_id"].(string)
		node, _ := input["node_id"].(string)
		var raw map[string]any
		if user == "" || s.object("users", user, &raw) != nil {
			return fmt.Errorf("user not found")
		}
		if node == "" || s.object("nodes", node, &raw) != nil {
			return fmt.Errorf("node not found")
		}
	}
	return nil
}
func (s *Server) deleteObject(w http.ResponseWriter, r *http.Request, kind string) {
	id := chi.URLParam(r, "id")
	if kind == "tiers" {
		var references int
		err := s.db.QueryRow("SELECT COUNT(*) FROM sg_objects WHERE (kind='policies' AND json_extract(data,'$.tier_id')=?) OR (kind='tiers' AND json_extract(data,'$.fallback_tier_id')=?)", id, id).Scan(&references)
		if err != nil {
			fail(w, 500, "reference check failed")
			return
		}
		if references > 0 {
			fail(w, 409, "tier is referenced by another policy or fallback tier")
			return
		}
	}
	if err := s.auditAction(r, kind+".delete", kind, id); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	result, err := s.db.Exec("DELETE FROM sg_objects WHERE kind=? AND id=?", kind, id)
	if err != nil {
		fail(w, 500, "delete failed")
		return
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		fail(w, 404, "object not found")
		return
	}
	_ = s.gateway.Clear()
	writeJSON(w, 200, map[string]bool{"ok": true})
}
func (s *Server) geoDenied(r *http.Request) bool {
	rules, err := s.objects("geo-rules")
	if err != nil {
		return false
	}
	ip := s.clientIP(r)
	addr, _ := netip.ParseAddr(ip)
	if addr.IsValid() {
		addr = addr.Unmap()
	}
	intel, _ := s.intel.Lookup(ip)
	for _, raw := range rules {
		var rule struct {
			Enabled bool     `json:"enabled"`
			Type    string   `json:"rule_type"`
			Action  string   `json:"action"`
			Values  []string `json:"values"`
		}
		if json.Unmarshal(raw, &rule) != nil || !rule.Enabled {
			continue
		}
		matched := false
		for _, v := range rule.Values {
			switch rule.Type {
			case "cidr":
				prefix, e := netip.ParsePrefix(v)
				matched = matched || e == nil && prefix.Contains(addr)
			case "country":
				matched = matched || intel != nil && intel.Confidence != "low" && strings.EqualFold(v, intel.Country)
			case "asn":
				matched = matched || intel != nil && strconv.Itoa(intel.ASN) == v
			case "network_type":
				matched = matched || intel != nil && intel.NetworkType == v
			}
		}
		if matched {
			return rule.Action == "deny"
		}
	}
	return false
}
func (s *Server) backups(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(s.config.Database.BackupPath)
	if err != nil && !os.IsNotExist(err) {
		fail(w, 500, "backup listing failed")
		return
	}
	data := []map[string]any{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sgb") {
			continue
		}
		info, e := entry.Info()
		if e == nil {
			data = append(data, map[string]any{"name": entry.Name(), "size": info.Size(), "created_at": info.ModTime()})
		}
	}
	sort.Slice(data, func(i, j int) bool { return data[i]["name"].(string) > data[j]["name"].(string) })
	writeJSON(w, 200, map[string]any{"data": data})
}
func (s *Server) createBackup(w http.ResponseWriter, r *http.Request) {
	if err := s.auditAction(r, "backup.create", "backups", ""); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	name := "subgate-" + time.Now().UTC().Format("20060102T150405") + "-" + token()[:8] + ".sgb"
	dir := s.config.Database.BackupPath
	if err := os.MkdirAll(dir, 0700); err != nil {
		fail(w, 500, "backup directory unavailable")
		return
	}
	tmp := filepath.Join(dir, name+".sqlite.tmp")
	defer os.Remove(tmp)
	if err := s.db.Backup(tmp); err != nil {
		fail(w, 500, "database snapshot failed")
		return
	}
	raw, err := os.ReadFile(tmp)
	if err != nil {
		fail(w, 500, "backup read failed")
		return
	}
	raw, err = s.encrypt(raw, "subgate-backup-v1")
	if err != nil {
		fail(w, 500, "backup encryption failed")
		return
	}
	if err = os.WriteFile(filepath.Join(dir, name), append([]byte("SUBGATE1"), raw...), 0600); err != nil {
		fail(w, 500, "backup write failed")
		return
	}
	writeJSON(w, 201, map[string]any{"name": name, "size": len(raw) + 8, "encrypted": true})
}
func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if filepath.Base(name) != name || strings.ContainsAny(name, "\\/:") || !strings.HasSuffix(name, ".sgb") {
		fail(w, 400, "invalid backup name")
		return
	}
	if err := s.auditAction(r, "backup.download", "backups", name); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name))
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeFile(w, r, filepath.Join(s.config.Database.BackupPath, name))
}
func (s *Server) seedDemo() error {
	var count int
	_ = s.db.QueryRow("SELECT COUNT(*) FROM sg_objects WHERE kind='nodes'").Scan(&count)
	if count > 0 {
		return nil
	}
	for i, item := range []struct{ name, addr, country, status string }{{"Helsinki · Edge 01", "192.0.2.10", "FI", "online"}, {"Amsterdam · Edge 02", "192.0.2.20", "NL", "online"}, {"Frankfurt · Edge 03", "192.0.2.30", "DE", "online"}, {"Stockholm · Edge 04", "192.0.2.40", "SE", "offline"}} {
		id := fmt.Sprintf("demo-node-%d", i+1)
		n := panel.Node{ID: id, Name: item.name, IPAddress: item.addr, Country: item.country, Status: item.status, Tags: []string{"demo", "europe"}, UsersOnline: 0, Inbounds: []panel.Inbound{{Tag: "vless-reality", Type: "vless", Network: "tcp", Security: "reality", Port: 443}}}
		if err := s.saveObject("nodes", id, n); err != nil {
			return err
		}
		if _, err := s.db.Exec("INSERT OR IGNORE INTO nodes(id,name,ip_address,status) VALUES(?,?,?,?)", id, n.Name, n.IPAddress, n.Status); err != nil {
			return err
		}
	}
	for i, name := range []string{"alex", "maria", "nikita", "daria", "operator", "test-user"} {
		id := strconv.Itoa(i + 1)
		u := panel.User{ID: id, ShortUUID: "demo-" + name, Username: name, Email: name + "@example.invalid", Status: "active", TrafficLimitBytes: 100 << 30, UsedTrafficBytes: int64(i+1) * 3 << 30, ExpireAt: time.Now().Add(30 * 24 * time.Hour).Format(time.RFC3339)}
		if i == 5 {
			u.Status = "disabled"
		}
		if err := s.saveObject("users", id, u); err != nil {
			return err
		}
	}
	if err := s.saveObject("tiers", "europe", map[string]any{"id": "europe", "name": "Europe", "strategy": "leastLoad", "min_healthy": 1, "node_ids": []string{"demo-node-1", "demo-node-2", "demo-node-3"}, "enabled": true}); err != nil {
		return err
	}
	return s.saveObject("alerts", "demo-offline", map[string]any{"id": "demo-offline", "name": "Stockholm is offline", "severity": "warning", "node_id": "demo-node-4", "message": "Example alert. Demo data is not a measurement.", "created_at": time.Now()})
}
func csvRows(w http.ResponseWriter, rows [][]string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=activity.csv")
	writer := csv.NewWriter(w)
	for _, row := range rows {
		for i, cell := range row {
			if len(cell) > 0 && strings.ContainsAny(cell[:1], "=+-@\t\r") {
				row[i] = "'" + cell
			}
		}
		_ = writer.Write(row)
	}
	writer.Flush()
}
