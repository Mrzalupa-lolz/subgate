package server

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/config"
	"github.com/subgate/subgate/core/internal/health"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

func TestMaintenanceSuppression(t *testing.T) {
	s := newTestServer(t)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	var connects atomic.Int32
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			connects.Add(1)
			c.Close()
		}
	}()
	host, p, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(p)
	n := panel.Node{ID: "maintained", IPAddress: host, Inbounds: []panel.Inbound{{Port: port, Network: "tcp"}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	prior := health.NodeHealth{NodeID: n.ID, Status: health.StatusQuarantined, ConsecutiveFailures: 4, QuarantinedUntil: time.Now().Add(time.Minute), LastHealthCheck: time.Now().Add(-time.Minute)}
	s.health.RestoreNode(prior)
	if err := s.saveObject("health-state", n.ID, prior); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.config.Health.MaintenanceWindows = []config.MaintenanceWindow{{NodeID: n.ID, From: now.Add(-time.Minute), Until: now.Add(time.Minute)}}
	for i := 0; i < 3; i++ {
		if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
			t.Fatal(err)
		}
	}
	if state, _ := s.health.SnapshotNode(n.ID); state != prior {
		t.Fatal("maintenance changed state", state)
	}
	var persisted health.NodeHealth
	if err := s.object("health-state", n.ID, &persisted); err != nil || persisted.Status != prior.Status || persisted.ConsecutiveFailures != prior.ConsecutiveFailures || !persisted.LastHealthCheck.Equal(prior.LastHealthCheck) || !persisted.QuarantinedUntil.Equal(prior.QuarantinedUntil) {
		t.Fatal("maintenance changed persisted state", persisted, err)
	}
	var sample nodeProbe
	if err := s.object("health", n.ID, &sample); err != nil || !sample.Maintenance || sample.QuorumMet {
		t.Fatal("maintenance not visible", sample, err)
	}
	if connects.Load() != 0 {
		t.Fatal("maintenance dialed endpoint")
	}
	if w := perform(s, "POST", "/api/v1/nodes/maintained/probe", nil, loginTest(t, s), "", true); w.Code != 409 {
		t.Fatal("manual probe bypassed maintenance", w.Code)
	}
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	targets := perform(s, "GET", "/api/v1/agents/health-targets", nil, nil, token, false)
	var envelope struct {
		Data []wire.HealthTarget `json:"data"`
	}
	if targets.Code != 200 || json.Unmarshal(targets.Body.Bytes(), &envelope) != nil {
		t.Fatal("targets unavailable", targets.Code)
	}
	for _, target := range envelope.Data {
		if target.NodeID == n.ID {
			t.Fatal("maintained node sent to Agent")
		}
	}
	// Other nodes remain probed and window is [from,until), not indefinite.
	other := n
	other.ID = "other"
	if err := s.scheduledNodeProbe(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.health.GetNodeHealth(other.ID); status != health.StatusHealthy {
		t.Fatal("maintenance expanded to fleet", status)
	}
	if !s.inMaintenance(n.ID, now) || s.inMaintenance(n.ID, now.Add(-2*time.Minute)) || s.inMaintenance(n.ID, s.config.Health.MaintenanceWindows[0].Until) {
		t.Fatal("window boundaries")
	}
	s.config.Health.MaintenanceWindows[0].Until = now.Add(-time.Second)
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if state, _ := s.health.SnapshotNode(n.ID); state.LastHealthCheck.Equal(prior.LastHealthCheck) {
		t.Fatal("probes did not resume")
	}
}

func TestMaintenanceVotesOverlappingElapsedWindowAreIgnored(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.Quorum = 2
	n := panel.Node{ID: "target", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 1, Network: "tcp"}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.config.Health.MaintenanceWindows = []config.MaintenanceWindow{{NodeID: n.ID, From: now.Add(-45 * time.Second), Until: now.Add(-15 * time.Second)}}
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: now.Add(-60 * time.Second), CompletedAt: now.Add(-10 * time.Second), Verified: "tls"}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if state, _ := s.health.SnapshotNode(n.ID); state.ConsecutiveFailures != 0 {
		t.Fatal("spanning agent vote counted after maintenance", state)
	}
}

func TestMaintenanceLegacyVotesUseReceiveTimeAsObservationEnd(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.Quorum = 2
	n := panel.Node{ID: "legacy-target", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 1, Network: "tcp"}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	s.config.Health.MaintenanceWindows = []config.MaintenanceWindow{{NodeID: n.ID, From: now.Add(-45 * time.Second), Until: now.Add(-15 * time.Second)}}
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	// Older Agents omit completed_at. They must not turn a potentially
	// maintenance-spanning observation into a point vote after the window.
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: now.Add(-60 * time.Second), Verified: "tcp"}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if state, _ := s.health.SnapshotNode(n.ID); state.ConsecutiveFailures != 0 {
		t.Fatal("legacy spanning vote counted after maintenance", state)
	}
}

func TestMaintenanceSuppressesWindowEntirelyInsideSlowProbe(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.ProbeTimeout = 250 * time.Millisecond
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(io.Discard, c) }()
		}
	}()
	host, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	n := panel.Node{ID: "spanning", IPAddress: host, Inbounds: []panel.Inbound{{Port: port, Network: "tcp", Security: "tls"}}}
	now := time.Now()
	s.config.Health.MaintenanceWindows = []config.MaintenanceWindow{{NodeID: n.ID, From: now.Add(50 * time.Millisecond), Until: now.Add(150 * time.Millisecond)}}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	state, _ := s.health.SnapshotNode(n.ID)
	if state.ConsecutiveFailures != 0 {
		t.Fatal("elapsed maintenance counted as failed probe", state)
	}
	var sample nodeProbe
	if err := s.object("health", n.ID, &sample); err != nil || !sample.Maintenance || sample.QuorumMet {
		t.Fatal("overlapping maintenance not recorded", sample, err)
	}
}
