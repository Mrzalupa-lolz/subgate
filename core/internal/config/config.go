package config

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds all SubGate configuration
type Config struct {
	Demo bool `yaml:"demo"`
	// Core server settings
	HTTP HTTPConfig `yaml:"http"`

	// Database configuration
	Database DatabaseConfig `yaml:"database"`

	// Panel integration
	Panel PanelConfig `yaml:"panel"`

	// IP intelligence
	IPIntel IPIntelConfig `yaml:"ipintel"`

	// Telegram notifications
	Telegram TelegramConfig `yaml:"telegram"`

	// Authentication
	Auth AuthConfig `yaml:"auth"`

	// Storage configuration
	Storage StorageConfig `yaml:"storage"`
}

// HTTPConfig holds HTTP server settings
type HTTPConfig struct {
	TrustedProxies []string `yaml:"trusted_proxies"`
	Listen         string   `yaml:"listen" default:":8080"`
	PublicURL      string   `yaml:"public_url"`
	// MetricsToken lets a Prometheus scraper read /metrics with a bearer
	// token. Without it /metrics requires an owner session.
	MetricsToken string    `yaml:"metrics_token"`
	TLS          TLSConfig `yaml:"tls"`
}

// TLSConfig holds TLS settings
type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}

// DatabaseConfig holds SQLite settings
type DatabaseConfig struct {
	URL        string `yaml:"url" default:"sqlite:file:/data/subgate.db?mode=rwc"`
	BackupPath string `yaml:"backup_path" default:"/data/backups"`
}

// PanelConfig holds Remnawave panel integration settings
type PanelConfig struct {
	// Writes are disabled unless explicitly enabled for a writable panel.
	AllowWrites  bool          `yaml:"allow_writes"`
	URL          string        `yaml:"url"`
	APIToken     string        `yaml:"api_token"`
	Timeout      time.Duration `yaml:"timeout" default:"30s"`
	SyncInterval time.Duration `yaml:"sync_interval"`
}

// IPIntelConfig holds IP intelligence settings
type IPIntelConfig struct {
	Enabled  bool   `yaml:"enabled" default:"true"`
	DataPath string `yaml:"data_path" default:"/data/ipintel"`
}

// TelegramConfig holds Telegram bot settings
type TelegramConfig struct {
	Enabled        bool          `yaml:"enabled" default:"false"`
	Token          string        `yaml:"token"`
	AllowedUserIDs []int64       `yaml:"allowed_user_ids"`
	QuietHours     QuietHours    `yaml:"quiet_hours"`
	AlertCooldown  time.Duration `yaml:"alert_cooldown" default:"5m"`
}

// QuietHours defines quiet hours for notifications
type QuietHours struct {
	Enabled   bool   `yaml:"enabled" default:"true"`
	StartHour int    `yaml:"start_hour" default:"22"` // 22:00
	EndHour   int    `yaml:"end_hour" default:"7"`    // 07:00
	Timezone  string `yaml:"timezone" default:"UTC"`
}

// AuthConfig holds authentication settings
type AuthConfig struct {
	AdminUsername   string        `yaml:"admin_username"`
	AdminPassword   string        `yaml:"admin_password"`
	SessionTimeout  time.Duration `yaml:"session_timeout" default:"24h"`
	PasswordMinLen  int           `yaml:"password_min_len" default:"12"`
	TOTPRequired    bool          `yaml:"totp_required" default:"false"` // legacy option; true rejected
	WebAuthnEnabled bool          `yaml:"webauthn_enabled" default:"false"`
	LoginRateLimit  int           `yaml:"login_rate_limit" default:"5"` // attempts per minute
}

// StorageConfig holds storage settings
type StorageConfig struct {
	EncryptionKeyFile          string                       `yaml:"encryption_key_file"`
	PreviousEncryptionKeyFiles []string                     `yaml:"previous_encryption_key_files"`
	ActivityJournal            ActivityJournalStorageConfig `yaml:"activity_journal"`
}

// ActivityJournalStorageConfig holds activity journal storage settings
type ActivityJournalStorageConfig struct {
	HotTTL time.Duration `yaml:"hot_ttl" default:"336h"` // 14 days
}

var (
	// Global config (protected by RWMutex for hot-reload)
	globalConfig *Config
	configMutex  sync.RWMutex
)

// Load loads configuration from file (YAML or JSON)
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		// Try to load from environment if file doesn't exist
		if os.IsNotExist(err) {
			return LoadFromEnv()
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := Defaults()
	if err := yaml.Unmarshal(data, cfg); err != nil {
		// Try JSON fallback
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("failed to parse config: %w", err)
		}
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	configMutex.Lock()
	globalConfig = cfg
	configMutex.Unlock()

	return cfg, nil
}

