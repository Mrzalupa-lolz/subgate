package server

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/subgate/subgate/core/internal/health"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/healthprobe"
	"github.com/subgate/subgate/pkg/wire"
)

type nodeProbe struct {
	ID          string              `json:"id"`
	NodeID      string              `json:"node_id"`
	Protocol    string              `json:"protocol"`
	Transport   string              `json:"transport"`
	Security    string              `json:"security"`
	Port        int                 `json:"port"`
	Success     bool                `json:"success"`
	Verified    string              `json:"verified"`
	Generic     bool                `json:"generic"`
	Duration    int64               `json:"duration_ms"`
	Error       string              `json:"error,omitempty"`
	Timestamp   time.Time           `json:"timestamp"`
	CompletedAt time.Time           `json:"completed_at,omitempty"`
	Status      health.HealthStatus `json:"status,omitempty"`
	QuorumMet   bool                `json:"quorum_met"`
	Vantages    int                 `json:"vantages"`
	Maintenance bool                `json:"maintenance"`
}

func (s *Server) probeNode(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var n panel.Node
	if s.object("nodes", id, &n) != nil {
		fail(w, 404, "node not found")
		return
	}
	if s.inMaintenance(id, time.Now()) {
		fail(w, 409, "node maintenance window is active")
		return
	}
	if err := s.auditAction(r, "node.probe", "nodes", id); err != nil {
		fail(w, 500, "audit unavailable")
		return
	}
	count := min(len(n.Inbounds), 16)
	if count == 0 {
		fail(w, 409, "no inbound endpoints are configured; sync the panel first")
		return
	}
	results := make([]nodeProbe, count)
	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	var workers sync.WaitGroup
	slots := make(chan struct{}, 4)
	for i := range results {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
			}
			results[i] = probeInbound(ctx, id, n.IPAddress, n.Inbounds[i])
		}(i)
	}
	workers.Wait()
	// Report only the reached verification level: TCP/TLS or configured HTTP
	// upgrade. Proxy authentication and REALITY acceptance are not implied.
	best := results[0]
	var unverified *nodeProbe
	for _, result := range results {
		if result.Verified == "unverified" {
			copy := result
			unverified = &copy
		}
		if result.Success && (!best.Success || result.Duration < best.Duration) {
			best = result
		}
	}
	if !best.Success && unverified != nil {
		best = *unverified
	}
	if !best.Success && len(n.Inbounds) > count {
		best.Verified = "unverified"
		best.Error = "endpoint coverage exceeds the per-node limit"
	}
	best.Timestamp = started
	best.CompletedAt = time.Now()
	if err := s.saveObject("health", id, best); err != nil {
		fail(w, 500, "probe persistence failed")
		return
	}
	writeJSON(w, 200, map[string]any{"data": results})
}

func probeInbound(ctx context.Context, id, host string, inbound panel.Inbound) nodeProbe {
	r := healthprobe.Run(ctx, wire.HealthTarget{NodeID: id, Address: host, Inbounds: []wire.HealthInbound{{Protocol: inbound.Type, Network: inbound.Network, Security: inbound.Security, Port: inbound.Port, HTTP: inbound.HTTP}}}, 3*time.Second)
	return nodeProbe{ID: id, NodeID: id, Protocol: r.Protocol, Transport: r.Transport, Security: r.Security, Port: r.Port, Timestamp: r.Timestamp, CompletedAt: r.CompletedAt, Verified: r.Verified, Generic: r.Generic, Duration: r.DurationMS, Success: r.Success, Error: r.Error}
}
