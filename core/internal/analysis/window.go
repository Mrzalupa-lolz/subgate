// Package analysis aggregates Xray metadata into bounded, explainable observations.
// A threshold crossing is never a verdict or an authorization to shape traffic.
package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/netip"
	"strings"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

const MaxEvents = 8192

type Policy struct {
	Connections         int      `json:"connections_per_minute"`
	Ports               int      `json:"ports_per_host_per_minute"`
	Hosts               int      `json:"ips_per_minute"`
	ShaperMbps          int      `json:"shaper_mbps"`
	ShaperMinutes       int      `json:"shaper_minutes"`
	AutomaticShaper     bool     `json:"automatic_shaper"`
	Allowlist           []string `json:"allowlist"`
	RestrictedProtocols []string `json:"restricted_protocols"`
	OverloadPercent     int      `json:"overload_percent"`
	RecoveryPercent     int      `json:"recovery_percent"`
	SustainedMinutes    int      `json:"sustained_minutes"`
}

func DefaultPolicy() Policy {
	return Policy{Connections: 3000, Ports: 128, Hosts: 512, OverloadPercent: 85, RecoveryPercent: 70, SustainedMinutes: 3}
}

func (p Policy) Valid() bool {
	for _, values := range []struct {
		list []string
		max  int
	}{{p.Allowlist, 256}, {p.RestrictedProtocols, 128}} {
		for _, value := range values.list {
			if value == "" || len(value) > values.max || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00") {
				return false
			}
		}
	}
	return p.Connections >= 100 && p.Connections <= 8000 && p.Ports >= 16 && p.Ports <= 1024 && p.Hosts >= 32 && p.Hosts <= 4096 && p.ShaperMbps >= 0 && p.ShaperMbps <= 100000 && p.ShaperMinutes >= 0 && p.ShaperMinutes <= 1440 &&
		(p.OverloadPercent == 0 || p.OverloadPercent >= 50 && p.OverloadPercent <= 100) && (p.RecoveryPercent == 0 || p.RecoveryPercent >= 10 && p.RecoveryPercent < p.OverloadPercent) && (p.SustainedMinutes == 0 || p.SustainedMinutes >= 2 && p.SustainedMinutes <= 60) && len(p.Allowlist) <= 10000 && len(p.RestrictedProtocols) <= 32 && (!p.AutomaticShaper || p.ShaperMbps > 0 && p.ShaperMinutes > 0 && len(p.RestrictedProtocols) > 0)
}

func (p Policy) Normalized() Policy {
	if p.OverloadPercent == 0 {
		p.OverloadPercent = 85
	}
	if p.RecoveryPercent == 0 {
		p.RecoveryPercent = 70
	}
	if p.SustainedMinutes == 0 {
		p.SustainedMinutes = 3
	}
	return p
}

// Window is persisted encrypted, once per touched user/node/minute in a batch.
// Counters describe distinct log records, not concurrent sockets or wire packets.
type Window struct {
	Count     int                     `json:"count"`
	IPs       map[string]bool         `json:"ips"`
	Ports     map[string]map[int]bool `json:"ports"`
	First     time.Time               `json:"first"`
	Last      time.Time               `json:"last"`
	Rejected  int                     `json:"rejected"`
	Truncated bool                    `json:"truncated"`
}

type Signal struct {
	Kind      string    `json:"kind"`
	Value     int       `json:"value"`
	Threshold int       `json:"threshold"`
	Target    string    `json:"target,omitempty"`
	First     time.Time `json:"first_seen"`
	Last      time.Time `json:"last_seen"`
	Truncated bool      `json:"truncated"`
}

// Fingerprint covers the timestamp and tuple; the caller binds it to the
// account/node/window and persists it in the same transaction as the aggregate.
func Fingerprint(e wire.Activity) string {
	e.Timestamp = e.Timestamp.UTC()
	raw, _ := json.Marshal(e)
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:16])
}

// Add accepts an already deduplicated event. Fingerprints live in the indexed
// ingestion ledger, avoiding full-set JSON decoding for every small batch.
func (w *Window) Add(e wire.Activity) {
	if w.IPs == nil {
		w.IPs = map[string]bool{}
		w.Ports = map[string]map[int]bool{}
	}
	if w.Count >= MaxEvents {
		w.Truncated = true
		return
	}
	w.Count++
	if w.First.IsZero() || e.Timestamp.Before(w.First) {
		w.First = e.Timestamp
	}
	if e.Timestamp.After(w.Last) {
		w.Last = e.Timestamp
	}
	if e.Result == "rejected" || e.Result == "failed" {
		w.Rejected++
	}
	host := strings.ToLower(strings.TrimSuffix(strings.Trim(e.DstHost, "[]"), "."))
	if ip, err := netip.ParseAddr(host); err == nil {
		w.IPs[ip.Unmap().String()] = true
	}
	if host == "" || e.DstPort < 1 {
		return
	}
	// Track up to 128 destination hosts; exhaustion is explicit, never a verdict.
	ports := w.Ports[host]
	if ports == nil {
		if len(w.Ports) >= 128 {
			w.Truncated = true
			return
		}
		ports = map[int]bool{}
		w.Ports[host] = ports
	}
	ports[e.DstPort] = true
}

func (w *Window) Signals(p Policy) []Signal {
	result := []Signal{}
	add := func(kind string, value, threshold int, target string) {
		if value >= threshold {
			result = append(result, Signal{kind, value, threshold, target, w.First, w.Last, w.Truncated})
		}
	}
	add("connection_rate", w.Count, p.Connections, "")
	add("destination_breadth", len(w.IPs), p.Hosts, "")
	// One bounded observation per rule/window, with the largest fanout as evidence.
	maxPorts, target := 0, ""
	for host, ports := range w.Ports {
		if len(ports) > maxPorts || len(ports) == maxPorts && host < target {
			maxPorts, target = len(ports), host
		}
	}
	add("port_breadth", maxPorts, p.Ports, target)
	return result
}
