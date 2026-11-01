package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/health"
	"github.com/subgate/subgate/core/internal/panel"
)

func TestScheduledHealthTransitions(t *testing.T) {
	s := newTestServer(t)
	// Short configured times exercise automatic recovery without bypassing the
	// production minimum quarantine and exponential probe backoff behavior.
	s.config.Health.MinQuarantine = 20 * time.Millisecond
	s.config.Health.BackoffBase = 10 * time.Millisecond
	s.config.Health.BackoffMax = 20 * time.Millisecond
	if err := s.initializeHealth(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("DELETE FROM sg_objects WHERE kind='nodes'"); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	host, rawPort, _ := net.SplitHostPort(address)
	port, _ := strconv.Atoi(rawPort)
	serve := func(l net.Listener) {
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				c.Close()
			}
		}()
	}
	serve(listener)
	defer func() { listener.Close() }()
	if err := s.saveObject("nodes", "scheduled", panel.Node{ID: "scheduled", IPAddress: host, Inbounds: []panel.Inbound{{Port: port, Type: "vless", Network: "tcp"}}}); err != nil {
		t.Fatal(err)
	}
	// Production startup starts this same worker alongside panel synchronization.
	s.config.Health.ProbeInterval = 20 * time.Millisecond
	s.startBackground()
	waitStatus := func(want string) {
		t.Helper()
		deadline := time.NewTimer(3 * time.Second)
		defer deadline.Stop()
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			var state struct {
				Status          string `json:"Status"`
				LastHealthCheck time.Time
			}
			if err := s.object("health-state", "scheduled", &state); err == nil && state.Status == want && !state.LastHealthCheck.IsZero() {
				return
			}
			select {
			case <-deadline.C:
				t.Fatalf("scheduler did not persist %s", want)
			case <-tick.C:
			}
		}
	}
	waitStatus("healthy")
	listener.Close()
	waitStatus("quarantined")
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	// Startup restores the persisted quarantine rather than starting from unknown.
	restarted, err := New(s.config, s.db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Shutdown(context.Background())
	if status, ok := restarted.health.GetNodeHealth("scheduled"); !ok || status != "quarantined" {
		t.Fatal("quarantine not restored", status, ok)
	}
	listener, err = net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	serve(listener)
	restarted.startBackground()
	// Reuse the persisted view to observe autonomous recovery.
	s = restarted
	waitStatus("healthy")
	if w := perform(s, "GET", "/api/v1/health/probes", nil, loginTest(t, s), "", false); w.Code != 200 {
		t.Fatal("health API", w.Code)
	}
}

func TestScheduledHealthUnknownCancellationAndAtomicFailure(t *testing.T) {
	s := newTestServer(t)
	unknown := panel.Node{ID: "unknown", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 443, Network: "quic"}}}
	if err := s.saveObject("nodes", unknown.ID, unknown); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), unknown); err != nil {
		t.Fatal(err)
	}
	var sample nodeProbe
	if err := s.object("health", "unknown", &sample); err != nil || sample.Verified != "unverified" || sample.Success || sample.Status != health.StatusUnknown {
		t.Fatal("unknown became evidence", sample, err)
	}
	state, ok := s.health.SnapshotNode("unknown")
	if !ok || state.ConsecutiveFailures != 0 {
		t.Fatal("unverifiable failure counted", state)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := s.scheduledNodeProbe(ctx, panel.Node{ID: "canceled", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 1, Network: "tcp"}}}); err == nil {
		t.Fatal("cancellation not reported")
	}
	if _, ok := s.health.GetNodeHealth("canceled"); ok {
		t.Fatal("canceled probe mutated health")
	}
	// Make the second half of the atomic snapshot fail. Neither probe nor state
	// may advance; this must hold regardless of map iteration order.
	if _, err := s.db.Exec(`CREATE TRIGGER synthetic_health_fail BEFORE INSERT ON sg_objects WHEN NEW.kind='health-state' BEGIN SELECT RAISE(FAIL,'synthetic'); END`); err != nil {
		t.Fatal(err)
	}
	failed := panel.Node{ID: "unknown", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: 1, Network: "tcp"}}}
	if err := s.saveObject("nodes", failed.ID, failed); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), failed); err == nil {
		t.Fatal("persistence failure ignored")
	}
	if current, _ := s.health.SnapshotNode("unknown"); current != state {
		t.Fatal("memory state not rolled back", current)
	}
	var after nodeProbe
	if err := s.object("health", "unknown", &after); err != nil || after != sample {
		t.Fatal("probe partially committed", after, err)
	}
}

func TestScheduledHealthIncompleteEndpointsCannotQuarantine(t *testing.T) {
	s := newTestServer(t)
	s.config.Health.ProbeTimeout = 30 * time.Millisecond
	stall, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer stall.Close()
	go func() {
		for {
			c, err := stall.Accept()
			if err != nil {
				return
			}
			go func() { defer c.Close(); _, _ = io.Copy(io.Discard, c) }()
		}
	}()
	good, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer good.Close()
	go func() {
		for {
			c, err := good.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	_, stallPort, _ := net.SplitHostPort(stall.Addr().String())
	p1, _ := strconv.Atoi(stallPort)
	_, goodPort, _ := net.SplitHostPort(good.Addr().String())
	p2, _ := strconv.Atoi(goodPort)
	n := panel.Node{ID: "partial", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{{Port: p1, Network: "tcp", Security: "tls"}, {Port: p2, Network: "tcp"}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
			t.Fatal(err)
		}
	}
	state, _ := s.health.SnapshotNode("partial")
	if state.ConsecutiveFailures != 0 || state.Status == health.StatusQuarantined {
		t.Fatal("incomplete endpoint coverage treated as definitive node failure", state)
	}
}
