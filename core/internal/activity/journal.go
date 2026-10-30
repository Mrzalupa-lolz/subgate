// Package activity implements real user activity journal (F20)
package activity

import (
	"bytes"
	"compress/gzip"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/klauspost/compress/zstd"
)

// Event represents a user activity event (connection to a destination)
type Event struct {
	Timestamp     time.Time
	UserID        string
	User          string
	NodeID        string
	InboundTag    string
	OutboundTag   string
	SrcIP         string
	SrcASN        int
	SrcCountry    string
	DstHost       string
	DstIP         string
	DstPort       int
	Network       string // tcp, udp
	Protocol      string
	Result        string // accepted, rejected, failed
	BytesUplink   int64
	BytesDownlink int64
}

type Tier string

const (
	Hot  Tier = "hot"
	Warm Tier = "warm"
	Cold Tier = "cold"
)

// CoalescedEvent represents multiple identical events coalesced into one
type CoalescedEvent struct {
	User          string
	DstHost       string
	DstPort       int
	Network       string
	InboundTag    string
	SrcIP         string
	FirstSeen     time.Time
	LastSeen      time.Time
	Count         int
	BytesUplink   int64
	BytesDownlink int64
	Tier          string
}

// Journal manages activity event collection and storage
type Journal struct {
	events             map[string]*CoalescedEvent // Key: user+dst+port+net
	userIndex          map[string][]string        // user -> event keys
	mu                 sync.RWMutex
	logger             *slog.Logger
	chatterCap         int             // Max distinct destinations per user per minute
	coalesceBucket     time.Duration   // Bucket size for coalescing (default 10s)
	retentionDays      int             // Hot tier retention
	ignoreList         map[string]bool // OS telemetry, NTP, etc.
	minuteDestinations map[string]map[int64]map[string]struct{}
	chatterOverflow    map[string]bool
	clock              func() time.Time
}

// NewJournal creates a new activity journal
func NewJournal(logger *slog.Logger) *Journal {
	return &Journal{
		events:         make(map[string]*CoalescedEvent),
		userIndex:      make(map[string][]string),
		logger:         logger,
		chatterCap:     300, // Per minute, per user
		coalesceBucket: 10 * time.Second,
		retentionDays:  14,
		ignoreList: map[string]bool{
			"captive.apple.com": true,
			"ntp.ubuntu.com":    true,
			"time.google.com":   true,
			"dns.google":        true,
		},
		minuteDestinations: make(map[string]map[int64]map[string]struct{}),
		chatterOverflow:    make(map[string]bool),
		clock:              time.Now,
	}
}

// AddEvent adds a raw event to the journal, coalescing as needed
func (j *Journal) AddEvent(event *Event) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}

	j.mu.Lock()
	defer j.mu.Unlock()
	if event.Timestamp.IsZero() {
		eventCopy := *event
		eventCopy.Timestamp = j.clock()
		event = &eventCopy
	}
	// Check ignore list
	if j.ignoreList[event.DstHost] {
		return nil
	}

	// Create coalescence key
	key := fmt.Sprintf("%q:%q:%d:%q:%q:%q:%q:%d",
		event.User,
		event.DstHost,
		event.DstPort,
		event.Network,
		event.InboundTag,
		event.SrcIP,
		event.NodeID,
		event.Timestamp.UnixNano()/int64(j.coalesceBucket),
	)

	// Check chatter cap (distinct destinations per user in a sliding minute).
	minute := event.Timestamp.Unix() / 60
	byMinute := j.minuteDestinations[event.User]
	if byMinute == nil {
		byMinute = make(map[int64]map[string]struct{})
		j.minuteDestinations[event.User] = byMinute
	}
	for bucket := range byMinute {
		if bucket < minute-1 {
			delete(byMinute, bucket)
		}
	}
	destination := fmt.Sprintf("%s:%d:%s", strings.ToLower(event.DstHost), event.DstPort, event.Network)
	current := byMinute[minute]
	if current == nil {
		current = make(map[string]struct{})
		byMinute[minute] = current
	}
	_, knownDestination := current[destination]
	if !knownDestination && len(current) >= j.chatterCap && !j.chatterOverflow[event.User] {
		// Over cap - collapse into summary
		j.collapseUserActivity(event.User)
		j.chatterOverflow[event.User] = true
	}
	current[destination] = struct{}{}

	// Get or create coalesced event
	coalesced, exists := j.events[key]
	if !exists {
		coalesced = &CoalescedEvent{
			User:       event.User,
			DstHost:    event.DstHost,
			DstPort:    event.DstPort,
			Network:    event.Network,
			InboundTag: event.InboundTag,
			SrcIP:      event.SrcIP,
			FirstSeen:  event.Timestamp,
			Tier:       string(Hot),
		}

		j.userIndex[event.User] = append(j.userIndex[event.User], key)
		j.events[key] = coalesced
	}

	// Update coalesced event
	if event.Timestamp.After(coalesced.LastSeen) {
		coalesced.LastSeen = event.Timestamp
	}
	if event.Timestamp.Before(coalesced.FirstSeen) {
		coalesced.FirstSeen = event.Timestamp
	}
	coalesced.Count++
	coalesced.BytesUplink += event.BytesUplink
	coalesced.BytesDownlink += event.BytesDownlink

	return nil
}

