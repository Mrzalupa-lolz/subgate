// Package preflight checks deployment dependencies without enrolling or sending telemetry.
package preflight

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/subgate/subgate/agent/internal/agent"
	"github.com/subgate/subgate/pkg/xray"
)

func Check(core, stats, accessLog, tokenFile string, logger *slog.Logger) error {
	base, err := agent.NormalizeCore(core)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Get(base + "/health")
	if err != nil {
		return fmt.Errorf("Core health: %w", err)
	}
	defer resp.Body.Close()
	var health struct {
		Database bool `json:"database"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&health) != nil || !health.Database {
		return fmt.Errorf("Core /health is not ready (HTTP %d); check URL, TLS and reverse proxy", resp.StatusCode)
	}
	f, err := os.Open(accessLog)
	if err != nil {
		return fmt.Errorf("access.log: %w", err)
	}
	info, err := f.Stat()
	f.Close()
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("access.log must be a readable regular file")
	}
	// Exercise the same volume and UID used for the durable credential before
	// consuming a one-time token. Never overwrite an existing token here.
	f, err = os.CreateTemp(filepath.Dir(tokenFile), ".preflight-*")
	if err != nil {
		return fmt.Errorf("credential directory is not writable: %w", err)
	}
	name := f.Name()
	f.Close()
	os.Remove(name)
	source, err := xray.NewStatsClient(stats, logger)
	if err != nil {
		return err
	}
	defer source.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err = source.QueryStats(ctx); err != nil {
		return fmt.Errorf("Xray StatsService: %w; enable StatsService or set --stats-addr to its local address", err)
	}
	return nil
}
