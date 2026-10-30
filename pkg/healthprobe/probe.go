// Package healthprobe checks node reachability from Core and outbound Agents.
package healthprobe

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strconv"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

func Run(parent context.Context, target wire.HealthTarget, timeout time.Duration) wire.HealthResult {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	started := time.Now()
	best := wire.HealthResult{NodeID: target.NodeID, Timestamp: started, Verified: "unverified", Error: "no verifiable inbound endpoints"}
	endpoints := target.Inbounds[:min(len(target.Inbounds), 16)]
	var unverified wire.HealthResult
	incomplete := false
	for i, inbound := range endpoints {
		result := check(ctx, target, inbound)
		if result.Verified == "unverified" {
			unverified = result
			incomplete = true
			continue
		}
		if best.Verified == "unverified" || result.Success && (!best.Success || result.DurationMS < best.DurationMS) {
			best = result
		}
		if ctx.Err() != nil {
			if !best.Success && i+1 < len(endpoints) {
				best.Verified = "unverified"
				best.Error = "node deadline expired before endpoint coverage completed"
			}
			break
		}
	}
	if !best.Success && incomplete {
		best = unverified
	}
	if !best.Success && len(target.Inbounds) > len(endpoints) {
		best.Verified = "unverified"
		best.Error = "endpoint coverage exceeds the per-node limit"
	}
	best.Timestamp = started
	best.CompletedAt = time.Now()
	return best
}

func check(ctx context.Context, target wire.HealthTarget, in wire.HealthInbound) wire.HealthResult {
	d := Describe(in)
	r := wire.HealthResult{NodeID: target.NodeID, Timestamp: time.Now(), Verified: "unverified", Generic: d.Generic, Protocol: in.Protocol, Transport: in.Network, Security: in.Security, Port: in.Port}
	if in.Port < 1 || in.Port > 65535 || target.Address == "" || len(target.Address) > 253 {
		r.Error = "inbound endpoint unavailable"
		return r
	}
	if d.Probe == "unverified" {
		r.Error = d.Reason
		return r
	}
	if d.Probe == "ws" || d.Probe == "httpupgrade" || d.Probe == "xhttp-options" || d.Probe == "xhttp-h3-options" {
		valid := validHTTP(in.HTTP)
		if d.Probe == "xhttp-options" || d.Probe == "xhttp-h3-options" {
			valid = validXHTTP(in.HTTP)
		}
		if !valid {
			r.Error = "invalid HTTP transport probe settings"
			return r
		}
		start := time.Now()
		r.Verified = d.Probe
		if d.Probe == "xhttp-h3-options" {
			err := xhttpH3Probe(ctx, target, in)
			r.Success = err == nil
			if errors.Is(err, errIncompleteQUIC) {
				r.Verified = "unverified"
			}
		} else if d.Probe == "xhttp-options" {
			r.Success = xhttpOptionsProbe(ctx, target, in) == nil
		} else {
			r.Success = upgradeProbe(ctx, target, in, d.Probe == "ws") == nil
		}
		r.DurationMS = time.Since(start).Milliseconds()
		if !r.Success {
			r.Error = d.Probe + " probe failed"
		}
		return r
	}
	if d.Probe == "grpc" {
		if !validHTTPFor(in.HTTP, true) {
			r.Error = "invalid gRPC transport probe settings"
			return r
		}
		start := time.Now()
		r.Verified = "grpc"
		r.Success = grpcProbe(ctx, target, in) == nil
		r.DurationMS = time.Since(start).Milliseconds()
		if !r.Success {
			r.Error = "grpc probe failed"
		}
		return r
	}
	start := time.Now()
	dialer := net.Dialer{Timeout: 3 * time.Second}
	c, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(target.Address, strconv.Itoa(in.Port)))
	r.Verified = "tcp"
	if err == nil && d.Probe == "tls" {
		tc := tls.Client(c, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: target.Address})
		_ = tc.SetDeadline(time.Now().Add(3 * time.Second))
		err = tc.HandshakeContext(ctx)
		r.Verified = "tls"
		tc.Close()
	} else if c != nil {
		c.Close()
	}
	r.DurationMS = time.Since(start).Milliseconds()
	r.Success = err == nil
	if err != nil {
		r.Error = r.Verified + " probe failed"
	}
	return r
}
