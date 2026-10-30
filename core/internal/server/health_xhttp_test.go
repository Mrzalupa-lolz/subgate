package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

func TestXHTTPConfiguredPathProbe(t *testing.T) {
	var accepted atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "OPTIONS" || r.URL.Path != "/synthetic-xhttp/" || r.Host != "probe.example.test" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(404)
			return
		}
		accepted.Add(1)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
		w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
		w.WriteHeader(200)
	}))
	defer upstream.Close()
	host, p, _ := net.SplitHostPort(strings.TrimPrefix(upstream.URL, "http://"))
	port, _ := strconv.Atoi(p)
	raw := fmt.Sprintf(`{"type":"vless","network":"xhttp","port":%d,"rawInbound":{"settings":{"clients":[{"id":"synthetic-credential"}]},"streamSettings":{"network":"xhttp","xhttpSettings":{"path":"/synthetic-xhttp","host":"probe.example.test","mode":"packet-up","headers":{"Authorization":"synthetic-secret"}}}}}`, port)
	var inbound panel.Inbound
	if err := json.Unmarshal([]byte(raw), &inbound); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t)
	n := panel.Node{ID: "xhttp", IPAddress: host, Inbounds: []panel.Inbound{inbound}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	var probe nodeProbe
	if err := s.object("health", n.ID, &probe); err != nil || !probe.Success || probe.Verified != "xhttp-options" || accepted.Load() != 1 {
		t.Fatal("XHTTP preflight not observed", probe, accepted.Load(), err)
	}
	session := loginTest(t, s)
	w := perform(s, "POST", "/api/v1/nodes/xhttp/probe", nil, session, "", true)
	if w.Code != 200 || accepted.Load() != 2 || !strings.Contains(w.Body.String(), `"verified":"xhttp-options"`) {
		t.Fatal("manual preflight differs", w.Code)
	}
	token, _ := enrollTest(t, s, session, "demo-node-1")
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: time.Now(), Verified: "xhttp-options", Success: true}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal("Agent preflight rejected", w.Code)
	}
	s.config.Health.Quorum = 2
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || !probe.QuorumMet || probe.Vantages != 2 {
		t.Fatal("preflight quorum not met", probe, err)
	}
	s.config.Health.Quorum = 1
	data, _ := json.Marshal(healthTarget(n))
	if strings.Contains(string(data), "synthetic-credential") || strings.Contains(string(data), "synthetic-secret") {
		t.Fatal("raw config forwarded")
	}
	raw = strings.Replace(raw, "/synthetic-xhttp", "/wrong", 1)
	if err := json.Unmarshal([]byte(raw), &n.Inbounds[0]); err != nil {
		t.Fatal(err)
	}
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || probe.Success || probe.Verified != "xhttp-options" {
		t.Fatal("TCP success masked wrong XHTTP route", probe, err)
	}
}

func TestXHTTPH3OnlyDoesNotBecomeTCPFailure(t *testing.T) {
	if os.Getenv("SUBGATE_SYNTHETIC_CORE_H3_PORT") == "" {
		var tcpConnections, accepted atomic.Int32
		upstream := httptest.NewUnstartedServer(http.NotFoundHandler())
		upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateNew {
				tcpConnections.Add(1)
			}
		}
		upstream.StartTLS()
		defer upstream.Close()
		udp, err := net.ListenPacket("udp", upstream.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer udp.Close()
		listener, err := quic.ListenEarly(udp, http3.ConfigureTLSConfig(&tls.Config{Certificates: upstream.TLS.Certificates}), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		h3 := &http3.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/synthetic/" || r.Host != "probe.example.test" || r.Method != "OPTIONS" || r.ProtoMajor != 3 {
				w.WriteHeader(404)
				return
			}
			accepted.Add(1)
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "POST")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		})}
		defer h3.Close()
		go h3.ServeListener(listener)
		root := filepath.Join(t.TempDir(), "synthetic-root.crt")
		if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: upstream.Certificate().Raw}), 0600); err != nil {
			t.Fatal(err)
		}
		self, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, self, "-test.run=^TestXHTTPH3OnlyDoesNotBecomeTCPFailure$")
		cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+root, "SSL_CERT_DIR="+t.TempDir(), "SUBGATE_SYNTHETIC_CORE_H3_PORT="+strconv.Itoa(udp.LocalAddr().(*net.UDPAddr).Port))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("isolated Core HTTP3 failed: %v\n%s", err, out)
		}
		if accepted.Load() < 5 || tcpConnections.Load() != 0 {
			t.Fatal("HTTP3 not observed or TCP probed", accepted.Load(), tcpConnections.Load())
		}
		return
	}
	port, _ := strconv.Atoi(os.Getenv("SUBGATE_SYNTHETIC_CORE_H3_PORT"))
	raw := fmt.Sprintf(`{"type":"vless","network":"xhttp","security":"tls","port":%d,"rawInbound":{"streamSettings":{"network":"xhttp","security":"tls","tlsSettings":{"alpn":["h3"],"serverName":"example.com"},"xhttpSettings":{"path":"/synthetic","host":"probe.example.test"}}}}`, port)
	var inbound panel.Inbound
	if err := json.Unmarshal([]byte(raw), &inbound); err != nil {
		t.Fatal(err)
	}
	s := newTestServer(t)
	n := panel.Node{ID: "h3-only", IPAddress: "127.0.0.1", Inbounds: []panel.Inbound{inbound}}
	if err := s.saveObject("nodes", n.ID, n); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
			t.Fatal(err)
		}
	}
	var probe nodeProbe
	state, _ := s.health.SnapshotNode(n.ID)
	if err := s.object("health", n.ID, &probe); err != nil || !probe.Success || probe.Verified != "xhttp-h3-options" || state.ConsecutiveFailures != 0 {
		t.Fatal("H3-only guessed TCP or false failure", probe, state, err)
	}
	session := loginTest(t, s)
	w := perform(s, "POST", "/api/v1/nodes/h3-only/probe", nil, session, "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"verified":"xhttp-h3-options"`) {
		t.Fatal("manual H3 differs", w.Code)
	}
	token, _ := enrollTest(t, s, session, "demo-node-1")
	report := wire.HealthReport{Results: []wire.HealthResult{{NodeID: n.ID, Timestamp: time.Now(), Verified: "xhttp-h3-options", Success: true}}}
	if w := perform(s, "PUT", "/api/v1/agents/health-results", report, nil, token, false); w.Code != 200 {
		t.Fatal("Agent H3 report rejected", w.Code)
	}
	s.config.Health.Quorum = 2
	if err := s.scheduledNodeProbe(context.Background(), n); err != nil {
		t.Fatal(err)
	}
	if err := s.object("health", n.ID, &probe); err != nil || !probe.QuorumMet || probe.Vantages != 2 {
		t.Fatal("H3 quorum not met", probe, err)
	}
}
