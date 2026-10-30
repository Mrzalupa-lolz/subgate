// Package tier implements real tier selection and balancing logic
package tier

import (
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"sync"
)

// Strategy defines balancing strategy
type Strategy string

const (
	StrategyRandom     Strategy = "random"
	StrategyRoundRobin Strategy = "roundRobin"
	StrategyLeastPing  Strategy = "leastPing"
	StrategyLeastLoad  Strategy = "leastLoad"
)

// Tier represents a service tier (group of nodes)
type Tier struct {
	ID           string
	Name         string
	Strategy     Strategy
	MinHealthy   int
	FallbackTier *Tier
	Nodes        []NodeInfo
	LastIndex    int // For round-robin
	mu           sync.RWMutex
	logger       *slog.Logger
}

// NodeInfo represents node availability info
type NodeInfo struct {
	ID       string
	Name     string
	Healthy  bool
	PingMs   int
	Load     float64 // 0-100%
	Capacity int     // Mbps
	Users    int     // Connected users
}

// NewTier creates a new tier
func NewTier(id, name string, strategy Strategy, minHealthy int, logger *slog.Logger) *Tier {
	return &Tier{
		ID:         id,
		Name:       name,
		Strategy:   strategy,
		MinHealthy: minHealthy,
		Nodes:      []NodeInfo{},
		logger:     logger,
	}
}

// AddNode adds a node to the tier
func (t *Tier) AddNode(node NodeInfo) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.Nodes = append(t.Nodes, node)
	t.logger.Debug("Added node to tier",
		slog.String("tier_id", t.ID),
		slog.String("node_id", node.ID),
	)
}

// SelectNode selects a node based on strategy
func (t *Tier) SelectNode() (*NodeInfo, error) {
	return t.selectNode(make(map[*Tier]bool))
}

func (t *Tier) selectNode(visited map[*Tier]bool) (*NodeInfo, error) {
	if visited[t] {
		return nil, fmt.Errorf("tier fallback cycle")
	}
	visited[t] = true
	t.mu.Lock()

	// Get healthy nodes
	healthyNodes := []NodeInfo{}
	for _, node := range t.Nodes {
		if node.Healthy {
			healthyNodes = append(healthyNodes, node)
		}
	}

	if len(healthyNodes) == 0 {
		// Try fallback tier
		if t.FallbackTier != nil {
			fallback := t.FallbackTier
			t.mu.Unlock()
			t.logger.Warn("No healthy nodes in tier, using fallback",
				slog.String("tier_id", t.ID),
			)
			return fallback.selectNode(visited)
		}

		t.mu.Unlock()
		return nil, fmt.Errorf("no healthy nodes in tier %s and no fallback", t.ID)
	}

	// Check minimum healthy threshold
	if len(healthyNodes) < t.MinHealthy {
		if t.FallbackTier != nil {
			fallback := t.FallbackTier
			t.mu.Unlock()
			t.logger.Warn("Tier below minimum healthy threshold, using fallback",
				slog.String("tier_id", t.ID),
				slog.Int("healthy", len(healthyNodes)),
				slog.Int("min_healthy", t.MinHealthy),
			)
			return fallback.selectNode(visited)
		}
	}
	defer t.mu.Unlock()

	// Select based on strategy
	var selected *NodeInfo

	switch t.Strategy {
	case StrategyRandom:
		selected = &healthyNodes[rand.IntN(len(healthyNodes))]

	case StrategyRoundRobin:
		selected = &healthyNodes[t.LastIndex%len(healthyNodes)]
		t.LastIndex++

	case StrategyLeastPing:
		sort.Slice(healthyNodes, func(i, j int) bool {
			return healthyNodes[i].PingMs < healthyNodes[j].PingMs
		})
		selected = &healthyNodes[0]

	case StrategyLeastLoad:
		sort.Slice(healthyNodes, func(i, j int) bool {
			return healthyNodes[i].Load < healthyNodes[j].Load
		})
		selected = &healthyNodes[0]

	default:
		selected = &healthyNodes[0]
	}

	return selected, nil
}

// UpdateNodeHealth updates node health status
func (t *Tier) UpdateNodeHealth(nodeID string, healthy bool) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i, node := range t.Nodes {
		if node.ID == nodeID {
			t.Nodes[i].Healthy = healthy
			t.logger.Debug("Updated node health",
				slog.String("tier_id", t.ID),
				slog.String("node_id", nodeID),
				slog.Bool("healthy", healthy),
			)
			return
		}
	}
}

// UpdateNodeMetrics updates node performance metrics
func (t *Tier) UpdateNodeMetrics(nodeID string, pingMs int, load float64) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for i, node := range t.Nodes {
		if node.ID == nodeID {
			t.Nodes[i].PingMs = pingMs
			t.Nodes[i].Load = load
			return
		}
	}
}

// HealthyCount returns number of healthy nodes
func (t *Tier) HealthyCount() int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	count := 0
	for _, node := range t.Nodes {
		if node.Healthy {
			count++
		}
	}

	return count
}

// Statistics returns tier statistics
func (t *Tier) Statistics() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	healthy := 0
	for _, node := range t.Nodes {
		if node.Healthy {
			healthy++
		}
	}
	total := len(t.Nodes)

	avgLoad := 0.0
	if total > 0 {
		totalLoad := 0.0
		for _, node := range t.Nodes {
			totalLoad += node.Load
		}
		avgLoad = totalLoad / float64(total)
	}

	return map[string]interface{}{
		"tier_id":     t.ID,
		"total_nodes": total,
		"healthy":     healthy,
		"avg_load":    avgLoad,
		"strategy":    t.Strategy,
	}
}
