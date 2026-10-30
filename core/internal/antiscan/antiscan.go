// Package antiscan implements scanner detection (F12)
package antiscan

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ScannerSignal represents a potential scanner detection
type ScannerSignal struct {
	Type            string // 'uuid_enum', 'rate_anomaly', 'ua_scanner', 'handshake_burst'
	SourceIP        string
	ASN             int
	Country         string
	Count           int
	FirstSeen       time.Time
	LastSeen        time.Time
	EscalationLevel Level
}

// Level represents escalation level
type Level int

const (
	LevelNone Level = iota
	LevelTarpit
	LevelDeny
	LevelExport
)

// Engine detects scanner activity
type Engine struct {
	ipActivity map[string]*IPActivity // IP -> activity tracker
	mu         sync.RWMutex
	logger     *slog.Logger
	allowlist  map[string]bool
}

// IPActivity tracks activity for an IP
type IPActivity struct {
	IP                string
	FirstSeen         time.Time
	LastSeen          time.Time
	RequestCount      int
	UniqueUUIDs       int
	ScannerUACount    int
	HandshakeFailures int
	EscalationLevel   Level
	TarpitUntil       time.Time
	UUIDs             map[string]struct{}
	RequestTimes      []time.Time
	HandshakeTimes    []time.Time
}

// NewEngine creates a new scanner detection engine
func NewEngine(logger *slog.Logger) *Engine {
	return &Engine{
		ipActivity: make(map[string]*IPActivity),
		logger:     logger,
		allowlist:  make(map[string]bool),
	}
}

// AnalyzeRequest analyzes a subscription request for scanner signatures
func (e *Engine) AnalyzeRequest(clientIP, shortUUID, userAgent string) ([]ScannerSignal, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var signals []ScannerSignal

	// Get or create IP activity record
	activity, exists := e.ipActivity[clientIP]
	if !exists {
		activity = &IPActivity{
			IP:        clientIP,
			FirstSeen: time.Now(),
			UUIDs:     make(map[string]struct{}),
		}
		e.ipActivity[clientIP] = activity
	}
	now := time.Now()
	if activity.UUIDs == nil {
		activity.UUIDs = make(map[string]struct{})
	}
	cutoff := now.Add(-time.Minute)
	keptRequests := activity.RequestTimes[:0]
	for _, at := range activity.RequestTimes {
		if !at.Before(cutoff) {
			keptRequests = append(keptRequests, at)
		}
	}
	activity.RequestTimes = append(keptRequests, now)

	// Check allowlist
	if e.allowlist[clientIP] {
		activity.LastSeen = now
		activity.RequestCount++
		return nil, nil
	}

	activity.LastSeen = now
	activity.RequestCount++

	// Detector 1: UUID enumeration (multiple unique UUIDs from same IP)
	if shortUUID != "" {
		activity.UUIDs[shortUUID] = struct{}{}
		activity.UniqueUUIDs = len(activity.UUIDs)
		if activity.UniqueUUIDs > 50 && activity.RequestCount < 100 {
			signals = append(signals, ScannerSignal{
				Type:            "uuid_enum",
				SourceIP:        clientIP,
				Count:           activity.UniqueUUIDs,
				FirstSeen:       activity.FirstSeen,
				LastSeen:        now,
				EscalationLevel: LevelTarpit,
			})
		}
	}

	// Detector 2: Rate anomaly (100+ requests/min from same IP)
	if len(activity.RequestTimes) > 100 {
		signals = append(signals, ScannerSignal{
			Type:            "rate_anomaly",
			SourceIP:        clientIP,
			Count:           activity.RequestCount,
			FirstSeen:       activity.FirstSeen,
			LastSeen:        now,
			EscalationLevel: LevelTarpit,
		})
	}

	// Detector 3: Scanner User-Agent detection
	if isScannerUA(userAgent) {
		activity.ScannerUACount++
		signals = append(signals, ScannerSignal{
			Type:            "ua_scanner",
			SourceIP:        clientIP,
			Count:           activity.ScannerUACount,
			FirstSeen:       activity.FirstSeen,
			LastSeen:        now,
			EscalationLevel: LevelDeny,
		})
	}

	// Update escalation level
	if len(signals) > 0 {
		for _, sig := range signals {
			if sig.EscalationLevel > activity.EscalationLevel {
				activity.EscalationLevel = sig.EscalationLevel
			}
		}

		e.logger.Warn("Scanner signals detected",
			slog.String("ip", clientIP),
			slog.Int("signal_count", len(signals)),
		)
	}

	return signals, nil
}

