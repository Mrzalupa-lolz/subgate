package healthprobe

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
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

func validHTTP(p *wire.HealthHTTP) bool {
	return validHTTPFor(p, false)
}

func validHTTPFor(p *wire.HealthHTTP, grpc bool) bool {
	if p == nil || p.Path == "" || len(p.Path) > 4096 || !strings.HasPrefix(p.Path, "/") || (!grpc && strings.HasPrefix(p.Path, "//")) {
		return false
	}
	u, err := url.Parse(p.Path)
	// Prefix an origin for gRPC's valid empty-service paths such as //Tun;
	// the request origin and dial destination are fixed independently below.
	if grpc {
		u, err = url.Parse("http://probe.invalid" + p.Path)
	}
	if err != nil || (!grpc && (u.IsAbs() || u.Host != "")) || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	for _, v := range []string{p.Path, p.Host, p.ServerName} {
		for _, c := range v {
			if c < 33 || c == 127 {
				return false
			}
		}
	}
	if len(p.Host) > 253 || len(p.ServerName) > 253 || strings.ContainsAny(p.Host, "/\\@?#") || strings.ContainsAny(p.ServerName, "/\\@?#:") {
		return false
	}
	if _, ok := httpHostname(p.Host); !ok {
		return false
	}
	return true
}

func httpHostname(host string) (string, bool) {
	if host == "" {
		return "", true
	}
	if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		ip := strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
		return ip, net.ParseIP(ip) != nil
	}
	if strings.Contains(host, ":") {
		name, p, err := net.SplitHostPort(host)
		port, e := strconv.Atoi(p)
		return name, err == nil && e == nil && name != "" && port > 0 && port <= 65535
	}
	return host, !strings.ContainsAny(host, "[]")
}

// upgradeProbe connects only to the synchronized node address. HTTP Host and
// SNI never change the destination, and no proxy, redirect, body or credentials
// are used. WebSocket additionally requires the RFC6455 upgrade challenge;
// Xray HTTPUpgrade uses only101/Upgrade/Connection, as in pinned26.7.28.
func upgradeProbe(ctx context.Context, target wire.HealthTarget, in wire.HealthInbound, websocket bool) error {
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
		stream = tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: name})
		if err = stream.(*tls.Conn).HandshakeContext(ctx); err != nil {
			return err
		}
	}
	request, err := http.NewRequestWithContext(ctx, "GET", "http://probe.invalid"+in.HTTP.Path, nil)
	if err != nil {
		return err
	}
	request.Host = in.HTTP.Host
	if request.Host == "" {
		request.Host = net.JoinHostPort(target.Address, strconv.Itoa(in.Port))
	}
	key := ""
	request.Header.Set("Upgrade", "websocket")
	request.Header.Set("Connection", "Upgrade")
	if websocket {
		var nonce [16]byte
		if _, err = rand.Read(nonce[:]); err != nil {
			return err
		}
		key = base64.StdEncoding.EncodeToString(nonce[:])
		request.Header.Set("Sec-WebSocket-Key", key)
		request.Header.Set("Sec-WebSocket-Version", "13")
	}
	if err = request.Write(stream); err != nil {
		return err
	}
	response, err := http.ReadResponse(bufio.NewReader(io.LimitReader(stream, 16*1024)), request)
	if err != nil {
		return err
	}
	digest := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	if response.StatusCode != 101 || !strings.EqualFold(response.Header.Get("Upgrade"), "websocket") || !headerToken(response.Header.Get("Connection"), "upgrade") || (websocket && response.Header.Get("Sec-WebSocket-Accept") != base64.StdEncoding.EncodeToString(digest[:])) {
		return errors.New("HTTP upgrade handshake rejected")
	}
	return nil
}

func headerToken(value, token string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}
