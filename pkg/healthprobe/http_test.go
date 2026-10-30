package healthprobe

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
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
	"testing"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

func TestWebSocketProbeRejectsInvalidAcceptance(t *testing.T) {
	for _, tc := range []struct {
		name, response string
		success        bool
	}{
		{"good", "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: keep-alive, Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", true},
		{"plain-http", "HTTP/1.1 200 OK\r\nContent-Length: 0\r\n\r\n", false},
		{"redirect", "HTTP/1.1 302 Found\r\nLocation: http://127.0.0.1:1/\r\nContent-Length: 0\r\n\r\n", false},
		{"fake-101", "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: invalid\r\n\r\n", false},
		{"no-upgrade", "HTTP/1.1 101 Switching Protocols\r\nSec-WebSocket-Accept: %s\r\n\r\n", false},
		{"oversize", "HTTP/1.1 101 Switching Protocols\r\nX-Padding: " + strings.Repeat("x", 20000) + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				h := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
				conn, _, err := w.(http.Hijacker).Hijack()
				if err != nil {
					return
				}
				defer conn.Close()
				response := strings.ReplaceAll(tc.response, "%s", base64.StdEncoding.EncodeToString(h[:]))
				fmt.Fprint(conn, response)
			}))
			defer srv.Close()
			target := httpTarget(t, srv.URL)
			r := Run(context.Background(), target, time.Second)
			if r.Success != tc.success || r.Verified != "ws" {
				t.Fatal(r)
			}
		})
	}
}

func TestWebSocketProbeTLSValidCertificateAndHostPort(t *testing.T) {
	// A fresh process isolates synthetic trust roots from the test runner's
	// cached system roots; production code always uses normal TLS verification.
	if raw := os.Getenv("SUBGATE_TLS_PROBE_CHILD_TARGET"); raw != "" {
		var target wire.HealthTarget
		if err := json.Unmarshal([]byte(raw), &target); err != nil {
			t.Fatal(err)
		}
		for _, network := range []string{"ws", "httpupgrade", "xhttp"} {
			verified := network
			if network == "xhttp" {
				verified = "xhttp-options"
			}
			target.Inbounds[0].Network = network
			target.Inbounds[0].HTTP.ServerName = ""
			r := Run(context.Background(), target, time.Second)
			if !r.Success || r.Verified != verified {
				t.Fatal("trusted upgrade TLS rejected", r)
			}
			target.Inbounds[0].HTTP.ServerName = "wrong.example.test"
			r = Run(context.Background(), target, time.Second)
			if r.Success || r.Verified != verified {
				t.Fatal("TLS hostname verification bypassed", r)
			}
		}
		return
	}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "OPTIONS" {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
			w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
			return
		}
		h := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		fmt.Fprintf(conn, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(h[:]))
	}))
	defer srv.Close()
	target := httpTarget(t, srv.URL)
	name := "127.0.0.1"
	if len(srv.Certificate().DNSNames) > 0 {
		name = srv.Certificate().DNSNames[0]
	}
	target.Inbounds[0].Security = "tls"
	target.Inbounds[0].HTTP.Host = net.JoinHostPort(name, strconv.Itoa(target.Inbounds[0].Port))
	root := filepath.Join(t.TempDir(), "synthetic-root.crt")
	if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(target)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestWebSocketProbeTLSValidCertificateAndHostPort$")
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+root, "SSL_CERT_DIR="+t.TempDir(), "SUBGATE_TLS_PROBE_CHILD_TARGET="+string(raw))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated trusted TLS check failed: %v\n%s", err, out)
	}
}

func httpTarget(t *testing.T, address string) wire.HealthTarget {
	t.Helper()
	host, p, err := net.SplitHostPort(strings.TrimPrefix(strings.TrimPrefix(address, "http://"), "https://"))
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(p)
	return wire.HealthTarget{NodeID: "ws", Address: host, Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: "ws", Port: port, HTTP: &wire.HealthHTTP{Path: "/probe", Host: "configured.example.test"}}}}
}

func TestWebSocketProbeDeadlineAndInvalidConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	target := httpTarget(t, srv.URL)
	started := time.Now()
	r := Run(context.Background(), target, 30*time.Millisecond)
	if r.Success || r.Verified != "ws" || time.Since(started) > time.Second {
		t.Fatal(r)
	}
	for _, path := range []string{"http://other.test/probe", "//other.test/probe", "/probe\r\nX-Injected: yes", "/" + strings.Repeat("x", 4096)} {
		target.Inbounds[0].HTTP.Path = path
		r = Run(context.Background(), target, time.Second)
		if r.Success || r.Verified != "unverified" {
			t.Fatal("unsafe path probed", r)
		}
	}
}

func TestWebSocketProbeTLSUsesSNIAndVerifiesCertificate(t *testing.T) {
	for _, tc := range []struct{ name, host, sni, want string }{
		{"explicit", "configured.example.test", "configured.example.test", "configured.example.test"},
		{"host-port", "configured.example.test:443", "", "configured.example.test"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seen := make(chan string, 1)
			srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("HTTP sent despite untrusted TLS certificate") }))
			srv.TLS = &tls.Config{GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) { seen <- hello.ServerName; return nil, nil }}
			srv.StartTLS()
			defer srv.Close()
			target := httpTarget(t, srv.URL)
			target.Inbounds[0].Security = "tls"
			target.Inbounds[0].HTTP.Host = tc.host
			target.Inbounds[0].HTTP.ServerName = tc.sni
			r := Run(context.Background(), target, time.Second)
			if r.Success || r.Verified != "ws" {
				t.Fatal(r)
			}
			select {
			case name := <-seen:
				if name != tc.want {
					t.Fatal(name)
				}
			case <-time.After(time.Second):
				t.Fatal("TLS SNI not observed")
			}
		})
	}
}
