package server

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/db"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/core/internal/quota"
	"github.com/subgate/subgate/pkg/wire"
)

func TestQuotaTelemetryUsesCommittedUsageAndSurvivesRestart(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	for _, rule := range []quota.Rule{
		{ID: "user", NodeID: "demo-node-1", UserID: "1", Scope: "per_user", LimitBytes: 100, Enabled: true},
		{ID: "node", NodeID: "demo-node-1", Scope: "per_node", LimitBytes: 1000, Enabled: true},
	} {
		w := perform(s, "PUT", "/api/v1/quotas/"+rule.ID, rule, session, "", true)
		if w.Code != 200 {
			t.Fatalf("quota: %d %s", w.Code, w.Body)
		}
		var saved quota.Rule
		if err := json.Unmarshal(w.Body.Bytes(), &saved); err != nil || saved.Period != quota.PeriodMonth || saved.UpdatedAt.IsZero() {
			t.Errorf("quota response is not normalized: %+v %v", saved, err)
		}
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: "usage", Samples: []wire.Sample{
		{UserID: "1", Timestamp: now, UplinkDelta: 30, DownlinkDelta: 40},
		{UserID: "2", Timestamp: now, Scope: "user", UplinkDelta: 20},
		{UserID: "inbound", Timestamp: now, Scope: "inbound", DownlinkDelta: 90},
	}}
	for _, batchID := range []string{"usage", "usage", "repacked"} {
		batch.BatchID = batchID
		w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false)
		if w.Code != 200 {
			t.Fatalf("telemetry: %d %s", w.Code, w.Body)
		}
	}
	assertQuotaRemaining(t, s, "1", 30)
	assertQuotaRemaining(t, s, "2", 910)
	// A correction of an already persisted sample replaces its bytes. Both
	// traffic reporting and quotas must observe the same committed value.
	batch.BatchID = "corrected"
	batch.Samples = batch.Samples[:1]
	batch.Samples[0].UplinkDelta = 10
	batch.Samples[0].DownlinkDelta = 40
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatalf("correction: %d %s", w.Code, w.Body)
	}
	assertQuotaRemaining(t, s, "1", 50)
	// Reopen a real SQLite snapshot and reconstruct the Server, so no manager
	// state can accidentally make this restart test pass.
	path := filepath.Join(t.TempDir(), "restart.db")
	if err := s.db.Backup(path); err != nil {
		t.Fatal(err)
	}
	database, err := db.New(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg := *s.config
	cfg.Database.URL = path
	restarted, err := New(&cfg, database, s.logger)
	if err != nil {
		database.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { restarted.Shutdown(context.Background()); database.Close() })
	assertQuotaRemaining(t, restarted, "1", 50)
	assertQuotaRemaining(t, restarted, "2", 930)
	if w := perform(restarted, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatalf("restart replay: %d %s", w.Code, w.Body)
	}
	assertQuotaRemaining(t, restarted, "1", 50)
}

func assertQuotaRemaining(t *testing.T, s *Server, user string, want int64) {
	t.Helper()
	if got := s.quota.Remaining("demo-node-1", user); got != want {
		t.Fatalf("user %s remaining=%d, want %d", user, got, want)
	}
}

