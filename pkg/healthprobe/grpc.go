package healthprobe

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/subgate/subgate/pkg/wire"
	"golang.org/x/net/http2"
	"google.golang.org/protobuf/encoding/protowire"
)

// An empty protobuf Hunk uses a five-byte gRPC envelope and no proxy payload,
// then closes the input stream. Only200 + application/grpc + final grpc-status0
// proves method acceptance.
func grpcProbe(parent context.Context, target wire.HealthTarget, in wire.HealthInbound) error {
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	address := net.JoinHostPort(target.Address, strconv.Itoa(in.Port))
	transport := &http2.Transport{AllowHTTP: true, MaxHeaderListSize: 16 * 1024, DisableCompression: true, DialTLSContext: func(dialCtx context.Context, _, _ string, _ *tls.Config) (net.Conn, error) {
		conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(dialCtx, "tcp", address)
		if err != nil {
			return nil, err
		}
		context.AfterFunc(ctx, func() { conn.Close() })
		if deadline, ok := ctx.Deadline(); ok {
			conn.SetDeadline(deadline)
		}
		if strings.EqualFold(in.Security, "tls") {
			name := in.HTTP.ServerName
			if name == "" {
				name, _ = httpHostname(in.HTTP.Host)
			}
			if name == "" {
				name = target.Address
			}
			secure := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: name, NextProtos: []string{"h2"}})
			if err := secure.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, err
			}
			if secure.ConnectionState().NegotiatedProtocol != "h2" {
				conn.Close()
				return nil, errors.New("HTTP/2 ALPN required")
			}
			return secure, nil
		}
		return conn, nil
	}}
	defer transport.CloseIdleConnections()
	scheme := "http"
	if strings.EqualFold(in.Security, "tls") {
		scheme = "https"
	}
	req, err := http.NewRequestWithContext(ctx, "POST", scheme+"://probe.invalid"+in.HTTP.Path, bytes.NewReader([]byte{0, 0, 0, 0, 0}))
	if err != nil {
		return err
	}
	req.Host = in.HTTP.Host
	if req.Host == "" {
		req.Host = address
	}
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("TE", "trailers")
	response, err := transport.RoundTrip(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4097))
	if err != nil || len(body) > 4096 || !validGRPCFrames(body) {
		return errors.New("gRPC response incomplete or oversized")
	}
	status := response.Trailer.Get("Grpc-Status")
	if status == "" {
		status = response.Header.Get("Grpc-Status")
	}
	ctype := response.Header.Get("Content-Type")
	if response.StatusCode != 200 || (ctype != "application/grpc" && !strings.HasPrefix(ctype, "application/grpc+")) || status != "0" {
		return errors.New("gRPC method rejected")
	}
	return nil
}

func validGRPCFrames(body []byte) bool {
	for len(body) > 0 {
		if len(body) < 5 || body[0] != 0 {
			return false
		}
		length := uint64(binary.BigEndian.Uint32(body[1:5]))
		if length > uint64(len(body)-5) {
			return false
		}
		message := body[5 : 5+int(length)]
		for len(message) > 0 {
			_, _, n := protowire.ConsumeField(message)
			if n < 0 {
				return false
			}
			message = message[n:]
		}
		body = body[5+int(length):]
	}
	return true
}