// AnalyzeHandshakeFailures analyzes failed handshake patterns
func (e *Engine) AnalyzeHandshakeFailures(clientIP string, count int, duration time.Duration) *ScannerSignal {
	e.mu.Lock()
	defer e.mu.Unlock()

	activity, exists := e.ipActivity[clientIP]
	if !exists {
		activity = &IPActivity{
			IP:        clientIP,
			FirstSeen: time.Now(),
			UUIDs:     make(map[string]struct{}),
		}
		e.ipActivity[clientIP] = activity
	}

	now := time.Now()
	if count < 0 {
		count = 0
	}
	for i := 0; i < count; i++ {
		activity.HandshakeTimes = append(activity.HandshakeTimes, now)
	}
	cutoff := now.Add(-time.Minute)
	kept := activity.HandshakeTimes[:0]
	for _, at := range activity.HandshakeTimes {
		if !at.Before(cutoff) {
			kept = append(kept, at)
		}
	}
	activity.HandshakeTimes = kept
	activity.HandshakeFailures = len(kept)

	// Detector 4: Handshake failure burst (100+ failures in 60s)
	if activity.HandshakeFailures > 100 && duration < 60*time.Second {
		e.logger.Warn("Handshake failure burst detected",
			slog.String("ip", clientIP),
			slog.Int("count", activity.HandshakeFailures),
		)

		activity.EscalationLevel = LevelExport

		return &ScannerSignal{
			Type:            "handshake_burst",
			SourceIP:        clientIP,
			Count:           activity.HandshakeFailures,
			FirstSeen:       activity.FirstSeen,
			LastSeen:        now,
			EscalationLevel: LevelExport,
		}
	}

	return nil
}

// GetActivity returns activity for an IP
func (e *Engine) GetActivity(clientIP string) *IPActivity {
	e.mu.RLock()
	defer e.mu.RUnlock()

	activity, exists := e.ipActivity[clientIP]
	if !exists {
		return nil
	}

	return activity
}

// ApplyTarpit temporarily tarpits an IP (delay response)
func (e *Engine) ApplyTarpit(clientIP string, duration time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()

	activity, exists := e.ipActivity[clientIP]
	if !exists {
		return
	}

	activity.TarpitUntil = time.Now().Add(duration)
	e.logger.Info("Tarpit applied", slog.String("ip", clientIP), slog.Duration("duration", duration))
}

// ShouldTarpit checks if an IP should be tarpited
func (e *Engine) ShouldTarpit(clientIP string) (bool, time.Duration) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	activity, exists := e.ipActivity[clientIP]
	if !exists {
		return false, 0
	}

	if time.Now().Before(activity.TarpitUntil) {
		remaining := time.Until(activity.TarpitUntil)
		return true, remaining
	}

	return false, 0
}

// Allowlist adds an IP to the allowlist
func (e *Engine) Allowlist(clientIP string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	e.allowlist[clientIP] = true
	e.logger.Info("IP allowlisted", slog.String("ip", clientIP))
}

// RemoveFromAllowlist removes an IP from the allowlist
func (e *Engine) RemoveFromAllowlist(clientIP string) {
	e.mu.Lock()
	defer e.mu.Unlock()

	delete(e.allowlist, clientIP)
	e.logger.Info("IP removed from allowlist", slog.String("ip", clientIP))
}

// GetAllowlist returns the allowlist
func (e *Engine) GetAllowlist() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var list []string
	for ip := range e.allowlist {
		list = append(list, ip)
	}

	return list
}

// Statistics returns engine statistics
func (e *Engine) Statistics() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	return map[string]interface{}{
		"tracked_ips": len(e.ipActivity),
		"allowlisted": len(e.allowlist),
		"timestamp":   time.Now().Unix(),
	}
}

// Cleanup removes old entries (older than 24h)
func (e *Engine) Cleanup() {
	e.mu.Lock()
	defer e.mu.Unlock()

	cutoff := time.Now().Add(-24 * time.Hour)
	removed := 0

	for ip, activity := range e.ipActivity {
		if activity.LastSeen.Before(cutoff) {
			delete(e.ipActivity, ip)
			removed++
		}
	}

	if removed > 0 {
		e.logger.Debug("Cleaned up old IP activity records", slog.Int("count", removed))
	}
}

// Helper function: detect scanner User-Agent
func isScannerUA(ua string) bool {
	scannerPatterns := []string{
		"curl", "wget", "sqlmap", "nikto", "nmap",
		"metasploit", "masscan", "nessus", "qualys",
		"go-http", "python-requests", "scrapy",
		"headless", "phantomjs", "chromedriver",
	}

	lowerUA := strings.ToLower(ua)
	for _, pattern := range scannerPatterns {
		if strings.Contains(lowerUA, pattern) {
			return true
		}
	}

	return false
}
