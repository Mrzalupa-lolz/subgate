package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/health"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

func TestQuarantineFiltersSubscription(t *testing.T) {
	s := newTestServer(t)
	for _, n := range []panel.Node{{ID: "primary", IPAddress: "primary.example", Status: "online"}, {ID: "fallback", IPAddress: "fallback.example", Status: "online"}} {
		if err := s.saveObject("nodes", n.ID, n); err != nil {
			t.Fatal(err)
		}
	}
	save := func(kind, id string, v any) {
		t.Helper()
		if err := s.saveObject(kind, id, v); err != nil {
			t.Fatal(err)
		}
	}
	save("users", "health-user", panel.User{ID: "health-user", ShortUUID: "health-short", Status: "active"})
	save("pins", "health-pin", map[string]any{"user_id": "health-user", "node_id": "primary", "enabled": true})
	save("tiers", "health-first", routingTier{ID: "health-first", Nodes: []string{"primary"}, MinHealthy: 1, Fallback: "health-next", Strategy: "roundRobin", Enabled: true})
	save("tiers", "health-next", routingTier{ID: "health-next", Nodes: []string{"fallback"}, MinHealthy: 1, Strategy: "roundRobin", Enabled: true})
	save("policies", "health-policy", map[string]any{"id": "health-policy", "enabled": true, "tier_id": "health-first", "priority": -100, "conditions": map[string]any{}})
	raw := []byte("vless://synthetic@primary.example:443#primary\nfuture://synthetic@fallback.example:443#fallback\nfuture://synthetic@unknown.example:443?opaque=keep\n")
	req := httptest.NewRequest("GET", "http://console.test/sub/health-short", nil)
	req.SetPathValue("shortUuid", "health-short")
	for _, status := range []health.HealthStatus{health.StatusQuarantined, health.StatusRecovering} {
		s.health.RestoreNode(health.NodeHealth{NodeID: "primary", Status: status, QuarantinedUntil: time.Now().Add(time.Minute)})
		out, policy, err := s.transformSubscription(req, raw, nil)
		if err != nil || strings.Contains(string(out), "primary.example") || !strings.Contains(string(out), "fallback.example") || !strings.Contains(string(out), "opaque=keep") || policy != "tier" {
			t.Fatalf("%s ignored by pinned/tier selection: %q %s %v", status, out, policy, err)
		}
	}
	s.health.RestoreNode(health.NodeHealth{NodeID: "fallback", Status: health.StatusQuarantined})
	if out, _, err := s.transformSubscription(req, raw, nil); err != nil || string(out) != string(raw) {
		t.Fatal("exhausted fallback tiers must serve original including opaque endpoints", string(out), err)
	}
	s.health.RestoreNode(health.NodeHealth{NodeID: "fallback", Status: health.StatusHealthy})
	// No policy or pin is required to suppress a quarantined mapped endpoint.
	if _, err := s.db.Exec("DELETE FROM sg_objects WHERE kind IN ('pins','policies')"); err != nil {
		t.Fatal(err)
	}
	out, _, err := s.transformSubscription(req, raw, nil)
	if err != nil || strings.Contains(string(out), "primary.example") || !strings.Contains(string(out), "fallback.example") {
		t.Fatal("unconfigured routing bypassed quarantine", string(out), err)
	}
	s.health.RestoreNode(health.NodeHealth{NodeID: "fallback", Status: health.StatusQuarantined})
	allMapped := []byte("vless://synthetic@primary.example:443\nfuture://synthetic@fallback.example:443\n")
	out, _, err = s.transformSubscription(req, allMapped, nil)
	if err != nil || string(out) != string(allMapped) {
		t.Fatal("all quarantined must fail open", string(out), err)
	}
	// Recovery makes the endpoint available again on the same upstream bytes.
	s.health.RestoreNode(health.NodeHealth{NodeID: "primary", Status: health.StatusHealthy})
	out, _, err = s.transformSubscription(req, allMapped, nil)
	if err != nil || !strings.Contains(string(out), "primary.example") || strings.Contains(string(out), "fallback.example") {
		t.Fatal("health recovery not applied", string(out), err)
	}
}

