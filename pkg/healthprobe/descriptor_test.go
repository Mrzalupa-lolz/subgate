package healthprobe

import (
	"context"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

func TestUnknownTransportIsNotTCP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	host, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	for _, network := range []string{"udp", "future-network", "mkcp", "kcp", "hysteria", "quic"} {
		r := Run(context.Background(), wire.HealthTarget{NodeID: "node", Address: host, Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: network, Port: port}}}, time.Second)
		if r.Success || r.Verified != "unverified" {
			t.Fatal("unverifiable transport treated as TCP", network, r)
		}
	}
}

func TestUnknownEndpointPreventsFalseNodeFailure(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, p, _ := net.SplitHostPort(l.Addr().String())
	port, _ := strconv.Atoi(p)
	l.Close()
	r := Run(context.Background(), wire.HealthTarget{NodeID: "node", Address: host, Inbounds: []wire.HealthInbound{{Protocol: "vless", Network: "raw", Port: port}, {Protocol: "future-protocol", Network: "future-network", Port: port}}}, time.Second)
	if r.Success || r.Verified != "unverified" {
		t.Fatal("unknown endpoint counted as node failure", r)
	}
}

func TestDescriptorCapabilities(t *testing.T) {
	for _, tc := range []struct {
		protocol, network, security, probe string
		generic                            bool
	}{
		{"vless", "raw", "none", "tcp", false},
		{"vmess", "TCP", "TLS", "tls", false},
		{"trojan", "ws", "tls", "tls", false},
		{"shadowsocks", "websocket", "", "tcp", false},
		{"socks", "grpc", "tls", "tls", false},
		{"mixed", "httpupgrade", "", "tcp", false},
		{"http", "xhttp", "reality", "tcp", false},
		{"dokodemo-door", "splithttp", "none", "tcp", false},
		{"tunnel", "", "", "tcp", false},
		{"wireguard", "raw", "", "unverified", false},
		{"hysteria", "raw", "", "unverified", false},
		{"tun", "", "", "unverified", false},
		{"future-protocol", "raw", "tls", "tls", true},
		{"future-protocol", "", "", "unverified", true},
		{"future-protocol", "future-transport", "", "unverified", true},
		{"future-protocol", "raw", "future-security", "unverified", true},
	} {
		d := Describe(wire.HealthInbound{Protocol: tc.protocol, Network: tc.network, Security: tc.security})
		if d.Probe != tc.probe || d.Generic != tc.generic || d.Probe == "unverified" && d.Reason == "" {
			t.Fatal(tc, d)
		}
	}
}
