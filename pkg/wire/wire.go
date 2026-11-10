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
