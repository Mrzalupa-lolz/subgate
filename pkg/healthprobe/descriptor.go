package healthprobe

import (
	"strings"

	"github.com/subgate/subgate/pkg/wire"
)

// Registry names/aliases come from Xray v26.7.28:
// https://github.com/XTLS/Xray-core/blob/v26.7.28/infra/conf/xray.go
// https://github.com/XTLS/Xray-core/blob/v26.7.28/infra/conf/transport_internet.go
// This describes reachability only, not proxy authentication or user counters.
var protocols = map[string]string{
	"tunnel": "tcp", "dokodemo-door": "tcp", "http": "tcp",
	"shadowsocks": "tcp", "mixed": "tcp", "socks": "tcp",
	"vless": "tcp", "vmess": "tcp", "trojan": "tcp",
	"wireguard": "unverified", "hysteria": "unverified", "tun": "unverified",
}

var transports = map[string]string{
	"": "tcp", "raw": "tcp", "tcp": "tcp", "xhttp": "tcp",
	"splithttp": "tcp", "grpc": "tcp", "ws": "tcp",
	"websocket": "tcp", "httpupgrade": "tcp",
	"kcp": "unverified", "mkcp": "unverified", "hysteria": "unverified",
	// Generic UDP and removed legacy modes remain explicitly unverified.
	"udp": "unverified", "quic": "unverified", "http": "unverified",
	"h2": "unverified", "h3": "unverified",
}

type Descriptor struct {
	Generic bool
	Probe   string
	Reason  string
}

// Describe has an opaque default: unknown protocols use the explicit transport
// and security, while unknown transport/security require a configured probe.
func Describe(in wire.HealthInbound) Descriptor {
	protocol, known := protocols[strings.ToLower(in.Protocol)]
	d := Descriptor{Generic: !known, Probe: "unverified"}
	if in.HTTP != nil && in.HTTP.HTTP3 && (!strings.EqualFold(in.Security, "tls") || !(strings.EqualFold(in.Network, "xhttp") || strings.EqualFold(in.Network, "splithttp"))) {
		d.Reason = "HTTP3 requires XHTTP over TLS"
		return d
	}
	if !known && in.Network == "" {
		d.Reason = "unknown protocol requires an explicit transport"
		return d
	}
	transport, transportKnown := transports[strings.ToLower(in.Network)]
	if !transportKnown {
		d.Reason = "unknown transport; reachability is unverified"
		return d
	}
	if transport == "unverified" || known && protocol == "unverified" {
		d.Reason = "a transport or credentialed probe is required"
		return d
	}
	switch strings.ToLower(in.Security) {
	case "", "none", "reality":
		d.Probe = "tcp"
	case "tls":
		d.Probe = "tls"
	default:
		d.Reason = "unknown security; reachability is unverified"
	}
	if in.HTTP != nil && (strings.EqualFold(in.Network, "ws") || strings.EqualFold(in.Network, "websocket") || strings.EqualFold(in.Network, "httpupgrade")) {
		if d.Probe == "tcp" && strings.EqualFold(in.Security, "reality") {
			d.Probe = "unverified"
			d.Reason = "HTTP upgrade with REALITY is unsupported by Xray26.7.28"
		} else if d.Probe != "unverified" {
			d.Probe = "ws"
			if strings.EqualFold(in.Network, "httpupgrade") {
				d.Probe = "httpupgrade"
			}
		}
	}
	if in.HTTP != nil && (strings.EqualFold(in.Network, "grpc") || strings.EqualFold(in.Network, "xhttp") || strings.EqualFold(in.Network, "splithttp")) && d.Probe != "unverified" {
		d.Probe = "xhttp-options"
		if strings.EqualFold(in.Network, "grpc") {
			d.Probe = "grpc"
		} else if in.HTTP.HTTP3 {
			d.Probe = "xhttp-h3-options"
		}
		if strings.EqualFold(in.Security, "reality") {
			d.Probe = "unverified"
			d.Reason = "a REALITY probe is required"
		}
	}
	return d
}