// LoadFromEnv loads configuration from environment variables
func LoadFromEnv() (*Config, error) {
	cfg := &Config{
		Demo: getEnv("SUBGATE_DEMO", "false") == "true",
		HTTP: HTTPConfig{
			TrustedProxies: splitList(getEnv("SUBGATE_TRUSTED_PROXIES", "")),
			Listen:         getEnv("SUBGATE_LISTEN", ":8080"),
			PublicURL:      getEnv("SUBGATE_PUBLIC_URL", ""),
			MetricsToken:   getEnv("SUBGATE_METRICS_TOKEN", ""),
		},
		Database: DatabaseConfig{
			URL:        getEnv("SUBGATE_DATABASE_URL", "file:data/subgate.db"),
			BackupPath: getEnv("SUBGATE_BACKUP_PATH", "data/backups"),
		},
		Panel: PanelConfig{
			URL:      getEnv("SUBGATE_PANEL_URL", ""),
			APIToken: getEnv("SUBGATE_PANEL_TOKEN", ""),
		},
		Auth: AuthConfig{
			AdminUsername: getEnv("SUBGATE_ADMIN_USERNAME", "admin"),
			AdminPassword: getEnv("SUBGATE_ADMIN_PASSWORD", ""),
		},
		Storage: StorageConfig{EncryptionKeyFile: getEnv("SUBGATE_STORAGE_KEY_FILE", ""), PreviousEncryptionKeyFiles: splitList(getEnv("SUBGATE_PREVIOUS_STORAGE_KEY_FILES", ""))},
		IPIntel: IPIntelConfig{DataPath: getEnv("SUBGATE_IPINTEL_PATH", "")},
	}
	if token := getEnv("SUBGATE_TELEGRAM_TOKEN", ""); token != "" {
		cfg.Telegram.Enabled = true
		cfg.Telegram.Token = token
		for _, raw := range splitList(getEnv("SUBGATE_TELEGRAM_CHAT_IDS", "")) {
			id, err := strconv.ParseInt(raw, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("SUBGATE_TELEGRAM_CHAT_IDS must be a comma-separated list of numeric chat ids")
			}
			cfg.Telegram.AllowedUserIDs = append(cfg.Telegram.AllowedUserIDs, id)
		}
		cfg.Telegram.QuietHours.Timezone = getEnv("SUBGATE_TELEGRAM_TIMEZONE", "UTC")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Defaults() *Config {
	return &Config{HTTP: HTTPConfig{Listen: "127.0.0.1:8080"}, Database: DatabaseConfig{URL: "file:data/subgate.db", BackupPath: "data/backups"}, Panel: PanelConfig{Timeout: 15 * time.Second, SyncInterval: time.Minute}, Auth: AuthConfig{AdminUsername: "admin", SessionTimeout: 24 * time.Hour, PasswordMinLen: 12, LoginRateLimit: 5}}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if len(c.Storage.PreviousEncryptionKeyFiles) > 8 {
		return fmt.Errorf("at most eight previous storage encryption keys are supported")
	}
	if c.Auth.TOTPRequired || c.Auth.WebAuthnEnabled {
		return fmt.Errorf("single-owner authentication supports login and password only; disable totp_required and webauthn_enabled")
	}
	if c.HTTP.Listen == "" {
		c.HTTP.Listen = ":8080"
	}
	if c.Database.URL == "" {
		return fmt.Errorf("database URL is required")
	}
	if !c.Demo && c.Panel.URL == "" {
		return fmt.Errorf("panel URL is required")
	}
	if !c.Demo && c.Panel.APIToken == "" {
		return fmt.Errorf("panel API token is required")
	}
	for _, raw := range []string{c.Panel.URL} {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
			return fmt.Errorf("panel URLs must be absolute HTTP(S) URLs without credentials, query or fragment")
		}
	}
	if c.HTTP.PublicURL != "" {
		u, err := url.Parse(c.HTTP.PublicURL)
		if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
			return fmt.Errorf("public URL must be an HTTP(S) origin without credentials, path, query or fragment")
		}
	}
	if c.Panel.Timeout <= 0 {
		c.Panel.Timeout = 15 * time.Second
	}
	for _, proxy := range c.HTTP.TrustedProxies {
		if _, err := netip.ParsePrefix(proxy); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR: %s", proxy)
		}
	}
	if c.Panel.SyncInterval < 10*time.Second {
		c.Panel.SyncInterval = time.Minute
	}
	if c.Auth.AdminUsername == "" {
		c.Auth.AdminUsername = "admin"
	}
	if c.Auth.SessionTimeout <= 0 {
		c.Auth.SessionTimeout = 24 * time.Hour
	}
	if c.Auth.LoginRateLimit <= 0 {
		c.Auth.LoginRateLimit = 5
	}
	if c.Database.BackupPath == "" {
		c.Database.BackupPath = "data/backups"
	}
	if c.HTTP.TLS.Enabled && (c.HTTP.TLS.Cert == "" || c.HTTP.TLS.Key == "") {
		return fmt.Errorf("TLS certificate and key are required")
	}
	return nil
}

// Get returns the current global configuration
func Get() *Config {
	configMutex.RLock()
	defer configMutex.RUnlock()
	return globalConfig
}

// Set updates the global configuration (used for hot-reload)
func Set(cfg *Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}
	configMutex.Lock()
	globalConfig = cfg
	configMutex.Unlock()
	return nil
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func splitList(raw string) []string {
	out := []string{}
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}
