//go:build linux

package worker

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/subgate/subgate/agent/internal/agent"
	"github.com/subgate/subgate/pkg/wire"
)

type captureBatch struct {
	iface string
	flows []wire.DPIFlow
	err   string
}
type captureEnvelope struct {
	Flows     []wire.DPIFlow `json:"flows"`
	Dropped   uint64         `json:"dropped"`
	Truncated bool           `json:"truncated"`
}

func capture(ctx context.Context, executable, iface string, out chan<- captureBatch) {
	for ctx.Err() == nil {
		var salt [16]byte
		if _, err := rand.Read(salt[:]); err != nil {
			return
		}
		prefix := hex.EncodeToString(salt[:])
		cmd := exec.CommandContext(ctx, executable, "--interface", iface)
		cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin"}
		stdout, err := cmd.StdoutPipe()
		if err == nil {
			err = cmd.Start()
		}
		if err == nil {
			scanner := bufio.NewScanner(stdout)
			scanner.Buffer(make([]byte, 4096), 2<<20)
			for scanner.Scan() {
				var envelope captureEnvelope
				if json.Unmarshal(scanner.Bytes(), &envelope) != nil || len(envelope.Flows) > 2000 {
					err = fmt.Errorf("invalid collector output")
					break
				}
				batch := captureBatch{iface: iface, flows: envelope.Flows}
				if envelope.Dropped > 0 || envelope.Truncated {
					batch.err = "collector loss or flow table exhaustion"
				}
				for i := range batch.flows {
					batch.flows[i].ID = prefix + ":" + batch.flows[i].ID
					batch.flows[i].Interface = iface
				}
				select {
				case out <- batch:
				case <-ctx.Done():
					cmd.Process.Kill()
					cmd.Wait()
					return
				}
			}
			cmd.Process.Kill()
			waitErr := cmd.Wait()
			if err == nil {
				err = scanner.Err()
			}
			if err == nil {
				err = waitErr
			}
		}
		select {
		case out <- captureBatch{iface: iface, err: "collector unavailable"}:
		case <-ctx.Done():
			return
		}
		timer := time.NewTimer(5 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

type counters struct {
	rx, tx, drops, total, idle uint64
	at                         time.Time
}

func readCounters(iface string) (c counters, err error) {
	c.at = time.Now().UTC()
	read := func(name string) (uint64, error) {
		raw, e := os.ReadFile(filepath.Join("/sys/class/net", iface, "statistics", name))
		if e != nil {
			return 0, e
		}
		return strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
	}
	if c.rx, err = read("rx_bytes"); err != nil {
		return
	}
	if c.tx, err = read("tx_bytes"); err != nil {
		return
	}
	var rx, tx uint64
	if rx, err = read("rx_dropped"); err != nil {
		return
	}
	if tx, err = read("tx_dropped"); err != nil {
		return
	}
	c.drops = rx + tx
	raw, e := os.ReadFile("/proc/stat")
	if e != nil {
		return c, e
	}
	line := strings.Fields(strings.SplitN(string(raw), "\n", 2)[0])
	if len(line) < 9 {
		return c, fmt.Errorf("invalid CPU counters")
	}
	for i, value := range line[1:9] {
		n, e := strconv.ParseUint(value, 10, 64)
		if e != nil {
			return c, e
		}
		c.total += n
		if i == 3 || i == 4 {
			c.idle += n
		}
	}
	return
}

func resourceDelta(old, current counters, capacity int) *wire.Resources {
	interval := current.at.Sub(old.at).Milliseconds()
	if old.at.IsZero() || interval < 1000 || interval > 15000 || current.rx < old.rx || current.tx < old.tx || current.drops < old.drops || current.total <= old.total || current.idle < old.idle {
		return nil
	}
	cpu := 100 * (1 - float64(current.idle-old.idle)/float64(current.total-old.total))
	if cpu < 0 || cpu > 100 {
		return nil
	}
	return &wire.Resources{Timestamp: current.at, IntervalMS: interval, CapacityMbps: capacity, RXBytes: current.rx - old.rx, TXBytes: current.tx - old.tx, Dropped: current.drops - old.drops, CPUPercent: cpu}
}

func call(ctx context.Context, client *http.Client, base, token, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("Core request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("Core returned HTTP %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20+1))
	if err != nil || len(raw) > 2<<20 {
		return fmt.Errorf("Core response exceeds limit")
	}
	if out != nil {
		return json.Unmarshal(raw, out)
	}
	return nil
}

// Run is the optional Linux node worker. Network I/O has a separate deadline;
// expiry runs every second and also from an independent systemd timer.
func Run(ctx context.Context, configPath, journal string, logger *slog.Logger) error {
	cfg, err := LoadConfig(configPath)
	if err != nil {
		return err
	}
	base, err := agent.NormalizeCore(cfg.CoreURL)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(cfg.TokenFile)
	if err != nil {
		return fmt.Errorf("agent credential unavailable")
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return fmt.Errorf("empty agent credential")
	}
	if !filepath.IsAbs(cfg.Collector) {
		return fmt.Errorf("collector path must be absolute")
	}
	info, err := os.Stat(cfg.Collector)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("collector must be a protected regular executable")
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && os.Geteuid() == 0 && stat.Uid != 0 {
		return fmt.Errorf("collector must be root-owned")
	}
	workerCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var captures sync.WaitGroup
	defer captures.Wait()
	batches := make(chan captureBatch, 64)
	errorsByInterface := map[string]string{}
	for _, b := range cfg.Bindings {
		iface := b.Interface
		errorsByInterface[iface] = "collector starting"
		captures.Add(1)
		go func() { defer captures.Done(); capture(workerCtx, cfg.Collector, iface, batches) }()
	}
	defer cancel() // Cancel collectors before waiting for them.
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	shaper := Shaper{Path: journal, Run: RunCommand, Now: time.Now}
	// Cleanup from prior crashes before accepting new commands. Failure is visible.
	cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = shaper.Reconcile(cleanupCtx, []wire.ShaperAction{}, nil, nil, true)
	cleanupCancel()
	if err != nil {
		return err
	}
	flows := []wire.DPIFlow{}
	pendingResults := []wire.ShaperResult{}
	previous, _ := readCounters(cfg.ResourceInterface)
	poll := time.NewTicker(10 * time.Second)
	defer poll.Stop()
	expiry := time.NewTicker(time.Second)
	defer expiry.Stop()
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = shaper.Reconcile(cleanupCtx, []wire.ShaperAction{}, nil, nil, true)
	}()
	for {
		select {
		case <-ctx.Done():
			return nil
		case batch := <-batches:
			errorsByInterface[batch.iface] = batch.err
			if len(flows)+len(batch.flows) > 10000 {
				errorsByInterface[batch.iface] = "worker flow buffer exhausted"
			} else {
				flows = append(flows, batch.flows...)
			}
		case <-expiry.C:
			latest, e := LoadConfig(configPath)
			if e != nil {
				latest.Bindings = nil
				logger.Warn("worker config unavailable; removing restrictions")
			}
			operationCtx, done := context.WithTimeout(ctx, 3*time.Second)
			results, e := shaper.Reconcile(operationCtx, nil, latest.Bindings, latest.Allowlist, false)
			done()
			if e != nil {
				logger.Warn("shaper expiry failed", "error", e)
			}
			pendingResults = append(pendingResults, results...)
		case <-poll.C:
			latest, e := LoadConfig(configPath)
			if e != nil {
				logger.Warn("worker configuration unavailable")
				continue
			}
			// Capture topology changes require a service restart; lease/allowlist
			// refreshes are applied without restarting or extending actions.
			collectorError := ""
			for _, message := range errorsByInterface {
				if message != "" {
					collectorError = message
					break
				}
			}
			if len(latest.Bindings) != len(cfg.Bindings) {
				collectorError = "capture topology changed; restart worker"
			} else {
				for i, b := range latest.Bindings {
					if b.Interface != cfg.Bindings[i].Interface || b.UserID != cfg.Bindings[i].UserID {
						collectorError = "capture topology changed; restart worker"
					}
				}
			}
			current, e := readCounters(cfg.ResourceInterface)
			var resources *wire.Resources
			if e == nil {
				resources = resourceDelta(previous, current, cfg.CapacityMbps)
				previous = current
			} else {
				collectorError = "node resource collection failed"
			}
			operationCtx, done := context.WithTimeout(ctx, 8*time.Second)
			// Bounded chunks, no payload, no replay of stale resource windows.
			firstReport := true
			for firstReport || len(flows) > 0 || resources != nil {
				firstReport = false
				n := min(2000, len(flows))
				report := wire.MonitorReport{NodeID: cfg.NodeID, Resources: resources, Bindings: latest.Bindings, Flows: flows[:n], CollectorError: collectorError}
				e = call(operationCtx, client, base, token, "POST", "/api/v1/agents/monitor", report, nil)
				if e != nil {
					break
				}
				flows = flows[n:]
				resources = nil
			}
			if e == nil && len(pendingResults) > 0 {
				e = call(operationCtx, client, base, token, "PUT", "/api/v1/agents/shaper-results", pendingResults, nil)
				if e == nil {
					pendingResults = nil
				}
			}
			var envelope struct {
				Data       []wire.ShaperAction `json:"data"`
				ServerTime time.Time           `json:"server_time"`
			}
			if e == nil {
				e = call(operationCtx, client, base, token, "GET", "/api/v1/agents/shaper", nil, &envelope)
			}
			if e == nil {
				skew := time.Since(envelope.ServerTime)
				if skew > 5*time.Second || skew < -5*time.Second || len(envelope.Data) > 2000 {
					e = fmt.Errorf("invalid command time or count")
				}
			}
			if e == nil {
				for _, a := range envelope.Data {
					if a.NodeID != cfg.NodeID {
						e = fmt.Errorf("command node mismatch")
						break
					}
				}
			}
			if e == nil {
				desired := envelope.Data
				if desired == nil {
					desired = []wire.ShaperAction{}
				}
				var results []wire.ShaperResult
				results, e = shaper.Reconcile(operationCtx, desired, latest.Bindings, latest.Allowlist, false)
				pendingResults = append(pendingResults, results...)
			}
			done()
			if e != nil {
				logger.Warn("node monitor cycle failed", "error", e)
			}
			// Expired metadata is discarded visibly rather than poisoning ingest.
			kept := flows[:0]
			for _, f := range flows {
				if f.Timestamp.After(time.Now().Add(-2 * time.Minute)) {
					kept = append(kept, f)
				} else {
					errorsByInterface[f.Interface] = "DPI delivery gap"
				}
			}
			flows = kept
			if len(pendingResults) > 2000 {
				pendingResults = pendingResults[len(pendingResults)-2000:]
				logger.Warn("old shaper reports dropped; reconciliation will report current state")
			}
		}
	}
}
