package server

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/analysis"
	"github.com/subgate/subgate/pkg/wire"
)

func TestAnalysisTelemetryAtomicDedupAndReview(t *testing.T) {
	s := newTestServer(t)
	owner := loginTest(t, s)
	credential, _ := enrollTest(t, s, owner, "demo-node-1")
	p := analysis.DefaultPolicy()
	p.Ports = 16
	if w := perform(s, "PUT", "/api/v1/analysis/policy", p, owner, "", true); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	// Events all belong to a single, recent UTC minute, including near boundaries.
	at := time.Now().UTC().Truncate(time.Minute).Add(-time.Minute)
	batch := wire.Telemetry{BatchID: "analysis-1", NodeID: "demo-node-1"}
	for i := 1; i <= 16; i++ {
		batch.Events = append(batch.Events, wire.Activity{UserID: "1", Timestamp: at.Add(time.Duration(i) * time.Millisecond), DstHost: "evidence.example", DstPort: i, Result: "accepted"})
	}
	ingest := func() {
		t.Helper()
		if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
			t.Fatal(w.Code, w.Body)
		}
	}
	// A failed analysis write must roll back both the batch receipt and journal.
	if _, err := s.db.Exec("CREATE TRIGGER analysis_failure BEFORE INSERT ON sg_incidents BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 500 {
		t.Fatal(w.Code)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_activity").Scan(&count); err != nil || count != 0 {
		t.Fatal("journal committed without analysis", count, err)
	}
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_analysis_seen").Scan(&count); err != nil || count != 0 {
		t.Fatal("deduplication ledger committed without analysis", count, err)
	}
	if _, err := s.db.Exec("DROP TRIGGER analysis_failure"); err != nil {
		t.Fatal(err)
	}
	ingest()
	ingest()
	batch.BatchID = "repacked"
	ingest()
	list := func() []incident {
		t.Helper()
		w := perform(s, "GET", "/api/v1/incidents", nil, owner, "", false)
		var result struct {
			Data []incident `json:"data"`
		}
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &result) != nil {
			t.Fatal(w.Code, w.Body)
		}
		return result.Data
	}
	items := list()
	if len(items) != 1 || items[0].Value != 16 {
		t.Fatalf("duplicate evidence: %+v", items)
	}
	id := items[0].ID
	if _, err := s.db.Exec("CREATE TRIGGER review_audit_failure BEFORE INSERT ON audit_log WHEN NEW.action LIKE 'incident.review.%' BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "PUT", "/api/v1/incidents/"+id+"/review", map[string]string{"status": "confirmed"}, owner, "", true); w.Code != 500 {
		t.Fatal("review without audit committed", w.Code)
	}
	if items := list(); items[0].Status != "new" {
		t.Fatal("review survived audit failure", items[0].Status)
	}
	if _, err := s.db.Exec("DROP TRIGGER review_audit_failure"); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "GET", "/api/v1/incidents", nil, nil, "", false); w.Code != 401 {
		t.Fatal("anonymous evidence", w.Code)
	}
	if w := perform(s, "PUT", "/api/v1/incidents/"+id+"/review", map[string]string{"status": "false_positive", "note": "known workload"}, owner, "", false); w.Code != 403 {
		t.Fatal("missing CSRF accepted", w.Code)
	}
	if w := perform(s, "PUT", "/api/v1/incidents/"+id+"/review", map[string]string{"status": "false_positive", "note": "known workload"}, owner, "", true); w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	// A fresh observer has no private process-local aggregate to recover.
	observer := &Server{db: s.db, aead: s.aead, previousAEADs: s.previousAEADs}
	tx, err := s.db.BeginTx()
	if err != nil {
		t.Fatal(err)
	}
	batch.Events = append(batch.Events, wire.Activity{UserID: "1", Timestamp: at.Add(time.Second), DstHost: "evidence.example", DstPort: 17})
	err = observer.analyzeTelemetry(tx, batch, time.Now())
	if err == nil {
		err = tx.Commit()
	} else {
		tx.Rollback()
	}
	s.db.TxUnlock()
	if err != nil {
		t.Fatal(err)
	}
	items = list()
	if len(items) != 1 || items[0].Status != "false_positive" || items[0].Note != "known workload" || items[0].Value != 17 {
		t.Fatalf("review lost on new evidence: %+v", items)
	}
	var payload, review []byte
	if err := s.db.QueryRow("SELECT payload,review FROM sg_incidents WHERE id=?", id).Scan(&payload, &review); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "evidence.example") || strings.Contains(string(review), "known workload") {
		t.Fatal("plaintext evidence persisted")
	}
	// Late batches cannot generate a new present-time observation.
	batch.BatchID = "late"
	for i := range batch.Events {
		batch.Events[i].Timestamp = at.Add(-10 * time.Minute)
	}
	ingest()
	if len(list()) != 1 {
		t.Fatal("late batch generated current incident")
	}
}

func TestAnalysisPolicyAndReviewAuditAreAtomic(t *testing.T) {
	s := newTestServer(t)
	owner := loginTest(t, s)
	if w := perform(s, "GET", "/api/v1/analysis/policy", nil, owner, "", false); w.Code != 200 || !strings.Contains(w.Body.String(), `"shaper_mbps":0`) {
		t.Fatal(w.Code, w.Body)
	}
	p := analysis.DefaultPolicy()
	p.Ports = 1
	if w := perform(s, "PUT", "/api/v1/analysis/policy", p, owner, "", true); w.Code != 400 {
		t.Fatal("invalid threshold accepted", w.Code)
	}
	if _, err := s.db.Exec("CREATE TRIGGER analysis_audit_failure BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	p = analysis.DefaultPolicy()
	p.Ports = 32
	if w := perform(s, "PUT", "/api/v1/analysis/policy", p, owner, "", true); w.Code != 500 {
		t.Fatal(w.Code)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_objects WHERE kind='analysis-policy'").Scan(&count); err != nil || count != 0 {
		t.Fatal("policy survived failed audit", count, err)
	}
}
