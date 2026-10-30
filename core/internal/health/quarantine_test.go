package health

import (
	"log/slog"
	"testing"
	"time"
)

func TestQuarantineThresholdsMinimumTimeAndBackoff(t *testing.T) {
	e := NewEngine(slog.Default(), Options{FailureThreshold: 2, SuccessThreshold: 3, MinQuarantine: time.Minute, BackoffBase: time.Second, BackoffMax: 4 * time.Second})
	e.AddNode("node")
	start := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	record := func(offset time.Duration, success bool, want HealthStatus, failures, successes int) NodeHealth {
		t.Helper()
		e.RecordProbeResult(ProbeResult{NodeID: "node", Timestamp: start.Add(offset), Success: success, Verifiable: true})
		n, _ := e.SnapshotNode("node")
		if n.Status != want || n.ConsecutiveFailures != failures || n.ConsecutiveSuccesses != successes {
			t.Fatalf("at %s: status/counters: %+v", offset, n)
		}
		return n
	}
	record(0, false, StatusSuspected, 1, 0)
	q := record(time.Second, false, StatusQuarantined, 2, 0)
	if !q.QuarantinedUntil.Equal(start.Add(61*time.Second)) || !q.NextProbeAt.Equal(start.Add(2*time.Second)) {
		t.Fatal("initial quarantine bounds", q)
	}
	n := record(2*time.Second, false, StatusQuarantined, 3, 0)
	if !n.NextProbeAt.Equal(start.Add(4 * time.Second)) {
		t.Fatal("backoff did not double", n)
	}
	n = record(4*time.Second, false, StatusQuarantined, 4, 0)
	if !n.NextProbeAt.Equal(start.Add(8 * time.Second)) {
		t.Fatal("backoff did not reach cap", n)
	}
	n = record(8*time.Second, false, StatusQuarantined, 5, 0)
	if !n.NextProbeAt.Equal(start.Add(12*time.Second)) || !n.QuarantinedUntil.Equal(q.QuarantinedUntil) {
		t.Fatal("backoff overflow or moving minimum", n)
	}
	record(12*time.Second, true, StatusRecovering, 0, 1)
	record(13*time.Second, true, StatusRecovering, 0, 2)
	record(14*time.Second, true, StatusRecovering, 0, 3)
	record(60*time.Second, true, StatusRecovering, 0, 4)
	n = record(61*time.Second, true, StatusHealthy, 0, 5)
	if !n.QuarantinedUntil.IsZero() || !n.NextProbeAt.IsZero() {
		t.Fatal("healthy retains quarantine/backoff", n)
	}
	// M is consecutive; a failure during recovery keeps the node excluded.
	record(62*time.Second, false, StatusSuspected, 1, 0)
	record(63*time.Second, false, StatusQuarantined, 2, 0)
	record(124*time.Second, true, StatusRecovering, 0, 1)
	record(125*time.Second, true, StatusRecovering, 0, 2)
	record(126*time.Second, false, StatusQuarantined, 1, 0)
	record(187*time.Second, true, StatusRecovering, 0, 1)
	record(188*time.Second, true, StatusRecovering, 0, 2)
	record(189*time.Second, true, StatusHealthy, 0, 3)
}

func TestQuarantineRecoverySurvivesStateRestore(t *testing.T) {
	opts := Options{FailureThreshold: 1, SuccessThreshold: 3, MinQuarantine: time.Minute, BackoffBase: time.Second, BackoffMax: time.Minute}
	e := NewEngine(slog.Default(), opts)
	e.AddNode("node")
	start := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	e.RecordProbeResult(ProbeResult{NodeID: "node", Timestamp: start, Verifiable: true})
	e.RecordProbeResult(ProbeResult{NodeID: "node", Timestamp: start.Add(time.Second), Success: true, Verifiable: true})
	n, _ := e.SnapshotNode("node")
	restarted := NewEngine(slog.Default(), opts)
	restarted.RestoreNode(n)
	restarted.RecordProbeResult(ProbeResult{NodeID: "node", Timestamp: start.Add(time.Minute), Success: true, Verifiable: true})
	if status, _ := restarted.GetNodeHealth("node"); status != StatusRecovering {
		t.Fatal("restart forgot M counter", status)
	}
	restarted.RecordProbeResult(ProbeResult{NodeID: "node", Timestamp: start.Add(61 * time.Second), Success: true, Verifiable: true})
	if status, _ := restarted.GetNodeHealth("node"); status != StatusHealthy {
		t.Fatal("restart could not complete recovery", status)
	}
}
