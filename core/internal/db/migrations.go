package db

import (
	"database/sql"
	"embed"
	"fmt"
	"log/slog"
)

//go:embed schema.sql
var schemaFS embed.FS

// RunMigrations initializes the database schema
func RunMigrations(db *DB) error {
	// Create migration tracking table if not exists
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Check if schema already applied
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM schema_migrations WHERE version = 1").Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	// If schema already applied, skip
	if count > 0 {
		slog.Info("Database schema already initialized")
		return nil
	}

	// Load and apply schema
	schemaData, err := schemaFS.ReadFile("schema.sql")
	if err != nil {
		return fmt.Errorf("failed to read schema file: %w", err)
	}

	// Execute schema
	if _, err := db.Exec(string(schemaData)); err != nil {
		return fmt.Errorf("failed to apply schema: %w", err)
	}

	// Record migration
	_, err = db.Exec("INSERT INTO schema_migrations (version, name) VALUES (1, 'init')")
	if err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	slog.Info("Database schema initialized successfully")
	return nil
}

// RollbackMigrations removes all tables (dev/test only)
func RollbackMigrations(db *DB) error {
	tables := []string{
		"activity_retention_state",
		"activity_dictionaries",
		"activity_segments",
		"log_samples",
		"log_signal_aggregates",
		"speed_rollup_1h",
		"speed_rollup_1m",
		"speed_rollup_10s",
		"probes",
		"agents",
		"alerts",
		"baselines",
		"block_check_results",
		"bans",
		"scanner_events",
		"conn_origin_aggregates",
		"sub_request_aggregates",
		"sub_requests",
		"ipintel_sources",
		"asn_overrides",
		"geo_rules",
		"injections",
		"usage_snapshots",
		"quotas",
		"pins",
		"user_subscriptions",
		"users_cache",
		"policies",
		"tiers",
		"quarantine_events",
		"health_checks",
		"health_state",
		"host_node_mappings",
		"hosts",
		"nodes",
		"audit_log",
		"api_keys",
		"admin_users",
		"settings",
		"schema_migrations",
	}

	for _, table := range tables {
		if _, err := db.Exec(fmt.Sprintf("DROP TABLE IF EXISTS %s", table)); err != nil {
			return fmt.Errorf("failed to drop table %s: %w", table, err)
		}
	}

	slog.Info("Database schema rolled back")
	return nil
}
