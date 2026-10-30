// Package quota implements deterministic per-user and per-node byte quotas.
// It is independent of SQLite so it can be used in the gateway and by the
// agent without introducing a storage dependency. A usage reader lets the
// gateway check committed telemetry without maintaining a second usage ledger.
package quota

import (
	"fmt"
	"math"
	"sync"
	"time"
	_ "time/tzdata" // Calendar quotas must also work in minimal static deployments.
)

type Period string

const (
	PeriodDay     Period = "day"
	PeriodWeek    Period = "week"
	PeriodMonth   Period = "month"
	PeriodRolling Period = "rolling"
)

type Rule struct {
	ID         string    `json:"id"`
	NodeID     string    `json:"node_id"`
	UserID     string    `json:"user_id,omitempty"`
	Scope      string    `json:"scope"` // per_user or per_node
	LimitBytes int64     `json:"limit_bytes"`
	Period     Period    `json:"period"`
	Timezone   string    `json:"timezone,omitempty"`
	Enabled    bool      `json:"enabled"`
	GraceBytes int64     `json:"grace_bytes,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Usage struct {
	RuleID      string    `json:"rule_id"`
	NodeID      string    `json:"node_id"`
	UserID      string    `json:"user_id,omitempty"`
	Bytes       int64     `json:"bytes"`
	LimitBytes  int64     `json:"limit_bytes"`
	Remaining   int64     `json:"remaining"`
	PeriodStart time.Time `json:"period_start"`
	PeriodEnd   time.Time `json:"period_end"`
	Allowed     bool      `json:"allowed"`
}

type Manager struct {
	mu    sync.Mutex
	clock func() time.Time
	rules map[string]Rule
	used  map[string]int64
	start map[string]time.Time
	// rolling keeps timestamped consumption so a rolling 30-day rule does not
	// lose usage merely because the wall clock crossed an arbitrary boundary.
	rolling map[string][]usageSample
	reader  UsageReader
}

// UsageReader returns committed bytes in [start, end). Read errors bypass the
// affected rule, preserving the gateway's fail-open behavior.
type UsageReader func(rule Rule, start, end time.Time) (int64, error)

func (m *Manager) SetUsageReader(reader UsageReader) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reader = reader
}

type usageSample struct {
	at    time.Time
	bytes int64
}

func New() *Manager {
	return &Manager{clock: time.Now, rules: map[string]Rule{}, used: map[string]int64{}, start: map[string]time.Time{}, rolling: map[string][]usageSample{}}
}

func (m *Manager) SetClock(clock func() time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if clock != nil {
		m.clock = clock
	}
}

// Normalize validates a rule without mutating manager state.
func Normalize(rule Rule) (Rule, error) {
	if rule.ID == "" || len(rule.ID) > 128 {
		return rule, fmt.Errorf("quota id is required")
	}
	if rule.LimitBytes <= 0 {
		return rule, fmt.Errorf("quota limit must be positive")
	}
	if rule.Scope != "per_user" && rule.Scope != "per_node" {
		return rule, fmt.Errorf("quota scope must be per_user or per_node")
	}
	if rule.NodeID == "" {
		return rule, fmt.Errorf("quota node_id is required")
	}
	if rule.Scope == "per_user" && rule.UserID == "" {
		return rule, fmt.Errorf("per_user quota requires user_id")
	}
	if rule.Period == "" {
		rule.Period = PeriodMonth
	}
	if rule.Period != PeriodDay && rule.Period != PeriodWeek && rule.Period != PeriodMonth && rule.Period != PeriodRolling {
		return rule, fmt.Errorf("unsupported quota period")
	}
	if rule.GraceBytes < 0 {
		return rule, fmt.Errorf("grace_bytes cannot be negative")
	}
	if rule.GraceBytes > math.MaxInt64-rule.LimitBytes {
		return rule, fmt.Errorf("limit_bytes plus grace_bytes overflows")
	}
	if rule.Timezone != "" {
		if _, err := time.LoadLocation(rule.Timezone); err != nil {
			return rule, fmt.Errorf("unsupported quota timezone")
		}
	}
	return rule, nil
}

func (m *Manager) Upsert(rule Rule) error {
	rule, err := Normalize(rule)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if rule.UpdatedAt.IsZero() {
		rule.UpdatedAt = m.clock().UTC()
	}
	m.rules[rule.ID] = rule
	return nil
}

func (m *Manager) Delete(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, id)
	delete(m.used, id)
	delete(m.start, id)
	delete(m.rolling, id)
}
func (m *Manager) Rule(id string) (Rule, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rule, ok := m.rules[id]
	return rule, ok
}
func (m *Manager) Rules() []Rule {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Rule, 0, len(m.rules))
	for _, rule := range m.rules {
		out = append(out, rule)
	}
	return out
}

func periodBounds(now time.Time, period Period, timezone string) (time.Time, time.Time) {
	loc := time.UTC
	if timezone != "" {
		if loaded, err := time.LoadLocation(timezone); err == nil {
			loc = loaded
		}
	}
	local := now.In(loc)
	var start, end time.Time
	switch period {
	case PeriodDay:
		start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		end = start.AddDate(0, 0, 1)
	case PeriodWeek:
		start = time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
		weekday := int(start.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		start = start.AddDate(0, 0, -(weekday - 1))
		end = start.AddDate(0, 0, 7)
	case PeriodRolling:
		start = now.Add(-30 * 24 * time.Hour)
		end = now.Add(time.Nanosecond)
	default:
		start = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc)
		end = start.AddDate(0, 1, 0)
	}
	return start.UTC(), end.UTC()
}

func (m *Manager) resetIfNeeded(rule Rule, now time.Time) (time.Time, time.Time) {
	start, end := periodBounds(now, rule.Period, rule.Timezone)
	if rule.Period == PeriodRolling {
		m.pruneRolling(rule.ID, start)
		return start, end
	}
	if previous, ok := m.start[rule.ID]; !ok || !previous.Equal(start) {
		m.start[rule.ID] = start
		m.used[rule.ID] = 0
	}
	return start, end
}

func (m *Manager) pruneRolling(id string, cutoff time.Time) int64 {
	samples := m.rolling[id]
	if len(samples) == 0 {
		return 0
	}
	kept := samples[:0]
	var total int64
	for _, sample := range samples {
		if sample.at.Before(cutoff) {
			continue
		}
		kept = append(kept, sample)
		total = saturatingAdd(total, sample.bytes)
	}
	if len(kept) == 0 {
		delete(m.rolling, id)
	} else {
		m.rolling[id] = kept
	}
	return total
}

func (m *Manager) usage(rule Rule, now time.Time) int64 {
	start, _ := periodBounds(now, rule.Period, rule.Timezone)
	if rule.Period == PeriodRolling {
		return m.pruneRolling(rule.ID, start)
	}
	return m.used[rule.ID]
}

func (m *Manager) Check(nodeID, userID string, bytes int64) []Usage {
	if bytes < 0 {
		bytes = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock().UTC()
	var out []Usage
	for _, rule := range m.rules {
		if !rule.Enabled || rule.NodeID != nodeID || (rule.Scope == "per_user" && rule.UserID != userID) {
			continue
		}
		start, end := m.resetIfNeeded(rule, now)
		used := m.usage(rule, now)
		// Future-dated telemetry must not prematurely exhaust a quota.
		cutoff := now.Add(time.Millisecond)
		if rule.Period != PeriodRolling && end.Before(cutoff) {
			cutoff = end
		}
		if m.reader != nil {
			var err error
			used, err = m.reader(rule, start, cutoff)
			if err != nil || used < 0 {
				continue
			}
		}
		limit := rule.LimitBytes + rule.GraceBytes
		out = append(out, Usage{RuleID: rule.ID, NodeID: nodeID, UserID: userID, Bytes: used, LimitBytes: rule.LimitBytes, Remaining: max64(0, limit-used), PeriodStart: start, PeriodEnd: end, Allowed: withinLimit(used, bytes, limit)})
	}
	return out
}

func (m *Manager) Consume(nodeID, userID string, bytes int64) (bool, []Usage) {
	if bytes < 0 {
		return false, nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock().UTC()
	var checks []Usage
	allowed := true
	for _, rule := range m.rules {
		if !rule.Enabled || rule.NodeID != nodeID || (rule.Scope == "per_user" && rule.UserID != userID) {
			continue
		}
		start, end := m.resetIfNeeded(rule, now)
		used := m.usage(rule, now)
		limit := rule.LimitBytes + rule.GraceBytes
		ok := withinLimit(used, bytes, limit)
		if !ok {
			allowed = false
		}
		checks = append(checks, Usage{RuleID: rule.ID, NodeID: nodeID, UserID: userID, Bytes: used, LimitBytes: rule.LimitBytes, Remaining: max64(0, limit-used), PeriodStart: start, PeriodEnd: end, Allowed: ok})
	}
	if allowed {
		for _, rule := range m.rules {
			if rule.Enabled && rule.NodeID == nodeID && (rule.Scope == "per_node" || rule.UserID == userID) {
				if rule.Period == PeriodRolling {
					if bytes > 0 {
						m.rolling[rule.ID] = append(m.rolling[rule.ID], usageSample{at: now, bytes: bytes})
					}
				} else {
					m.used[rule.ID] += bytes
				}
			}
		}
	}
	return allowed, checks
}

// Record accounts bytes that have already traversed the gateway. Unlike
// Consume, it records traffic even when the limit was already exceeded.
// Telemetry is an observation after delivery, so dropping those samples would
// make the next period appear to have less usage than actually occurred.
func (m *Manager) Record(nodeID, userID string, bytes int64) {
	if bytes <= 0 {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock().UTC()
	for _, rule := range m.rules {
		if !rule.Enabled || rule.NodeID != nodeID || (rule.Scope == "per_user" && rule.UserID != userID) {
			continue
		}
		m.resetIfNeeded(rule, now)
		if rule.Period == PeriodRolling {
			m.rolling[rule.ID] = append(m.rolling[rule.ID], usageSample{at: now, bytes: bytes})
			continue
		}
		if bytes > math.MaxInt64-m.used[rule.ID] {
			m.used[rule.ID] = math.MaxInt64
		} else {
			m.used[rule.ID] += bytes
		}
	}
}

func (m *Manager) Remaining(nodeID, userID string) int64 {
	checks := m.Check(nodeID, userID, 0)
	remaining := int64(-1)
	for _, check := range checks {
		if remaining < 0 || check.Remaining < remaining {
			remaining = check.Remaining
		}
	}
	return remaining
}
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

func withinLimit(used, additional, limit int64) bool {
	return used <= limit && additional <= limit-used
}

func saturatingAdd(a, b int64) int64 {
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}
