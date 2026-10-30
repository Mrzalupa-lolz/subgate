package agent

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/subgate/subgate/pkg/wire"
	"github.com/subgate/subgate/pkg/xray"
)

type Config struct {
	CoreAddress, NodeID, Token, StatsAddress, AccessLog, SpoolFile string
	Logger                                                         *slog.Logger
}
type Agent struct {
	config                    *Config
	ctx                       context.Context
	cancel                    context.CancelFunc
	wg                        sync.WaitGroup
	client                    *http.Client
	stats                     *xray.StatsClient
	started                   time.Time
	mu                        sync.Mutex
	statsError, logsError     string
	dropped, expired          uint64
	events                    chan wire.Activity
	pending                   []wire.Telemetry
	pendingCreated            time.Time
	spoolError, deliveryError string
	pendingBatches            int
	statsIntervalMS           int64
	statsQueryMS              int64
	statsUsers                int
	statsScope                string
	healthCursor              int
}

const (
	maxBatchSamples       = 5000
	maxBatchEvents        = 1000
	maxBatchBytes         = 2 << 20
	maxSpoolBytes         = 16 << 20
	spoolMagic            = "SUBGATE-SPOOL-1\x00"
	deliveryQueueCapacity = 8
)

type queuedTelemetry struct {
	batch   wire.Telemetry
	created time.Time
}

type spoolRecord struct {
	CreatedAt time.Time        `json:"created_at"`
	Batches   []wire.Telemetry `json:"batches"`
}
type coreHTTPError struct{ status int }

func (e *coreHTTPError) Error() string { return fmt.Sprintf("Core returned HTTP %d", e.status) }

func NormalizeCore(raw string) (string, error) {
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(strings.TrimRight(raw, "/"))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("invalid Core URL")
	}
	if u.Scheme != "https" {
		host := u.Hostname()
		ip := net.ParseIP(host)
		if u.Scheme != "http" || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return "", fmt.Errorf("remote Core requires HTTPS")
		}
	}
	return strings.TrimRight(u.String(), "/"), nil
}
func NewAgent(cfg *Config) (*Agent, error) {
	if cfg.NodeID == "" || cfg.Token == "" {
		return nil, fmt.Errorf("node-id and token are required")
	}
	if len(cfg.NodeID) > 256 {
		return nil, fmt.Errorf("node-id exceeds limit")
	}
	base, err := NormalizeCore(cfg.CoreAddress)
	if err != nil {
		return nil, err
	}
	cfg.CoreAddress = base
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.StatsAddress == "" {
		cfg.StatsAddress = "auto"
	}
	stats, err := xray.NewStatsClient(cfg.StatsAddress, cfg.Logger)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	a := &Agent{config: cfg, ctx: ctx, cancel: cancel, stats: stats, client: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, started: time.Now(), events: make(chan wire.Activity, 5000)}
	if err := a.loadSpool(); err != nil {
		stats.Close()
		cancel()
		return nil, err
	}
	a.pendingBatches = len(a.pending)
	if a.expired > 0 {
		a.config.Logger.Warn("expired telemetry spool discarded", "expired_items", a.expired)
	}
	return a, nil
}

