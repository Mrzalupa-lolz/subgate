package healthprobe

import (
	"context"
	"crypto/x509"
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
)

// Opt-in: starts a separate loopback-only Xray with synthetic config. It never
// changes the running panel/node or uses any panel client credential.
func TestLiveWebSocketTransportProbe(t *testing.T) {
	testLiveUpgradeTransport(t, "ws", "wsSettings")
}

func TestLiveHTTPUpgradeTransportProbe(t *testing.T) {
	testLiveUpgradeTransport(t, "httpupgrade", "httpupgradeSettings")
}

func testLiveUpgradeTransport(t *testing.T, network, settings string) {
	testLiveHTTPTransport(t, network, settings, map[string]any{"path": "/synthetic-live", "host": "probe.example.test"}, "/synthetic-live", true)
}

func TestLiveGRPCTransportProbe(t *testing.T) {
	for _, tc := range []struct {
		name, service, path string
		multi               bool
	}{
		{"simple", "synthetic.Live", "/synthetic.Live/Tun", false},
		{"multi", "synthetic.Live", "/synthetic.Live/TunMulti", true},
		{"custom", "/synthetic/live/open|multi", "/synthetic/live/open", false},
		{"custom-multi", "/synthetic/live/open|multi", "/synthetic/live/multi", true},
		{"default", "", "//Tun", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			testLiveHTTPTransport(t, "grpc", "grpcSettings", map[string]any{"serviceName": tc.service, "multiMode": tc.multi}, tc.path, false)
		})
	}
}

func TestLiveXHTTPConfiguredPathProbe(t *testing.T) {
	for _, tc := range []struct{ network, mode string }{
		{"xhttp", "auto"}, {"xhttp", "packet-up"}, {"xhttp", "stream-up"}, {"xhttp", "stream-one"}, {"splithttp", "auto"},
	} {
		t.Run(tc.network+"-"+tc.mode, func(t *testing.T) {
			testLiveHTTPTransport(t, tc.network, "xhttpSettings", map[string]any{"path": "/synthetic-live", "host": "probe.example.test", "mode": tc.mode}, "/synthetic-live/", true)
		})
	}
	for _, path := range []string{"/literal%25", "/literal%", "/literal#fragment"} {
		t.Run(path, func(t *testing.T) {
			testLiveHTTPTransport(t, "xhttp", "xhttpSettings", map[string]any{"path": path, "host": "probe.example.test"}, path+"/", true)
		})
	}
}

func TestLiveXHTTPH3ConfiguredPathProbe(t *testing.T) {
	if os.Getenv("SUBGATE_SYNTHETIC_XRAY_BINARY") == "" {
		t.Skip("isolated Xray binary not configured")
	}
	if dir := os.Getenv("SUBGATE_SYNTHETIC_LIVE_H3_DIR"); dir != "" {
		tlsSettings := map[string]any{"alpn": []string{"h3"}, "certificates": []any{map[string]any{"certificateFile": filepath.Join(dir, "synthetic.crt"), "keyFile": filepath.Join(dir, "synthetic.key")}}}
		for _, mode := range []string{"auto", "packet-up", "stream-up", "stream-one"} {
			t.Run(mode, func(t *testing.T) {
				testLiveHTTPTransport(t, "xhttp", "xhttpSettings", map[string]any{"path": "/synthetic-live", "host": "probe.example.test", "mode": mode}, "/synthetic-live/", true, tlsSettings)
			})
		}
		t.Run("literal-percent", func(t *testing.T) {
			testLiveHTTPTransport(t, "splithttp", "xhttpSettings", map[string]any{"path": "/literal%25", "host": "probe.example.test"}, "/literal%25/", true, tlsSettings)
		})
		return
	}
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	dir := t.TempDir()
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	key, err := x509.MarshalPKCS8PrivateKey(srv.TLS.Certificates[0].PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"synthetic.crt": cert, "synthetic.key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "-test.run=^TestLiveXHTTPH3ConfiguredPathProbe$", "-test.v")
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE="+filepath.Join(dir, "synthetic.crt"), "SSL_CERT_DIR="+t.TempDir(), "SUBGATE_SYNTHETIC_LIVE_H3_DIR="+dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("actual Xray HTTP3 failed: %v\n%s", err, out)
	}
	t.Log(string(out))
}

func testLiveHTTPTransport(t *testing.T, network, settings string, values map[string]any, requestPath string, rejectHost bool, tlsSettings ...map[string]any) {
	t.Helper()
	binary := os.Getenv("SUBGATE_SYNTHETIC_XRAY_BINARY")
	if binary == "" {
		t.Skip("isolated Xray binary not configured")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("Xray binary must be absolute")
	}
	version, err := exec.Command(binary, "version").Output()
	if err != nil || !strings.HasPrefix(string(version), "Xray 26.7.28 ") {
		t.Fatal("test requires Xray26.7.28")
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	stream := map[string]any{"network": network, "security": "none", settings: values}
	if len(tlsSettings) > 0 {
		stream["security"], stream["tlsSettings"] = "tls", tlsSettings[0]
	}
	config := map[string]any{
		"log":       map[string]any{"loglevel": "none"},
		"inbounds":  []any{map[string]any{"listen": "127.0.0.1", "port": port, "protocol": "vless", "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": "00000000-0000-4000-8000-000000000001"}}}, "streamSettings": stream}},
		"outbounds": []any{map[string]any{"protocol": "blackhole"}},
	}
	raw, _ := json.Marshal(config)
	path := filepath.Join(t.TempDir(), "config.json")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "run", "-config", path)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		t.Fatal("cannot start isolated Xray")
	}
	t.Cleanup(func() { cmd.Process.Kill(); cmd.Wait() })
	target := wire.HealthTarget{NodeID: "live-http", Address: "127.0.0.1", Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: network, Port: port, HTTP: &wire.HealthHTTP{Path: requestPath, Host: "probe.example.test"}}}}
	verified := network
	if network == "xhttp" || network == "splithttp" {
		verified = "xhttp-options"
	}
	if len(tlsSettings) > 0 {
		target.Inbounds[0].Security = "tls"
		target.Inbounds[0].HTTP.HTTP3 = true
		target.Inbounds[0].HTTP.ServerName = "example.com"
		verified = "xhttp-h3-options"
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		probeTimeout := 200 * time.Millisecond
		if network == "grpc" {
			probeTimeout = time.Second
		}
		r := Run(context.Background(), target, probeTimeout)
		if r.Success && r.Verified == verified {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("real Xray upgrade handshake not verified", r)
		}
		time.Sleep(20 * time.Millisecond)
	}
	negative := []struct{ path, host string }{{"/wrong", "probe.example.test"}}
	if rejectHost {
		negative = append(negative, struct{ path, host string }{requestPath, "wrong.example.test"})
	} else {
		negative = append(negative, struct{ path, host string }{requestPath + "-missing", "probe.example.test"})
	}
	for _, tc := range negative {
		target.Inbounds[0].HTTP.Path, target.Inbounds[0].HTTP.Host = tc.path, tc.host
		r := Run(context.Background(), target, time.Second)
		if r.Success || r.Verified != verified {
			t.Fatal("real Xray rejection hidden", r)
		}
	}
	t.Logf("Xray26.7.28 %s: configured route accepted; rejection cases passed", network)
}
