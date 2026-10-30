// Package notify delivers operational alerts to configured destinations.
// Telegram is implemented directly against the Bot API so the Core binary
// does not need a bot framework or a long-lived polling goroutine.
package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Severity string

const (
	Info     Severity = "info"
	Warning  Severity = "warning"
	Critical Severity = "critical"
)

type Alert struct {
	Category string            `json:"category"`
	Severity Severity          `json:"severity"`
	Title    string            `json:"title"`
	Message  string            `json:"message"`
	NodeID   string            `json:"node_id,omitempty"`
	Dedupe   string            `json:"dedupe_key,omitempty"`
	Fields   map[string]string `json:"fields,omitempty"`
	Created  time.Time         `json:"created_at"`
}

func (a Alert) String() string {
	severity := string(a.Severity)
	if severity == "" {
		severity = string(Info)
	}
	if a.Title == "" {
		a.Title = "SubGate alert"
	}
	if a.Message == "" {
		return "[" + strings.ToUpper(severity) + "] " + a.Title
	}
	return "[" + strings.ToUpper(severity) + "] " + a.Title + "\n" + a.Message
}

type Sender interface {
	Send(context.Context, Alert) error
}

type Telegram struct {
	Token    string
	ChatIDs  []int64
	Client   *http.Client
	Logger   *slog.Logger
	Endpoint string
}

func (t *Telegram) Send(ctx context.Context, alert Alert) error {
	if t == nil || t.Token == "" || len(t.ChatIDs) == 0 {
		return fmt.Errorf("telegram notifier is not configured")
	}
	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = "https://api.telegram.org"
	}
	endpoint = strings.TrimRight(endpoint, "/") + "/bot" + url.PathEscape(t.Token) + "/sendMessage"
	for _, chatID := range t.ChatIDs {
		payload := map[string]any{"chat_id": chatID, "text": alert.String(), "disable_web_page_preview": true}
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("telegram request failed: %w", err)
		}
		responseBody, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("telegram returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(responseBody)))
		}
		var result struct {
			OK          bool   `json:"ok"`
			Description string `json:"description"`
		}
		if json.Unmarshal(responseBody, &result) != nil || !result.OK {
			return fmt.Errorf("telegram rejected message: %s", result.Description)
		}
	}
	return nil
}

type Dispatcher struct {
	mu       sync.Mutex
	senders  []Sender
	cooldown time.Duration
	quiet    QuietHours
	last     map[string]time.Time
	logger   *slog.Logger
	clock    func() time.Time
}

type QuietHours struct {
	Enabled   bool
	StartHour int
	EndHour   int
	Location  *time.Location
}

func NewDispatcher(logger *slog.Logger, cooldown time.Duration, quiet QuietHours, senders ...Sender) *Dispatcher {
	if cooldown <= 0 {
		cooldown = 5 * time.Minute
	}
	if quiet.Location == nil {
		quiet.Location = time.UTC
	}
	if quiet.StartHour < 0 || quiet.StartHour > 23 {
		quiet.StartHour = 22
	}
	if quiet.EndHour < 0 || quiet.EndHour > 23 {
		quiet.EndHour = 7
	}
	return &Dispatcher{senders: append([]Sender(nil), senders...), cooldown: cooldown, quiet: quiet, last: map[string]time.Time{}, logger: logger, clock: time.Now}
}

func (d *Dispatcher) Add(sender Sender) {
	if sender == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.senders = append(d.senders, sender)
}

func (d *Dispatcher) InQuietHours(at time.Time) bool {
	d.mu.Lock()
	quiet := d.quiet
	d.mu.Unlock()
	return inQuietHours(quiet, at)
}

func inQuietHours(quiet QuietHours, at time.Time) bool {
	if !quiet.Enabled || quiet.StartHour == quiet.EndHour {
		return false
	}
	hour := at.In(quiet.Location).Hour()
	if quiet.StartHour < quiet.EndHour {
		return hour >= quiet.StartHour && hour < quiet.EndHour
	}
	return hour >= quiet.StartHour || hour < quiet.EndHour
}

func (d *Dispatcher) Send(ctx context.Context, alert Alert) error {
	if alert.Created.IsZero() {
		alert.Created = d.clock().UTC()
	}
	if alert.Severity == "" {
		alert.Severity = Info
	}
	key := alert.Dedupe
	if key == "" {
		key = alert.Category + "\x00" + alert.NodeID + "\x00" + alert.Title
	}
	d.mu.Lock()
	if previous, ok := d.last[key]; ok && alert.Created.Sub(previous) < d.cooldown {
		d.mu.Unlock()
		return nil
	}
	if inQuietHours(d.quiet, alert.Created) && alert.Severity != Critical {
		d.mu.Unlock()
		return nil
	}
	senders := append([]Sender(nil), d.senders...)
	d.last[key] = alert.Created
	d.mu.Unlock()
	if len(senders) == 0 {
		return nil
	}
	var first error
	for _, sender := range senders {
		if err := sender.Send(ctx, alert); err != nil {
			if first == nil {
				first = err
			}
			if d.logger != nil {
				d.logger.Warn("alert delivery failed", "category", alert.Category, "error", err)
			}
		}
	}
	return first
}

// ParseChatIDs parses the comma-separated form used by environment variables.
func ParseChatIDs(raw string) []int64 {
	var result []int64
	for _, part := range strings.Split(raw, ",") {
		if value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
			result = append(result, value)
		}
	}
	return result
}