func (a *Agent) spoolAEAD() (cipher.AEAD, error) {
	key := sha256.Sum256([]byte("subgate-agent-spool-v1\x00" + a.config.NodeID + "\x00" + a.config.Token))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func (a *Agent) spoolAAD() []byte { return []byte("subgate-agent-spool-v1:" + a.config.NodeID) }
func (a *Agent) loadSpool() error {
	if a.config.SpoolFile == "" {
		return nil
	}
	f, err := os.Open(a.config.SpoolFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("pending telemetry spool unavailable")
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxSpoolBytes+65537))
	closeErr := f.Close()
	if err != nil || len(raw) > maxSpoolBytes+65536 {
		return fmt.Errorf("pending telemetry spool exceeds size limit")
	}
	if closeErr != nil {
		return fmt.Errorf("pending telemetry spool unavailable")
	}
	if len(raw) < len(spoolMagic) || string(raw[:len(spoolMagic)]) != spoolMagic {
		return fmt.Errorf("pending telemetry spool is corrupted")
	}
	aead, err := a.spoolAEAD()
	if err != nil {
		return fmt.Errorf("pending telemetry spool is corrupted")
	}
	n := aead.NonceSize()
	if len(raw) <= len(spoolMagic)+n {
		return fmt.Errorf("pending telemetry spool is corrupted")
	}
	plain, err := aead.Open(nil, raw[len(spoolMagic):len(spoolMagic)+n], raw[len(spoolMagic)+n:], a.spoolAAD())
	if err != nil {
		return fmt.Errorf("pending telemetry spool is corrupted")
	}
	var record spoolRecord
	if json.Unmarshal(plain, &record) != nil || len(record.Batches) == 0 || record.CreatedAt.IsZero() {
		return fmt.Errorf("pending telemetry spool is corrupted")
	}
	for i := range record.Batches {
		if !validTelemetry(&record.Batches[i]) || record.Batches[i].NodeID != a.config.NodeID {
			return fmt.Errorf("pending telemetry spool is corrupted")
		}
	}
	if telemetryExpired(record.Batches, record.CreatedAt, time.Now()) {
		if err := os.Remove(a.config.SpoolFile); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("expired telemetry spool cannot be removed")
		}
		a.expired = telemetryCount(record.Batches)
		return nil
	}
	a.pending, a.pendingCreated = record.Batches, record.CreatedAt
	return nil
}
func validTelemetry(batch *wire.Telemetry) bool {
	if batch == nil || batch.BatchID == "" || len(batch.BatchID) > 128 || batch.NodeID == "" || len(batch.NodeID) > 256 || len(batch.Samples) > maxBatchSamples || len(batch.Events) > maxBatchEvents || len(batch.Samples)+len(batch.Events) == 0 {
		return false
	}
	raw, err := json.Marshal(batch)
	return err == nil && len(raw) <= maxBatchBytes
}
func (a *Agent) persistSpool(batches []wire.Telemetry, created time.Time) error {
	if a.config.SpoolFile == "" {
		return nil
	}
	if len(batches) == 0 {
		return a.clearSpool()
	}
	for i := range batches {
		if !validTelemetry(&batches[i]) {
			return fmt.Errorf("telemetry batch exceeds spool limits")
		}
	}
	aead, err := a.spoolAEAD()
	if err != nil {
		return err
	}
	plain, err := json.Marshal(spoolRecord{CreatedAt: created, Batches: batches})
	if err != nil {
		return err
	}
	if len(plain) > maxSpoolBytes+32768 {
		return fmt.Errorf("pending telemetry spool exceeds size limit")
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	out := append([]byte(spoolMagic), aead.Seal(nonce, nonce, plain, a.spoolAAD())...)
	dir := filepath.Dir(a.config.SpoolFile)
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".pending-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			os.Remove(tmpName)
		}
	}()
	if err = tmp.Chmod(0600); err == nil {
		_, err = tmp.Write(out)
	}
	if err == nil {
		err = tmp.Sync()
	}
	if err == nil {
		err = tmp.Close()
	}
	if err != nil {
		return err
	}
	if err = os.Rename(tmpName, a.config.SpoolFile); err != nil {
		return err
	}
	ok = true
	return syncSpoolDirectory(dir)
}
func (a *Agent) clearSpool() error {
	if a.config.SpoolFile == "" {
		return nil
	}
	err := os.Remove(a.config.SpoolFile)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return syncSpoolDirectory(filepath.Dir(a.config.SpoolFile))
}
func syncSpoolDirectory(dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}
func telemetryCount(batches []wire.Telemetry) uint64 {
	var count uint64
	for _, batch := range batches {
		count += uint64(len(batch.Samples) + len(batch.Events))
	}
	return count
}
func telemetryExpired(batches []wire.Telemetry, created, now time.Time) bool {
	if created.IsZero() || !created.After(now.Add(-24*time.Hour)) || created.After(now.Add(5*time.Minute)) {
		return true
	}
	for _, batch := range batches {
		for _, sample := range batch.Samples {
			if sample.Timestamp.IsZero() || !sample.Timestamp.After(now.Add(-24*time.Hour)) {
				return true
			}
		}
		for _, event := range batch.Events {
			if event.Timestamp.IsZero() || !event.Timestamp.After(now.Add(-24*time.Hour)) {
				return true
			}
		}
	}
	return false
}
func batchID() (string, error) {
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return "", err
	}
	return hex.EncodeToString(id), nil
}

