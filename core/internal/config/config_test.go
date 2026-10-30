package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestConfigurationYAMLAndEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "subgate.yaml")
	if err := os.WriteFile(path, []byte("demo: true\nhttp:\n  listen: 127.0.0.1:8090\n  public_url: https://console.example.com\nauth:\n  admin_username: operator\n  session_timeout: 2h\npanel:\n  sync_interval: 30s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Demo || cfg.HTTP.Listen != "127.0.0.1:8090" || cfg.HTTP.PublicURL != "https://console.example.com" || cfg.Auth.AdminUsername != "operator" || cfg.Auth.SessionTimeout != 2*time.Hour || cfg.Panel.SyncInterval != 30*time.Second {
		t.Fatalf("YAML overrides lost: %+v", cfg)
	}
	t.Setenv("SUBGATE_DEMO", "true")
	t.Setenv("SUBGATE_PUBLIC_URL", "https://console.example.com")
	t.Setenv("SUBGATE_DATABASE_URL", ":memory:")
	t.Setenv("SUBGATE_ADMIN_USERNAME", "environment-admin")
	t.Setenv("SUBGATE_ADMIN_PASSWORD", "test-password-123")
	cfg, err = Load(filepath.Join(t.TempDir(), "absent.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Demo || cfg.Database.URL != ":memory:" || cfg.Auth.AdminUsername != "environment-admin" || cfg.HTTP.PublicURL != "https://console.example.com" || cfg.Panel.Timeout <= 0 || cfg.Auth.SessionTimeout <= 0 {
		t.Fatalf("environment config invalid: %+v", cfg)
	}
}

func TestMaintenanceWindowValidation(t *testing.T) {
	now := time.Now()
	for _, window := range []MaintenanceWindow{{NodeID: "", From: now, Until: now.Add(time.Hour)}, {NodeID: "node", Until: now}, {NodeID: "node", From: now, Until: now}, {NodeID: "node", From: now, Until: now.Add(-time.Hour)}} {
		cfg := Defaults()
		cfg.Demo = true
		cfg.Health.MaintenanceWindows = []MaintenanceWindow{window}
		if cfg.Validate() == nil {
			t.Fatal("invalid maintenance window accepted", window)
		}
	}
	cfg := Defaults()
	cfg.Demo = true
	cfg.Health.MaintenanceWindows = []MaintenanceWindow{{NodeID: "node", From: now, Until: now.Add(time.Hour)}}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "synthetic-maintenance.yaml")
	if err := os.WriteFile(path, []byte("demo: true\nhealth:\n  maintenance_windows:\n    - node_id: synthetic-node\n      from: 2026-10-14T08:00:00Z\n      until: 2026-10-14T09:00:00Z\n"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || len(loaded.Health.MaintenanceWindows) != 1 || loaded.Health.MaintenanceWindows[0].NodeID != "synthetic-node" || loaded.Health.MaintenanceWindows[0].Until.Sub(loaded.Health.MaintenanceWindows[0].From) != time.Hour {
		t.Fatal("YAML maintenance load failed", err)
	}
}

func TestQuarantineConfiguration(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.Health.FailureThreshold = -1 },
		func(c *Config) { c.Health.SuccessThreshold = 1001 },
		func(c *Config) { c.Health.MinQuarantine = -time.Second },
		func(c *Config) { c.Health.BackoffBase = time.Hour; c.Health.BackoffMax = time.Minute },
		func(c *Config) { c.Health.BackoffMax = 25 * time.Hour },
	} {
		c := Defaults()
		c.Demo = true
		change(c)
		if c.Validate() == nil {
			t.Fatal("invalid quarantine config accepted")
		}
	}
	path := filepath.Join(t.TempDir(), "synthetic-health.yaml")
	if err := os.WriteFile(path, []byte("demo: true\nhealth:\n  failure_threshold: 5\n  success_threshold: 4\n  min_quarantine: 2m\n  backoff_base: 10s\n  backoff_max: 5m\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil || c.Health.FailureThreshold != 5 || c.Health.SuccessThreshold != 4 || c.Health.MinQuarantine != 2*time.Minute || c.Health.BackoffBase != 10*time.Second || c.Health.BackoffMax != 5*time.Minute {
		t.Fatal("quarantine config not loaded", err)
	}
}

func TestRejectUnsafeEndpoints(t *testing.T) {
	for _, raw := range []string{"ftp://example.com", "https://user:password@example.com", "/relative", "https://example.com/?token=secret", "https://example.com/#fragment"} {
		t.Run(raw, func(t *testing.T) {
			cfg := Defaults()
			cfg.Demo = true
			cfg.Panel.URL = raw
			if cfg.Validate() == nil {
				t.Fatal("unsafe panel URL accepted")
			}
		})
	}
	for _, raw := range []string{"ftp://example.com", "https://user:password@example.com", "/relative", "https://example.com/path", "https://example.com/?token=x", "https://example.com/#fragment"} {
		cfg := Defaults()
		cfg.Demo = true
		cfg.HTTP.PublicURL = raw
		if cfg.Validate() == nil {
			t.Errorf("unsafe public origin accepted: %s", raw)
		}
	}
	cfg := Defaults()
	if cfg.Validate() == nil {
		t.Fatal("production without panel accepted")
	}
	cfg.Demo = true
	cfg.HTTP.TLS.Enabled = true
	if cfg.Validate() == nil {
		t.Fatal("TLS without key accepted")
	}
}
