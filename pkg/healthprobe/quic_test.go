package healthprobe

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
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

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/subgate/subgate/pkg/wire"
	"golang.org/x/net/dns/dnsmessage"
)

func TestQUICHandshakeProbe(t *testing.T) {
	if raw := os.Getenv("SUBGATE_SYNTHETIC_H3_TARGET"); raw != "" {
		var target wire.HealthTarget
		if err := json.Unmarshal([]byte(raw), &target); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			name, path, host, sni string
			success               bool
		}{
			{"route", "/probe/", "probe.example.test", "", true},
			{"literal", "/literal%25/", "probe.example.test", "", true},
			{"wrong-path", "/wrong/", "probe.example.test", "", false},
			{"wrong-host", "/probe/", "wrong.example.test", "", false},
			{"wrong-sni", "/probe/", "probe.example.test", "wrong.example.test", false},
			{"plain200", "/plain/", "probe.example.test", "", false},
			{"redirect", "/redirect/", "probe.example.test", "", false},
			{"oversize", "/oversize/", "probe.example.test", "", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				p := target.Inbounds[0].HTTP
				p.Path, p.Host = tc.path, tc.host
				p.ServerName = "example.com"
				if tc.sni != "" {
					p.ServerName = tc.sni
				}
				r := Run(context.Background(), target, time.Second)
				if r.Success != tc.success || r.Verified != "xhttp-h3-options" {
					t.Fatal("HTTP3 verdict differs", r)
				}
			})
		}
		t.Run("multiple-addresses", func(t *testing.T) {
			old := net.DefaultResolver
			net.DefaultResolver = syntheticDNSResolver(t, []net.IP{net.ParseIP("127.0.0.2"), net.ParseIP("127.0.0.1")})
			defer func() { net.DefaultResolver = old }()
			target.Address = "synthetic-multi.test"
			target.Inbounds[0].HTTP.Path = "/probe/"
			target.Inbounds[0].HTTP.Host = "probe.example.test"
			target.Inbounds[0].HTTP.ServerName = "example.com"
			if r := Run(context.Background(), target, time.Second); !r.Success || r.Verified != "xhttp-h3-options" {
				t.Fatal("working alternative IP ignored", r)
			}
			target.Address = "127.0.0.1"
		})
		target.Inbounds[0].HTTP.Path = "/stall/"
		target.Inbounds[0].HTTP.ServerName = "example.com"
		started := time.Now()
		r := Run(context.Background(), target, 40*time.Millisecond)
		if r.Success || r.Verified != "xhttp-h3-options" || time.Since(started) > time.Second {
			t.Fatal("HTTP3 deadline ignored", r)
		}
		return
	}
	// Certificate and trust are synthetic and isolated in a fresh test process.
	tlsServer := httptest.NewTLSServer(http.NotFoundHandler())
	defer tlsServer.Close()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	listener, err := quic.ListenEarly(udp, http3.ConfigureTLSConfig(&tls.Config{Certificates: tlsServer.TLS.Certificates}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	srv := &http3.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 3 || r.Method != "OPTIONS" || r.Host != "probe.example.test" || r.Header.Get("Authorization") != "" || r.ContentLength > 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		switch r.URL.Path {
		case "/probe/", "/literal%25/", "/oversize/":
		case "/plain/":
			return
		case "/redirect/":
			w.Header().Set("Location", "https://127.0.0.1:1/")
			w.WriteHeader(http.StatusFound)
			return
		case "/stall/":
			<-r.Context().Done()
			return
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Path == "/oversize/" {
			w.Header().Set("X-Padding", strings.Repeat("x", 20000))
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	})}
	defer srv.Close()
	go srv.ServeListener(listener)
	target := wire.HealthTarget{NodeID: "h3", Address: "127.0.0.1", Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: "xhttp", Security: "tls", Port: udp.LocalAddr().(*net.UDPAddr).Port, HTTP: &wire.HealthHTTP{Path: "/probe/", Host: "probe.example.test", ServerName: "example.com", HTTP3: true}}}}
	root := filepath.Join(t.TempDir(), "synthetic-root.crt")
	if err := os.WriteFile(root, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tlsServer.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(target)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestQUICHandshakeProbe$", "-test.v")
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+root, "SSL_CERT_DIR="+t.TempDir(), "SUBGATE_SYNTHETIC_H3_TARGET="+string(raw))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated HTTP3 checks failed: %v\n%s", err, out)
	}
}

func syntheticDNSResolver(t *testing.T, ips []net.IP) *net.Resolver {
	t.Helper()
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	go func() {
		var buf [4096]byte
		for {
			n, addr, err := conn.ReadFrom(buf[:])
			if err != nil {
				return
			}
			var request dnsmessage.Message
			if request.Unpack(buf[:n]) != nil || len(request.Questions) != 1 {
				continue
			}
			response := dnsmessage.Message{Header: dnsmessage.Header{ID: request.ID, Response: true, RecursionAvailable: true}, Questions: request.Questions}
			q := request.Questions[0]
			if q.Type == dnsmessage.TypeA {
				for _, ip := range ips {
					v := ip.To4()
					if v == nil {
						continue
					}
					response.Answers = append(response.Answers, dnsmessage.Resource{Header: dnsmessage.ResourceHeader{Name: q.Name, Type: dnsmessage.TypeA, Class: dnsmessage.ClassINET, TTL: 1}, Body: &dnsmessage.AResource{A: [4]byte{v[0], v[1], v[2], v[3]}}})
				}
			}
			packet, err := response.Pack()
			if err == nil {
				conn.WriteTo(packet, addr)
			}
		}
	}()
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", conn.LocalAddr().String())
	}}
}

