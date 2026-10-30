// Package wire defines the versioned Core/Agent JSON contract.
package wire

import "time"

type Sample struct {
	UserID      string    `json:"user_id"`
	Timestamp   time.Time `json:"timestamp"`
	UplinkBps   int64     `json:"uplink_bps"`
	DownlinkBps int64     `json:"downlink_bps"`
	// Speeds are payload bits/second; deltas are payload bytes, never wire bytes.
	UplinkDelta   int64  `json:"uplink_delta,omitempty"`
	DownlinkDelta int64  `json:"downlink_delta,omitempty"`
	IntervalMS    int64  `json:"interval_ms,omitempty"`
	Scope         string `json:"scope,omitempty"` // user (default), or inbound (not per-user)
	Gap           bool   `json:"gap,omitempty"`
	Warming       bool   `json:"warming,omitempty"`
}
type Activity struct {
	UserID      string    `json:"user_id"`
	Timestamp   time.Time `json:"timestamp"`
	SrcIP       string    `json:"src_ip"`
	DstHost     string    `json:"dst_host"`
	DstPort     int       `json:"dst_port"`
	Network     string    `json:"network"`
	InboundTag  string    `json:"inbound_tag"`
	OutboundTag string    `json:"outbound_tag"`
	Result      string    `json:"result"`
}
type Telemetry struct {
	BatchID string     `json:"batch_id"`
	NodeID  string     `json:"node_id"`
	Samples []Sample   `json:"samples"`
	Events  []Activity `json:"events"`
	SentAt  time.Time  `json:"sent_at,omitempty"`
}
type Heartbeat struct {
	NodeID          string    `json:"node_id"`
	Version         string    `json:"version"`
	Timestamp       time.Time `json:"timestamp"`
	MemoryMB        int       `json:"memory_mb"`
	UptimeSeconds   int64     `json:"uptime_seconds"`
	StatsError      string    `json:"stats_error"`
	LogsError       string    `json:"logs_error"`
	DroppedEvents   uint64    `json:"dropped_events"`
	StatsIntervalMS int64     `json:"stats_interval_ms,omitempty"`
	StatsQueryMS    int64     `json:"stats_query_ms,omitempty"`
	StatsUsers      int       `json:"stats_users,omitempty"`
	StatsScope      string    `json:"stats_scope,omitempty"`
	PendingBatches  int       `json:"pending_batches,omitempty"`
}

// Health targets contain synchronized endpoints only, never panel credentials.
type HealthInbound struct {
	Protocol string      `json:"protocol"`
	Network  string      `json:"network"`
	Security string      `json:"security"`
	Port     int         `json:"port"`
	HTTP     *HealthHTTP `json:"http,omitempty"`
}

// HealthHTTP contains only transport probe parameters, never client credentials.
type HealthHTTP struct {
	Path       string `json:"path"`
	Host       string `json:"host,omitempty"`
	ServerName string `json:"server_name,omitempty"`
	HTTP3      bool   `json:"http3,omitempty"`
}
type HealthTarget struct {
	NodeID   string          `json:"node_id"`
	Address  string          `json:"address"`
	Inbounds []HealthInbound `json:"inbounds"`
}
type HealthResult struct {
	NodeID      string    `json:"node_id"`
	Timestamp   time.Time `json:"timestamp"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
	Success     bool      `json:"success"`
	Verified    string    `json:"verified"`
	Generic     bool      `json:"generic"`
	DurationMS  int64     `json:"duration_ms"`
	Protocol    string    `json:"protocol,omitempty"`
	Transport   string    `json:"transport,omitempty"`
	Security    string    `json:"security,omitempty"`
	Port        int       `json:"port,omitempty"`
	Error       string    `json:"error,omitempty"`
}
type HealthReport struct {
	Results []HealthResult `json:"results"`
}
