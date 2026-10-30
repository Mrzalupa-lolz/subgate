package healthprobe

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/subgate/subgate/pkg/wire"
)

var errIncompleteQUIC = errors.New("QUIC address coverage incomplete")

// Xray26.7.28 XHTTP uses QUIC/HTTP3 when TLS ALPN is exactly [h3].
// A successful OPTIONS verifies the route as well as the QUIC TLS handshake,
// without creating a proxy session or sending any client credentials.
func xhttpH3Probe(parent context.Context, target wire.HealthTarget, in wire.HealthInbound) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	name := in.HTTP.ServerName
	if name == "" {
		name, _ = httpHostname(in.HTTP.Host)
	}
	if name == "" {
		name = target.Address
	}
	// Resolve with the probe context before opening a socket. DialAddr uses
	// contextless DNS in quic-go v0.61.0 and leaks its socket on resolve error.
	addresses, err := net.DefaultResolver.LookupIPAddr(ctx, target.Address)
	if err != nil {
		return err
	}
	if len(addresses) == 0 {
		return errors.New("node address has no IP")
	}
	var lastErr error
	limit := min(len(addresses), 8)
	for i, address := range addresses[:limit] {
		if ctx.Err() != nil {
			return errIncompleteQUIC
		}
		// Reserve time for every resolved candidate instead of letting an
		// unreachable first IPv6/A record hide a reachable alternative.
		deadline, _ := ctx.Deadline()
		attempt, stop := context.WithTimeout(ctx, time.Until(deadline)/time.Duration(limit-i))
		lastErr = xhttpH3AddressProbe(attempt, target, in, name, address)
		stop()
		if lastErr == nil {
			return nil
		}
	}
	if len(addresses) > limit {
		return errIncompleteQUIC
	}
	return lastErr
}

func xhttpH3AddressProbe(ctx context.Context, target wire.HealthTarget, in wire.HealthInbound, name string, address net.IPAddr) error {
	endpoint := &net.UDPAddr{IP: address.IP, Zone: address.Zone, Port: in.Port}
	network := "udp6"
	if endpoint.IP.To4() != nil {
		network = "udp4"
	}
	packetConn, err := net.ListenUDP(network, nil)
	if err != nil {
		return err
	}
	defer packetConn.Close()
	transport := &http3.Transport{
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS13, ServerName: name},
		MaxResponseHeaderBytes: 16 * 1024,
		DisableCompression:     true,
		QUICConfig: &quic.Config{
			HandshakeIdleTimeout:           time.Second,
			MaxIdleTimeout:                 3 * time.Second,
			MaxIncomingStreams:             -1,
			MaxIncomingUniStreams:          3, // HTTP3 control and QPACK streams.
			InitialStreamReceiveWindow:     16 * 1024,
			MaxStreamReceiveWindow:         64 * 1024,
			InitialConnectionReceiveWindow: 64 * 1024,
			MaxConnectionReceiveWindow:     256 * 1024,
		},
		// Host/URL/SNI never select a destination. Dial only the panel endpoint.
		Dial: func(ctx context.Context, _ string, tlsConfig *tls.Config, config *quic.Config) (*quic.Conn, error) {
			return quic.Dial(ctx, packetConn, endpoint, tlsConfig, config)
		},
	}
	defer transport.Close()
	request, err := xhttpRequest(ctx, target, in, "https")
	if err != nil {
		return err
	}
	// RoundTrip doesn't follow redirects or read proxy environment variables.
	response, err := transport.RoundTrip(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	return xhttpAccepted(response)
}