func TestQuarantineBackoffSuppressesCoreAndAgentTargets(t *testing.T) {
	s := newTestServer(t)
	n := panel.Node{ID: "backoff-node", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 1, Network: "tcp"}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	prior := health.NodeHealth{NodeID: n.ID, Status: health.StatusQuarantined, ConsecutiveFailures: 3, NextProbeAt: time.Now().Add(time.Minute)}
	s.health.RestoreNode(prior)
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if current, _ := s.health.SnapshotNode(n.ID); current != prior {
		t.Fatal("Core ignored probe backoff", current)
	}
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	w := perform(s, "GET", "/api/v1/agents/health-targets", nil, nil, token, false)
	var targets struct {
		Data []wire.HealthTarget `json:"data"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &targets) != nil {
		t.Fatal("target response", w.Code)
	}
	for _, target := range targets.Data {
		if target.NodeID == n.ID {
			t.Fatal("Agent ignored probe backoff")
		}
	}
}

func TestQuarantineUpdatesCachedSubscriptionWithoutPanelRefetch(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.db.Exec("DELETE FROM sg_objects WHERE kind IN ('pins','policies')"); err != nil {
		t.Fatal(err)
	}
	for _, n := range []panel.Node{{ID: "cached-a", IPAddress: "cached-a.example", Status: "online"}, {ID: "cached-b", IPAddress: "cached-b.example", Status: "online"}} {
		if err := s.saveObject("nodes", n.ID, n); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.saveObject("users", "cached-user", panel.User{ID: "cached-user", ShortUUID: "cached-short", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	raw := "future://synthetic@cached-a.example:443\nvless://synthetic@cached-b.example:443\n"
	var fetches atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fetches.Add(1); _, _ = w.Write([]byte(raw)) }))
	defer upstream.Close()
	s.gateway.Configure(upstream.URL, s)
	request := func() *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest("GET", "http://console.test/sub/cached-short", nil)
		r.SetPathValue("shortUuid", "cached-short")
		w := httptest.NewRecorder()
		s.gateway.HandleRequest(w, r)
		if w.Code != 200 {
			t.Fatal(w.Code)
		}
		return w
	}
	if w := request(); w.Body.String() != raw {
		t.Fatal("initial bytes", w.Body.String())
	}
	s.health.RestoreNode(health.NodeHealth{NodeID: "cached-a", Status: health.StatusQuarantined})
	if w := request(); strings.Contains(w.Body.String(), "cached-a.example") || !strings.Contains(w.Body.String(), "cached-b.example") || w.Header().Get("X-Cache-Status") != "hit" {
		t.Fatal("cached subscription ignored health", w.Body.String(), w.Header())
	}
	s.health.RestoreNode(health.NodeHealth{NodeID: "cached-a", Status: health.StatusHealthy})
	if w := request(); w.Body.String() != raw || fetches.Load() != 1 {
		t.Fatal("recovery refetched or mutated upstream cache", w.Body.String(), fetches.Load())
	}
}

func TestRoutingDoesNotObserveRolledBackQuarantine(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.db.Exec("DELETE FROM sg_objects WHERE kind IN ('pins','policies')"); err != nil {
		t.Fatal(err)
	}
	if err := s.saveObject("nodes", "pending", panel.Node{ID: "pending", IPAddress: "pending.example", Status: "online"}); err != nil {
		t.Fatal(err)
	}
	if err := s.saveObject("users", "pending-user", panel.User{ID: "pending-user", ShortUUID: "pending-short", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	raw := []byte("vless://synthetic@pending.example:443\nfuture://synthetic@unmapped.example:443\n")
	req := httptest.NewRequest("GET", "http://console.test/sub/pending-short", nil)
	req.SetPathValue("shortUuid", "pending-short")
	// The writer holds this lock across the in-memory advance and SQL commit.
	// An in-flight transition that rolls back must not affect subscription bytes.
	s.healthMu.Lock()
	s.health.RestoreNode(health.NodeHealth{NodeID: "pending", Status: health.StatusQuarantined})
	done := make(chan []byte, 1)
	go func() { out, _, _ := s.transformSubscription(req, raw, nil); done <- out }()
	select {
	case out := <-done:
		s.health.RestoreNode(health.NodeHealth{NodeID: "pending", Status: health.StatusHealthy})
		s.healthMu.Unlock()
		t.Fatalf("uncommitted quarantine affected subscription: %q", out)
	case <-time.After(100 * time.Millisecond):
	}
	s.health.RestoreNode(health.NodeHealth{NodeID: "pending", Status: health.StatusHealthy})
	s.healthMu.Unlock()
	select {
	case out := <-done:
		if string(out) != string(raw) {
			t.Fatal("rollback changed subscription", string(out))
		}
	case <-time.After(time.Second):
		t.Fatal("routing stuck after rollback")
	}
}

func TestQuarantineMinimumAndBackoffStartAfterSlowProbe(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.FailureThreshold = 1
	s.config.Health.MinQuarantine = 50 * time.Millisecond
	s.config.Health.BackoffBase = 50 * time.Millisecond
	s.config.Health.ProbeTimeout = 200 * time.Millisecond
	if err := s.initializeHealth(); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		c, err := l.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(io.Discard, c)
	}()
	host, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	n := panel.Node{ID: "slow-quarantine", IPAddress: host, Inbounds: []panel.Inbound{{Port: port, Network: "tcp", Security: "tls"}}}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	state, _ := s.health.SnapshotNode(n.ID)
	var sample nodeProbe
	if err := s.object("health", n.ID, &sample); err != nil {
		t.Fatal(err)
	}
	if state.Status != health.StatusQuarantined || state.QuarantinedUntil.Sub(sample.CompletedAt) < 50*time.Millisecond || state.NextProbeAt.Sub(sample.CompletedAt) < 50*time.Millisecond {
		t.Fatal("hold/backoff elapsed before slow probe finished", state, sample.CompletedAt)
	}
}
