// Package logs classifies Xray error and debug lines into stable operational
// signals. Parsing is intentionally tolerant: an unknown line is returned as
// an unparsed signal instead of being discarded or causing a worker to stop.
package logs

import (
	"encoding/json"
	"net"
	"regexp"
	"strings"
	"sync"
	"time"
)

type Level string

const (
	Debug   Level = "debug"
	Info    Level = "info"
	Warning Level = "warning"
	Error   Level = "error"
	Unknown Level = "unknown"
)

type Class string

const (
	Handshake    Class = "handshake"
	DNS          Class = "dns"
	Timeout      Class = "timeout"
	Reset        Class = "reset"
	Auth         Class = "auth"
	Transport    Class = "transport"
	UnknownClass Class = "unknown"
)

type Signal struct {
	Timestamp   time.Time `json:"timestamp"`
	Level       Level     `json:"level"`
	Class       Class     `json:"class"`
	Message     string    `json:"message"`
	SourceIP    string    `json:"source_ip,omitempty"`
	InboundTag  string    `json:"inbound_tag,omitempty"`
	OutboundTag string    `json:"outbound_tag,omitempty"`
	Parsed      bool      `json:"parsed"`
}

var (
	levelPattern = regexp.MustCompile(`(?i)\b(debug|info|warning|warn|error|fatal)\b`)
	ipPattern    = regexp.MustCompile(`(?:from|source|src|client)[ =:]+(?:tcp:|udp:)?\[?([0-9a-fA-F:.]+)\]?`)
	tagPattern   = regexp.MustCompile(`(?i)(?:inbound|inboundTag|inbound tag)[ =:]+["']?([\w.-]+)|(?:outbound|outboundTag|outbound tag)[ =:]+["']?([\w.-]+)`)
)

// ParseLine parses JSON structured logs and common Xray text lines.
func ParseLine(line string, at time.Time) Signal {
	line = strings.TrimSpace(line)
	if at.IsZero() {
		at = time.Now().UTC()
	}
	if line == "" {
		return Signal{Timestamp: at, Level: Unknown, Class: UnknownClass, Parsed: false}
	}
	var structured struct {
		Time      string `json:"time"`
		Timestamp string `json:"timestamp"`
		Level     string `json:"level"`
		Message   string `json:"message"`
		Error     string `json:"error"`
		Source    string `json:"source"`
		Inbound   string `json:"inbound"`
		Outbound  string `json:"outbound"`
	}
	message := line
	if strings.HasPrefix(line, "{") && json.Unmarshal([]byte(line), &structured) == nil {
		if structured.Time != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, structured.Time); err == nil {
				at = parsed
			}
		} else if structured.Timestamp != "" {
			if parsed, err := time.Parse(time.RFC3339Nano, structured.Timestamp); err == nil {
				at = parsed
			}
		}
		message = structured.Message
		if message == "" {
			message = structured.Error
		}
		if message == "" {
			message = line
		}
	}
	signal := Signal{Timestamp: at, Message: message, Level: parseLevel(structured.Level, line), Class: classify(message), Parsed: true}
	if match := ipPattern.FindStringSubmatch(message); len(match) > 1 && net.ParseIP(match[1]) != nil {
		signal.SourceIP = match[1]
	}
	if structured.Source != "" && net.ParseIP(structured.Source) != nil {
		signal.SourceIP = structured.Source
	}
	signal.InboundTag, signal.OutboundTag = structured.Inbound, structured.Outbound
	if signal.InboundTag == "" || signal.OutboundTag == "" {
		for _, match := range tagPattern.FindAllStringSubmatch(message, -1) {
			if signal.InboundTag == "" && match[1] != "" {
				signal.InboundTag = match[1]
			}
			if signal.OutboundTag == "" && match[2] != "" {
				signal.OutboundTag = match[2]
			}
		}
	}
	return signal
}

func parseLevel(value, line string) Level {
	raw := strings.ToLower(strings.TrimSpace(value))
	if raw == "" {
		match := levelPattern.FindStringSubmatch(line)
		if len(match) > 1 {
			raw = strings.ToLower(match[1])
		}
	}
	switch raw {
	case "debug":
		return Debug
	case "info":
		return Info
	case "warn", "warning":
		return Warning
	case "error", "fatal":
		return Error
	default:
		return Unknown
	}
}

func classify(message string) Class {
	lower := strings.ToLower(message)
	switch {
	case strings.Contains(lower, "handshake"), strings.Contains(lower, "tls") && strings.Contains(lower, "fail"):
		return Handshake
	case strings.Contains(lower, "dns"), strings.Contains(lower, "no such host"), strings.Contains(lower, "name resolution"):
		return DNS
	case strings.Contains(lower, "timeout"), strings.Contains(lower, "timed out"), strings.Contains(lower, "deadline exceeded"):
		return Timeout
	case strings.Contains(lower, "reset by peer"), strings.Contains(lower, "broken pipe"), strings.Contains(lower, "connection reset"):
		return Reset
	case strings.Contains(lower, "auth"), strings.Contains(lower, "unauthorized"), strings.Contains(lower, "invalid user"):
		return Auth
	case strings.Contains(lower, "connection"), strings.Contains(lower, "network"), strings.Contains(lower, "transport"):
		return Transport
	default:
		return UnknownClass
	}
}

type Aggregate struct {
	Class Class     `json:"class"`
	Level Level     `json:"level"`
	Count int64     `json:"count"`
	Last  time.Time `json:"last"`
}

type Aggregator struct {
	mu    sync.Mutex
	items map[string]Aggregate
}

func NewAggregator() *Aggregator { return &Aggregator{items: map[string]Aggregate{}} }
func (a *Aggregator) Add(signal Signal) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	key := string(signal.Class) + "\x00" + string(signal.Level) + "\x00" + signal.InboundTag + "\x00" + signal.OutboundTag
	item := a.items[key]
	item.Class, item.Level, item.Last = signal.Class, signal.Level, signal.Timestamp
	item.Count++
	a.items[key] = item
}
func (a *Aggregator) Snapshot() []Aggregate {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]Aggregate, 0, len(a.items))
	for _, item := range a.items {
		out = append(out, item)
	}
	return out
}
