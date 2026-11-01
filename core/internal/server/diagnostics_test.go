package server

import (
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/subgate/subgate/core/internal/panel"
)

func TestBlockCheckRestrictsTargetToSyncedNodeAndHonorsTLS(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			_, _ = io.WriteString(conn, "plaintext response")
			conn.Close()
		}
	}()
	host, rawPort, _ := net.SplitHostPort(listener.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	if err := s.saveObject("nodes", "diagnostic", panel.Node{ID: "diagnostic", IPAddress: host, Inbounds: []panel.Inbound{{Port: port}}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, node string
		input      map[string]any
		want       int
	}{
		{"unknown node", "unknown", map[string]any{"address": host, "port": port}, 404},
		{"different address", "diagnostic", map[string]any{"address": "localhost", "port": port}, 400},
		{"different port", "diagnostic", map[string]any{"port": port + 1}, 400},
		{"invalid port", "diagnostic", map[string]any{"port": 65536}, 400},
		{"negative port", "diagnostic", map[string]any{"port": -1}, 400},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := perform(s, "POST", "/api/v1/nodes/"+tc.node+"/block-check", tc.input, session, "", true)
			if w.Code != tc.want {
				t.Errorf("got %d %s; want %d", w.Code, w.Body, tc.want)
			}
		})
	}
	w := perform(s, "POST", "/api/v1/nodes/diagnostic/block-check", map[string]any{"port": port, "tls": true, "server_name": "node.example"}, session, "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("TLS request accepted plaintext transport: %d %s", w.Code, w.Body)
	}
	w = perform(s, "POST", "/api/v1/nodes/diagnostic/block-check", map[string]any{"port": port}, session, "", true)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"verdict":"ok"`) {
		t.Fatalf("configured endpoint: %d %s", w.Code, w.Body)
	}
}

func TestYouTubeAPIRejectsUntrustedEndpoints(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	for _, endpoint := range []string{
		"http://127.0.0.1:1/", "http://169.254.169.254/latest/meta-data/", "file:///etc/passwd",
		"https://www.youtube.com.evil.example/", "https://www.youtube.com:8443/", "https://user@www.youtube.com/",
	} {
		w := perform(s, "POST", "/api/v1/youtube/check", map[string]any{"endpoint": endpoint}, session, "", true)
		if w.Code != 400 {
			t.Errorf("endpoint %q accepted: %d %s", endpoint, w.Code, w.Body)
		}
	}
}

type diagnosticTransport func(*http.Request) (*http.Response, error)

func (f diagnosticTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestYouTubeAPIDoesNotFollowRedirectToInternalService(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	var internalRequests int
	http.DefaultTransport = diagnosticTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() == "www.youtube.com" {
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"http://127.0.0.1/private"}}, Body: io.NopCloser(strings.NewReader("")), Request: r}, nil
		}
		internalRequests++
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("secret")), Request: r}, nil
	})
	w := perform(s, "POST", "/api/v1/youtube/check", map[string]any{"endpoint": "https://www.youtube.com/watch?v=test"}, session, "", true)
	if w.Code != 200 || internalRequests != 0 || !strings.Contains(w.Body.String(), `"error"`) {
		t.Fatalf("unsafe redirect: code=%d internal requests=%d body=%s", w.Code, internalRequests, w.Body)
	}
}

func TestQuotaAuditFailureRollsBackMutation(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	input := map[string]any{"node_id": "fi", "scope": "per_node", "limit_bytes": 100, "period": "day", "enabled": true}
	if w := perform(s, "PUT", "/api/v1/quotas/one", input, session, "", true); w.Code != 200 {
		t.Fatal(w.Body)
	}
	if _, err := s.db.Exec("DROP TABLE audit_log"); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"PUT", "DELETE"} {
		if w := perform(s, method, "/api/v1/quotas/one", input, session, "", true); w.Code != 500 {
			t.Errorf("unaudited %s succeeded: %d %s", method, w.Code, w.Body)
		}
	}
	var stored map[string]any
	if err := s.object("quotas", "one", &stored); err != nil {
		t.Fatal("audit failure deleted quota:", err)
	}
}