// splitTelemetry bounds both the Core JSON body and total queued memory. Rejected
// measurements are counted instead of sending a batch Core cannot accept.
func splitTelemetry(nodeID string, samples []wire.Sample, events []wire.Activity, now time.Time) ([]wire.Telemetry, uint64, uint64, error) {
	batches := []wire.Telemetry{}
	var dropped, expired uint64
	var batch wire.Telemetry
	size, total := 0, 0
	start := func() error {
		id, err := batchID()
		if err != nil {
			return err
		}
		batch = wire.Telemetry{BatchID: id, NodeID: nodeID, Samples: []wire.Sample{}, Events: []wire.Activity{}, SentAt: now.UTC()}
		raw, _ := json.Marshal(batch)
		size = len(raw)
		return nil
	}
	flush := func() {
		if len(batch.Samples)+len(batch.Events) == 0 {
			return
		}
		if total+size > maxSpoolBytes {
			dropped += uint64(len(batch.Samples) + len(batch.Events))
		} else {
			batches = append(batches, batch)
			total += size
		}
	}
	if err := start(); err != nil {
		return nil, 0, 0, err
	}
	validTime := func(at time.Time) bool {
		return !at.IsZero() && at.After(now.Add(-24*time.Hour)) && at.Before(now.Add(5*time.Minute))
	}
	for _, sample := range samples {
		if !validTime(sample.Timestamp) {
			expired++
			continue
		}
		if sample.UserID == "" || len(sample.UserID) > 256 || sample.UplinkBps < 0 || sample.DownlinkBps < 0 || sample.UplinkBps > 1e15 || sample.DownlinkBps > 1e15 {
			dropped++
			continue
		}
		raw, _ := json.Marshal(sample)
		cost := len(raw)
		if len(batch.Samples) > 0 {
			cost++
		}
		if len(batch.Samples) >= maxBatchSamples || size+cost > maxBatchBytes {
			flush()
			if err := start(); err != nil {
				return nil, dropped, expired, err
			}
			cost = len(raw)
		}
		if total+size+cost > maxSpoolBytes {
			dropped++
			continue
		}
		batch.Samples = append(batch.Samples, sample)
		size += cost
	}
	for _, event := range events {
		if !validTime(event.Timestamp) {
			expired++
			continue
		}
		if !validActivity(event) {
			dropped++
			continue
		}
		raw, _ := json.Marshal(event)
		cost := len(raw)
		if len(batch.Events) > 0 {
			cost++
		}
		if len(batch.Events) >= maxBatchEvents || size+cost > maxBatchBytes {
			flush()
			if err := start(); err != nil {
				return nil, dropped, expired, err
			}
			cost = len(raw)
		}
		if total+size+cost > maxSpoolBytes {
			dropped++
			continue
		}
		batch.Events = append(batch.Events, event)
		size += cost
	}
	flush()
	return batches, dropped, expired, nil
}

func telemetryBatchBytes(batch wire.Telemetry) int {
	raw, err := json.Marshal(batch)
	if err != nil {
		return maxBatchBytes
	}
	return len(raw)
}

func (a *Agent) recordDropped(count uint64) {
	if count == 0 {
		return
	}
	a.mu.Lock()
	a.dropped += count
	a.mu.Unlock()
}

// appendPending is called by the delivery owner. It keeps the in-memory retry
// queue bounded even when Core is unavailable for a long time. Oldest batches
// are discarded explicitly and reported through heartbeat diagnostics.
func (a *Agent) appendPending(batch wire.Telemetry, created time.Time) {
	if len(batch.Samples)+len(batch.Events) == 0 {
		return
	}
	if batch.SentAt.IsZero() {
		batch.SentAt = created.UTC()
	}
	if !validTelemetry(&batch) {
		a.recordDropped(uint64(len(batch.Samples) + len(batch.Events)))
		return
	}
	if a.pendingCreated.IsZero() {
		a.pendingCreated = created
	}
	a.pending = append(a.pending, batch)
	for len(a.pending) > 0 {
		total := 0
		for _, item := range a.pending {
			total += telemetryBatchBytes(item)
		}
		if total <= maxSpoolBytes {
			break
		}
		removed := a.pending[0]
		a.pending = a.pending[1:]
		a.recordDropped(uint64(len(removed.Samples) + len(removed.Events)))
	}
	a.mu.Lock()
	a.pendingBatches = len(a.pending)
	a.mu.Unlock()
}

