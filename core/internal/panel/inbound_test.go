package panel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInboundSanitizesWebSocketProbeConfig(t *testing.T) {
	for _, tc := range []struct {
		name, stream    string
		path, host, sni string
	}{
		{"method-precedence", `{"network":"raw","method":"ws","wsSettings":{"path":"/socket?ed=2048","host":"probe.example.test","headers":{"Authorization":"synthetic-secret"}},"tlsSettings":{"serverName":"sni.example.test","certificates":[{"key":"synthetic-key"}]}}`, "/socket", "probe.example.test", "sni.example.test"},
		{"header-host-default-path", `{"network":"websocket","wsSettings":{"headers":{"HOST":"probe.example.test"}}}`, "/", "probe.example.test", ""},
		{"mismatch", `{"network":"raw","wsSettings":{"path":"/socket"}}`, "", "", ""},
		{"mismatched-security", `{"network":"ws","security":"reality","wsSettings":{"path":"/socket"}}`, "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var in Inbound
			data := `{"type":"vless","network":"ws","port":443,"rawInbound":{"settings":{"clients":[{"id":"synthetic-credential"}]},"streamSettings":` + tc.stream + `}}`
			if err := json.Unmarshal([]byte(data), &in); err != nil {
				t.Fatal(err)
			}
			if in.HTTP == nil || in.HTTP.Path != tc.path || in.HTTP.Host != tc.host || in.HTTP.ServerName != tc.sni {
				t.Fatal(in)
			}
			stored, _ := json.Marshal(in)
			for _, secret := range []string{"synthetic-secret", "synthetic-key", "synthetic-credential", "rawInbound", "Authorization"} {
				if strings.Contains(string(stored), secret) {
					t.Fatal("raw settings retained")
				}
			}
		})
	}
}

func TestInboundSanitizesXHTTPProbeConfig(t *testing.T) {
	for _, tc := range []struct{ name, settings, path, host string }{
		{"default", `{"path":"socket?opaque=keep","host":"probe.example.test"}`, "/socket/", "probe.example.test"},
		{"nonpath", `{"path":"/socket","sessionIDPlacement":"header","seqPlacement":"query"}`, "/socket", ""},
		{"extra", `{"path":"/outer","host":"outer.example.test","extra":{"path":"/ignored","host":"ignored.example.test","sessionIDPlacement":"header","seqPlacement":"query"}}`, "/outer", "outer.example.test"},
		{"bad-mode", `{"path":"/socket","mode":"unknown"}`, "", ""},
		{"bad-placement", `{"path":"/socket","sessionIDPlacement":"unknown"}`, "", ""},
		{"bad-header", `{"path":"/socket","headers":{"Host":"other.example.test"}}`, "", ""},
		{"bad-extra", `{"path":"/socket","extra":"unknown"}`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, network := range []string{"xhttp", "splithttp"} {
				data := `{"type":"vless","network":"` + network + `","rawInbound":{"settings":{"clients":[{"id":"synthetic-credential"}]},"streamSettings":{"method":"` + network + `","xhttpSettings":` + tc.settings + `}}}`
				var in Inbound
				if err := json.Unmarshal([]byte(data), &in); err != nil {
					t.Fatal(err)
				}
				if in.HTTP == nil || in.HTTP.Path != tc.path || in.HTTP.Host != tc.host {
					t.Fatal(in)
				}
				stored, _ := json.Marshal(in)
				if strings.Contains(string(stored), "synthetic-credential") {
					t.Fatal("raw credentials retained")
				}
			}
		})
	}
}

func TestInboundXHTTPALPNChoosesVerifiableListener(t *testing.T) {
	for _, tc := range []struct {
		alpn string
		h3   bool
	}{
		{`["h3"]`, true},
		{`["h3","h2","http/1.1"]`, false},
		{`[]`, false},
		{`null`, false},
	} {
		t.Run(tc.alpn, func(t *testing.T) {
			var in Inbound
			data := `{"type":"vless","network":"xhttp","security":"tls","rawInbound":{"streamSettings":{"network":"xhttp","security":"tls","tlsSettings":{"alpn":` + tc.alpn + `},"xhttpSettings":{"path":"/socket"}}}}`
			if err := json.Unmarshal([]byte(data), &in); err != nil {
				t.Fatal(err)
			}
			if in.HTTP == nil || in.HTTP.Path != "/socket/" || in.HTTP.HTTP3 != tc.h3 {
				t.Fatal("wrong listener descriptor", in)
			}
		})
	}
}

