//go:build linux

package main

import (
	"context"
	"flag"
	"github.com/subgate/subgate/agent/internal/worker"
	"github.com/subgate/subgate/pkg/wire"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	config := flag.String("config", "/etc/subgate/worker.json", "private node worker configuration")
	journal := flag.String("journal", "/var/lib/subgate-worker/leases.json", "durable TC lease journal")
	expire := flag.Bool("expire-only", false, "remove expired or locally disallowed restrictions")
	cleanup := flag.Bool("cleanup", false, "remove every journaled restriction")
	flag.Parse()
	logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	var err error
	if *expire || *cleanup {
		cfg, loadErr := worker.LoadConfig(*config)
		if loadErr != nil {
			cfg.Bindings = nil
		}
		operation, done := context.WithTimeout(ctx, 10*time.Second)
		defer done()
		_, err = (worker.Shaper{Path: *journal, Run: worker.RunCommand, Now: time.Now}).Reconcile(operation, []wire.ShaperAction(nil), cfg.Bindings, cfg.Allowlist, *cleanup)
	} else {
		err = worker.Run(ctx, *config, *journal, logger)
	}
	if err != nil {
		logger.Error("node worker failed", "error", err)
		os.Exit(1)
	}
}
