// Package health implements the health engine with quarantine logic
package health

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HealthStatus represents the health status of a node
type HealthStatus string

const (
	StatusHealthy     HealthStatus = "healthy"
	StatusSuspected   HealthStatus = "suspected"
	StatusQuarantined HealthStatus = "quarantined"
	StatusRecovering  HealthStatus = "recovering"
	StatusUnknown     HealthStatus = "unknown"
)

// ProbeType represents the type of health probe
type ProbeType string

const (
	ProbeTCP  ProbeType = "tcp"
	ProbeTLS  ProbeType = "tls"
	ProbeQUIC ProbeType = "quic"
	ProbeHTTP ProbeType = "http"
)

// NodeHealth tracks health state for a node
type NodeHealth struct {
	NodeID               string
	Status               HealthStatus
	LastHealthCheck      time.Time
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
	NextProbeAt          time.Time
	LastSuccessTime      time.Time
	QuarantinedUntil     time.Time
	ManualOverride       bool
}

// ProbeResult represents the result of a health probe
type ProbeResult struct {
	NodeID       string
	ProbeType    ProbeType
	Success      bool
	DurationMs   int
	ErrorMessage string
	Timestamp    time.Time
	Verifiable   bool
}

// Engine manages health checks and quarantine logic
type Options struct {
	FailureThreshold int
	SuccessThreshold int
	MinQuarantine    time.Duration
	BackoffBase      time.Duration
	BackoffMax       time.Duration
}

type Engine struct {
	nodes          map[string]*NodeHealth
	mu             sync.RWMutex
	logger         *slog.Logger
	maxConsecutive int
	minSuccesses   int
	quarantineTTL  time.Duration
	backoffBase    time.Duration
	backoffMax     time.Duration
}

// NewEngine creates a new health engine
func NewEngine(logger *slog.Logger, options ...Options) *Engine {
	o := Options{FailureThreshold: 3, SuccessThreshold: 2, MinQuarantine: time.Minute, BackoffBase: 30 * time.Second, BackoffMax: 15 * time.Minute}
	if len(options) > 0 {
		o = options[0]
	}
	return &Engine{
		nodes:          make(map[string]*NodeHealth),
		logger:         logger,
		maxConsecutive: o.FailureThreshold,
		minSuccesses:   o.SuccessThreshold,
		quarantineTTL:  o.MinQuarantine,
		backoffBase:    o.BackoffBase,
		backoffMax:     o.BackoffMax,
	}
}

// AddNode registers a node for health monitoring
func (e *Engine) AddNode(nodeID string) *NodeHealth {
	e.mu.Lock()
	defer e.mu.Unlock()

	node := &NodeHealth{
		NodeID: nodeID,
		Status: StatusUnknown,
	}

	e.nodes[nodeID] = node
	e.logger.Info("Added node to health monitoring", slog.String("node_id", nodeID))

	copy := *node
	return &copy
}

// GetNodeHealth returns the current health status of a node
func (e *Engine) GetNodeHealth(nodeID string) (HealthStatus, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	node, exists := e.nodes[nodeID]
	if !exists {
		return "", false
	}

	return node.Status, true
}

// SnapshotNode returns an independent copy for persistence.
func (e *Engine) SnapshotNode(id string) (NodeHealth, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	if node, ok := e.nodes[id]; ok {
		return *node, true
	}
	return NodeHealth{}, false
}

func (e *Engine) SnapshotNodes() map[string]NodeHealth {
	e.mu.RLock()
	defer e.mu.RUnlock()
	states := make(map[string]NodeHealth, len(e.nodes))
	for id, node := range e.nodes {
		states[id] = *node
	}
	return states
}

// RestoreNode restores committed local state at startup or after a failed write.
func (e *Engine) RestoreNode(node NodeHealth) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.nodes[node.NodeID] = &node
}

// RetainNodes bounds runtime state to the currently synchronized fleet.
func (e *Engine) RetainNodes(ids map[string]bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for id := range e.nodes {
		if !ids[id] {
			delete(e.nodes, id)
		}
	}
}

// RecordProbeResult records the result of a health probe
func (e *Engine) RecordProbeResult(result ProbeResult) {
	e.mu.Lock()
	defer e.mu.Unlock()

	node, exists := e.nodes[result.NodeID]
	if !exists {
		return
	}
	if node.ManualOverride {
		return
	}
	if result.Timestamp.IsZero() || (!node.LastHealthCheck.IsZero() && !result.Timestamp.After(node.LastHealthCheck)) {
		return
	}

	if result.Success {
		// Probe succeeded
		node.LastSuccessTime = result.Timestamp
		node.ConsecutiveFailures = 0
		node.ConsecutiveSuccesses = min(node.ConsecutiveSuccesses+1, 1000000)
		node.NextProbeAt = time.Time{}

		// Transition from quarantine to recovering
		if node.Status == StatusQuarantined || node.Status == StatusRecovering {
			if node.ConsecutiveSuccesses >= e.minSuccesses && !result.Timestamp.Before(node.QuarantinedUntil) {
				node.Status = StatusHealthy
				node.QuarantinedUntil = time.Time{}
				e.logger.Info("Node recovered from quarantine", slog.String("node_id", result.NodeID))
			} else {
				node.Status = StatusRecovering
			}
		} else {
			node.Status = StatusHealthy
		}
	} else {
		// Probe failed
		node.ConsecutiveFailures = min(node.ConsecutiveFailures+1, 1000000)
		node.ConsecutiveSuccesses = 0

		if (node.ConsecutiveFailures >= e.maxConsecutive || node.Status == StatusRecovering) && node.Status != StatusQuarantined {
			// Quarantine the node
			node.Status = StatusQuarantined
			node.QuarantinedUntil = result.Timestamp.Add(e.quarantineTTL)

			e.logger.Warn("Node quarantined",
				slog.String("node_id", result.NodeID),
				slog.Int("consecutive_failures", node.ConsecutiveFailures),
			)
		} else if node.Status != StatusQuarantined && node.ConsecutiveFailures >= 1 {
			node.Status = StatusSuspected
		}
		if node.Status == StatusQuarantined {
			delay := e.backoffBase
			for i := e.maxConsecutive; i < node.ConsecutiveFailures && delay < e.backoffMax; i++ {
				if delay >= e.backoffMax/2 {
					delay = e.backoffMax
					break
				}
				delay *= 2
			}
			node.NextProbeAt = result.Timestamp.Add(delay)
		}
	}

	node.LastHealthCheck = result.Timestamp

	e.logger.Debug("Probe result recorded",
		slog.String("node_id", result.NodeID),
		slog.String("probe_type", string(result.ProbeType)),
		slog.Bool("success", result.Success),
		slog.String("status", string(node.Status)),
	)
}

