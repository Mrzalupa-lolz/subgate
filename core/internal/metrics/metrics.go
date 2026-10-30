// Package metrics provides a small dependency-free Prometheus registry.
//
// The registry is intentionally limited to the primitives SubGate exposes:
// counters, gauges, and histograms.  Values are safe to update from request
// handlers and background workers and are rendered in the Prometheus text
// format without holding locks while user code runs.
package metrics

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Counter is a monotonically increasing metric.
type Counter struct{ value atomic.Uint64 }

func (c *Counter) Add(value uint64) { c.value.Add(value) }
func (c *Counter) Inc()             { c.Add(1) }
func (c *Counter) Value() uint64    { return c.value.Load() }

// Gauge is a metric that can move in either direction.
type Gauge struct{ value atomic.Int64 }

func (g *Gauge) Set(value int64) { g.value.Store(value) }
func (g *Gauge) Add(value int64) { g.value.Add(value) }
func (g *Gauge) Value() int64    { return g.value.Load() }

// Histogram stores cumulative buckets, a count and a sum. Values are in the
// unit selected by the caller (usually seconds or bytes).
type Histogram struct {
	mu      sync.Mutex
	limits  []float64
	buckets []uint64
	count   uint64
	sum     float64
}

func newHistogram(limits []float64) *Histogram {
	copyLimits := append([]float64(nil), limits...)
	sort.Float64s(copyLimits)
	return &Histogram{limits: copyLimits, buckets: make([]uint64, len(copyLimits)+1)}
}

func (h *Histogram) Observe(value float64) {
	if math.IsNaN(value) {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	index := sort.SearchFloat64s(h.limits, value)
	if index >= len(h.buckets) {
		index = len(h.buckets) - 1
	}
	h.buckets[index]++
	h.count++
	h.sum += value
}

// Snapshot returns cumulative bucket counts, count and sum.
func (h *Histogram) Snapshot() (limits []float64, buckets []uint64, count uint64, sum float64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]float64(nil), h.limits...), append([]uint64(nil), h.buckets...), h.count, h.sum
}

type counterMetric struct {
	help  string
	value *Counter
}
type gaugeMetric struct {
	help  string
	value *Gauge
}
type histogramMetric struct {
	help  string
	value *Histogram
}

// Registry contains named metrics. Registering the same name with the same
// type returns the existing metric; a type mismatch is rejected.
type Registry struct {
	mu         sync.RWMutex
	counters   map[string]counterMetric
	gauges     map[string]gaugeMetric
	histograms map[string]histogramMetric
}

func NewRegistry() *Registry {
	return &Registry{counters: map[string]counterMetric{}, gauges: map[string]gaugeMetric{}, histograms: map[string]histogramMetric{}}
}

func validName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == ':' || (i > 0 && r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	return true
}

func (r *Registry) Counter(name, help string) (*Counter, error) {
	if !validName(name) {
		return nil, fmt.Errorf("invalid metric name %q", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if metric, ok := r.counters[name]; ok {
		return metric.value, nil
	}
	if _, ok := r.gauges[name]; ok {
		return nil, fmt.Errorf("metric %q already registered as gauge", name)
	}
	if _, ok := r.histograms[name]; ok {
		return nil, fmt.Errorf("metric %q already registered as histogram", name)
	}
	value := &Counter{}
	r.counters[name] = counterMetric{help: help, value: value}
	return value, nil
}

func (r *Registry) MustCounter(name, help string) *Counter {
	value, err := r.Counter(name, help)
	if err != nil {
		panic(err)
	}
	return value
}

func (r *Registry) Gauge(name, help string) (*Gauge, error) {
	if !validName(name) {
		return nil, fmt.Errorf("invalid metric name %q", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if metric, ok := r.gauges[name]; ok {
		return metric.value, nil
	}
	if _, ok := r.counters[name]; ok {
		return nil, fmt.Errorf("metric %q already registered as counter", name)
	}
	if _, ok := r.histograms[name]; ok {
		return nil, fmt.Errorf("metric %q already registered as histogram", name)
	}
	value := &Gauge{}
	r.gauges[name] = gaugeMetric{help: help, value: value}
	return value, nil
}

func (r *Registry) MustGauge(name, help string) *Gauge {
	value, err := r.Gauge(name, help)
	if err != nil {
		panic(err)
	}
	return value
}

func (r *Registry) Histogram(name, help string, limits []float64) (*Histogram, error) {
	if !validName(name) {
		return nil, fmt.Errorf("invalid metric name %q", name)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if metric, ok := r.histograms[name]; ok {
		return metric.value, nil
	}
	if _, ok := r.counters[name]; ok {
		return nil, fmt.Errorf("metric %q already registered as counter", name)
	}
	if _, ok := r.gauges[name]; ok {
		return nil, fmt.Errorf("metric %q already registered as gauge", name)
	}
	value := newHistogram(limits)
	r.histograms[name] = histogramMetric{help: help, value: value}
	return value, nil
}

func (r *Registry) MustHistogram(name, help string, limits []float64) *Histogram {
	value, err := r.Histogram(name, help, limits)
	if err != nil {
		panic(err)
	}
	return value
}

// Handler exposes the registry in Prometheus's text exposition format.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(r.Render()))
	})
}

func (r *Registry) Render() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var names []string
	for name := range r.counters {
		names = append(names, name)
	}
	for name := range r.gauges {
		names = append(names, name)
	}
	for name := range r.histograms {
		names = append(names, name)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, name := range names {
		if metric, ok := r.counters[name]; ok {
			writeHelpType(&b, name, metric.help, "counter")
			fmt.Fprintf(&b, "%s %d\n", name, metric.value.Value())
			continue
		}
		if metric, ok := r.gauges[name]; ok {
			writeHelpType(&b, name, metric.help, "gauge")
			fmt.Fprintf(&b, "%s %d\n", name, metric.value.Value())
			continue
		}
		metric := r.histograms[name]
		writeHelpType(&b, name, metric.help, "histogram")
		limits, buckets, count, sum := metric.value.Snapshot()
		var cumulative uint64
		for i, limit := range limits {
			cumulative += buckets[i]
			fmt.Fprintf(&b, "%s_bucket{le=\"%g\"} %d\n", name, limit, cumulative)
		}
		cumulative += buckets[len(buckets)-1]
		fmt.Fprintf(&b, "%s_bucket{le=\"+Inf\"} %d\n%s_count %d\n%s_sum %g\n", name, cumulative, name, count, name, sum)
	}
	return b.String()
}

func writeHelpType(b *strings.Builder, name, help, typ string) {
	if help == "" {
		help = name
	}
	help = strings.ReplaceAll(strings.ReplaceAll(help, "\\", "\\\\"), "\n", " ")
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s %s\n", name, help, name, typ)
}
