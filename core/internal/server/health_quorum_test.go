package server

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/health"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

func TestProbeQuorum(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.Quorum = 2
	n := panel.Node{ID: "target", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 1, Network: "tcp"}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	session := loginTest(t, s)
	token, _ := enrollTest(t, s, session, "demo-node-1")
	check := func(want health.HealthStatus, failures int) {
		t.Helper()
		if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
			t.Fatal(err)
		}
		state, _ := s.health.SnapshotNode(n.ID)
		if state.Status != want || state.ConsecutiveFailures != failures {
			t.Fatalf("state %v failures %d", state.Status, state.ConsecutiveFailures)
		}
	}
	check(health.StatusUnknown, 0) // one Core vote is insufficient
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: time.Now(), Verified: "tcp", Success: false}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal("report rejected", w.Code)
	}
	check(health.StatusSuspected, 1)
	// Repeated source reports replace its one vote, never create a second vantage.
	for i := 0; i < 3; i++ {
		if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	check(health.StatusSuspected, 2)
	report.Results[0].Success = true
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	check(health.StatusSuspected, 2) // split votes are not consensus
	report.Results[0].Success = false
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	// Revocation removes the previous vote before it can quarantine the target.
	if _, err := s.db.Exec("UPDATE agents SET status='revoked' WHERE node_id='demo-node-1'"); err != nil {
		t.Fatal(err)
	}
	check(health.StatusSuspected, 2)
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 401 {
		t.Fatal("revoked reporter accepted", w.Code)
	}
	newToken, _ := enrollTest(t, s, session, "demo-node-1")
	check(health.StatusSuspected, 2) // old credential's vote remains invalid
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, newToken, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	check(health.StatusQuarantined, 3)
}

type quorumRaceHandler struct {
	slog.Handler
	onQuarantine func()
}

func (h quorumRaceHandler) Handle(ctx context.Context, r slog.Record) error {
	if r.Message == "Node quarantined" {
		h.onQuarantine()
	}
	return nil
}

func TestProbeQuorumRevocationAndTransitionAreSerialized(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.Quorum = 2
	n := panel.Node{ID: "target", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 1, Network: "tcp"}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: time.Now(), Verified: "tcp"}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	done := make(chan error, 1)
	revokedDuringTransition := false
	called := false
	h := quorumRaceHandler{Handler: slog.Default().Handler(), onQuarantine: func() {
		called = true
		go func() {
			_, err := s.db.Exec("UPDATE agents SET status='revoked' WHERE node_id='demo-node-1'")
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
			revokedDuringTransition = true
		case <-time.After(20 * time.Millisecond):
		}
	}}
	s.health = health.NewEngine(slog.New(h))
	s.health.RestoreNode(health.NodeHealth{NodeID: n.ID, Status: health.StatusSuspected, ConsecutiveFailures: 2})
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("race window not exercised")
	}
	if !revokedDuringTransition {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	var state health.NodeHealth
	if err := s.object("health-state", n.ID, &state); err != nil {
		t.Fatal(err)
	}
	if revokedDuringTransition && state.Status == health.StatusQuarantined {
		t.Fatal("quarantine committed with already revoked source")
	}
}

func TestProbeQuorumVotesCannotRegressOrMultiply(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.Quorum = 2
	n := panel.Node{ID: "target", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 1, Network: "tcp"}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	old := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: time.Now(), Verified: "tcp", Success: false}}}
	fresh := old
	fresh.Results = append([]wire.HealthResult(nil), old.Results...)
	fresh.Results[0].Timestamp = old.Results[0].Timestamp.Add(time.Millisecond)
	fresh.Results[0].Success = true
	for _, report := range []wire.HealthReport{old, fresh, old} {
		if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if state, _ := s.health.SnapshotNode(n.ID); state.ConsecutiveFailures != 0 {
		t.Fatal("old report replaced newer evidence", state)
	}
	s.config.Health.Quorum = 3
	fresh.Results[0].Timestamp = time.Now().Add(2 * time.Millisecond)
	fresh.Results[0].Success = false
	for i := 0; i < 3; i++ {
		if w := perform(s, "PUT", "/api/v1/agents/health-results", fresh, nil, token, false); w.Code != 200 {
			t.Fatal(w.Code)
		}
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	var probe nodeProbe
	if err := s.object("health", n.ID, &probe); err != nil || probe.Vantages != 2 || probe.QuorumMet {
		t.Fatal("one source multiplied its votes", probe, err)
	}
	// Accepted evidence must expire even if no replacement report arrives.
	var vote healthVote
	if err := s.object("health-vote", hash("demo-node-1\x00"+n.ID), &vote); err != nil {
		t.Fatal(err)
	}
	vote.ReceivedAt = time.Now().Add(-2 * time.Minute)
	vote.Result.Timestamp = vote.ReceivedAt
	if err := s.saveObject("health-vote", hash("demo-node-1\x00"+n.ID), vote); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || probe.Vantages != 1 {
		t.Fatal("stale persisted vote counted", probe, err)
	}
}

func TestProbeReportCannotBorrowRotatedCredential(t *testing.T) {
	s := newTestServer(t)
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: "demo-node-2", Timestamp: time.Now(), Verified: "tcp"}}}
	// Simulate a request whose middleware completed just before key rotation.
	if _, err := s.db.Exec("UPDATE agents SET token_hash=? WHERE node_id='demo-node-1'", hash("synthetic-replacement")); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(report)
	r := httptest.NewRequest("PUT", "/api/v1/agents/health-results", bytes.NewReader(raw)).WithContext(context.WithValue(context.Background(), nodeKey, "demo-node-1"))
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.agentHealthResults(w, r)
	if w.Code != 401 {
		t.Fatal("in-flight old token borrowed new token hash", w.Code)
	}
}

func TestProbeReportsRejectInvalidAndAnonymous(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	token, _ := enrollTest(t, s, session, "demo-node-1")
	for _, path := range []string{"/api/v1/agents/health-targets", "/api/v1/agents/health-results"} {
		method := "GET"
		if path == "/api/v1/agents/health-results" {
			method = "PUT"
		}
		if w := perform(s, method, path, wire.HealthReport{}, nil, "", false); w.Code != 401 {
			t.Fatal("anonymous report API", w.Code)
		}
	}
	for _, result := range []wire.HealthResult{
		{NodeID: "not-a-synced-node", Timestamp: time.Now(), Verified: "tcp"},
		{NodeID: "demo-node-1", Timestamp: time.Now().Add(-2 * time.Minute), Verified: "tcp"},
		{NodeID: "demo-node-1", Timestamp: time.Now().Add(time.Minute), Verified: "tcp"},
		{NodeID: "demo-node-1", Timestamp: time.Now(), Verified: "imaginary"},
		{NodeID: "demo-node-1", Timestamp: time.Now(), CompletedAt: time.Now().Add(-time.Second), Verified: "tcp"},
		{NodeID: "demo-node-1", Timestamp: time.Now(), CompletedAt: time.Now().Add(time.Minute), Verified: "tcp"},
	} {
		if w := perform(s, "PUT", "/api/v1/agents/health-results", wire.HealthReport{Results: []wire.HealthResult{result}}, nil, token, false); w.Code != 400 {
			t.Fatal("invalid report", w.Code)
		}
	}
	if w := perform(s, "GET", "/api/v1/agents/health-targets", nil, nil, token, false); w.Code != 200 {
		t.Fatal("targets", w.Code)
	}
}
