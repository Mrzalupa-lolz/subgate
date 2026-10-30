package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/subgate/subgate/core/internal/health"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/healthprobe"
)

func (s *Server) initializeHealth() error {
	c := s.config.Health
	s.health = health.NewEngine(s.logger, health.Options{FailureThreshold: c.FailureThreshold, SuccessThreshold: c.SuccessThreshold, MinQuarantine: c.MinQuarantine, BackoffBase: c.BackoffBase, BackoffMax: c.BackoffMax})
	states, err := s.objects("health-state")
	if err != nil {
		return err
	}
	for _, raw := range states {
		var state health.NodeHealth
		if err := json.Unmarshal(raw, &state); err != nil || state.NodeID == "" {
			return fmt.Errorf("invalid persisted health state")
		}
		s.health.RestoreNode(state)
	}
	return nil
}

// Only publish committed state to routing and Agent scheduling. The same lock
// covers the Engine advance, durable transaction and rollback below.
func (s *Server) committedHealthSnapshot() map[string]health.NodeHealth {
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	return s.health.SnapshotNodes()
}

func (s *Server) excludedHealthNodes() map[string]bool {
	excluded := map[string]bool{}
	for id, state := range s.committedHealthSnapshot() {
		if state.Status == health.StatusQuarantined || state.Status == health.StatusRecovering {
			excluded[id] = true
		}
	}
	return excluded
}

func (s *Server) startBackground() {
	s.wg.Add(2)
	go s.background()
	go s.healthBackground()
}

func (s *Server) healthBackground() {
	defer s.wg.Done()
	ticker := time.NewTicker(s.config.Health.ProbeInterval)
	defer ticker.Stop()
	for {
		if err := s.scheduledHealth(s.ctx); err != nil && s.ctx.Err() == nil {
			s.logger.Warn("scheduled health cycle failed")
		}
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// One cycle at a time, four node workers, at most sixteen endpoints per node.
// It runs independently of slow panel polling, and never writes to the panel.
func (s *Server) scheduledHealth(ctx context.Context) error {
	rawNodes, err := s.objects("nodes")
	if err != nil {
		return err
	}
	nodes := make([]panel.Node, 0, len(rawNodes))
	ids := make(map[string]bool, len(rawNodes))
	for _, raw := range rawNodes {
		var n panel.Node
		if err := json.Unmarshal(raw, &n); err != nil {
			return err
		}
		nodes = append(nodes, n)
		ids[n.ID] = true
	}
	s.health.RetainNodes(ids)
	jobs := make(chan panel.Node)
	var workers sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	for i := 0; i < min(4, len(nodes)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for n := range jobs {
				if ctx.Err() != nil {
					return
				}
				if err := s.scheduledNodeProbe(ctx, n); err != nil {
					errMu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					errMu.Unlock()
				}
			}
		}()
	}
	for _, n := range nodes {
		select {
		case jobs <- n:
		case <-ctx.Done():
			close(jobs)
			workers.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	workers.Wait()
	return firstErr
}

func (s *Server) scheduledNodeProbe(parent context.Context, n panel.Node) error {
	if parent.Err() != nil {
		return parent.Err()
	}
	if s.inMaintenance(n.ID, time.Now()) {
		status, _ := s.health.GetNodeHealth(n.ID)
		if status == "" {
			status = health.StatusUnknown
		}
		return s.saveObject("health", n.ID, nodeProbe{ID: n.ID, NodeID: n.ID, Timestamp: time.Now(), Verified: "unverified", Status: status, Maintenance: true, Error: "maintenance window active"})
	}
	if state, ok := s.health.SnapshotNode(n.ID); ok && time.Now().Before(state.NextProbeAt) {
		return nil
	}
	result := healthprobe.Run(parent, healthTarget(n), s.config.Health.ProbeTimeout)
	best := nodeProbe{ID: n.ID, NodeID: n.ID, Timestamp: result.Timestamp, CompletedAt: result.CompletedAt, Success: result.Success, Verified: result.Verified, Generic: result.Generic, Error: result.Error, Protocol: result.Protocol, Transport: result.Transport, Security: result.Security, Port: result.Port, Duration: result.DurationMS}
	// Cancellation by shutdown does not become a false network failure.
	if parent.Err() != nil {
		return parent.Err()
	}
	s.healthMu.Lock()
	defer s.healthMu.Unlock()
	previous, exists := s.health.SnapshotNode(n.ID)
	if !exists {
		previous = health.NodeHealth{NodeID: n.ID, Status: health.StatusUnknown}
		s.health.RestoreNode(previous)
	}
	err := s.enrollmentTransaction(func(tx *sql.Tx) error {
		// Credential revocation/report updates and state persistence share the
		// DB write lock, so a vote cannot be invalidated between read and commit.
		success, quorum, vantages, err := s.probeQuorum(tx, best)
		if err != nil {
			return err
		}
		best.QuorumMet = quorum
		best.Vantages = vantages
		atCommit := time.Now()
		best.Maintenance = s.inMaintenance(n.ID, atCommit) || s.overlapsMaintenance(n.ID, best.Timestamp, atCommit)
		if best.Maintenance {
			best.QuorumMet = false
		}
		if quorum && !best.Maintenance {
			// Hold/backoff start at the transition, after the observation and DB
			// lock wait. Probe start/end remain separate evidence timestamps.
			s.health.RecordProbeResult(health.ProbeResult{NodeID: n.ID, Success: success, Timestamp: atCommit, Verifiable: true})
		}
		state, _ := s.health.SnapshotNode(n.ID)
		best.Status = state.Status
		for kind, data := range map[string]any{"health": best, "health-state": state} {
			raw, err := json.Marshal(data)
			if err != nil {
				return err
			}
			if _, err := tx.Exec("INSERT INTO sg_objects(kind,id,data,updated_at) VALUES(?,?,?,?) ON CONFLICT(kind,id) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at", kind, n.ID, string(raw), time.Now().Unix()); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		s.health.RestoreNode(previous)
	}
	return err
}
