package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

func TestHTTPUpgradeTransportProbe(t *testing.T) {
	var accepted atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/synthetic-upgrade" || r.Host != "probe.example.test" || r.Header.Get("Connection") != "Upgrade" || r.Header.Get("Upgrade") != "websocket" || r.Header.Get("Sec-WebSocket-Key") != "" {
			w.WriteHeader(404)
			return
		}
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprint(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n\r\n")
		accepted.Add(1)
	}))
	defer upstream.Close()
	host, p, _ := net.SplitHostPort(strings.TrimPrefix(upstream.URL, "http://"))
	port, _ := strconv.Atoi(p)
	raw := fmt.Sprintf(`{"type":"vless","network":"httpupgrade","port":%d,"rawInbound":{"settings":{"clients":[{"id":"synthetic-credential"}]},"streamSettings":{"method":"httpupgrade","httpupgradeSettings":{"path":"/synthetic-upgrade?ed=1024","host":"probe.example.test","headers":{"Authorization":"synthetic-secret"}}}}}`, port)
	var inbound panel.Inbound
	if err := json.Unmarshal([]byte(raw), &inbound); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t)
	n := panel.Node{ID: "httpupgrade", IPAddress: host, Inbounds: []panel.Inbound{inbound}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	var probe nodeProbe
	if err := s.object("health", n.ID, &probe); err != nil || !probe.Success || probe.Verified != "httpupgrade" || accepted.Load() != 1 {
		t.Fatal("HTTPUpgrade acceptance not verified", probe, accepted.Load(), err)
	}
	session := loginTest(t, s)
	w := perform(s, "POST", "/api/v1/nodes/httpupgrade/probe", nil, session, "", true)
	if w.Code != 200 || accepted.Load() != 2 || !strings.Contains(w.Body.String(), `"verified":"httpupgrade"`) {
		t.Fatal("manual HTTPUpgrade differs", w.Code)
	}
	token, _ := enrollTest(t, s, session, "demo-node-1")
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: time.Now(), Verified: "httpupgrade", Success: true}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal("Agent HTTPUpgrade rejected", w.Code)
	}
	s.config.Health.Quorum = 2
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || !probe.QuorumMet || probe.Vantages != 2 {
		t.Fatal("HTTPUpgrade quorum not met", probe, err)
	}
	s.config.Health.Quorum = 1
	data, _ := json.Marshal(healthTarget(n))
	if strings.Contains(string(data), "synthetic-secret") || strings.Contains(string(data), "synthetic-credential") {
		t.Fatal("HTTPUpgrade leaked raw config")
	}
	raw = strings.Replace(raw, "/synthetic-upgrade?ed=1024", "/wrong", 1)
	if err := json.Unmarshal([]byte(raw), &n.Inbounds[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || probe.Success || probe.Verified != "httpupgrade" {
		t.Fatal("TCP masked HTTPUpgrade rejection", probe, err)
	}
}
