package server

import (
	"context"
	"encoding/json"
	"net"
	"strconv"
	"testing"

	"github.com/subgate/subgate/core/internal/panel"
)

func TestUnknownProtocolGenericHandling(t *testing.T) {
	s := newTestServer(t)
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
			c.Close()
		}
	}()
	host, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	n := panel.Node{ID: "future-protocol", IPAddress: host, Status: "online", Inbounds: []panel.Inbound{{Type: "never-seen-protocol", Network: "raw", Port: port}}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	var sample nodeProbe
	if err := s.object("health", n.ID, &sample); err != nil || !sample.Generic || !sample.Success || sample.Verified != "tcp" || sample.Protocol != "never-seen-protocol" {
		t.Fatal("generic TCP probe not identified", sample, err)
	}
	w := perform(s, "POST", "/api/v1/nodes/future-protocol/probe", nil, loginTest(t, s), "", true)
	var manual struct {
		Data []nodeProbe `json:"data"`
	}
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &manual) != nil || len(manual.Data) != 1 || !manual.Data[0].Generic || !manual.Data[0].Success {
		t.Fatal("manual generic probe differs", w.Code)
	}
	// Unknown transport/security cannot be inferred to use TCP even at a live
	// TCP port, and unverified observations never become quarantine evidence.
	for _, in := range []panel.Inbound{{Type: "never-seen-protocol", Network: "future-datagram", Port: port}, {Type: "never-seen-protocol", Network: "udp", Port: port}, {Type: "never-seen-protocol", Network: "raw", Security: "future-security", Port: port}} {
		n.Inbounds = []panel.Inbound{in}
		for i := 0; i < 4; i++ {
			if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
				t.Fatal(err)
			}
		}
		if err := s.object("health", n.ID, &sample); err != nil || sample.Verified != "unverified" || !sample.Generic || sample.Success {
			t.Fatal("unknown capabilities guessed TCP", sample, err)
		}
		state, _ := s.health.SnapshotNode(n.ID)
		if state.ConsecutiveFailures != 0 {
			t.Fatal("unverifiable probe caused quarantine", state)
		}
	}

}

func TestManualUnknownEndpointPreventsFalseNodeFailure(t *testing.T) {
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, p, _ := net.SplitHostPort(closed.Addr().String())
	port, _ := strconv.Atoi(p)
	closed.Close()
	known := panel.Inbound{Type: "vless", Network: "raw", Port: port}
	unknown := panel.Inbound{Type: "future", Network: "future-transport", Port: port}
	many := make([]panel.Inbound, 17)
	for i := range many {
		many[i] = known
	}
	for _, tc := range []struct {
		name     string
		inbounds []panel.Inbound
	}{
		{"unknown-last", []panel.Inbound{known, unknown}},
		{"unknown-first", []panel.Inbound{unknown, known}},
		{"coverage-limit", many},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			n := panel.Node{ID: "incomplete", IPAddress: "127.0.0.1", Inbounds: tc.inbounds}
			if err := s.saveObject("nodes", n.ID, n); err != nil {
				t.Fatal(err)
			}
			w := perform(s, "POST", "/api/v1/nodes/incomplete/probe", nil, loginTest(t, s), "", true)
			if w.Code != 200 {
				t.Fatal(w.Code)
			}
			var sample nodeProbe
			if err := s.object("health", n.ID, &sample); err != nil || sample.Success || sample.Verified != "unverified" {
				t.Fatalf("incomplete manual coverage recorded as failure: %+v, %v", sample, err)
			}
		})
	}
}
