package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

func TestAgentReportsActualHealthProbes(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			c, err := listener.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	host, portText, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(portText)
	var report wire.HealthReport
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-agent-token" {
			t.Error("missing agent auth")
		}
		switch r.URL.Path {
		case "/api/v1/agents/health-targets":
			json.NewEncoder(w).Encode(map[string]any{"data": []wire.HealthTarget{{NodeID: "target", Address: host, Inbounds: []wire.HealthInbound{{Protocol: "future-protocol", Port: port, Network: "tcp"}}}}})
		case "/api/v1/agents/health-results":
			if r.Method != "PUT" {
				t.Error("method", r.Method)
			}
			if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
				t.Error(err)
			}
		default:
			t.Error("unexpected path", r.URL.Path)
		}
	}))
	defer core.Close()
	a := testAgent(t, core.URL, "", "synthetic-agent-token")
	if err := a.collectHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || !report.Results[0].Success || report.Results[0].Verified != "tcp" || !report.Results[0].Generic || report.Results[0].Protocol != "future-protocol" || report.Results[0].Timestamp.IsZero() || !report.Results[0].CompletedAt.After(report.Results[0].Timestamp) {
		t.Fatal("actual connect not reported", report)
	}
	listener.Close()
	if err := a.collectHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Success {
		t.Fatal("closed port not reported", report)
	}
}

func TestAgentReportsCompletedProbesFromPartialCycle(t *testing.T) {
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
	host, portText, _ := net.SplitHostPort(stall.Addr().String())
	port, _ := strconv.Atoi(portText)
	targets := []wire.HealthTarget{{NodeID: "fast", Address: host, Inbounds: []wire.HealthInbound{{Port: 1, Network: "tcp"}}}}
	for i := 0; i < 15; i++ {
		targets = append(targets, wire.HealthTarget{NodeID: fmt.Sprintf("slow-%d", i), Address: host, Inbounds: []wire.HealthInbound{{Port: port, Network: "tcp", Security: "tls"}}})
	}
	var report wire.HealthReport
	seen := map[string]bool{}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"data": targets})
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Error(err)
		}
		for _, result := range report.Results {
			seen[result.NodeID] = true
		}
	}))
	defer core.Close()
	a := testAgent(t, core.URL, "", "synthetic-agent-token")
	if err := a.collectHealthCycle(context.Background(), 50*time.Millisecond, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if len(report.Results) == 0 || len(report.Results) >= len(targets) {
		t.Fatal("partial completed probes lost", len(report.Results))
	}
	found := false
	for _, r := range report.Results {
		if r.NodeID == "fast" {
			found = true
		}
		if r.Success || r.Timestamp.IsZero() {
			t.Fatal("incorrect partial evidence", r)
		}
	}
	if !found {
		t.Fatal("fast completed probe not reported")
	}
	for i := 0; i < 6; i++ {
		if err := a.collectHealthCycle(context.Background(), 50*time.Millisecond, 20*time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	if len(seen) != len(targets) {
		t.Fatal("later targets starved across partial cycles", len(seen))
	}
}
