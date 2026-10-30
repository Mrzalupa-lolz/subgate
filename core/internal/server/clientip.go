package server

import (
	"net/http"
	"net/netip"
	"strings"
)

func (s *Server) clientIP(r *http.Request) string {
	peer, err := netip.ParseAddr(remoteIP(r))
	if err != nil {
		return remoteIP(r)
	}
	peer = peer.Unmap()
	trusted := func(ip netip.Addr) bool {
		for _, raw := range s.config.HTTP.TrustedProxies {
			prefix, err := netip.ParsePrefix(raw)
			if err == nil && prefix.Contains(ip) {
				return true
			}
		}
		return false
	}
	if !trusted(peer) {
		return peer.String()
	}
	values := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(values) > 32 {
		return peer.String()
	}
	for i := len(values) - 1; i >= 0; i-- {
		if !trusted(peer) {
			break
		}
		next, err := netip.ParseAddr(strings.TrimSpace(values[i]))
		if err != nil {
			return peer.String()
		}
		peer = next.Unmap()
	}
	return peer.String()
}