func queueTelemetry(queue chan queuedTelemetry, item queuedTelemetry, dropped func(uint64)) {
	select {
	case queue <- item:
		return
	default:
	}
	// Keep sampling non-blocking during a Core outage. Removing the oldest
	// unsent batch makes the loss explicit and preserves the newest rates.
	select {
	case old := <-queue:
		dropped(uint64(len(old.batch.Samples) + len(old.batch.Events)))
	default:
	}
	select {
	case queue <- item:
	default:
		dropped(uint64(len(item.batch.Samples) + len(item.batch.Events)))
	}
}

func (a *Agent) deliverPending() error {
	if len(a.pending) == 0 {
		return nil
	}
	if telemetryExpired(a.pending, a.pendingCreated, time.Now()) {
		if err := a.clearSpool(); err != nil {
			a.mu.Lock()
			a.spoolError = "expired telemetry spool cannot be removed; collection paused"
			a.mu.Unlock()
			return err
		}
		count := telemetryCount(a.pending)
		a.pending = nil
		a.mu.Lock()
		a.expired += count
		a.pendingBatches = 0
		a.deliveryError = ""
		a.spoolError = ""
		a.mu.Unlock()
		a.config.Logger.Warn("retained telemetry expired after 24 hours", "expired_items", count)
		return nil
	}
	if err := a.persistSpool(a.pending, a.pendingCreated); err != nil {
		a.mu.Lock()
		a.spoolError = "pending telemetry spool write failed; collection paused"
		a.mu.Unlock()
		return err
	}
	a.mu.Lock()
	a.spoolError = ""
	a.mu.Unlock()
	if err := a.send("/api/v1/agents/telemetry", &a.pending[0]); err != nil {
		var httpErr *coreHTTPError
		if errors.As(err, &httpErr) && httpErr.status >= 400 && httpErr.status < 500 && httpErr.status != 408 && httpErr.status != 429 {
			if httpErr.status == 401 || httpErr.status == 403 {
				a.mu.Lock()
				a.deliveryError = fmt.Sprintf("agent credential rejected (HTTP %d); re-enroll before restarting; pending telemetry retained", httpErr.status)
				a.mu.Unlock()
				return err
			}
			// A validation rejection will never succeed for this exact payload.
			// Remove it durably and count the rejected measurements.
			remaining := a.pending[1:]
			if saveErr := a.persistSpool(remaining, a.pendingCreated); saveErr != nil {
				a.mu.Lock()
				a.spoolError = "rejected telemetry acknowledgement persistence failed; batch retained"
				a.mu.Unlock()
				return saveErr
			}
			count := uint64(len(a.pending[0].Samples) + len(a.pending[0].Events))
			a.pending = remaining
			a.mu.Lock()
			a.dropped += count
			a.pendingBatches = len(remaining)
			a.deliveryError = fmt.Sprintf("Core rejected telemetry (HTTP %d); %d items discarded", httpErr.status, count)
			a.mu.Unlock()
			a.config.Logger.Warn("Core rejected telemetry batch", "status", httpErr.status, "dropped_items", count)
			return nil
		}
		a.mu.Lock()
		a.deliveryError = "telemetry delivery failed; collection paused while retrying retained batch"
		a.mu.Unlock()
		return err
	}
	remaining := a.pending[1:]
	// If persisting the acknowledgement fails, retain and replay the original
	// BatchID. Core acknowledges duplicates, making this crash window safe.
	if err := a.persistSpool(remaining, a.pendingCreated); err != nil {
		a.mu.Lock()
		a.spoolError = "telemetry acknowledgement persistence failed; batch retained"
		a.mu.Unlock()
		return err
	}
	a.pending = remaining
	a.mu.Lock()
	a.pendingBatches = len(a.pending)
	a.deliveryError = ""
	a.spoolError = ""
	a.mu.Unlock()
	return nil
}
func (a *Agent) send(path string, input any) error {
	raw, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if len(raw) > maxBatchBytes {
		return fmt.Errorf("Core request exceeds 2 MiB limit")
	}
	req, err := http.NewRequestWithContext(a.ctx, "POST", a.config.CoreAddress+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.config.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("Core request failed: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &coreHTTPError{status: resp.StatusCode}
	}
	return nil
}

func (a *Agent) collectSamples() ([]wire.Sample, error) {
	started := time.Now()
	values, err := a.stats.QueryStats(a.ctx)
	queryMS := time.Since(started).Milliseconds()
	scope := "user"
	if err == nil && len(values) == 0 {
		// Xray installations without user counters still expose inbound
		// counters. They are deliberately tagged as coarse and never treated
		// as a named subscriber by Core or the console.
		inbound, inboundErr := a.stats.QueryInboundSpeeds(a.ctx)
		if inboundErr != nil {
			err = inboundErr
		} else {
			values = inbound
			scope = "inbound"
		}
	}
	now := time.Now().UTC()
	samples := make([]wire.Sample, 0, len(values))
	for _, value := range values {
		at := value.LastUpdate
		if at.IsZero() {
			at = now
		}
		itemScope := value.Scope
		if itemScope == "" {
			itemScope = scope
		}
		userID := value.Email
		if itemScope == "inbound" {
			userID = "inbound:" + userID
		}
		interval := value.IntervalMS
		if interval <= 0 {
			interval = 1000
		}
		samples = append(samples, wire.Sample{
			UserID: userID, Timestamp: at.UTC(), UplinkBps: value.UplinkBps, DownlinkBps: value.DownlinkBps,
			UplinkDelta: value.UplinkDelta, DownlinkDelta: value.DownlinkDelta, IntervalMS: interval,
			Scope: itemScope, Gap: value.Gap, Warming: value.Warming,
		})
	}
	a.mu.Lock()
	a.statsQueryMS = queryMS
	a.statsIntervalMS = 1000
	a.statsUsers = len(values)
	a.statsScope = scope
	a.statsError = ""
	if err != nil {
		a.statsError = err.Error()
	}
	a.mu.Unlock()
	return samples, err
}

func (a *Agent) drainEvents() []wire.Activity {
	events := make([]wire.Activity, 0, 1000)
	for len(events) < 1000 {
		select {
		case event := <-a.events:
			events = append(events, event)
		default:
			return events
		}
	}
	return events
}

func (a *Agent) deliveryLoop(queue <-chan queuedTelemetry, result chan<- error) {
	defer a.wg.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case item := <-queue:
			a.appendPending(item.batch, item.created)
		case <-ticker.C:
			if err := a.deliverPending(); err != nil {
				var httpErr *coreHTTPError
				if errors.As(err, &httpErr) && (httpErr.status == 401 || httpErr.status == 403) {
					select {
					case result <- fmt.Errorf("agent credential rejected; re-enroll before restarting (pending telemetry preserved)"):
					default:
					}
					return
				}
				a.config.Logger.Warn("telemetry delivery failed; batch retained", "error", err)
			}
		case <-a.ctx.Done():
			return
		}
	}
}