// GetUserActivity returns all activity for a user
func (j *Journal) GetUserActivity(user string) []CoalescedEvent {
	j.mu.RLock()
	defer j.mu.RUnlock()

	keys := j.userIndex[user]
	events := make([]CoalescedEvent, 0, len(keys))

	for _, key := range keys {
		if coalesced, ok := j.events[key]; ok {
			events = append(events, *coalesced)
		}
	}

	return events
}

// GetDistinctDestinations returns distinct destinations visited by a user
func (j *Journal) GetDistinctDestinations(user string) []string {
	j.mu.RLock()
	defer j.mu.RUnlock()

	dests := []string{}
	seen := make(map[string]bool)

	for _, key := range j.userIndex[user] {
		if coalesced, ok := j.events[key]; ok {
			if !seen[coalesced.DstHost] {
				dests = append(dests, coalesced.DstHost)
				seen[coalesced.DstHost] = true
			}
		}
	}

	return dests
}

// collapseUserActivity creates a summary row when chatter cap exceeded
func (j *Journal) collapseUserActivity(user string) {
	// Create a summary entry instead of individual connections
	summary := fmt.Sprintf("%s:__summary__:0:tcp:__summary__:__summary__", user)

	// Remove old entries for this user (keeping count)
	totalCount := 0
	totalUplink := int64(0)
	totalDownlink := int64(0)

	for _, key := range j.userIndex[user] {
		if coalesced, ok := j.events[key]; ok {
			totalCount += coalesced.Count
			totalUplink += coalesced.BytesUplink
			totalDownlink += coalesced.BytesDownlink
			delete(j.events, key)
		}
	}

	// Create summary
	j.events[summary] = &CoalescedEvent{
		User:          user,
		DstHost:       "__summary__",
		Count:         totalCount,
		BytesUplink:   totalUplink,
		BytesDownlink: totalDownlink,
		FirstSeen:     j.clock(),
		LastSeen:      j.clock(),
		Tier:          string(Hot),
	}

	j.userIndex[user] = []string{summary}

	if j.logger != nil {
		j.logger.Info("Chatter cap exceeded, collapsed user activity", slog.String("user", user), slog.Int("total_count", totalCount))
	}
}

// Cleanup removes old events based on retention policy
func (j *Journal) Cleanup(retentionHours int) {
	j.mu.Lock()
	defer j.mu.Unlock()

	cutoff := j.clock().Add(-time.Duration(retentionHours) * time.Hour)
	removed := 0

	for key, coalesced := range j.events {
		if coalesced.LastSeen.Before(cutoff) {
			delete(j.events, key)
			removed++
		}
	}
	for user, keys := range j.userIndex {
		kept := keys[:0]
		for _, key := range keys {
			if _, ok := j.events[key]; ok {
				kept = append(kept, key)
			}
		}
		if len(kept) == 0 {
			delete(j.userIndex, user)
			delete(j.minuteDestinations, user)
			delete(j.chatterOverflow, user)
		} else {
			j.userIndex[user] = kept
		}
	}

	if removed > 0 {
		if j.logger != nil {
			j.logger.Debug("Cleaned up old activity events", slog.Int("count", removed), slog.Duration("retention", time.Duration(retentionHours)*time.Hour))
		}
	}
}

// Statistics returns journal statistics
func (j *Journal) Statistics() map[string]interface{} {
	j.mu.RLock()
	defer j.mu.RUnlock()

	totalUsers := len(j.userIndex)
	totalEvents := 0

	for _, coalesced := range j.events {
		totalEvents += coalesced.Count
	}

	return map[string]interface{}{
		"total_users":     totalUsers,
		"total_events":    totalEvents,
		"coalesc_entries": len(j.events),
		"timestamp":       j.clock().Unix(),
	}
}

