package healthprobe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/subgate/subgate/pkg/wire"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

func TestGRPCProbeRequiresSuccessfulStatus(t *testing.T) {
	for _, tc := range []struct {
		name, ctype, status string
		code                int
		body                string
		success             bool
	}{
		{"ok", "application/grpc", "0", 200, "", true},
		{"unknown-method", "application/grpc", "12", 200, "", false},
		{"missing-status", "application/grpc", "", 200, "", false},
		{"ordinary-http", "text/html", "0", 200, "", false},
		{"not-found", "application/grpc", "0", 404, "", false},
		{"empty-frame", "application/grpc", "0", 200, string([]byte{0, 0, 0, 0, 0}), true},
		{"invalid-frame", "application/grpc", "0", 200, "bad", false},
		{"invalid-protobuf", "application/grpc", "0", 200, string([]byte{0, 0, 0, 0, 1, 0xff}), false},
		{"oversize-body", "application/grpc", "0", 200, strings.Repeat("x", 4097), false},
		{"oversize-header", "application/grpc", "0", 200, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor != 2 || r.Method != "POST" || r.URL.Path != "/synthetic.Service/Tun" || r.Header.Get("Content-Type") != "application/grpc" {
					t.Error("wrong gRPC request")
				}
				body, _ := io.ReadAll(io.LimitReader(r.Body, 6))
				if !bytes.Equal(body, []byte{0, 0, 0, 0, 0}) {
					t.Error("nonempty proxy payload sent")
				}
				w.Header().Set("Content-Type", tc.ctype)
				w.Header().Set("Trailer", "Grpc-Status")
				if tc.name == "oversize-header" {
					w.Header().Set("X-Padding", strings.Repeat("x", 20000))
				}
				w.WriteHeader(tc.code)
				w.Write([]byte(tc.body))
				if tc.status != "" {
					w.Header().Set("Grpc-Status", tc.status)
				}
			}), &http2.Server{}))
			defer srv.Close()
			target := httpTarget(t, srv.URL)
			target.Inbounds[0].Network = "grpc"
			target.Inbounds[0].HTTP.Path = "/synthetic.Service/Tun"
			r := Run(context.Background(), target, time.Second)
			if r.Verified != "grpc" || r.Success != tc.success {
				t.Fatal(r)
			}
		})
	}
}

func TestGRPCProbeTLSCertificateSNIAndDeadline(t *testing.T) {
	if raw := os.Getenv("SUBGATE_GRPC_TLS_CHILD_TARGET"); raw != "" {
		var target wire.HealthTarget
		if err := json.Unmarshal([]byte(raw), &target); err != nil {
			t.Fatal(err)
		}
		r := Run(context.Background(), target, time.Second)
		if !r.Success || r.Verified != "grpc" {
			t.Fatal("trusted gRPC TLS failed", r)
		}
		target.Inbounds[0].HTTP.ServerName = "wrong.example.test"
		r = Run(context.Background(), target, time.Second)
		if r.Success || r.Verified != "grpc" {
			t.Fatal("gRPC hostname verification bypassed", r)
		}
		return
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			t.Error("HTTP/2 not negotiated")
		}
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Grpc-Status", "0")
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	defer srv.Close()
	target := httpTarget(t, srv.URL)
	target.Inbounds[0].Network = "grpc"
	target.Inbounds[0].Security = "tls"
	name := "127.0.0.1"
	if len(srv.Certificate().DNSNames) > 0 {
		name = srv.Certificate().DNSNames[0]
	}
	target.Inbounds[0].HTTP.Host = net.JoinHostPort(name, strings.Split(srv.URL, ":")[2])
	r := Run(context.Background(), target, time.Second)
	if r.Success || r.Verified != "grpc" {
		t.Fatal("untrusted TLS accepted", r)
	}
	root := filepath.Join(t.TempDir(), "root.crt")
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
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestGRPCProbeTLSCertificateSNIAndDeadline$")
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+root, "SSL_CERT_DIR="+t.TempDir(), "SUBGATE_GRPC_TLS_CHILD_TARGET="+string(raw))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("trusted gRPC check failed %v\n%s", err, out)
	}
	// A TLS endpoint lacking h2 is a failure rather than an HTTP1 fallback.
	http1 := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("gRPC fell back to HTTP1") }))
	http1.TLS = &tls.Config{NextProtos: []string{"http/1.1"}}
	http1.StartTLS()
	defer http1.Close()
	target = httpTarget(t, http1.URL)
	target.Inbounds[0].Network = "grpc"
	target.Inbounds[0].Security = "tls"
	r = Run(context.Background(), target, time.Second)
	if r.Success || r.Verified != "grpc" {
		t.Fatal("HTTP1 endpoint accepted", r)
	}
	stall := httptest.NewServer(h2c.NewHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }), &http2.Server{}))
	defer stall.Close()
	target = httpTarget(t, stall.URL)
	target.Inbounds[0].Network = "grpc"
	started := time.Now()
	r = Run(context.Background(), target, 30*time.Millisecond)
	if r.Success || r.Verified != "grpc" || time.Since(started) > time.Second {
		t.Fatal("gRPC deadline ignored", r)
	}
}
