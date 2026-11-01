package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/subgate/subgate/agent/internal/agent"
	"github.com/subgate/subgate/agent/internal/enrollment"
	"github.com/subgate/subgate/agent/internal/preflight"
)

var version = "0.2.0"

func main() {
	enroll := len(os.Args) > 1 && os.Args[1] == "enroll"
	check := len(os.Args) > 1 && os.Args[1] == "check"
	health := len(os.Args) > 1 && os.Args[1] == "health"
	args := os.Args[1:]
	if enroll || check || health {
		args = args[1:]
	}
	flags := flag.NewFlagSet("subgate-agent", flag.ExitOnError)
	core := flags.String("core-addr", env("SUBGATE_CORE_URL", "http://127.0.0.1:8080"), "Core HTTPS URL (HTTP allowed for loopback)")
	node := flags.String("node-id", env("NODE_ID", ""), "Remnawave node UUID")
	secret := flags.String("token", env("SUBGATE_AGENT_TOKEN", ""), "Agent token; prefer token-file")
	tokenFile := flags.String("token-file", "data/agent-token", "Agent token file")
	bootstrap := flags.String("enrollment-token", env("ENROLLMENT_TOKEN", ""), "One-time enrollment token")
	stats := flags.String("stats-addr", "auto", "Xray StatsService: auto, loopback host:port, unix:/path or unix:@name")
	logFile := flags.String("access-log", "", "Xray access.log path (must contain email field)")
	spoolFile := flags.String("spool-file", "data/pending.sgb", "Encrypted pending telemetry file (empty disables persistence)")
	showVersion := flags.Bool("version", false, "Print version")
	healthFile := flags.String("health-file", "", "Write readiness after successful heartbeats (health subcommand reads it)")
	_ = flags.Parse(args)
	if *showVersion {
		fmt.Println(version)
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if health {
		if err := agent.CheckHeartbeat(*healthFile); err != nil {
			logger.Error("agent not ready", "error", err)
			os.Exit(1)
		}
		return
	}
	if check {
		if err := preflight.Check(*core, *stats, *logFile, *tokenFile, logger); err != nil {
			logger.Error("preflight failed; enrollment not attempted", "error", err)
			os.Exit(1)
		}
		logger.Info("Core, access.log, credential volume and Xray StatsService OK")
		return
	}
	if enroll {
		if *bootstrap == "" || *node == "" {
			logger.Error("enrollment-token and node-id are required")
			os.Exit(1)
		}
		err := enrollment.EnrollToFile(*core, *bootstrap, *node, *tokenFile)
		if err != nil {
			logger.Error("enrollment failed", "error", err)
			os.Exit(1)
		}
		logger.Info("agent enrolled; token saved", "path", *tokenFile)
		return
	}
	if *secret == "" {
		raw, err := os.ReadFile(*tokenFile)
		if err == nil {
			*secret = strings.TrimSpace(string(raw))
		}
	}
	instance, err := agent.NewAgent(&agent.Config{CoreAddress: *core, NodeID: *node, Token: *secret, StatsAddress: *stats, AccessLog: *logFile, SpoolFile: *spoolFile, HealthFile: *healthFile, Logger: logger})
	if err != nil {
		logger.Error("agent configuration failed", "error", err)
		os.Exit(1)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	done := make(chan error, 1)
	go func() { done <- instance.Start() }()
	select {
	case <-signals:
		instance.Stop()
		err = <-done
	case err = <-done:
		instance.Stop()
	}
	if err != nil {
		logger.Error("agent stopped with error", "error", err)
		os.Exit(1)
	}
}
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