// Export exports activity for a user in a compact format
func (j *Journal) Export(user string, format string) ([]byte, error) {
	activity := j.GetUserActivity(user)

	switch format {
	case "json":
		return j.exportJSON(activity)
	case "csv":
		return j.exportCSV(activity)
	case "ndjson", "jsonl":
		return j.exportNDJSON(activity)
	case "json.gz":
		return j.compress(activity, "gzip")
	case "json.zst", "zstd":
		return j.compress(activity, "zstd")
	default:
		return nil, fmt.Errorf("unsupported format: %s", format)
	}
}

// exportJSON exports activity as JSON
func (j *Journal) exportJSON(events []CoalescedEvent) ([]byte, error) {
	return json.Marshal(events)
}

func (j *Journal) exportNDJSON(events []CoalescedEvent) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	for _, event := range events {
		if err := encoder.Encode(event); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}

func (j *Journal) compress(events []CoalescedEvent, format string) ([]byte, error) {
	raw, err := j.exportNDJSON(events)
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	var writer io.WriteCloser
	if format == "zstd" {
		writer, err = zstd.NewWriter(&b)
	} else {
		writer = gzip.NewWriter(&b)
	}
	if err != nil {
		return nil, err
	}
	if _, err = writer.Write(raw); err != nil {
		_ = writer.Close()
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// Search returns matching activity in chronological or reverse chronological order.
func (j *Journal) Search(query string, reverse bool, limit int) []CoalescedEvent {
	j.mu.RLock()
	defer j.mu.RUnlock()
	query = strings.ToLower(strings.TrimSpace(query))
	out := make([]CoalescedEvent, 0, len(j.events))
	for _, event := range j.events {
		if query != "" && !strings.Contains(strings.ToLower(event.User), query) && !strings.Contains(strings.ToLower(event.DstHost), query) {
			continue
		}
		out = append(out, *event)
	}
	sort.SliceStable(out, func(i, k int) bool {
		if reverse {
			return out[i].LastSeen.After(out[k].LastSeen)
		}
		return out[i].LastSeen.Before(out[k].LastSeen)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Compact assigns hot, warm and cold tiers and removes entries outside cold retention.
func (j *Journal) Compact(now time.Time, hot, warm, cold time.Duration) map[Tier][]CoalescedEvent {
	if now.IsZero() {
		now = j.clock()
	}
	if hot <= 0 {
		hot = 14 * 24 * time.Hour
	}
	if warm <= 0 {
		warm = 90 * 24 * time.Hour
	}
	if cold <= 0 {
		cold = 365 * 24 * time.Hour
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	result := map[Tier][]CoalescedEvent{Hot: {}, Warm: {}, Cold: {}}
	for key, event := range j.events {
		age := now.Sub(event.LastSeen)
		if age > cold {
			delete(j.events, key)
			continue
		}
		tier := Cold
		if age <= hot {
			tier = Hot
		} else if age <= warm {
			tier = Warm
		}
		event.Tier = string(tier)
		result[tier] = append(result[tier], *event)
	}
	for user, keys := range j.userIndex {
		kept := keys[:0]
		for _, key := range keys {
			if _, ok := j.events[key]; ok {
				kept = append(kept, key)
			}
		}
		if len(kept) == 0 {
			delete(j.userIndex, user)
		} else {
			j.userIndex[user] = kept
		}
	}
	return result
}

// exportCSV exports activity as CSV
func (j *Journal) exportCSV(events []CoalescedEvent) ([]byte, error) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	if err := w.Write([]string{"user", "host", "port", "network", "first_seen", "last_seen", "count"}); err != nil {
		return nil, err
	}
	for _, event := range events {
		row := []string{event.User, event.DstHost, strconv.Itoa(event.DstPort), event.Network, event.FirstSeen.Format(time.RFC3339Nano), event.LastSeen.Format(time.RFC3339Nano), strconv.Itoa(event.Count)}
		for i, v := range row {
			if len(v) > 0 && (v[0] == '=' || v[0] == '+' || v[0] == '-' || v[0] == '@') {
				row[i] = "'" + v
			}
		}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return b.Bytes(), w.Error()
}

// AddIgnoredHost adds a host to the ignore list
func (j *Journal) AddIgnoredHost(host string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ignoreList[host] = true
}

// GetChatterCapExceeded returns users who have exceeded chatter cap
func (j *Journal) GetChatterCapExceeded() []string {
	j.mu.RLock()
	defer j.mu.RUnlock()

	var exceeded []string
	minute := j.clock().Unix() / 60
	for user, buckets := range j.minuteDestinations {
		if len(buckets[minute]) >= j.chatterCap {
			exceeded = append(exceeded, user)
		}
	}

	return exceeded
}
