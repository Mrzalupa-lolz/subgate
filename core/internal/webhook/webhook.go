// Package webhook handles Remnawave panel webhook events
package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"
)

// WebhookEvent represents a panel webhook event
type WebhookEvent struct {
	Type      string          `json:"type"` // 'user_updated', 'node_updated', 'config_changed', etc.
	Timestamp time.Time       `json:"timestamp"`
	EventID   string          `json:"event_id"` // For deduplication
	Data      json.RawMessage `json:"data"`
}

// Handler processes webhook events
type Handler struct {
	secretKey       string
	processedEvents map[string]time.Time // event_id -> processed_time for dedup
	mu              sync.RWMutex
	logger          *slog.Logger
	listeners       []func(*WebhookEvent)
}

// NewHandler creates a new webhook handler
func NewHandler(secretKey string, logger *slog.Logger) *Handler {
	return &Handler{
		secretKey:       secretKey,
		processedEvents: make(map[string]time.Time),
		logger:          logger,
		listeners:       []func(*WebhookEvent){},
	}
}

// VerifySignature verifies the webhook signature
func (h *Handler) VerifySignature(body []byte, signature string) bool {
	expected := h.computeSignature(body)
	return hmac.Equal([]byte(signature), []byte(expected))
}

// computeSignature computes the HMAC-SHA256 signature
func (h *Handler) computeSignature(body []byte) string {
	hash := hmac.New(sha256.New, []byte(h.secretKey))
	hash.Write(body)
	return hex.EncodeToString(hash.Sum(nil))
}

// ProcessWebhook processes an incoming webhook
func (h *Handler) ProcessWebhook(bodyReader io.Reader, signature string) (*WebhookEvent, error) {
	// Read body
	body, err := io.ReadAll(bodyReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	// Verify signature
	if !h.VerifySignature(body, signature) {
		return nil, fmt.Errorf("invalid signature")
	}

	// Parse event
	var event WebhookEvent
	if err := json.Unmarshal(body, &event); err != nil {
		return nil, fmt.Errorf("failed to parse event: %w", err)
	}

	// Check for replay attacks (deduplication)
	if h.isDuplicate(event.EventID) {
		h.logger.Warn("duplicate webhook event", slog.String("event_id", event.EventID))
		return nil, fmt.Errorf("duplicate event")
	}

	// Mark as processed
	h.recordProcessed(event.EventID)

	h.logger.Info("webhook event processed",
		slog.String("type", event.Type),
		slog.String("event_id", event.EventID),
	)

	// Notify listeners
	for _, listener := range h.listeners {
		go listener(&event)
	}

	return &event, nil
}

// isDuplicate checks if an event was already processed
func (h *Handler) isDuplicate(eventID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	_, exists := h.processedEvents[eventID]
	return exists
}

// recordProcessed marks an event as processed
func (h *Handler) recordProcessed(eventID string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.processedEvents[eventID] = time.Now()

	// Cleanup old entries (older than 24h)
	cutoff := time.Now().Add(-24 * time.Hour)
	for id, ts := range h.processedEvents {
		if ts.Before(cutoff) {
			delete(h.processedEvents, id)
		}
	}
}

// AddListener registers a listener for webhook events
func (h *Handler) AddListener(listener func(*WebhookEvent)) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.listeners = append(h.listeners, listener)
}

// Event handlers for specific event types

// UserUpdatedEvent represents user update event data
type UserUpdatedEvent struct {
	UserID    string `json:"user_id"`
	ShortUUID string `json:"short_uuid"`
	Status    string `json:"status"` // 'active', 'disabled', 'expired'
	Email     string `json:"email"`
	Squad     string `json:"squad"`
}

// NodeUpdatedEvent represents node update event data
type NodeUpdatedEvent struct {
	NodeID string   `json:"node_id"`
	Status string   `json:"status"` // 'online', 'offline', 'maintenance'
	Tags   []string `json:"tags"`
}

// ConfigChangedEvent represents config change event data
type ConfigChangedEvent struct {
	ConfigType string          `json:"config_type"` // 'global', 'tier', 'policy'
	Changes    json.RawMessage `json:"changes"`
}

// ParseUserUpdatedEvent parses user updated event
func ParseUserUpdatedEvent(rawData json.RawMessage) (*UserUpdatedEvent, error) {
	var event UserUpdatedEvent
	if err := json.Unmarshal(rawData, &event); err != nil {
		return nil, fmt.Errorf("failed to parse user updated event: %w", err)
	}
	return &event, nil
}

// ParseNodeUpdatedEvent parses node updated event
func ParseNodeUpdatedEvent(rawData json.RawMessage) (*NodeUpdatedEvent, error) {
	var event NodeUpdatedEvent
	if err := json.Unmarshal(rawData, &event); err != nil {
		return nil, fmt.Errorf("failed to parse node updated event: %w", err)
	}
	return &event, nil
}

// ParseConfigChangedEvent parses config changed event
func ParseConfigChangedEvent(rawData json.RawMessage) (*ConfigChangedEvent, error) {
	var event ConfigChangedEvent
	if err := json.Unmarshal(rawData, &event); err != nil {
		return nil, fmt.Errorf("failed to parse config changed event: %w", err)
	}
	return &event, nil
}

// Statistics returns webhook handler statistics
func (h *Handler) Statistics() map[string]interface{} {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return map[string]interface{}{
		"processed_events": len(h.processedEvents),
		"listeners":        len(h.listeners),
	}
}
