package agent

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
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/subgate/subgate/pkg/wire"
)

func TestAgentReportsXHTTPConfiguredPathProbe(t *testing.T) {
	var accepted atomic.Int32
	xhttp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "OPTIONS" || r.URL.Path != "/agent-xhttp/" || r.Header.Get("Authorization") != "" {
			w.WriteHeader(404)
			return
		}
		accepted.Add(1)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
		w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
	}))
	defer xhttp.Close()
	host, p, _ := net.SplitHostPort(strings.TrimPrefix(xhttp.URL, "http://"))
	port, _ := strconv.Atoi(p)
	target := wire.HealthTarget{NodeID: "xhttp", Address: host, Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: "xhttp", Port: port, HTTP: &wire.HealthHTTP{Path: "/agent-xhttp/"}}}}
	var report wire.HealthReport
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-agent-token" {
			t.Error("missing Agent auth")
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
	if len(report.Results) != 1 || !report.Results[0].Success || report.Results[0].Verified != "xhttp-options" || accepted.Load() != 1 {
		t.Fatal("Agent preflight not observed", report)
	}
	target.Inbounds[0].HTTP.Path = "/wrong"
	if err := a.collectHealth(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(report.Results) != 1 || report.Results[0].Success || report.Results[0].Verified != "xhttp-options" {
		t.Fatal("Agent wrong XHTTP route accepted", report)
	}
}

func TestAgentReportsHTTP3Probe(t *testing.T) {
	if raw := os.Getenv("SUBGATE_SYNTHETIC_AGENT_H3_TARGET"); raw != "" {
		var target wire.HealthTarget
		if err := json.Unmarshal([]byte(raw), &target); err != nil {
			t.Fatal(err)
		}
		var report wire.HealthReport
		core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer synthetic-agent-token" {
				t.Error("missing Agent auth")
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
		for _, success := range []bool{true, false} {
			if !success {
				target.Inbounds[0].HTTP.Path = "/wrong/"
			}
			if err := a.collectHealth(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(report.Results) != 1 || report.Results[0].Success != success || report.Results[0].Verified != "xhttp-h3-options" {
				t.Fatal("Agent H3 report differs", report)
			}
		}
		return
	}
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
	var accepted atomic.Int32
	srv := &http3.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/agent-h3/" || r.Method != "OPTIONS" || r.ProtoMajor != 3 || r.Header.Get("Authorization") != "" {
			w.WriteHeader(404)
			return
		}
		accepted.Add(1)
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	})}
	defer srv.Close()
	go srv.ServeListener(listener)
	target := wire.HealthTarget{NodeID: "h3", Address: "127.0.0.1", Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: "xhttp", Security: "tls", Port: udp.LocalAddr().(*net.UDPAddr).Port, HTTP: &wire.HealthHTTP{HTTP3: true, Path: "/agent-h3/", ServerName: "example.com"}}}}
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
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestAgentReportsHTTP3Probe$")
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+root, "SSL_CERT_DIR="+t.TempDir(), "SUBGATE_SYNTHETIC_AGENT_H3_TARGET="+string(raw))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("isolated Agent H3 failed: %v\n%s", err, out)
	}
	if accepted.Load() != 1 {
		t.Fatal("Agent H3 request not observed", accepted.Load())
	}
}
