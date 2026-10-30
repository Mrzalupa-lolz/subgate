package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
)

func TestProbeRequiresTLSHandshakeAndMarksUDPUnverified(t *testing.T) {
	endpoint := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer endpoint.Close()
	host, rawPort, _ := net.SplitHostPort(endpoint.Listener.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	tcp := probeInbound(context.Background(), "node", host, panel.Inbound{Port: port, Network: "tcp"})
	if !tcp.Success || tcp.Verified != "tcp" {
		t.Fatalf("TCP: %+v", tcp)
	}
	tls := probeInbound(context.Background(), "node", host, panel.Inbound{Port: port, Network: "tcp", Security: "tls"})
	if tls.Success || tls.Verified != "tls" || tls.Error == "" {
		t.Fatalf("untrusted TLS certificate incorrectly accepted: %+v", tls)
	}
	udp := probeInbound(context.Background(), "node", host, panel.Inbound{Port: port, Network: "quic"})
	if udp.Success || udp.Verified != "unverified" {
		t.Fatalf("UDP: %+v", udp)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := probeInbound(ctx, "node", host, panel.Inbound{Port: port, Network: "tcp"})
	if probe.Success {
		t.Fatal("canceled probe succeeded")
	}
}

func TestLeastPingRejectsStaleSamplesAndUsesFallback(t *testing.T) {
	s := newTestServer(t)
	nodes := map[string]panel.Node{"fast": {ID: "fast", Status: "online"}, "fresh": {ID: "fresh", Status: "online"}}
	if err := s.saveObject("tiers", "ping", routingTier{ID: "ping", Strategy: "leastPing", Nodes: []string{"fast", "fresh"}, Enabled: true, Fallback: "fallback"}); err != nil {
		t.Fatal(err)
	}
	if err := s.saveObject("tiers", "fallback", routingTier{ID: "fallback", Strategy: "random", Nodes: []string{"fresh"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	_ = s.saveObject("health", "fast", nodeProbe{Success: true, Verified: "tcp", Duration: 1, Timestamp: time.Now().Add(-time.Hour)})
	_ = s.saveObject("health", "fresh", nodeProbe{Success: true, Verified: "tls", Duration: 100, Timestamp: time.Now()})
	id, err := s.selectTier("ping", nodes, map[string]bool{})
	if err != nil || id != "fresh" {
		t.Fatalf("stale low latency selected: %s %v", id, err)
	}
	_ = s.saveObject("health", "fresh", nodeProbe{Success: false, Verified: "tls", Timestamp: time.Now()})
	id, err = s.selectTier("ping", nodes, map[string]bool{})
	if err != nil || id != "fresh" {
		t.Fatalf("fallback unavailable: %s %v", id, err)
	}
}
