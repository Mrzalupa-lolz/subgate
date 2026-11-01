package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/subgate/subgate/core/internal/analysis"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

type monitorState struct {
	Load           analysis.LoadState        `json:"load"`
	Window         *analysis.LoadWindow      `json:"window,omitempty"`
	Bindings       []wire.Binding            `json:"bindings"`
	Flows          map[string]attributedFlow `json:"flows"`
	CollectorError string                    `json:"collector_error"`
	Received       time.Time                 `json:"received"`
}

type attributedFlow struct {
	wire.DPIFlow
	UserID string `json:"attributed_user_id"`
}

func (s *Server) monitorStateTx(tx *sql.Tx, id string) (monitorState, error) {
	state := monitorState{Flows: map[string]attributedFlow{}}
	var encrypted []byte
	err := tx.QueryRow("SELECT payload FROM sg_monitor_states WHERE id=?", id).Scan(&encrypted)
	if err == sql.ErrNoRows {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	raw, err := s.decrypt(encrypted, "monitor:"+id)
	if err == nil {
		err = json.Unmarshal(raw, &state)
	}
	return state, err
}

func validBinding(b wire.Binding) bool {
	if b.UserID == "" || len(b.UserID) > 256 || !validInterface(b.Interface) || len(b.ShapeInterfaces) > 2 || b.ExpiresAt.IsZero() {
		return false
	}
	for _, name := range b.ShapeInterfaces {
		if !validInterface(name) {
			return false
		}
	}
	return true
}

func validInterface(v string) bool {
	if len(v) == 0 || len(v) > 15 || v == "lo" {
		return false
	}
	for _, c := range v {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-' || c == '.') {
			return false
		}
	}
	return true
}

// A binding is an operator attestation about an isolated capture plane, never
// an inference from an IP, destination, nDPI protocol or Xray access-log line.
func bindingFor(bindings []wire.Binding, iface string, now time.Time) *wire.Binding {
	var found *wire.Binding
	for i := range bindings {
		b := &bindings[i]
		if b.Interface == iface {
			if found != nil {
				return nil
			}
			found = b
		}
	}
	if found == nil || !found.Exclusive || found.ValidFrom.IsZero() || found.ValidFrom.After(now) || !found.ExpiresAt.After(now) || len(found.ShapeInterfaces) == 0 {
		return nil
	}
	for _, b := range bindings {
		if b.UserID == found.UserID && b.Interface != found.Interface {
			return nil
		}
		if b.UserID == found.UserID && b.Interface == found.Interface {
			continue
		}
		for _, name := range b.ShapeInterfaces {
			if slices.Contains(found.ShapeInterfaces, name) {
				return nil
			}
		}
	}
	return found
}

