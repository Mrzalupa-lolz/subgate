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

func TestTelegramFromEnvironment(t *testing.T) {
	t.Setenv("SUBGATE_DEMO", "true")
	t.Setenv("SUBGATE_TELEGRAM_TOKEN", "123:synthetic")
	t.Setenv("SUBGATE_TELEGRAM_CHAT_IDS", "42, -100123")
	cfg, err := LoadFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Telegram.Enabled || cfg.Telegram.Token != "123:synthetic" || len(cfg.Telegram.AllowedUserIDs) != 2 || cfg.Telegram.AllowedUserIDs[1] != -100123 {
		t.Fatalf("telegram env config: %+v", cfg.Telegram)
	}
	t.Setenv("SUBGATE_TELEGRAM_CHAT_IDS", "not-a-number")
	if _, err := LoadFromEnv(); err == nil {
		t.Fatal("invalid chat id accepted")
	}
}