func TestInboundXHTTPMaskedUDPRemainsUnverified(t *testing.T) {
	var in Inbound
	data := `{"type":"vless","network":"xhttp","security":"tls","rawInbound":{"streamSettings":{"network":"xhttp","security":"tls","tlsSettings":{"alpn":["h3"]},"finalmask":{"udp":[{"type":"salamander","settings":{"password":"synthetic-mask-secret"}}]},"xhttpSettings":{"path":"/socket"}}}}`
	if err := json.Unmarshal([]byte(data), &in); err != nil {
		t.Fatal(err)
	}
	if in.HTTP == nil || in.HTTP.Path != "" {
		t.Fatal("masked UDP guessed as plain QUIC", in)
	}
	raw, _ := json.Marshal(in)
	if strings.Contains(string(raw), "synthetic-mask-secret") {
		t.Fatal("mask credential retained")
	}
}

func TestInboundSanitizesGRPCProbeConfig(t *testing.T) {
	for _, tc := range []struct {
		name, service, path string
		multi               bool
	}{
		{"simple", "synthetic.Service", "/synthetic.Service/Tun", false},
		{"multi", "synthetic.Service", "/synthetic.Service/TunMulti", true},
		{"custom", "/synthetic/service/open|multi", "/synthetic/service/open", false},
		{"custom-multi", "/synthetic/service/open|multi", "/synthetic/service/multi", true},
		{"default", "", "//Tun", false},
		{"escaped", "service space", "/service%20space/Tun", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(map[string]any{"type": "vless", "network": "grpc", "rawInbound": map[string]any{"settings": map[string]any{"clients": []any{map[string]any{"id": "synthetic-credential"}}}, "streamSettings": map[string]any{"method": "grpc", "network": "raw", "grpcSettings": map[string]any{"serviceName": tc.service, "multiMode": tc.multi, "authority": "probe.example.test"}}}})
			var in Inbound
			if err := json.Unmarshal(data, &in); err != nil {
				t.Fatal(err)
			}
			if in.HTTP == nil || in.HTTP.Path != tc.path || in.HTTP.Host != "probe.example.test" {
				t.Fatal(in)
			}
			stored, _ := json.Marshal(in)
			if strings.Contains(string(stored), "synthetic-credential") {
				t.Fatal("raw credentials retained")
			}
		})
	}
}

func TestInboundSanitizesHTTPUpgradeProbeConfig(t *testing.T) {
	for _, tc := range []struct{ name, stream, path, host string }{
		{"method", `{"method":"httpupgrade","network":"raw","httpupgradeSettings":{"path":"/upgrade?ed=1024","host":"probe.example.test","headers":{"Authorization":"synthetic-secret"}}}`, "/upgrade", "probe.example.test"},
		{"default-path", `{"network":"httpupgrade","httpupgradeSettings":{"host":"probe.example.test"}}`, "/", "probe.example.test"},
		{"invalid-host-header", `{"network":"httpupgrade","httpupgradeSettings":{"path":"/upgrade","headers":{"Host":"other.example.test"}}}`, "", ""},
		{"mismatch", `{"network":"ws","httpupgradeSettings":{"path":"/upgrade"}}`, "", ""},
		{"malformed", `"invalid-config"`, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var in Inbound
			data := `{"type":"vless","network":"httpupgrade","port":443,"rawInbound":{"settings":{"clients":[{"id":"synthetic-credential"}]},"streamSettings":` + tc.stream + `}}`
			if tc.name == "malformed" {
				data = `{"type":"vless","network":"httpupgrade","port":443,"rawInbound":"invalid-config"}`
			}
			if err := json.Unmarshal([]byte(data), &in); err != nil {
				t.Fatal(err)
			}
			if in.HTTP == nil || in.HTTP.Path != tc.path || in.HTTP.Host != tc.host {
				t.Fatal(in)
			}
			stored, _ := json.Marshal(in)
			if strings.Contains(string(stored), "synthetic-secret") || strings.Contains(string(stored), "synthetic-credential") {
				t.Fatal("raw credentials retained")
			}
		})
	}
}
