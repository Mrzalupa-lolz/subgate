// Package tsdb implements the time-series database for bandwidth metrics
package tsdb

import (
	"log/slog"
	"sort"
	"sync"
	"time"
)

// Sample represents a per-user bandwidth sample
type Sample struct {
	UserID      int
	NodeID      string
	Timestamp   time.Time
	UplinkBps   int64
	DownlinkBps int64
}

// RollupEntry represents aggregated data
type RollupEntry struct {
	UserID       int
	NodeID       string
	Timestamp    time.Time
	UplinkAvg    int64
	DownlinkAvg  int64
	UplinkPeak   int64
	DownlinkPeak int64
	UplinkP95    int64
	DownlinkP95  int64
	SampleCount  int
}

// TSDB manages time-series data with multiple tiers
type TSDB struct {
	// Raw 1-second samples (in-memory ring buffer)
	rawRing     []Sample
	rawIndex    int
	rawCapacity int

	// Rollup data (10s, 1m, 1h) would be in SQLite

	mu     sync.RWMutex
	logger *slog.Logger
}

// NewTSDB creates a new TSDB instance
func NewTSDB(maxRawSamples int, logger *slog.Logger) *TSDB {
	if maxRawSamples < 1 {
		maxRawSamples = 1
	}
	return &TSDB{
		rawRing:     make([]Sample, maxRawSamples),
		rawIndex:    0,
		rawCapacity: maxRawSamples,
		logger:      logger,
	}
}

// AddSample adds a new bandwidth sample
func (t *TSDB) AddSample(sample Sample) {
	if sample.Timestamp.IsZero() {
		sample.Timestamp = time.Now()
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Add to ring buffer
	t.rawRing[t.rawIndex] = sample
	t.rawIndex = (t.rawIndex + 1) % t.rawCapacity

	t.logger.Debug("Added TSDB sample",
		slog.Int("user_id", sample.UserID),
		slog.String("node_id", sample.NodeID),
		slog.Int64("uplink_bps", sample.UplinkBps),
	)
}

// BulkAddSamples adds multiple samples in batch
func (t *TSDB) BulkAddSamples(samples []Sample) {
	t.mu.Lock()
	defer t.mu.Unlock()

	for _, sample := range samples {
		if sample.Timestamp.IsZero() {
			sample.Timestamp = time.Now()
		}

		t.rawRing[t.rawIndex] = sample
		t.rawIndex = (t.rawIndex + 1) % t.rawCapacity
	}

	t.logger.Info("Added TSDB samples", slog.Int("count", len(samples)))
}

// GetUserSpeed returns current speed for a user
func (t *TSDB) GetUserSpeed(userID int) (uplink, downlink int64, found bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Find latest sample for this user
	for i := 1; i <= len(t.rawRing); i++ {
		idx := (t.rawIndex - i + t.rawCapacity) % t.rawCapacity
		sample := t.rawRing[idx]

		if sample.UserID == userID && sample.Timestamp.After(time.Now().Add(-5*time.Second)) {
			return sample.UplinkBps, sample.DownlinkBps, true
		}

	}

	return 0, 0, false
}

// GetTopUsers returns the top N users by traffic
func (t *TSDB) GetTopUsers(count int) []Sample {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Simple implementation: last N non-zero samples
	results := []Sample{}
	seen := make(map[int]bool)

	if count <= 0 {
		return results
	}
	for i := 0; i < len(t.rawRing); i++ {
		idx := (t.rawIndex - i - 1 + t.rawCapacity) % t.rawCapacity
		sample := t.rawRing[idx]

		if sample.UserID > 0 && !seen[sample.UserID] && sample.Timestamp.After(time.Now().Add(-5*time.Second)) {
			results = append(results, sample)
			seen[sample.UserID] = true
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].UplinkBps+results[i].DownlinkBps > results[j].UplinkBps+results[j].DownlinkBps
	})
	if len(results) > count {
		results = results[:count]
	}
	return results
}

// GetStats returns TSDB statistics
func (t *TSDB) GetStats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Count non-empty entries
	count := 0
	for _, sample := range t.rawRing {
		if sample.UserID > 0 {
			count++
		}
	}

	return map[string]interface{}{
		"raw_samples":   count,
		"ring_capacity": t.rawCapacity,
		"current_index": t.rawIndex,
		"timestamp":     time.Now().Unix(),
	}
}

// Cleanup removes old data (called periodically)
func (t *TSDB) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-5 * time.Minute)
	expired := 0

	for i, sample := range t.rawRing {
		if sample.UserID > 0 && sample.Timestamp.Before(cutoff) {
			t.rawRing[i] = Sample{} // Clear
			expired++
		}
	}

	if expired > 0 {
		t.logger.Debug("Cleaned up expired samples", slog.Int("count", expired))
	}
}

// Memory returns approximate memory usage in bytes
func (t *TSDB) Memory() int64 {
	// Rough estimate: each Sample is ~60 bytes
	return int64(t.rawCapacity * 60)
}
