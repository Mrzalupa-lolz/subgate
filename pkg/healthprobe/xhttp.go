package healthprobe

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

// Pinned Xray26.7.28 checks Host/path before answering preflight, without
// creating an XHTTP session. This verifies HTTP route acceptance, not a tunnel.
func validXHTTP(p *wire.HealthHTTP) bool {
	if p == nil {
		return false
	}
	for _, c := range p.Path {
		if c < 33 || c == 127 {
			return false
		}
	}
	// Xray's normalized XHTTP path is literal, unlike WS/gRPC URI paths.
	copy := *p
	copy.Path = (&url.URL{Path: p.Path}).EscapedPath()
	return len(p.Path) <= 4096 && validHTTP(&copy)
}

func xhttpOptionsProbe(ctx context.Context, target wire.HealthTarget, in wire.HealthInbound) error {
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(target.Address, strconv.Itoa(in.Port)))
	if err != nil {
		return err
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	deadline := time.Now().Add(3 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err = conn.SetDeadline(deadline); err != nil {
		return err
	}
	var stream net.Conn = conn
	if strings.EqualFold(in.Security, "tls") {
		name := in.HTTP.ServerName
		if name == "" {
			name, _ = httpHostname(in.HTTP.Host)
		}
		if name == "" {
			name = target.Address
		}
		secure := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: name})
		if err = secure.HandshakeContext(ctx); err != nil {
			return err
		}
		stream = secure
	}
	req, err := xhttpRequest(ctx, target, in, "http")
	if err != nil {
		return err
	}
	if err = req.Write(stream); err != nil {
		return err
	}
	response, err := http.ReadResponse(bufio.NewReader(io.LimitReader(stream, 16*1024)), req)
	if err != nil {
		return err
	}
	return xhttpAccepted(response)
}

func xhttpRequest(ctx context.Context, target wire.HealthTarget, in wire.HealthInbound, scheme string) (*http.Request, error) {
	requestURL := &url.URL{Scheme: scheme, Host: "probe.invalid", Path: in.HTTP.Path}
	req, err := http.NewRequestWithContext(ctx, "OPTIONS", requestURL.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Host = in.HTTP.Host
	if req.Host == "" {
		req.Host = net.JoinHostPort(target.Address, strconv.Itoa(in.Port))
	}
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type")
	return req, nil
}

func xhttpAccepted(response *http.Response) error {
	if response.StatusCode != 200 || response.Header.Get("Access-Control-Allow-Origin") != "*" || response.Header.Get("Access-Control-Allow-Methods") != "POST" || !strings.EqualFold(response.Header.Get("Access-Control-Allow-Headers"), "Content-Type") {
		return errors.New("XHTTP preflight rejected")
	}
	return nil
}
