package server

import (
	"encoding/json"
	"github.com/subgate/subgate/pkg/wire"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestActivityReadExportAuditFilters(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	now := time.Now().UTC().Truncate(time.Second)
	batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: "audit-filters", Events: []wire.Activity{{UserID: "1", Timestamp: now, DstHost: "private.example.invalid", DstPort: 443}}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	filters := url.Values{"user": {"1"}, "node": {"demo-node-1"}, "domain": {"PRIVATE.example.invalid"}, "from": {now.Add(-time.Hour).Format(time.RFC3339)}, "to": {now.Add(time.Hour).Format(time.RFC3339)}, "limit": {"999999"}}
	for _, export := range []bool{false, true} {
		path := "/api/v1/activity"
		action := "activity.read"
		max := 1000
		if export {
			path += "/export"
			action = "activity.export"
			max = 10000
			filters.Set("format", "csv")
		}
		if w := perform(s, "GET", path+"?"+filters.Encode(), nil, session, "", false); w.Code != 200 || !strings.Contains(w.Body.String(), "private.example.invalid") {
			t.Fatalf("data response: %d %s", w.Code, w.Body)
		}
		var details string
		if err := s.db.QueryRow("SELECT COALESCE(details,'') FROM audit_log WHERE action=? AND status='success' ORDER BY id DESC LIMIT 1", action).Scan(&details); err != nil || details == "" {
			t.Errorf("missing %s filter audit: %v", action, err)
		}
		if strings.Contains(details, "private.example.invalid") || strings.Contains(details, "PRIVATE.example.invalid") {
			t.Error("sensitive filter persisted in plaintext")
		}
		w := perform(s, "GET", "/api/v1/audit", nil, session, "", false)
		var result struct {
			Data []struct {
				Action  string         `json:"action"`
				Details map[string]any `json:"details"`
			} `json:"data"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatalf("audit list: %d %s", w.Code, w.Body)
		}
		found := false
		for _, row := range result.Data {
			if row.Action != action {
				continue
			}
			found = true
			for _, key := range []string{"user", "node", "from", "to"} {
				if row.Details[key] != filters.Get(key) {
					t.Errorf("%s missing filter %s: %v", action, key, row.Details)
				}
			}
			if row.Details["domain"] != "private.example.invalid" || row.Details["limit"] != float64(max) || row.Details["returned"] != float64(1) {
				t.Errorf("%s effective audit: %v", action, row.Details)
			}
			format := "json"
			if export {
				format = "csv"
			}
			if row.Details["format"] != format {
				t.Errorf("missing format %v", row.Details)
			}
		}
		if !found {
			t.Errorf("audit action %s not visible", action)
		}
	}
	if _, err := s.db.Exec("CREATE TRIGGER reject_read_audit BEFORE INSERT ON audit_log WHEN NEW.action='activity.read' BEGIN SELECT RAISE(ABORT,'synthetic'); END"); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "GET", "/api/v1/activity?user=1", nil, session, "", false); w.Code != 500 || strings.Contains(w.Body.String(), "private.example.invalid") {
		t.Errorf("audit failure leaked data: %d %s", w.Code, w.Body)
	}
}

func TestActivityReadExportSharedRateLimit(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	for i := 0; i < 60; i++ {
		path := "/api/v1/activity"
		if i%2 == 1 {
			path += "/export"
		}
		if w := perform(s, "GET", path, nil, session, "", false); w.Code != 200 {
			t.Fatalf("read %d: %d", i, w.Code)
		}
	}
	for _, path := range []string{"/api/v1/activity", "/api/v1/activity/export"} {
		if w := perform(s, "GET", path, nil, session, "", false); w.Code != 429 || w.Header().Get("Retry-After") == "" {
			t.Errorf("shared budget %s: %d %s", path, w.Code, w.Header())
		}
	}
	if w := perform(s, "GET", "/api/v1/auth/me", nil, session, "", false); w.Code != 200 {
		t.Fatal("read budget blocked console")
	}
}

func TestActivityInvalidFiltersAreNotSuccessfulReads(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	for _, query := range []string{"from=invalid", "from=2026-01-02T00:00:00Z&to=2026-01-01T00:00:00Z", "format=invalid", "user=1&user=2", "q=unsupported"} {
		if w := perform(s, "GET", "/api/v1/activity?"+query, nil, session, "", false); w.Code != 400 {
			t.Errorf("invalid %s: %d", query, w.Code)
		}
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='activity.read' AND status='success'").Scan(&count); err != nil || count != 0 {
		t.Errorf("invalid reads audited as success: %d %v", count, err)
	}
}

func TestActivityAuditIntegrityAndQueryFailure(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	if w := perform(s, "GET", "/api/v1/activity?domain=private.example.invalid", nil, session, "", false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := s.db.Exec("UPDATE audit_log SET action='activity.export' WHERE action='activity.read'"); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "GET", "/api/v1/audit", nil, session, "", false); w.Code != 500 || strings.Contains(w.Body.String(), "private.example.invalid") {
		t.Errorf("tampered audit exposed: %d %s", w.Code, w.Body)
	}
	if _, err := s.db.Exec("DROP TABLE sg_activity"); err != nil {
		t.Fatal(err)
	} // synthetic in-memory state only
	if w := perform(s, "GET", "/api/v1/activity", nil, session, "", false); w.Code != 500 || strings.Contains(w.Body.String(), "sg_activity") {
		t.Errorf("query error exposed internals: %d %s", w.Code, w.Body)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='activity.read' AND status='failure'").Scan(&count); err != nil || count != 1 {
		t.Fatal("query failure outcome", count, err)
	}
}

func TestActivityReadBudgetRecovery(t *testing.T) {
	s := newTestServer(t)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 60; i++ {
		if ok, _ := s.allowActivityRead(start); !ok {
			t.Fatal("early exhaustion", i)
		}
	}
	if ok, retry := s.allowActivityRead(start.Add(59 * time.Second)); ok || retry < 1 || retry > 2 {
		t.Fatal("window limit", ok, retry)
	}
	if ok, _ := s.allowActivityRead(start.Add(time.Minute)); !ok {
		t.Fatal("budget never recovered")
	}
}

func TestActivityReadAuditsActualJSONFormat(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	if w := perform(s, "GET", "/api/v1/activity?format=csv", nil, session, "", false); w.Code != 400 {
		t.Fatalf("read accepts mismatched export format: %d %s", w.Code, w.Header())
	}
}
