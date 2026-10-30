package agent

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

	"github.com/subgate/subgate/pkg/wire"
)

func TestAgentReportsWebSocketTransportProbe(t *testing.T) {
	testAgentReportsUpgrade(t, "ws")
}

func TestAgentReportsHTTPUpgradeTransportProbe(t *testing.T) {
	testAgentReportsUpgrade(t, "httpupgrade")
}

func testAgentReportsUpgrade(t *testing.T, network string) {
	t.Helper()
	var observed atomic.Int32
	ws := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent-ws" || r.Host != "probe.example.test" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(404)
			return
		}
		if network == "httpupgrade" && r.Header.Get("Sec-WebSocket-Key") != "" {
			t.Error("WS challenge sent for HTTPUpgrade")
		}
		observed.Add(1)
		h := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		response := "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"
		if network == "ws" {
			response += "Sec-WebSocket-Accept: " + base64.StdEncoding.EncodeToString(h[:]) + "\r\n"
		}
		fmt.Fprint(conn, response+"\r\n")
	}))
	defer ws.Close()
	host, p, _ := net.SplitHostPort(strings.TrimPrefix(ws.URL, "http://"))
	port, _ := strconv.Atoi(p)
	target := wire.HealthTarget{NodeID: "upgrade", Address: host, Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: network, Port: port, HTTP: &wire.HealthHTTP{Path: "/agent-ws", Host: "probe.example.test"}}}}
	var report wire.HealthReport
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-agent-token" {
			t.Error("missing agent authentication")
		}
		if r.Method == "GET" {
			json.NewEncoder(w).Encode(map[string]any{"data": []wire.HealthTarget{target}})
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
			t.Error(err)
		}
	}))
	defer core.Close()
	a := testAgent(t, core.URL, "", "synthetic-agent-token")
	if err := a.collectHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || !report.Results[0].Success || report.Results[0].Verified != network || observed.Load() != 1 {
		t.Fatal("Agent WS not observed", report, observed.Load())
	}
	target.Inbounds[0].HTTP.Path = "/wrong"
	if err := a.collectHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Success || report.Results[0].Verified != network {
		t.Fatal("Agent WS rejection hidden", report)
	}
}
