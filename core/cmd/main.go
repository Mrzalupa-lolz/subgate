package main

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/subgate/subgate/core/internal/config"
	"github.com/subgate/subgate/core/internal/db"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/core/internal/server"
)

var version = "0.2.0"

func main() {
	server.Version = version
	path := flag.String("config", "", "Configuration YAML file; defaults to environment")
	level := flag.String("log-level", "info", "Log level: debug, info, warn, error")
	demo := flag.Bool("demo", false, "Run a local demo with explicitly synthetic data")
	listen := flag.String("listen", "", "Override listen address")
	databasePath := flag.String("database", "", "Override SQLite DSN")
	showVersion := flag.Bool("version", false, "Print version")
	check := flag.Bool("check", false, "Validate configuration and read-only panel access, without opening the database or starting Core")
	restore := flag.String("restore", "", "Decrypt a .sgb backup offline")
	keyFile := flag.String("key-file", "", "Original subgate.key for backup restoration")
	output := flag.String("output", "", "New database path for restoration (must not exist)")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(*level)}))
	if *restore != "" {
		if err := restoreBackup(*restore, *keyFile, *output); err != nil {
			logger.Error("restore failed", "error", err)
			os.Exit(1)
		}
		logger.Info("database restored; keep the original storage key beside it", "output", *output)
		return
	}
	var cfg *config.Config
	var err error
	if *demo {
		cfg = config.Defaults()
		cfg.Demo = true
		cfg.Database.URL = "file:data/demo.db"
		cfg.Database.BackupPath = "data/demo-backups"
	} else {
		cfg, err = config.Load(*path)
	}
	if err != nil {
		logger.Error("configuration failed", "error", err)
		os.Exit(1)
	}
	if *listen != "" {
		cfg.HTTP.Listen = *listen
	}
	if *databasePath != "" {
		cfg.Database.URL = *databasePath
	}
	if err = cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}
	if *check {
		if !cfg.Demo {
			if !strings.HasPrefix(cfg.HTTP.PublicURL, "https://") || strings.Contains(cfg.HTTP.PublicURL, "subgate.example.com") ||
				strings.HasPrefix(cfg.Panel.APIToken, "replace-with") || strings.HasPrefix(cfg.Auth.AdminPassword, "replace-with") ||
				len(cfg.Auth.AdminPassword) < 12 {
				logger.Error("set a real HTTPS public URL, panel token and admin password of at least 12 characters")
				os.Exit(1)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			client := panel.NewClient(cfg.Panel.URL, cfg.Panel.APIToken, logger)
			if _, err = client.ListNodesContext(ctx); err == nil {
				_, _, err = client.ListUsersContext(ctx, 0, 1)
			}
			if err != nil {
				logger.Error("panel preflight failed; check URL, API token and read access to nodes/users", "error", err)
				os.Exit(1)
			}
		}
		logger.Info("configuration and panel access OK; database untouched")
		return
	}
	database, err := db.New(cfg.Database.URL)
	if err != nil {
		logger.Error("database failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()
	if err = db.RunMigrations(database); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	srv, err := server.New(cfg, database, logger)
	if err != nil {
		logger.Error("server initialization failed", "error", err)
		os.Exit(1)
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	errors := make(chan error, 1)
	go func() { errors <- srv.Listen(cfg.HTTP.Listen) }()
	logger.Info("SubGate starting", "version", version, "listen", cfg.HTTP.Listen, "demo", cfg.Demo)
	if cfg.Demo {
		logger.Info("local demo credentials", "username", "admin", "password", "subgate-demo")
	}
	select {
	case sig := <-signals:
		logger.Info("shutdown requested", "signal", sig.String())
	case err = <-errors:
		if err != nil {
			logger.Error("HTTP server failed", "error", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if e := srv.Shutdown(ctx); e != nil {
		logger.Error("shutdown failed", "error", e)
		os.Exit(1)
	}
	if err != nil {
		os.Exit(1)
	}
}
func logLevel(raw string) slog.Level {
	switch strings.ToLower(raw) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
func restoreBackup(path, keyPath, output string) error {
	if keyPath == "" || output == "" {
		return fmt.Errorf("restore requires -key-file and -output")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(raw) < 8 || string(raw[:8]) != "SUBGATE1" {
		return fmt.Errorf("invalid backup header")
	}
	key, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	data := raw[8:]
	if len(data) < gcm.NonceSize() {
		return fmt.Errorf("truncated backup")
	}
	plain, err := gcm.Open(nil, data[:gcm.NonceSize()], data[gcm.NonceSize():], []byte("subgate-backup-v1"))
	if err != nil {
		return fmt.Errorf("backup integrity/key verification failed: %w", err)
	}
	if len(plain) < 16 || string(plain[:16]) != "SQLite format 3\x00" {
		return fmt.Errorf("backup is not a SQLite database")
	}
	f, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(plain)
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