func TestHTTP3UnsupportedSettingsStayUnverified(t *testing.T) {
	for _, tc := range []struct{ network, security, path string }{
		{"xhttp", "none", "/probe/"},
		{"xhttp", "reality", "/probe/"},
		{"grpc", "tls", "/probe/"},
		{"future-network", "tls", "/probe/"},
		{"xhttp", "tls", ""},
		{"xhttp", "tls", "/probe\r\nInjected:yes"},
	} {
		t.Run(tc.network+tc.security+tc.path, func(t *testing.T) {
			target := wire.HealthTarget{NodeID: "h3", Address: "127.0.0.1", Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: tc.network, Security: tc.security, Port: 1, HTTP: &wire.HealthHTTP{HTTP3: true, Path: tc.path}}}}
			if r := Run(context.Background(), target, time.Second); r.Success || r.Verified != "unverified" {
				t.Fatal("unsupported HTTP3 attempted", r)
			}
		})
	}
}

func TestHTTP3UntrustedCertificateAndUDPDeadline(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	udp, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer udp.Close()
	listener, err := quic.ListenEarly(udp, http3.ConfigureTLSConfig(&tls.Config{Certificates: srv.TLS.Certificates}), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	h3 := &http3.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("HTTP sent despite untrusted certificate") })}
	defer h3.Close()
	go h3.ServeListener(listener)
	target := httpTarget(t, srv.URL)
	target.Inbounds[0].Network, target.Inbounds[0].Security = "xhttp", "tls"
	target.Inbounds[0].Port = udp.LocalAddr().(*net.UDPAddr).Port
	target.Inbounds[0].HTTP.HTTP3 = true
	target.Inbounds[0].HTTP.ServerName = "example.com"
	if r := Run(context.Background(), target, time.Second); r.Success || r.Verified != "xhttp-h3-options" {
		t.Fatal("untrusted QUIC accepted", r)
	}
	// A TCP listener at the same endpoint never substitutes for UDP/QUIC.
	host, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	target.Address = host
	target.Inbounds[0].Port, _ = strconv.Atoi(port)
	start := time.Now()
	if r := Run(context.Background(), target, 40*time.Millisecond); r.Success || r.Verified != "xhttp-h3-options" || time.Since(start) > time.Second {
		t.Fatal("TCP endpoint substituted or UDP deadline ignored", r)
	}
}

func TestHTTP3ResolutionHonorsDeadlineAndClosesSockets(t *testing.T) {
	target := wire.HealthTarget{NodeID: "h3", Address: "bad\x00.invalid", Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: "xhttp", Security: "tls", Port: 443, HTTP: &wire.HealthHTTP{Path: "/probe/", HTTP3: true}}}}
	before, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 32; i++ {
		if r := Run(context.Background(), target, 40*time.Millisecond); r.Success {
			t.Fatal("invalid DNS name accepted", r)
		}
	}
	after, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) > len(before)+2 {
		t.Fatalf("failed resolution leaked descriptors: before=%d after=%d", len(before), len(after))
	}
	resolver := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	defer func() { net.DefaultResolver = resolver }()
	target.Address = "synthetic-blocked-dns.test"
	start := time.Now()
	if r := Run(context.Background(), target, 40*time.Millisecond); r.Success || r.Verified != "xhttp-h3-options" || time.Since(start) > time.Second {
		t.Fatal("DNS resolution ignored probe deadline", r, time.Since(start))
	}
}

func TestHTTP3UncoveredDNSCandidatesRemainUnverified(t *testing.T) {
	old := net.DefaultResolver
	defer func() { net.DefaultResolver = old }()
	net.DefaultResolver = syntheticDNSResolver(t, []net.IP{net.ParseIP("127.0.0.2"), net.ParseIP("127.0.0.3"), net.ParseIP("127.0.0.4"), net.ParseIP("127.0.0.5"), net.ParseIP("127.0.0.6"), net.ParseIP("127.0.0.7"), net.ParseIP("127.0.0.8"), net.ParseIP("127.0.0.9"), net.ParseIP("127.0.0.10")})
	target := wire.HealthTarget{NodeID: "h3", Address: "synthetic-multi.test", Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: "xhttp", Security: "tls", Port: 1, HTTP: &wire.HealthHTTP{Path: "/probe/", HTTP3: true}}}}
	if r := Run(context.Background(), target, 120*time.Millisecond); r.Success || r.Verified != "unverified" {
		t.Fatal("uncovered DNS addresses became verified failure", r)
	}
}
