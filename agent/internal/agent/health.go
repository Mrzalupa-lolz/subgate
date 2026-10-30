package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/subgate/subgate/pkg/healthprobe"
	"github.com/subgate/subgate/pkg/wire"
)

func (a *Agent) healthLoop() {
	defer a.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		if err := a.collectHealth(a.ctx); err != nil && a.ctx.Err() == nil {
			a.config.Logger.Warn("health probe reporting unavailable")
		}
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (a *Agent) collectHealth(parent context.Context) error {
	return a.collectHealthCycle(parent, 30*time.Second, 8*time.Second)
}

func (a *Agent) collectHealthCycle(parent context.Context, cycleTimeout, nodeTimeout time.Duration) error {
	ctx, cancel := context.WithTimeout(parent, cycleTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", a.config.CoreAddress+"/api/v1/agents/health-targets", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.Token)
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("health target request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return &coreHTTPError{status: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256<<10+1))
	if err != nil || len(raw) > 256<<10 {
		return fmt.Errorf("health targets exceed limit")
	}
	var envelope struct {
		Data []wire.HealthTarget `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Data) > 64 {
		return fmt.Errorf("invalid health targets")
	}
	seen := map[string]bool{}
	for _, t := range envelope.Data {
		if t.NodeID == "" || len(t.NodeID) > 256 || seen[t.NodeID] || len(t.Inbounds) > 16 || t.Address == "" || len(t.Address) > 253 {
			return fmt.Errorf("invalid health target")
		}
		seen[t.NodeID] = true
	}
	a.mu.Lock()
	start := 0
	if len(envelope.Data) > 0 {
		start = a.healthCursor % len(envelope.Data)
	}
	a.mu.Unlock()
	if start > 0 {
		envelope.Data = append(append([]wire.HealthTarget(nil), envelope.Data[start:]...), envelope.Data[:start]...)
	}
	results := make([]wire.HealthResult, len(envelope.Data))
	completed := make([]bool, len(envelope.Data))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for i := 0; i < min(4, len(envelope.Data)); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for i := range jobs {
				if ctx.Err() != nil {
					return
				}
				results[i] = healthprobe.Run(ctx, envelope.Data[i], nodeTimeout)
				completed[i] = ctx.Err() == nil
			}
		}()
	}
dispatch:
	for i := range envelope.Data {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	if parent.Err() != nil {
		return parent.Err()
	}
	finished := make([]wire.HealthResult, 0, len(results))
	next := start
	for i, done := range completed {
		if !done {
			next = start + i
			if i == 0 {
				next++
			}
			break
		}
	}
	// Collect all completed workers, including those after an unfinished index.
	for i, result := range results {
		if completed[i] {
			finished = append(finished, result)
		}
	}
	a.mu.Lock()
	a.healthCursor = next
	a.mu.Unlock()
	if len(finished) == 0 && ctx.Err() != nil {
		return ctx.Err()
	}
	// Report only completed observations; never convert canceled work to failure.
	// The report has its own short deadline so an expired cycle still makes progress.
	sendCtx, sendCancel := context.WithTimeout(parent, 5*time.Second)
	defer sendCancel()
	raw, err = json.Marshal(wire.HealthReport{Results: finished})
	if err != nil {
		return err
	}
	req, err = http.NewRequestWithContext(sendCtx, "PUT", a.config.CoreAddress+"/api/v1/agents/health-results", bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.Token)
	req.Header.Set("Content-Type", "application/json")
	sent, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("health result request failed")
	}
	defer sent.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(sent.Body, 64<<10))
	if sent.StatusCode != 200 {
		return &coreHTTPError{status: sent.StatusCode}
	}
	return nil
}