func (a *Agent) Start() error {
	defer a.stats.Close()
	defer a.client.CloseIdleConnections()
	defer func() { a.cancel(); a.wg.Wait() }()
	a.wg.Add(1)
	go a.heartbeatLoop()
	a.wg.Add(1)
	go a.healthLoop()
	if a.config.AccessLog != "" {
		a.wg.Add(1)
		go a.logsLoop()
	} else {
		a.mu.Lock()
		a.logsError = "access log collection disabled: configure -access-log"
		a.mu.Unlock()
	}
	queue := make(chan queuedTelemetry, deliveryQueueCapacity)
	deliveryResult := make(chan error, 1)
	a.wg.Add(1)
	go a.deliveryLoop(queue, deliveryResult)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-a.ctx.Done():
			select {
			case err := <-deliveryResult:
				return err
			default:
			}
			return nil
		case err := <-deliveryResult:
			return err
		case <-ticker.C:
			samples, statsErr := a.collectSamples()
			events := a.drainEvents()
			if len(samples) == 0 && len(events) == 0 {
				if statsErr != nil {
					a.config.Logger.Warn("telemetry sampling failed", "error", statsErr)
				}
				continue
			}
			created := time.Now().UTC()
			batches, dropped, expired, err := splitTelemetry(a.config.NodeID, samples, events, created)
			if err != nil {
				return err
			}
			for _, batch := range batches {
				queueTelemetry(queue, queuedTelemetry{batch: batch, created: created}, a.recordDropped)
			}
			a.mu.Lock()
			a.expired += expired
			a.mu.Unlock()
			a.recordDropped(dropped)
			if dropped > 0 || expired > 0 {
				a.config.Logger.Warn("telemetry items excluded by queue limits or age", "dropped_items", dropped, "expired_items", expired)
			}
		}
	}
}
func (a *Agent) heartbeatLoop() {
	defer a.wg.Done()
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		a.mu.Lock()
		statsError, logsError, dropped := a.statsError, a.logsError, a.dropped
		statsIntervalMS, statsQueryMS, statsUsers, statsScope, pendingBatches := a.statsIntervalMS, a.statsQueryMS, a.statsUsers, a.statsScope, a.pendingBatches
		if a.spoolError != "" {
			statsError = joinDiagnostic(statsError, a.spoolError)
		}
		if a.deliveryError != "" {
			statsError = joinDiagnostic(statsError, a.deliveryError)
		}
		if a.expired > 0 {
			statsError = joinDiagnostic(statsError, fmt.Sprintf("%d telemetry items expired outside the 24-hour Core window", a.expired))
		}
		if dropped > 0 {
			statsError = joinDiagnostic(statsError, fmt.Sprintf("%d telemetry items dropped by queue or input limits", dropped))
		}
		a.mu.Unlock()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		h := wire.Heartbeat{NodeID: a.config.NodeID, Version: "0.2.0", Timestamp: time.Now(), MemoryMB: int(m.Alloc >> 20), UptimeSeconds: int64(time.Since(a.started).Seconds()), StatsError: statsError, LogsError: logsError, DroppedEvents: dropped, StatsIntervalMS: statsIntervalMS, StatsQueryMS: statsQueryMS, StatsUsers: statsUsers, StatsScope: statsScope, PendingBatches: pendingBatches}
		if err := a.send("/api/v1/agents/heartbeat", h); err != nil {
			a.config.Logger.Warn("heartbeat failed", "error", err)
		}
		select {
		case <-a.ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (a *Agent) logsLoop() {
	defer a.wg.Done()
	for {
		a.mu.Lock()
		a.logsError = ""
		a.mu.Unlock()
		err := a.collectLogs()
		if a.ctx.Err() != nil {
			return
		}
		if err != nil {
			a.mu.Lock()
			a.logsError = err.Error()
			a.mu.Unlock()
			a.config.Logger.Warn("log collection unavailable", "error", err)
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-a.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func validActivity(event wire.Activity) bool {
	return event.UserID != "" && len(event.UserID) <= 256 && len(event.DstHost) <= 1024 && len(event.SrcIP) <= 128 && len(event.InboundTag) <= 256 && len(event.OutboundTag) <= 256 && len(event.Network) <= 32 && len(event.Result) <= 128 && event.DstPort >= 0 && event.DstPort <= 65535
}
func (a *Agent) collectLogs() error {
	ctx, cancel := context.WithCancel(a.ctx)
	defer cancel()
	incoming := make(chan wire.Activity)
	done := make(chan error, 1)
	go func() { done <- xray.Tail(ctx, a.config.AccessLog, incoming, a.config.Logger) }()
	for {
		select {
		case <-a.ctx.Done():
			cancel()
			return <-done
		case err := <-done:
			return err
		case event := <-incoming:
			if !validActivity(event) {
				a.mu.Lock()
				a.dropped++
				a.mu.Unlock()
				continue
			}
			select {
			case a.events <- event:
			default:
				a.mu.Lock()
				a.dropped++
				a.mu.Unlock()
			}
		}
	}
}
func (a *Agent) Stop() { a.cancel() }
func (a *Agent) Health() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return map[string]any{"node_id": a.config.NodeID, "uptime_seconds": time.Since(a.started).Seconds(), "stats_error": a.statsError, "logs_error": a.logsError, "delivery_error": a.deliveryError, "spool_error": a.spoolError, "pending_batches": a.pendingBatches, "dropped_items": a.dropped, "expired_items": a.expired, "durable_spool": a.config.SpoolFile != ""}
}
func joinDiagnostic(existing, extra string) string {
	if existing == "" {
		existing = extra
	} else {
		existing += "; " + extra
	}
	if len(existing) > 1024 {
		return existing[:1024]
	}
	return existing
}
