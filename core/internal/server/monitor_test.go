package server

import (
	"encoding/json"
	"fmt"
	"github.com/subgate/subgate/core/internal/analysis"
	"github.com/subgate/subgate/pkg/wire"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMonitorIndependentEvidenceActionsAuditAndCancellation(t *testing.T) {
	s := newTestServer(t)
	owner := loginTest(t, s)
	credential, _ := enrollTest(t, s, owner, "demo-node-1")
	now := time.Now().UTC()
	minute := now.Truncate(time.Minute).Add(-time.Minute)
	p := analysis.DefaultPolicy()
	p.AutomaticShaper = true
	p.ShaperMbps = 10
	p.ShaperMinutes = 5
	p.RestrictedProtocols = []string{"BitTorrent"}
	if w := perform(s, "PUT", "/api/v1/analysis/policy", p, owner, "", true); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	state := monitorState{Flows: map[string]attributedFlow{}, Load: analysis.LoadState{Active: true, Last: now}, Window: &analysis.LoadWindow{Minute: minute.Unix() / 60, Mbps: 900, Active: true, Complete: true}}
	raw, _ := json.Marshal(state)
	encrypted, _ := s.encrypt(raw, "monitor:demo-node-1")
	if _, err := s.db.Exec("INSERT INTO sg_monitor_states(id,payload,updated_at) VALUES(?,?,?)", "demo-node-1", encrypted, now.Unix()); err != nil {
		t.Fatal(err)
	}
	// Independent Xray samples, including metadata and complete minute coverage.
	batch := wire.Telemetry{BatchID: "independent-counters", NodeID: "demo-node-1"}
	for i := 1; i <= 59; i++ {
		batch.Samples = append(batch.Samples, wire.Sample{UserID: "1", Timestamp: minute.Add(time.Duration(i) * time.Second), IntervalMS: 1000, UplinkBps: 500000000})
	}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	binding := wire.Binding{UserID: "1", Interface: "sg-one", ShapeInterfaces: []string{"sg-one", "sg-peer"}, Exclusive: true, ValidFrom: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}
	report := wire.MonitorReport{NodeID: "demo-node-1", Bindings: []wire.Binding{binding}, Flows: []wire.DPIFlow{{ID: "flow1", Interface: "sg-one", Protocol: "BitTorrent", Confidence: "dpi", Version: "nDPI 4.2", Timestamp: minute.Add(-time.Second), Bytes: 1000, Packets: 10}}}
	post := func(want int) {
		t.Helper()
		if w := perform(s, "POST", "/api/v1/agents/monitor", report, nil, credential, false); w.Code != want {
			t.Fatal(w.Code, w.Body)
		}
	}
	countActions := func() int {
		t.Helper()
		var n int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_shaper_actions").Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	post(200)
	post(200)
	if countActions() != 0 {
		t.Fatal("single/replayed flow triggered sanction")
	}
	report.Flows = append(report.Flows, wire.DPIFlow{ID: "flow2", Interface: "sg-one", Protocol: "BitTorrent", Confidence: "dpi", Version: "nDPI 4.2", Timestamp: minute.Add(20 * time.Second), Bytes: 1000, Packets: 10})
	report.CollectorError = "lost packets"
	post(200)
	if countActions() != 0 {
		t.Fatal("loss allowed automatic action")
	}
	report.CollectorError = ""
	report.Bindings[0].Exclusive = false
	post(200)
	if countActions() != 0 {
		t.Fatal("shared capture allowed sanction")
	}
	// Previously ambiguous flow IDs stay untrusted even after the binding improves.
	report.Bindings[0].Exclusive = true
	post(200)
	if countActions() != 0 {
		t.Fatal("ambiguous evidence was retroactively attributed")
	}
	report.Flows[0].ID = "verified1"
	report.Flows[1].ID = "verified2"
	if _, err := s.db.Exec("CREATE TRIGGER shaper_audit_failure BEFORE INSERT ON audit_log WHEN NEW.action='shaper.plan' BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	post(500)
	if countActions() != 0 {
		t.Fatal("action survived audit failure")
	}
	if _, err := s.db.Exec("DROP TRIGGER shaper_audit_failure"); err != nil {
		t.Fatal(err)
	}
	post(200)
	post(200)
	if countActions() != 1 {
		t.Fatal("missing or duplicated action", countActions())
	}
	w := perform(s, "GET", "/api/v1/agents/shaper", nil, nil, credential, false)
	var commands struct {
		Data []wire.ShaperAction `json:"data"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &commands) != nil || len(commands.Data) != 1 {
		t.Fatal(w.Code, w.Body)
	}
	a := commands.Data[0]
	if a.Mbps != 10 || a.UserID != "1" || a.ExpiresAt.Sub(a.CreatedAt) != 5*time.Minute {
		t.Fatal(a)
	}
	if w := perform(s, "PUT", "/api/v1/agents/shaper-results", []wire.ShaperResult{{ID: a.ID, Status: "applied"}}, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	if w := perform(s, "GET", "/api/v1/analysis/shaper", nil, owner, "", false); w.Code != 200 || !strings.Contains(w.Body.String(), "applied") {
		t.Fatal(w.Code, w.Body)
	}
	if w := perform(s, "DELETE", "/api/v1/analysis/shaper/"+a.ID, nil, owner, "", false); w.Code != 403 {
		t.Fatal("cancel without CSRF accepted")
	}
	if w := perform(s, "DELETE", "/api/v1/analysis/shaper/"+a.ID, nil, owner, "", true); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = perform(s, "GET", "/api/v1/agents/shaper", nil, nil, credential, false)
	if json.Unmarshal(w.Body.Bytes(), &commands) != nil || len(commands.Data) != 0 {
		t.Fatal("cancelled action still desired", w.Body)
	}
	if w := perform(s, "PUT", "/api/v1/agents/shaper-results", []wire.ShaperResult{{ID: a.ID, Status: "removed"}}, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	var payload []byte
	if err := s.db.QueryRow("SELECT payload FROM sg_shaper_actions WHERE id=?", a.ID).Scan(&payload); err != nil || strings.Contains(string(payload), "BitTorrent") {
		t.Fatal("plaintext shaper reason persisted", err)
	}
}

func TestMonitorDecisionRejectsWeakEvidence(t *testing.T) {
	s := newTestServer(t)
	now := time.Now().UTC()
	minute := now.Truncate(time.Minute).Add(-time.Minute)
	for tick := 1; tick <= 59; tick++ {
		at := minute.Add(time.Duration(tick) * time.Second).UnixMilli()
		if _, err := s.db.Exec("INSERT INTO sg_speed(node_id,user_id,timestamp,uplink,downlink) VALUES('demo-node-1','1',?,500000000,0)", at); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec("INSERT INTO sg_speed_meta(node_id,user_id,timestamp,interval_ms,scope,received_at) VALUES('demo-node-1','1',?,1000,'user',?)", at, now.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	base := monitorState{Load: analysis.LoadState{Active: true, Last: now}, Window: &analysis.LoadWindow{Minute: minute.Unix() / 60, Mbps: 900, Active: true, Complete: true}, Bindings: []wire.Binding{{UserID: "1", Interface: "sg-one", ShapeInterfaces: []string{"sg-one"}, Exclusive: true, ValidFrom: now.Add(-time.Hour), ExpiresAt: now.Add(time.Hour)}}, Flows: map[string]attributedFlow{
		"one": {UserID: "1", DPIFlow: wire.DPIFlow{ID: "one", Interface: "sg-one", Protocol: "BitTorrent", Confidence: "dpi", Timestamp: minute.Add(-time.Second)}},
		"two": {UserID: "1", DPIFlow: wire.DPIFlow{ID: "two", Interface: "sg-one", Protocol: "BitTorrent", Confidence: "dpi", Timestamp: minute.Add(20 * time.Second)}},
	}}
	for name, change := range map[string]func(*monitorState, *analysis.Policy){
		"disabled":              func(_ *monitorState, p *analysis.Policy) { p.AutomaticShaper = false },
		"allowlist":             func(_ *monitorState, p *analysis.Policy) { p.Allowlist = []string{"1"} },
		"unrestricted protocol": func(_ *monitorState, p *analysis.Policy) { p.RestrictedProtocols = []string{"HTTP"} },
		"single flow":           func(st *monitorState, _ *analysis.Policy) { delete(st.Flows, "two") },
		"cache guess": func(st *monitorState, _ *analysis.Policy) {
			f := st.Flows["two"]
			f.Confidence = "guessed"
			st.Flows["two"] = f
		},
		"capture loss":      func(st *monitorState, _ *analysis.Policy) { f := st.Flows["two"]; f.Dropped = 1; st.Flows["two"] = f },
		"no overload":       func(st *monitorState, _ *analysis.Policy) { st.Window.Active = false },
		"partial resources": func(st *monitorState, _ *analysis.Policy) { st.Window.Complete = false },
		"stale resources":   func(st *monitorState, _ *analysis.Policy) { st.Load.Last = now.Add(-time.Minute) },
		"shared path":       func(st *monitorState, _ *analysis.Policy) { st.Bindings[0].Exclusive = false },
		"unknown account": func(st *monitorState, _ *analysis.Policy) {
			st.Bindings[0].UserID = "unknown"
			for id, f := range st.Flows {
				f.UserID = "unknown"
				st.Flows[id] = f
			}
		},
		"binding ends before TTL": func(st *monitorState, _ *analysis.Policy) { st.Bindings[0].ExpiresAt = now.Add(time.Second) },
		"different restricted protocols": func(st *monitorState, p *analysis.Policy) {
			p.RestrictedProtocols = append(p.RestrictedProtocols, "HTTP")
			f := st.Flows["two"]
			f.Protocol = "HTTP"
			st.Flows["two"] = f
		},
		"ambiguous statistics label": func(_ *monitorState, _ *analysis.Policy) {},
	} {
		t.Run(name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var state monitorState
			json.Unmarshal(raw, &state)
			p := analysis.DefaultPolicy()
			p.AutomaticShaper = true
			p.ShaperMbps = 10
			p.ShaperMinutes = 5
			p.RestrictedProtocols = []string{"BitTorrent"}
			change(&state, &p)
			tx, err := s.db.BeginTx()
			if err != nil {
				t.Fatal(err)
			}
			defer s.db.TxUnlock()
			defer tx.Rollback()
			if name == "ambiguous statistics label" {
				if _, err = tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('users','conflicting-account','{\"id\":\"conflicting-account\",\"username\":\"1\"}',?)", now.Unix()); err != nil {
					t.Fatal(err)
				}
			}
			if err = s.planShaperTx(tx, httptest.NewRequest("POST", "/", nil), "demo-node-1", state, p, now); err != nil {
				t.Fatal(err)
			}
			var n int
			if err = tx.QueryRow("SELECT COUNT(*) FROM sg_shaper_actions").Scan(&n); err != nil || n != 0 {
				t.Fatal("weak evidence produced a sanction", n, err)
			}
		})
	}
}

func TestMonitorValidationBindingIsolationAndPolicy(t *testing.T) {
	s := newTestServer(t)
	owner := loginTest(t, s)
	credential, _ := enrollTest(t, s, owner, "demo-node-1")
	for name, report := range map[string]wire.MonitorReport{
		"spoof":            {NodeID: "other"},
		"unsafe interface": {NodeID: "demo-node-1", Bindings: []wire.Binding{{UserID: "1", Interface: "eth0;reboot", ExpiresAt: time.Now()}}},
		"invalid quality":  {NodeID: "demo-node-1", Flows: []wire.DPIFlow{{ID: "a", Interface: "sg-one", Version: "4.2", Timestamp: time.Now(), Confidence: "100%"}}},
		"zero capacity":    {NodeID: "demo-node-1", Resources: &wire.Resources{Timestamp: time.Now(), IntervalMS: 10000}},
	} {
		t.Run(name, func(t *testing.T) {
			want := 400
			if name == "spoof" {
				want = 403
			}
			if w := perform(s, "POST", "/api/v1/agents/monitor", report, nil, credential, false); w.Code != want {
				t.Fatal(w.Code, w.Body)
			}
		})
	}
	p := analysis.DefaultPolicy()
	p.AutomaticShaper = true
	if w := perform(s, "PUT", "/api/v1/analysis/policy", p, owner, "", true); w.Code != 400 {
		t.Fatal("unset limits enabled automation")
	}
	now := time.Now()
	b := wire.Binding{UserID: "one", Interface: "sg-one", Exclusive: true, ValidFrom: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), ShapeInterfaces: []string{"sg-peer"}}
	for i, other := range []wire.Binding{{UserID: "two", Interface: "sg-one"}, {UserID: "two", Interface: "sg-two", ShapeInterfaces: []string{"sg-peer"}}, {UserID: "one", Interface: "sg-two"}} {
		t.Run(fmt.Sprint(i), func(t *testing.T) {
			if bindingFor([]wire.Binding{b, other}, "sg-one", now) != nil {
				t.Fatal("ambiguous binding accepted")
			}
		})
	}
	if w := perform(s, "GET", "/api/v1/analysis/shaper", nil, nil, "", false); w.Code != 401 {
		t.Fatal("unauthenticated evidence read")
	}
}