// ProbeNode performs a health probe on a node
func (e *Engine) ProbeNode(nodeID, address string, probeType ProbeType, timeout time.Duration) ProbeResult {
	result := ProbeResult{
		NodeID:     nodeID,
		ProbeType:  probeType,
		Timestamp:  time.Now(),
		Verifiable: true,
	}

	start := time.Now()

	switch probeType {
	case ProbeTCP:
		result.Success, result.ErrorMessage = e.probeTCP(address, timeout)
	case ProbeTLS:
		result.Success, result.ErrorMessage = e.probeTLS(address, timeout)
	case ProbeHTTP:
		result.Success, result.ErrorMessage = e.probeHTTP(address, timeout)
	case ProbeQUIC:
		result.Verifiable = false
		result.ErrorMessage = "QUIC handshake requires a protocol probe; reachability is unverified"
	default:
		result.Verifiable = false
		result.ErrorMessage = "unknown probe type; unverified"
	}

	result.DurationMs = int(time.Since(start).Milliseconds())

	// Record result
	if result.Verifiable {
		e.RecordProbeResult(result)
	}

	return result
}

// probeTCP performs a simple TCP connect probe
func (e *Engine) probeTCP(address string, timeout time.Duration) (bool, string) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false, err.Error()
	}
	defer conn.Close()
	return true, ""
}

// probeTLS performs a TLS handshake probe
func (e *Engine) probeTLS(address string, timeout time.Duration) (bool, string) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false, err.Error()
	}
	dialer := &tls.Dialer{NetDialer: &net.Dialer{Timeout: timeout}, Config: &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host}}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		return false, err.Error()
	}
	conn.Close()
	return true, ""
}

// probeHTTP performs an HTTP request probe
func (e *Engine) probeHTTP(address string, timeout time.Duration) (bool, string) {
	if !strings.HasPrefix(address, "http://") && !strings.HasPrefix(address, "https://") {
		return false, "HTTP probe requires an absolute URL"
	}
	client := &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(address)
	if err != nil {
		return false, err.Error()
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10))
	client.CloseIdleConnections()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return false, fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return true, ""
}

// probeUDP performs a UDP reachability probe
func (e *Engine) probeUDP(address string, timeout time.Duration) (bool, string) {
	// Send UDP packet and wait for response
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return false, fmt.Sprintf("resolve failed: %v", err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return false, fmt.Sprintf("dial failed: %v", err)
	}
	defer conn.Close()

	// Set read deadline
	conn.SetReadDeadline(time.Now().Add(timeout))

	// Send simple probe packet
	if _, err := conn.Write([]byte("probe")); err != nil {
		return false, fmt.Sprintf("write failed: %v", err)
	}

	// Try to read response (optional)
	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	if err != nil {
		return false, "UDP response not received; reachability is unverified"
	}

	return true, ""
}

// GetAllHealthStates returns health states for all nodes
func (e *Engine) GetAllHealthStates() map[string]HealthStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	results := make(map[string]HealthStatus)
	for nodeID, node := range e.nodes {
		results[nodeID] = node.Status
	}

	return results
}

// GetQuarantinedNodes returns nodes currently in quarantine
func (e *Engine) GetQuarantinedNodes() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var quarantined []string
	for nodeID, node := range e.nodes {
		if node.Status == StatusQuarantined {
			quarantined = append(quarantined, nodeID)
		}
	}

	return quarantined
}

// ReleaseQuarantine manually releases a node from quarantine
func (e *Engine) ReleaseQuarantine(nodeID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	node, exists := e.nodes[nodeID]
	if !exists {
		return fmt.Errorf("node not found: %s", nodeID)
	}

	if node.Status == StatusQuarantined || node.Status == StatusRecovering {
		node.Status = StatusHealthy
		node.ConsecutiveFailures = 0
		node.ConsecutiveSuccesses = 0
		node.NextProbeAt = time.Time{}
		node.QuarantinedUntil = time.Time{}

		e.logger.Info("Manually released node from quarantine", slog.String("node_id", nodeID))
	}

	return nil
}

// Statistics returns health engine statistics
func (e *Engine) Statistics() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	healthy := 0
	suspected := 0
	quarantined := 0

	for _, node := range e.nodes {
		switch node.Status {
		case StatusHealthy:
			healthy++
		case StatusSuspected:
			suspected++
		case StatusQuarantined:
			quarantined++
		}
	}

	return map[string]interface{}{
		"total_nodes": len(e.nodes),
		"healthy":     healthy,
		"suspected":   suspected,
		"quarantined": quarantined,
		"timestamp":   time.Now().Unix(),
	}
}
