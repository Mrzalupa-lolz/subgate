package tsdb

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

// LiveStore is a bounded index of independent (identity, node, scope) series.
// Late replay is accepted into history without replacing a newer live value.
type LiveStore struct {
	mu       sync.RWMutex
	latest   map[Series]*liveSeries
	capacity int
}
type Series struct{ User, Node, Scope string }
type liveSeries struct {
	Sample                               wire.Sample
	EMA3Up, EMA3Down, EMA10Up, EMA10Down float64
	Recent                               []wire.Sample
}
type LiveSample struct {
	wire.Sample
	NodeID      string `json:"node_id"`
	EMA3Up      int64  `json:"uplink_ema3_bps"`
	EMA3Down    int64  `json:"downlink_ema3_bps"`
	EMA10Up     int64  `json:"uplink_ema10_bps"`
	EMA10Down   int64  `json:"downlink_ema10_bps"`
	AverageUp   int64  `json:"uplink_avg_bps"`
	AverageDown int64  `json:"downlink_avg_bps"`
	PeakUp      int64  `json:"uplink_peak_bps"`
	PeakDown    int64  `json:"downlink_peak_bps"`
	P95Up       int64  `json:"uplink_p95_bps"`
	P95Down     int64  `json:"downlink_p95_bps"`
	AgeMS       int64  `json:"age_ms"`
	Status      string `json:"status"`
	Scope       string `json:"scope"`
	Gap         bool   `json:"gap"`
	Warming     bool   `json:"warming"`
}

func NewLiveStore(capacity int) *LiveStore {
	if capacity < 1 {
		capacity = 100000
	}
	return &LiveStore{latest: make(map[Series]*liveSeries), capacity: capacity}
}
func (l *LiveStore) Add(node string, samples []wire.Sample, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	// Expired identities are removed before admitting new ones. No unbounded map.
	if len(l.latest) >= l.capacity {
		for key, entry := range l.latest {
			if now.Sub(entry.Sample.Timestamp) > time.Minute {
				delete(l.latest, key)
			}
		}
	}
	for _, sample := range samples {
		if sample.Scope == "" {
			sample.Scope = "user"
		}
		if sample.IntervalMS <= 0 {
			sample.IntervalMS = 1000
		}
		key := Series{sample.UserID, node, sample.Scope}
		entry := l.latest[key]
		if entry != nil && !sample.Timestamp.After(entry.Sample.Timestamp) {
			continue
		}
		if now.Sub(sample.Timestamp) > time.Minute || sample.Timestamp.After(now.Add(time.Second)) {
			continue
		}
		if entry == nil {
			if len(l.latest) >= l.capacity {
				continue
			}
			entry = &liveSeries{}
			l.latest[key] = entry
		}
		dt := sample.Timestamp.Sub(entry.Sample.Timestamp).Seconds()
		if entry.Sample.Timestamp.IsZero() || sample.Gap || dt > 15 {
			entry.EMA3Up, entry.EMA10Up = float64(sample.UplinkBps), float64(sample.UplinkBps)
			entry.EMA3Down, entry.EMA10Down = float64(sample.DownlinkBps), float64(sample.DownlinkBps)
		} else {
			a3, a10 := 1-math.Exp(-dt/3), 1-math.Exp(-dt/10)
			entry.EMA3Up += a3 * (float64(sample.UplinkBps) - entry.EMA3Up)
			entry.EMA3Down += a3 * (float64(sample.DownlinkBps) - entry.EMA3Down)
			entry.EMA10Up += a10 * (float64(sample.UplinkBps) - entry.EMA10Up)
			entry.EMA10Down += a10 * (float64(sample.DownlinkBps) - entry.EMA10Down)
		}
		entry.Sample = sample
		// The precise minute series lives in SQLite. Keep at most 12 observations
		// here for smooth live numbers, independent of the number of idle users.
		if len(entry.Recent) == 12 {
			copy(entry.Recent, entry.Recent[1:])
			entry.Recent = entry.Recent[:11]
		}
		entry.Recent = append(entry.Recent, sample)
	}
}
func (l *LiveStore) Snapshot(now time.Time) []LiveSample {
	l.mu.RLock()
	defer l.mu.RUnlock()
	result := make([]LiveSample, 0, len(l.latest))
	for key, e := range l.latest {
		age := now.Sub(e.Sample.Timestamp).Milliseconds()
		if age < -1000 || age > 15000 {
			continue
		}
		freshFor := max(int64(3000), e.Sample.IntervalMS*2+500)
		status := "live"
		if age > freshFor || e.Sample.Gap {
			status = "gap"
		}
		if e.Sample.Warming {
			status = "warming"
		}
		up, peakUp, p95Up := recentDistribution(e.Recent, true)
		down, peakDown, p95Down := recentDistribution(e.Recent, false)
		s := LiveSample{Sample: e.Sample, NodeID: key.Node, EMA3Up: int64(e.EMA3Up), EMA3Down: int64(e.EMA3Down), EMA10Up: int64(e.EMA10Up), EMA10Down: int64(e.EMA10Down), AverageUp: up, AverageDown: down, PeakUp: peakUp, PeakDown: peakDown, P95Up: p95Up, P95Down: p95Down, AgeMS: max(0, age), Status: status, Scope: key.Scope, Gap: e.Sample.Gap, Warming: e.Sample.Warming}
		result = append(result, s)
	}
	return result
}

func recentDistribution(samples []wire.Sample, uplink bool) (average, peak, p95 int64) {
	values := make([]int64, 0, len(samples))
	for _, sample := range samples {
		// A gap is a missing interval. It must not be represented as a zero
		// measurement because that would dilute averages and hide outages.
		if sample.Gap {
			continue
		}
		if uplink {
			values = append(values, sample.UplinkBps)
		} else {
			values = append(values, sample.DownlinkBps)
		}
	}
	return Distribution(values)
}
func (l *LiveStore) RemoveNode(node string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key := range l.latest {
		if key.Node == node {
			delete(l.latest, key)
		}
	}
}

// Distribution is shared with the history rollup path. Missing measurements
// never become zeroes and cannot dilute an average.
func Distribution(samples []int64) (average, peak, p95 int64) {
	if len(samples) == 0 {
		return
	}
	for _, v := range samples {
		average += v
		peak = max(peak, v)
	}
	average /= int64(len(samples))
	values := append([]int64(nil), samples...)
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	p95 = values[int(math.Ceil(float64(len(values))*.95))-1]
	return
}
