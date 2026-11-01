package server

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/subgate/subgate/core/internal/panel"
)

func TestProbeRequiresTLSHandshakeAndMarksUDPUnverified(t *testing.T) {
	endpoint := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer endpoint.Close()
	host, rawPort, _ := net.SplitHostPort(endpoint.Listener.Addr().String())
	port, _ := strconv.Atoi(rawPort)
	tcp := probeInbound(context.Background(), "node", host, panel.Inbound{Port: port, Network: "tcp"})
	if !tcp.Success || tcp.Verified != "tcp" {
		t.Fatalf("TCP: %+v", tcp)
	}
	tls := probeInbound(context.Background(), "node", host, panel.Inbound{Port: port, Network: "tcp", Security: "tls"})
	if tls.Success || tls.Verified != "tls" || tls.Error == "" {
		t.Fatalf("untrusted TLS certificate incorrectly accepted: %+v", tls)
	}
	udp := probeInbound(context.Background(), "node", host, panel.Inbound{Port: port, Network: "quic"})
	if udp.Success || udp.Verified != "unverified" {
		t.Fatalf("UDP: %+v", udp)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	probe := probeInbound(ctx, "node", host, panel.Inbound{Port: port, Network: "tcp"})
	if probe.Success {
		t.Fatal("canceled probe succeeded")
	}
}
