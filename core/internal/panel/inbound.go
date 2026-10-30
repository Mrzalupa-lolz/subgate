package panel

import (
	"encoding/json"
	"net/url"
	"strings"

	"github.com/subgate/subgate/pkg/wire"
)

// UnmarshalJSON extracts a small transport descriptor and discards rawInbound.
// No client settings, keys, certificates, or arbitrary headers are retained.
func (in *Inbound) UnmarshalJSON(data []byte) error {
	type plain Inbound
	var value struct {
		plain
		Raw json.RawMessage `json:"rawInbound"`
	}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*in = Inbound(value.plain)
	kind := httpTransport(in.Network)
	if len(value.Raw) == 0 || string(value.Raw) == "null" || kind == "" {
		return nil
	}
	// Raw config is present: malformed/mismatched settings must not silently
	// downgrade a configured HTTP transport probe to a TCP-only success.
	in.HTTP = &wire.HealthHTTP{}
	var raw struct {
		Stream struct {
			Network   string                `json:"network"`
			Method    string                `json:"method"`
			Security  string                `json:"security"`
			WS        httpTransportSettings `json:"wsSettings"`
			Upgrade   httpTransportSettings `json:"httpupgradeSettings"`
			XHTTP     *xhttpProbeSettings   `json:"xhttpSettings"`
			SplitHTTP *xhttpProbeSettings   `json:"splithttpSettings"`
			FinalMask struct {
				UDP []json.RawMessage `json:"udp"`
			} `json:"finalmask"`
			GRPC struct {
				Authority   string `json:"authority"`
				ServiceName string `json:"serviceName"`
				MultiMode   bool   `json:"multiMode"`
			} `json:"grpcSettings"`
			TLS struct {
				ServerName string   `json:"serverName"`
				ALPN       []string `json:"alpn"`
			} `json:"tlsSettings"`
		} `json:"streamSettings"`
	}
	if json.Unmarshal(value.Raw, &raw) != nil {
		return nil
	}
	stream := raw.Stream
	network := stream.Network
	if stream.Method != "" {
		network = stream.Method
	}
	if httpTransport(network) != kind || normalizeSecurity(stream.Security) != normalizeSecurity(in.Security) {
		return nil
	}
	settings := stream.WS
	if kind == "xhttp" {
		// Pinned Xray listens only on UDP for TLS with exactly h3 ALPN.
		h3 := normalizeSecurity(stream.Security) == "tls" && len(stream.TLS.ALPN) == 1 && stream.TLS.ALPN[0] == "h3"
		// Unsupported packet masks require credentials or a specialized probe.
		if h3 && len(stream.FinalMask.UDP) > 0 {
			return nil
		}
		settings := stream.SplitHTTP
		if stream.XHTTP != nil {
			settings = stream.XHTTP
		}
		if settings == nil {
			settings = &xhttpProbeSettings{}
		}
		path, host, ok := xhttpProbePath(*settings)
		if ok {
			in.HTTP = &wire.HealthHTTP{Path: path, Host: host, ServerName: stream.TLS.ServerName, HTTP3: h3}
		}
		return nil
	}
	if kind == "grpc" {
		in.HTTP = &wire.HealthHTTP{Path: grpcMethod(stream.GRPC.ServiceName, stream.GRPC.MultiMode), Host: stream.GRPC.Authority, ServerName: stream.TLS.ServerName}
		return nil
	}
	if kind == "httpupgrade" {
		settings = stream.Upgrade
		for k := range settings.Headers {
			if strings.EqualFold(k, "Host") {
				return nil
			} // Invalid Xray26.7.28 config.
		}
	}
	path := settings.Path
	if path == "" {
		path = "/"
	}
	u, err := url.Parse(path)
	if err != nil {
		return nil
	}
	q := u.Query()
	q.Del("ed") // Xray26.7.28 removes HTTP-upgrade early-data parameter.
	u.RawQuery = q.Encode()
	host := settings.Host
	if host == "" {
		for k, v := range settings.Headers {
			if strings.EqualFold(k, "Host") {
				host = v
			}
		}
	}
	in.HTTP = &wire.HealthHTTP{Path: u.String(), Host: host, ServerName: stream.TLS.ServerName}
	return nil
}

type httpTransportSettings struct {
	Path    string            `json:"path"`
	Host    string            `json:"host"`
	Headers map[string]string `json:"headers"`
}

func httpTransport(network string) string {
	switch strings.ToLower(network) {
	case "ws", "websocket":
		return "ws"
	case "httpupgrade":
		return "httpupgrade"
	case "grpc":
		return "grpc"
	case "xhttp", "splithttp":
		return "xhttp"
	default:
		return ""
	}
}

type xhttpProbeSettings struct {
	Path               string            `json:"path"`
	Host               string            `json:"host"`
	Mode               string            `json:"mode"`
	SessionIDPlacement string            `json:"sessionIDPlacement"`
	SeqPlacement       string            `json:"seqPlacement"`
	Headers            map[string]string `json:"headers"`
	Extra              json.RawMessage   `json:"extra"`
}

func xhttpProbePath(settings xhttpProbeSettings) (string, string, bool) {
	if len(settings.Extra) > 0 {
		var extra xhttpProbeSettings
		if json.Unmarshal(settings.Extra, &extra) != nil {
			return "", "", false
		}
		extra.Path, extra.Host, extra.Mode = settings.Path, settings.Host, settings.Mode
		settings = extra
	}
	switch settings.Mode {
	case "", "auto", "packet-up", "stream-up", "stream-one":
	default:
		return "", "", false
	}
	for k := range settings.Headers {
		if strings.EqualFold(k, "Host") {
			return "", "", false
		}
	}
	for _, place := range []string{settings.SessionIDPlacement, settings.SeqPlacement} {
		switch place {
		case "", "path", "cookie", "header", "query":
		default:
			return "", "", false
		}
	}
	path := strings.SplitN(settings.Path, "?", 2)[0]
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if (settings.SessionIDPlacement == "" || settings.SessionIDPlacement == "path" || settings.SeqPlacement == "" || settings.SeqPlacement == "path") && !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path, settings.Host, true
}

// Mirrors pinned Xray26.7.28 transport/internet/grpc/config.go: normal service
// names and custom /service/path/tun|multi names are escaped independently.
func grpcMethod(name string, multi bool) string {
	method := "Tun"
	if multi {
		method = "TunMulti"
	}
	if !strings.HasPrefix(name, "/") {
		return "/" + url.PathEscape(name) + "/" + method
	}
	last := strings.LastIndex(name, "/")
	if last < 1 {
		last = 1
	}
	parts := strings.Split(name[1:last], "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	methods := strings.Split(name[strings.LastIndex(name, "/")+1:], "|")
	method = methods[0]
	if multi && len(methods) > 1 {
		method = methods[1]
	}
	return "/" + strings.Join(parts, "/") + "/" + url.PathEscape(method)
}

func normalizeSecurity(security string) string {
	if security == "" {
		return "none"
	}
	return strings.ToLower(security)
}