func (s *Server) monitorReport(w http.ResponseWriter, r *http.Request) {
	var input wire.MonitorReport
	if !decode(w, r, &input) {
		return
	}
	if input.NodeID != node(r) {
		fail(w, 403, "node identity mismatch")
		return
	}
	now := time.Now().UTC()
	if len(input.Bindings) > 2000 || len(input.Flows) > 2000 || len(input.CollectorError) > 512 {
		fail(w, 400, "monitor report exceeds limits")
		return
	}
	for _, b := range input.Bindings {
		if !validBinding(b) || b.ExpiresAt.After(now.Add(24*time.Hour)) {
			fail(w, 400, "invalid binding")
			return
		}
	}
	for _, f := range input.Flows {
		if f.ID == "" || len(f.ID) > 128 || !validInterface(f.Interface) || len(f.Protocol) > 128 || f.Version == "" || len(f.Version) > 128 || (f.Confidence != "dpi" && f.Confidence != "guessed" && f.Confidence != "unknown") || f.Timestamp.IsZero() || f.Timestamp.Before(now.Add(-5*time.Minute)) || f.Timestamp.After(now.Add(5*time.Second)) {
			fail(w, 400, "invalid DPI metadata")
			return
		}
	}
	if input.Resources != nil && (!analysis.ValidResources(*input.Resources) || input.Resources.Timestamp.Before(now.Add(-30*time.Second)) || input.Resources.Timestamp.After(now.Add(5*time.Second))) {
		fail(w, 400, "invalid resources")
		return
	}
	tx, err := s.db.BeginTx()
	if err != nil {
		fail(w, 500, "monitor transaction failed")
		return
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	state, err := s.monitorStateTx(tx, input.NodeID)
	if err != nil {
		fail(w, 500, "monitor integrity check failed")
		return
	}
	p, err := s.analysisPolicyTx(tx)
	if err != nil {
		fail(w, 500, "policy unavailable")
		return
	}
	p = p.Normalized()
	state.Bindings = input.Bindings
	state.CollectorError = input.CollectorError
	state.Received = now
	for id, f := range state.Flows {
		if f.Timestamp.Before(now.Add(-5 * time.Minute)) {
			delete(state.Flows, id)
		} else if b := bindingFor(state.Bindings, f.Interface, now); b == nil || b.UserID != f.UserID || f.Timestamp.Before(b.ValidFrom) || input.CollectorError != "" {
			f.UserID = ""
			state.Flows[id] = f
		}
	}
	for _, f := range input.Flows {
		key := analysisID(f.Interface, f.ID)
		if old, ok := state.Flows[key]; ok && !f.Timestamp.After(old.Timestamp) {
			continue
		}
		if len(state.Flows) >= 10000 {
			state.CollectorError = "DPI flow capacity exceeded"
			break
		}
		user := ""
		if b := bindingFor(state.Bindings, f.Interface, now); b != nil && !f.Timestamp.Before(b.ValidFrom) && f.Timestamp.Before(b.ExpiresAt) && input.CollectorError == "" {
			user = b.UserID
		}
		if old, ok := state.Flows[key]; ok && old.UserID != user {
			user = ""
		}
		state.Flows[key] = attributedFlow{DPIFlow: f, UserID: user}
		item := incident{ID: analysisID(input.NodeID, user, "dpi", f.Protocol, f.Timestamp.Unix()/60), NodeID: input.NodeID, UserID: user, Status: "new", Signal: analysis.Signal{Kind: "dpi_protocol", Value: 1, Threshold: 1, Target: f.Protocol + " / " + f.Confidence + " / " + f.Version, First: f.Timestamp, Last: f.Timestamp, Truncated: f.Truncated || f.Dropped > 0 || user == "" || input.CollectorError != ""}}
		if err = s.storeIncidentTx(tx, item); err != nil {
			fail(w, 500, "DPI observation failed")
			return
		}
	}
	if input.Resources != nil {
		if completed := state.Load.Add(*input.Resources, p); completed != nil {
			state.Window = completed
			if completed.Active && completed.Complete {
				at := time.Unix(completed.Minute*60, 0).UTC()
				item := incident{ID: analysisID(input.NodeID, "load", completed.Minute), NodeID: input.NodeID, Status: "new", Signal: analysis.Signal{Kind: "sustained_overload", Value: int(completed.Percent), Threshold: p.OverloadPercent, Target: fmt.Sprintf("%.1f Mbps; baseline %.1f Mbps; CPU %.1f%%", completed.Mbps, completed.Baseline, completed.CPU), First: at.Add(-time.Duration(p.SustainedMinutes-1) * time.Minute), Last: at.Add(time.Minute)}}
				if err = s.storeIncidentTx(tx, item); err != nil {
					fail(w, 500, "overload observation failed")
					return
				}
			}
		}
	}
	if err = s.planShaperTx(tx, r, input.NodeID, state, p, now); err != nil {
		fail(w, 500, "shaper planning failed")
		return
	}
	raw, _ := json.Marshal(state)
	encrypted, err := s.encrypt(raw, "monitor:"+input.NodeID)
	if err == nil {
		_, err = tx.Exec("INSERT INTO sg_monitor_states(id,payload,updated_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET payload=excluded.payload,updated_at=excluded.updated_at", input.NodeID, encrypted, now.Unix())
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, "monitor persistence failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) storeIncidentTx(tx *sql.Tx, item incident) error {
	raw, err := json.Marshal(item)
	if err != nil {
		return err
	}
	encrypted, err := s.encrypt(raw, "incident:"+item.ID)
	if err != nil {
		return err
	}
	_, err = tx.Exec("INSERT INTO sg_incidents(id,updated_at,status,payload) VALUES(?,?,'new',?) ON CONFLICT(id) DO UPDATE SET updated_at=excluded.updated_at,payload=excluded.payload", item.ID, item.Last.Unix(), encrypted)
	return err
}

// Independent evidence: packet DPI on an exclusive account interface, repeated
// distinct flows in two minutes, and sustained node load with measured Xray
// account contribution. A policy must explicitly restrict the detected protocol.
func (s *Server) planShaperTx(tx *sql.Tx, r *http.Request, nodeID string, state monitorState, p analysis.Policy, now time.Time) error {
	if !p.AutomaticShaper || state.CollectorError != "" || state.Window == nil || !state.Window.Active || !state.Window.Complete || !state.Load.Active || state.Load.Dropped > 0 || state.Load.Last.Before(now.Add(-20*time.Second)) || state.Window.Minute < now.Unix()/60-2 {
		return nil
	}
	for _, b := range state.Bindings {
		if bindingFor(state.Bindings, b.Interface, now) == nil || b.ExpiresAt.Before(now.Add(time.Duration(p.ShaperMinutes)*time.Minute)) || slices.Contains(p.Allowlist, b.UserID) {
			continue
		}
		var user panel.User
		var raw string
		err := tx.QueryRow("SELECT data FROM sg_objects WHERE kind='users' AND id=?", b.UserID).Scan(&raw)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		if err = json.Unmarshal([]byte(raw), &user); err != nil {
			return err
		}
		if user.ID != b.UserID {
			continue
		}
		aliases := []string{}
		for _, alias := range []string{b.UserID, user.Username, user.Email, user.ShortUUID} {
			if alias == "" || slices.Contains(aliases, alias) {
				continue
			}
			var n int
			err = tx.QueryRow("SELECT COUNT(*) FROM sg_objects WHERE kind='users' AND id!=? AND (id=? OR json_extract(data,'$.username')=? OR json_extract(data,'$.email')=? OR json_extract(data,'$.short_uuid')=?)", b.UserID, alias, alias, alias, alias).Scan(&n)
			if err != nil {
				return err
			}
			if n == 0 {
				aliases = append(aliases, alias)
			}
		}
		if len(aliases) == 0 {
			continue
		}
		var previous int64
		var active int
		if err = tx.QueryRow("SELECT COALESCE(MAX(created_at),0),COALESCE(SUM(CASE WHEN expires_at>? AND status!='cancelled' AND status!='removed' THEN 1 ELSE 0 END),0) FROM sg_shaper_actions WHERE node_id=? AND user_id=?", now.Unix(), nodeID, b.UserID).Scan(&previous, &active); err != nil {
			return err
		}
		if active > 0 {
			continue
		}
		minutes := map[int64]bool{}
		flows := map[string]bool{}
		protocolMinutes := map[string]map[int64]bool{}
		protocolFlows := map[string]map[string]bool{}
		protocol := ""
		bad := false
		for _, f := range state.Flows {
			if f.Interface != b.Interface || f.UserID != b.UserID || f.Timestamp.Before(b.ValidFrom) || f.Timestamp.Before(now.Add(-3*time.Minute)) {
				continue
			}
			if f.Truncated || f.Dropped > 0 {
				bad = true
			}
			if f.Timestamp.Unix() <= previous || f.Confidence != "dpi" || !slices.Contains(p.RestrictedProtocols, f.Protocol) {
				continue
			}
			minutes[f.Timestamp.Unix()/60] = true
			flows[f.ID] = true
			if protocolMinutes[f.Protocol] == nil {
				protocolMinutes[f.Protocol] = map[int64]bool{}
				protocolFlows[f.Protocol] = map[string]bool{}
			}
			protocolMinutes[f.Protocol][f.Timestamp.Unix()/60] = true
			protocolFlows[f.Protocol][f.ID] = true
		}
		for _, name := range p.RestrictedProtocols {
			if len(protocolMinutes[name]) >= 2 && len(protocolFlows[name]) >= 2 {
				protocol = name
				break
			}
		}
		if bad || len(minutes) < 2 || len(flows) < 2 || protocol == "" {
			continue
		}
		// Require full, current, non-overlapping Xray sampling for this account.
		predicate, args := identityPredicate(aliases)
		query := "SELECT s.timestamp,MAX(s.uplink,s.downlink),m.interval_ms,m.gap,m.warming FROM sg_speed s JOIN sg_speed_meta m USING(node_id,user_id,timestamp) WHERE s.node_id=? AND s." + predicate + " AND m.scope='user' AND s.timestamp>=? AND s.timestamp<? ORDER BY s.timestamp"
		params := []any{nodeID}
		params = append(params, args...)
		params = append(params, state.Window.Minute*60000, (state.Window.Minute+1)*60000)
		rows, err := tx.Query(query, params...)
		if err != nil {
			return err
		}
		var covered, last int64
		var weighted float64
		quality := true
		for rows.Next() {
			var at, bps, interval int64
			var gap, warming bool
			if err = rows.Scan(&at, &bps, &interval, &gap, &warming); err != nil {
				break
			}
			if gap || warming || interval < 1000 || interval > 15000 || at-interval < last || at-interval < state.Window.Minute*60000 {
				quality = false
			}
			covered += interval
			weighted += float64(bps) * float64(interval)
			last = at
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return err
		}
		if !quality || covered < 45000 || covered > 60000 || weighted/float64(max(covered, 1))/1e6 < state.Window.Mbps*.35 {
			continue
		}
		a := wire.ShaperAction{ID: token(), NodeID: nodeID, UserID: b.UserID, Mbps: p.ShaperMbps, CreatedAt: now, ExpiresAt: now.Add(time.Duration(p.ShaperMinutes) * time.Minute), Protocol: protocol, Reason: "restricted protocol " + protocol + "; repeated DPI flows; sustained overload; Xray contribution >=35%"}
		rawAction, _ := json.Marshal(a)
		encrypted, err := s.encrypt(rawAction, "shaper:"+a.ID)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO sg_shaper_actions(id,node_id,user_id,status,created_at,expires_at,payload) VALUES(?,?,?,'pending',?,?,?)", a.ID, nodeID, b.UserID, now.Unix(), a.ExpiresAt.Unix(), encrypted); err != nil {
			return err
		}
		if err = s.auditTx(tx, r, nil, "shaper.plan", "shaper", a.ID, "success"); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) shaperCommands(w http.ResponseWriter, r *http.Request) {
	tx, err := s.db.BeginTx()
	if err != nil {
		fail(w, 500, "command transaction failed")
		return
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	p, err := s.analysisPolicyTx(tx)
	if err != nil {
		fail(w, 500, "policy unavailable")
		return
	}
	state, err := s.monitorStateTx(tx, node(r))
	if err != nil {
		fail(w, 500, "monitor state unavailable")
		return
	}
	actions := []wire.ShaperAction{}
	now := time.Now()
	rows, err := tx.Query("SELECT id,payload FROM sg_shaper_actions WHERE node_id=? AND expires_at>? AND status IN ('pending','applied','failed') ORDER BY created_at LIMIT 2000", node(r), now.Unix())
	if err != nil {
		fail(w, 500, "commands unavailable")
		return
	}
	for rows.Next() {
		var id string
		var encrypted []byte
		if err = rows.Scan(&id, &encrypted); err != nil {
			break
		}
		var raw []byte
		raw, err = s.decrypt(encrypted, "shaper:"+id)
		if err != nil {
			break
		}
		var a wire.ShaperAction
		if err = json.Unmarshal(raw, &a); err != nil {
			break
		}
		bound := false
		for _, b := range state.Bindings {
			if b.UserID == a.UserID && bindingFor(state.Bindings, b.Interface, now) != nil {
				bound = true
			}
		}
		if p.AutomaticShaper && state.CollectorError == "" && slices.Contains(p.RestrictedProtocols, a.Protocol) && !slices.Contains(p.Allowlist, a.UserID) && bound && state.Received.After(now.Add(-30*time.Second)) {
			actions = append(actions, a)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "command integrity check failed")
		return
	}
	writeJSON(w, 200, map[string]any{"data": actions, "server_time": now.UTC()})
}

func (s *Server) shaperResults(w http.ResponseWriter, r *http.Request) {
	var results []wire.ShaperResult
	if !decode(w, r, &results) {
		return
	}
	if len(results) > 2000 {
		fail(w, 400, "too many results")
		return
	}
	for _, v := range results {
		if (v.Status != "applied" && v.Status != "removed" && v.Status != "failed") || len(v.Error) > 512 {
			fail(w, 400, "invalid shaper result")
			return
		}
	}
	tx, err := s.db.BeginTx()
	if err != nil {
		fail(w, 500, "result transaction failed")
		return
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	for _, v := range results {
		raw, _ := json.Marshal(v)
		encrypted, e := s.encrypt(raw, "shaper-result:"+v.ID)
		if e != nil {
			err = e
			break
		}
		var previous string
		e = tx.QueryRow("SELECT status FROM sg_shaper_actions WHERE id=? AND node_id=?", v.ID, node(r)).Scan(&previous)
		if e == sql.ErrNoRows {
			fail(w, 404, "command not found")
			return
		}
		if e != nil {
			err = e
			break
		}
		if previous == "cancelled" && v.Status != "removed" {
			continue
		}
		if previous == v.Status {
			continue
		}
		_, err = tx.Exec("UPDATE sg_shaper_actions SET status=?,report=? WHERE id=? AND node_id=?", v.Status, encrypted, v.ID, node(r))
		if err == nil {
			err = s.auditTx(tx, r, nil, "shaper."+v.Status, "shaper", v.ID, "success")
		}
		if err != nil {
			break
		}
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, "shaper result persistence failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) listShaper(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id,status,payload,report FROM sg_shaper_actions ORDER BY created_at DESC LIMIT ?", limit(r, 200, 500))
	if err != nil {
		fail(w, 500, "shaper query failed")
		return
	}
	items := []map[string]any{}
	for rows.Next() {
		var id, status string
		var payload, report []byte
		if err = rows.Scan(&id, &status, &payload, &report); err != nil {
			break
		}
		var raw []byte
		raw, err = s.decrypt(payload, "shaper:"+id)
		if err != nil {
			break
		}
		var a wire.ShaperAction
		if err = json.Unmarshal(raw, &a); err != nil {
			break
		}
		var result wire.ShaperResult
		if len(report) > 0 {
			raw, err = s.decrypt(report, "shaper-result:"+id)
			if err == nil {
				err = json.Unmarshal(raw, &result)
			}
			if err != nil {
				break
			}
		}
		if a.ExpiresAt.Before(time.Now()) && status != "removed" && status != "cancelled" {
			status = "expired"
		}
		items = append(items, map[string]any{"action": a, "status": status, "result": result})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "shaper integrity check failed")
		return
	}
	if err = s.auditAction(r, "shaper.read", "shaper", ""); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}

func (s *Server) cancelShaper(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	tx, err := s.db.BeginTx()
	if err != nil {
		fail(w, 500, "cancel transaction failed")
		return
	}
	defer s.db.TxUnlock()
	defer tx.Rollback()
	res, err := tx.Exec("UPDATE sg_shaper_actions SET status='cancelled' WHERE id=?", id)
	if err == nil {
		n, _ := res.RowsAffected()
		if n == 0 {
			fail(w, 404, "command not found")
			return
		}
		p := admin(r)
		err = s.auditTx(tx, r, &p, "shaper.cancel", "shaper", id, "success")
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 500, "cancel failed")
		return
	}
	writeJSON(w, 200, map[string]bool{"ok": true})
}

func (s *Server) listMonitor(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.Query("SELECT id,payload FROM sg_monitor_states ORDER BY id LIMIT 2000")
	if err != nil {
		fail(w, 500, "monitor query failed")
		return
	}
	items := []map[string]any{}
	for rows.Next() {
		var id string
		var encrypted []byte
		if err = rows.Scan(&id, &encrypted); err != nil {
			break
		}
		var raw []byte
		raw, err = s.decrypt(encrypted, "monitor:"+id)
		if err != nil {
			break
		}
		var state monitorState
		if err = json.Unmarshal(raw, &state); err != nil {
			break
		}
		items = append(items, map[string]any{"node_id": id, "received_at": state.Received, "capacity_mbps": state.Load.Capacity, "overloaded": state.Load.Active, "baseline_mbps": state.Load.Baseline, "baseline_windows": state.Load.BaselineWindows, "window": state.Window, "collector_error": state.CollectorError, "flows": len(state.Flows)})
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 500, "monitor integrity check failed")
		return
	}
	if err = s.auditAction(r, "monitor.read", "monitor", ""); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	writeJSON(w, 200, map[string]any{"data": items})
}
