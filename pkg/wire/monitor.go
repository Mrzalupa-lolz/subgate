package wire

import "time"

// Monitor reports contain metadata only. Dedicated interfaces must carry one
// canonical Remnawave account; shared/NAT interfaces are observation-only.
type Binding struct {
	UserID          string    `json:"user_id"`
	Interface       string    `json:"interface"`
	ShapeInterfaces []string  `json:"shape_interfaces"`
	Exclusive       bool      `json:"exclusive"`
	ValidFrom       time.Time `json:"valid_from"`
	ExpiresAt       time.Time `json:"expires_at"`
}

type DPIFlow struct {
	ID         string    `json:"id"`
	Interface  string    `json:"interface"`
	Protocol   string    `json:"protocol"`
	Confidence string    `json:"confidence"` // dpi, guessed, unknown
	Version    string    `json:"version"`
	Timestamp  time.Time `json:"timestamp"`
	Bytes      uint64    `json:"bytes"`
	Packets    uint64    `json:"packets"`
	Dropped    uint64    `json:"dropped"`
	Truncated  bool      `json:"truncated"`
}

type Resources struct {
	Timestamp    time.Time `json:"timestamp"`
	IntervalMS   int64     `json:"interval_ms"`
	CapacityMbps int       `json:"capacity_mbps"`
	RXBytes      uint64    `json:"rx_bytes"`
	TXBytes      uint64    `json:"tx_bytes"`
	Dropped      uint64    `json:"dropped"`
	CPUPercent   float64   `json:"cpu_percent"`
}

type MonitorReport struct {
	NodeID         string     `json:"node_id"`
	Resources      *Resources `json:"resources,omitempty"`
	Bindings       []Binding  `json:"bindings"`
	Flows          []DPIFlow  `json:"flows"`
	CollectorError string     `json:"collector_error,omitempty"`
}

type ShaperAction struct {
	ID        string    `json:"id"`
	NodeID    string    `json:"node_id"`
	UserID    string    `json:"user_id"`
	Mbps      int       `json:"mbps"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Reason    string    `json:"reason"`
	Protocol  string    `json:"protocol"`
}

type ShaperResult struct {
	ID     string `json:"id"`
	Status string `json:"status"` // applied, removed, failed
	Error  string `json:"error,omitempty"`
}