func TestQuotaTelemetryRollbackAndConcurrentReplay(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	rule := quota.Rule{ID: "user", NodeID: "demo-node-1", UserID: "1", Scope: "per_user", LimitBytes: 100, Period: quota.PeriodDay, Enabled: true}
	if w := perform(s, "PUT", "/api/v1/quotas/user", rule, session, "", true); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_activity BEFORE INSERT ON sg_activity BEGIN SELECT RAISE(ABORT,'injected failure'); END`); err != nil {
		t.Fatal(err)
	}
	batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: "rollback", Samples: []wire.Sample{{UserID: "1", Timestamp: time.Now().UTC(), UplinkDelta: 60}}, Events: []wire.Activity{{UserID: "1", Timestamp: time.Now().UTC()}}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 500 {
		t.Fatalf("transaction failure: %d %s", w.Code, w.Body)
	}
	assertQuotaRemaining(t, s, "1", 100)
	if _, err := s.db.Exec("DROP TRIGGER reject_activity"); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	errors := make(chan string, 8)
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
				errors <- fmt.Sprintf("%d %s", w.Code, w.Body)
			}
		}()
	}
	workers.Wait()
	close(errors)
	for err := range errors {
		t.Error(err)
	}
	assertQuotaRemaining(t, s, "1", 40)
}

func TestQuotaUsesSampleTimeAcrossPeriodsAndFailsOpenOnStorageError(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	for _, period := range []quota.Period{quota.PeriodDay, quota.PeriodWeek, quota.PeriodMonth, quota.PeriodRolling} {
		t.Run(string(period), func(t *testing.T) {
			rule := quota.Rule{ID: "period", NodeID: "demo-node-1", UserID: "1", Scope: "per_user", LimitBytes: 100, Period: period, Timezone: "Europe/Moscow", Enabled: true}
			if w := perform(s, "PUT", "/api/v1/quotas/period", rule, session, "", true); w.Code != 200 {
				t.Fatal(w.Body)
			}
			now := time.Now().UTC()
			s.quota.SetClock(func() time.Time { return now })
			checks := s.quota.Check("demo-node-1", "1", 0)
			start := checks[0].PeriodStart
			if _, err := s.db.Exec("DELETE FROM sg_usage"); err != nil {
				t.Fatal(err)
			}
			for i, at := range []time.Time{start.Add(-time.Millisecond), start, now.Add(6 * time.Minute)} {
				if _, err := s.db.Exec("INSERT INTO sg_usage VALUES(?,?,?,?,?)", "demo-node-1", "1", at.UnixMilli(), int64((i+1)*10), 0); err != nil {
					t.Fatal(err)
				}
			}
			// Only the sample at the period start belongs to the current view.
			assertQuotaRemaining(t, s, "1", 80)
			// Delayed replay is accounted at its original timestamp, not at the
			// manager's current clock (chosen beyond this calendar period).
			later := checks[0].PeriodEnd.Add(time.Hour)
			if period == quota.PeriodRolling {
				later = now.Add(31 * 24 * time.Hour)
			}
			s.quota.SetClock(func() time.Time { return later })
			batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: string(period), Samples: []wire.Sample{{UserID: "1", Timestamp: now, UplinkDelta: 40}}}
			if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
				t.Fatal(w.Body)
			}
			assertQuotaRemaining(t, s, "1", 100)
		})
	}
	if _, err := s.db.Exec("DROP TABLE sg_usage"); err != nil {
		t.Fatal(err)
	}
	assertQuotaRemaining(t, s, "1", -1)
}

func TestQuotaMutationDoesNotChangeStateWhenPersistenceOrAuditFails(t *testing.T) {
	for _, failure := range []string{"storage", "audit"} {
		t.Run(failure, func(t *testing.T) {
			s := newTestServer(t)
			session := loginTest(t, s)
			rule := quota.Rule{ID: "user", NodeID: "demo-node-1", UserID: "1", Scope: "per_user", LimitBytes: 100, Enabled: true}
			if w := perform(s, "PUT", "/api/v1/quotas/user", rule, session, "", true); w.Code != 200 {
				t.Fatal(w.Body)
			}
			statement := `CREATE TRIGGER reject_quota BEFORE UPDATE ON sg_objects WHEN OLD.kind='quotas' BEGIN SELECT RAISE(ABORT,'injected failure'); END`
			if failure == "audit" {
				statement = "DROP TABLE audit_log"
			}
			if _, err := s.db.Exec(statement); err != nil {
				t.Fatal(err)
			}
			rule.LimitBytes = 200
			if w := perform(s, "PUT", "/api/v1/quotas/user", rule, session, "", true); w.Code != 500 {
				t.Fatalf("failed mutation: %d %s", w.Code, w.Body)
			}
			assertQuotaRemaining(t, s, "1", 100)
			var stored quota.Rule
			if err := s.object("quotas", "user", &stored); err != nil || stored.LimitBytes != 100 {
				t.Fatalf("persisted quota changed: %+v %v", stored, err)
			}
			if failure == "audit" {
				if w := perform(s, "DELETE", "/api/v1/quotas/user", nil, session, "", true); w.Code != 500 {
					t.Fatalf("unaudited delete: %d %s", w.Code, w.Body)
				}
				assertQuotaRemaining(t, s, "1", 100)
			}
		})
	}
}

func TestQuotaResolvesXrayAliasesWithoutChargingAmbiguousUsers(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	user := panel.User{ID: "quota-user", Username: "alice", Email: "alice@example.invalid", ShortUUID: "alice-short"}
	if err := s.saveObject("users", user.ID, user); err != nil {
		t.Fatal(err)
	}
	rule := quota.Rule{ID: "aliases", NodeID: "demo-node-1", UserID: user.ID, Scope: "per_user", LimitBytes: 100, Enabled: true}
	if w := perform(s, "PUT", "/api/v1/quotas/aliases", rule, session, "", true); w.Code != 200 {
		t.Fatal(w.Body)
	}
	now := time.Now().UTC()
	batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: "aliases", Samples: []wire.Sample{
		{UserID: user.ID, Timestamp: now, UplinkDelta: 10},
		{UserID: user.Username, Timestamp: now, UplinkDelta: 10},
		{UserID: user.Email, Timestamp: now, UplinkDelta: 10},
		{UserID: user.ShortUUID, Timestamp: now, UplinkDelta: 10},
	}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body)
	}
	assertQuotaRemaining(t, s, user.ID, 60)
	if err := s.saveObject("users", "other", panel.User{ID: "other", Email: user.Email}); err != nil {
		t.Fatal(err)
	}
	assertQuotaRemaining(t, s, user.ID, 70)
}

func TestQuotaTelemetryOfflineReplayPreservesSampleTimestamp(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	rule := quota.Rule{ID: "offline", NodeID: "demo-node-1", UserID: "1", Scope: "per_user", LimitBytes: 100, Period: quota.PeriodRolling, Enabled: true}
	if w := perform(s, "PUT", "/api/v1/quotas/offline", rule, session, "", true); w.Code != 200 {
		t.Fatal(w.Body)
	}
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: "offline", SentAt: at, Samples: []wire.Sample{{UserID: "1", Timestamp: at, UplinkDelta: 70}}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatalf("offline batch rejected: %d %s", w.Code, w.Body)
	}
	var storedAt int64
	if err := s.db.QueryRow("SELECT timestamp FROM sg_usage WHERE node_id=? AND user_id=?", batch.NodeID, "1").Scan(&storedAt); err != nil || storedAt != at.UnixMilli() {
		t.Fatalf("replay timestamp changed: got %d want %d error=%v", storedAt, at.UnixMilli(), err)
	}
	// Repacking the same sample with a newer send time must still address
	// the same row rather than charge another observation.
	batch.BatchID = "repacked-offline"
	batch.SentAt = time.Now().UTC()
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatalf("repacked offline batch: %d %s", w.Code, w.Body)
	}
	assertQuotaRemaining(t, s, "1", 30)
	var rows int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_usage").Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("repacked sample duplicated: rows=%d error=%v", rows, err)
	}
}

func TestQuotaTelemetryCorrectionRemovesPreviousUsage(t *testing.T) {
	for _, scope := range []string{"", "user", "inbound"} {
		t.Run("scope="+scope, func(t *testing.T) {
			s := newTestServer(t)
			session := loginTest(t, s)
			credential, _ := enrollTest(t, s, session, "demo-node-1")
			rule := quota.Rule{ID: "correction", NodeID: "demo-node-1", UserID: "1", Scope: "per_user", LimitBytes: 100, Period: quota.PeriodRolling, Enabled: true}
			if w := perform(s, "PUT", "/api/v1/quotas/correction", rule, session, "", true); w.Code != 200 {
				t.Fatal(w.Body)
			}
			batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: "original", Samples: []wire.Sample{{UserID: "1", Timestamp: time.Now().UTC(), UplinkDelta: 70}}}
			if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
				t.Fatal(w.Body)
			}
			assertQuotaRemaining(t, s, "1", 30)
			batch.BatchID = "corrected"
			batch.Samples[0].Scope = scope
			batch.Samples[0].UplinkDelta = 0
			if scope == "inbound" {
				batch.Samples[0].UplinkDelta = 90
			}
			if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
				t.Fatalf("correction: %d %s", w.Code, w.Body)
			}
			assertQuotaRemaining(t, s, "1", 100)
			if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
				t.Fatalf("correction replay: %d %s", w.Code, w.Body)
			}
			assertQuotaRemaining(t, s, "1", 100)
		})
	}
}

func TestTelemetryRejectsExpiredOrFutureSendTime(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	for _, offset := range []time.Duration{-25 * time.Hour, 6 * time.Minute} {
		t.Run(offset.String(), func(t *testing.T) {
			batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: offset.String(), SentAt: time.Now().Add(offset), Samples: []wire.Sample{{UserID: "1", Timestamp: time.Now().UTC(), UplinkDelta: 70}}}
			if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 400 {
				t.Fatalf("invalid send time accepted: %d %s", w.Code, w.Body)
			}
		})
	}
	var rows int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_usage").Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rejected batches persisted usage: rows=%d error=%v", rows, err)
	}
}

func TestNodeQuotaUsesInboundFallbackWithoutDoubleCountingUsers(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	for _, rule := range []quota.Rule{
		{ID: "node", NodeID: "demo-node-1", Scope: "per_node", LimitBytes: 100, Period: quota.PeriodRolling, Enabled: true},
		{ID: "user", NodeID: "demo-node-1", UserID: "1", Scope: "per_user", LimitBytes: 100, Period: quota.PeriodRolling, Enabled: true},
	} {
		if w := perform(s, "PUT", "/api/v1/quotas/"+rule.ID, rule, session, "", true); w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: "inbound-only", Samples: []wire.Sample{
		{UserID: "inbound:a", Scope: "inbound", Timestamp: at, UplinkDelta: 10},
		{UserID: "inbound:b", Scope: "inbound", Timestamp: at, DownlinkDelta: 20},
	}}
	for _, id := range []string{"inbound-only", "inbound-only", "repacked-inbound"} {
		batch.BatchID = id
		if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
			t.Fatal(w.Body)
		}
	}
	assertQuotaRemaining(t, s, "unrestricted-user", 70)
	batch.BatchID = "users-available"
	batch.Samples = []wire.Sample{
		{UserID: "1", Scope: "user", Timestamp: at, UplinkDelta: 40},
		{UserID: "2", Scope: "user", Timestamp: at, DownlinkDelta: 10},
	}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body)
	}
	// At the same observation time user counters supersede inbound aggregates.
	assertQuotaRemaining(t, s, "unrestricted-user", 50)
	checks := s.quota.Check("demo-node-1", "1", 0)
	for _, check := range checks {
		if check.RuleID == "user" && check.Bytes != 40 {
			t.Fatalf("inbound traffic attributed to subscriber: %+v", check)
		}
	}
	// On resuming inbound polling, its delta can span the intervening user
	// observations. Without a stable disaggregation it must be bypassed.
	batch.BatchID = "overlapping-fallback"
	batch.Samples = []wire.Sample{{UserID: "inbound:a", Scope: "inbound", Timestamp: at.Add(time.Second), IntervalMS: 2000, UplinkDelta: 60}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body)
	}
	assertQuotaRemaining(t, s, "unrestricted-user", 50)
	batch.BatchID = "later-fallback"
	batch.Samples = []wire.Sample{{UserID: "inbound:a", Scope: "inbound", Timestamp: at.Add(2 * time.Second), IntervalMS: 1000, UplinkDelta: 20}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body)
	}
	assertQuotaRemaining(t, s, "unrestricted-user", 30)
	batch.BatchID = "corrected-fallback"
	batch.Samples[0].UplinkDelta = 0
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body)
	}
	assertQuotaRemaining(t, s, "unrestricted-user", 50)
}

func TestNodeQuotaDistinguishesOverlappingAndAdjacentIntervals(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	rule := quota.Rule{ID: "node", NodeID: "demo-node-1", Scope: "per_node", LimitBytes: 100, Period: quota.PeriodRolling, Enabled: true}
	if w := perform(s, "PUT", "/api/v1/quotas/node", rule, session, "", true); w.Code != 200 {
		t.Fatal(w.Body)
	}
	at := time.Now().UTC().Add(-time.Hour).Truncate(time.Millisecond)
	batch := wire.Telemetry{NodeID: "demo-node-1", BatchID: "earlier-inbound", Samples: []wire.Sample{
		{UserID: "inbound:a", Scope: "inbound", Timestamp: at.Add(time.Second), IntervalMS: 1000, UplinkDelta: 30},
	}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body)
	}
	assertQuotaRemaining(t, s, "any-user", 70)
	batch.BatchID = "later-user"
	batch.Samples = []wire.Sample{{UserID: "1", Scope: "user", Timestamp: at.Add(2 * time.Second), IntervalMS: 2000, UplinkDelta: 40}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body)
	}
	// A later user sample's interval overlaps the earlier inbound observation.
	assertQuotaRemaining(t, s, "any-user", 60)
	batch.BatchID = "adjacent-inbound"
	batch.Samples = []wire.Sample{{UserID: "inbound:a", Scope: "inbound", Timestamp: at.Add(3 * time.Second), IntervalMS: 1000, UplinkDelta: 20}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Body)
	}
	// The next inbound interval starts exactly when the user interval ends.
	assertQuotaRemaining(t, s, "any-user", 40)
}
