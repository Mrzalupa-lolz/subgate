package server

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
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

func TestHTTPTransportProbe(t *testing.T) {
	var accepted atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/synthetic-ws" || r.Host != "probe.example.test" || r.Header.Get("Upgrade") != "websocket" || r.Header.Get("Sec-WebSocket-Version") != "13" {
			w.WriteHeader(404)
			return
		}
		key := r.Header.Get("Sec-WebSocket-Key")
		h := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(h[:]))
		accepted.Add(1)
	}))
	defer upstream.Close()
	host, p, _ := net.SplitHostPort(strings.TrimPrefix(upstream.URL, "http://"))
	port, _ := strconv.Atoi(p)
	var inbound panel.Inbound
	raw := fmt.Sprintf(`{"tag":"ws","type":"vless","network":"ws","security":"none","port":%d,"rawInbound":{"settings":{"clients":[{"id":"synthetic-private-credential"}]},"streamSettings":{"network":"ws","security":"none","wsSettings":{"path":"/synthetic-ws?ed=2048","host":"probe.example.test"}}}}`, port)
	if err := json.Unmarshal([]byte(raw), &inbound); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t)
	n := panel.Node{ID: "http-probe", IPAddress: host, Inbounds: []panel.Inbound{inbound}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	var probe nodeProbe
	if err := s.object("health", n.ID, &probe); err != nil || !probe.Success || probe.Verified != "ws" || accepted.Load() != 1 {
		t.Fatal("configured WS handshake was not verified", probe, accepted.Load(), err)
	}
	w := perform(s, "POST", "/api/v1/nodes/http-probe/probe", nil, loginTest(t, s), "", true)
	if w.Code != 200 || accepted.Load() != 2 || !strings.Contains(w.Body.String(), `"verified":"ws"`) {
		t.Fatal("manual path did not verify WS handshake", w.Code, accepted.Load())
	}
	data, _ := json.Marshal(healthTarget(n))
	stored, _ := json.Marshal(n)
	if strings.Contains(string(data), "synthetic-private-credential") || strings.Contains(string(stored), "synthetic-private-credential") || !strings.Contains(string(data), "/synthetic-ws") {
		t.Fatal("target missed sanitized probe or retained raw credentials")
	}
	token, _ := enrollTest(t, s, loginTest(t, s), "demo-node-1")
	targetResponse := perform(s, "GET", "/api/v1/agents/health-targets", nil, nil, token, false)
	if targetResponse.Code != 200 || !strings.Contains(targetResponse.Body.String(), "/synthetic-ws") || strings.Contains(targetResponse.Body.String(), "synthetic-private-credential") {
		t.Fatal("agent target endpoint lost sanitized descriptor", targetResponse.Code)
	}
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: time.Now(), Verified: "ws", Success: true}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal("WS agent report rejected", w.Code)
	}
	s.config.Health.Quorum = 2
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || !probe.QuorumMet || probe.Vantages != 2 {
		t.Fatal("WS report not used in quorum", probe, err)
	}
	s.config.Health.Quorum = 1
	// The listener remains reachable over TCP; a wrong path must fail WS.
	raw = strings.Replace(raw, "/synthetic-ws?ed=2048", "/wrong", 1)
	if err := json.Unmarshal([]byte(raw), &n.Inbounds[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || probe.Success || probe.Verified != "ws" {
		t.Fatal("TCP success hid WS rejection", probe, err)
	}
}
