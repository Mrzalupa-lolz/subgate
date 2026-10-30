package config

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"net/url"
	"os"
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
	Health  HealthConfig  `yaml:"health"`
}

type HealthConfig struct {
	FailureThreshold   int                 `yaml:"failure_threshold"`
	SuccessThreshold   int                 `yaml:"success_threshold"`
	MinQuarantine      time.Duration       `yaml:"min_quarantine"`
	BackoffBase        time.Duration       `yaml:"backoff_base"`
	BackoffMax         time.Duration       `yaml:"backoff_max"`
	MaintenanceWindows []MaintenanceWindow `yaml:"maintenance_windows"`
	Quorum             int                 `yaml:"quorum"`
	ProbeInterval      time.Duration       `yaml:"probe_interval"`
	ProbeTimeout       time.Duration       `yaml:"probe_timeout"`
}

type MaintenanceWindow struct {
	NodeID string    `yaml:"node_id"`
	From   time.Time `yaml:"from"`
	Until  time.Time `yaml:"until"`
}

// HTTPConfig holds HTTP server settings
type HTTPConfig struct {
	TrustedProxies []string  `yaml:"trusted_proxies"`
	Listen         string    `yaml:"listen" default:":8080"`
	PublicURL      string    `yaml:"public_url"`
	TLS            TLSConfig `yaml:"tls"`
}

// TLSConfig holds TLS settings
type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
	Key     string `yaml:"key"`
}

// DatabaseConfig holds SQLite settings
type DatabaseConfig struct {
	URL            string        `yaml:"url" default:"sqlite:file:/data/subgate.db?mode=rwc"`
	MaxConnections int           `yaml:"max_connections" default:"25"`
	BackupPath     string        `yaml:"backup_path" default:"/data/backups"`
	VacuumInterval time.Duration `yaml:"vacuum_interval" default:"24h"`
}

// PanelConfig holds Remnawave panel integration settings
type PanelConfig struct {
	// Writes are disabled unless explicitly enabled for a writable panel.
	AllowWrites     bool          `yaml:"allow_writes"`
	URL             string        `yaml:"url"`
	APIToken        string        `yaml:"api_token"`
	Timeout         time.Duration `yaml:"timeout" default:"30s"`
	MaxRetries      int           `yaml:"max_retries" default:"3"`
	WebhookPath     string        `yaml:"webhook_path" default:"/webhook/panel"`
	WebhookKey      string        `yaml:"webhook_key"`
	SubscriptionURL string        `yaml:"subscription_url"`
	SyncInterval    time.Duration `yaml:"sync_interval"`
}

// IPIntelConfig holds IP intelligence settings
type IPIntelConfig struct {
	Enabled           bool          `yaml:"enabled" default:"true"`
	RefreshInterval   time.Duration `yaml:"refresh_interval" default:"24h"`
	DataPath          string        `yaml:"data_path" default:"/data/ipintel"`
	Sources           []DataSource  `yaml:"sources"`
	StaleAlertAge     time.Duration `yaml:"stale_alert_age" default:"168h"` // 7 days
	ConfidenceMinimum string        `yaml:"confidence_minimum" default:"medium"`
}

// DataSource describes an IP intelligence data source
type DataSource struct {
	Name    string `yaml:"name"`
	URL     string `yaml:"url"`
	Format  string `yaml:"format"` // mmdb, csv, json
	License string `yaml:"license"`
	Enabled bool   `yaml:"enabled" default:"true"`
}

// TelegramConfig holds Telegram bot settings
type TelegramConfig struct {
	Enabled        bool          `yaml:"enabled" default:"false"`
	Token          string        `yaml:"token"`
	WebhookURL     string        `yaml:"webhook_url"`
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
	JWTSecret       string        `yaml:"jwt_secret"`
	SessionTimeout  time.Duration `yaml:"session_timeout" default:"24h"`
	PasswordMinLen  int           `yaml:"password_min_len" default:"12"`
	TOTPRequired    bool          `yaml:"totp_required" default:"false"` // legacy option; true rejected
	WebAuthnEnabled bool          `yaml:"webauthn_enabled" default:"false"`
	LoginRateLimit  int           `yaml:"login_rate_limit" default:"5"` // attempts per minute
	LockoutDuration time.Duration `yaml:"lockout_duration" default:"15m"`
}

// StorageConfig holds storage settings
type StorageConfig struct {
	EncryptionKeyFile          string                       `yaml:"encryption_key_file"`
	PreviousEncryptionKeyFiles []string                     `yaml:"previous_encryption_key_files"`
	ActivityJournal            ActivityJournalStorageConfig `yaml:"activity_journal"`
	TSDB                       TSDBStorageConfig            `yaml:"tsdb"`
}

// ActivityJournalStorageConfig holds activity journal storage settings
type ActivityJournalStorageConfig struct {
	HotTTL      time.Duration `yaml:"hot_ttl" default:"336h"`           // 14 days
	WarmTTL     time.Duration `yaml:"warm_ttl" default:"2160h"`         // 90 days
	ColdTTL     time.Duration `yaml:"cold_ttl" default:"8760h"`         // 365 days
	DiskQuota   int64         `yaml:"disk_quota" default:"53687091200"` // 50 GB
	Path        string        `yaml:"path" default:"/data/activity"`
	Compression string        `yaml:"compression" default:"zstd"` // zstd or none
}

// TSDBStorageConfig holds TSDB storage settings
type TSDBStorageConfig struct {
	RawRetention       time.Duration `yaml:"raw_retention" default:"5m"`        // in-memory
	RawMaxMemory       int64         `yaml:"raw_max_memory" default:"67108864"` // 64 MB
	RollupRetention10s time.Duration `yaml:"rollup_10s_retention" default:"24h"`
	RollupRetention1m  time.Duration `yaml:"rollup_1m_retention" default:"720h"`  // 30 days
	RollupRetention1h  time.Duration `yaml:"rollup_1h_retention" default:"8760h"` // 1 year
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
		},
		Database: DatabaseConfig{
			URL:            getEnv("SUBGATE_DATABASE_URL", "file:data/subgate.db"),
			MaxConnections: 25,
			BackupPath:     getEnv("SUBGATE_BACKUP_PATH", "data/backups"),
		},
		Panel: PanelConfig{
			URL:             getEnv("SUBGATE_PANEL_URL", ""),
			APIToken:        getEnv("SUBGATE_PANEL_TOKEN", ""),
			SubscriptionURL: getEnv("SUBGATE_SUBSCRIPTION_URL", ""),
		},
		Auth: AuthConfig{
			JWTSecret:     getEnv("SUBGATE_JWT_SECRET", ""),
			AdminUsername: getEnv("SUBGATE_ADMIN_USERNAME", "admin"),
			AdminPassword: getEnv("SUBGATE_ADMIN_PASSWORD", ""),
		},
		Storage: StorageConfig{EncryptionKeyFile: getEnv("SUBGATE_STORAGE_KEY_FILE", ""), PreviousEncryptionKeyFiles: splitList(getEnv("SUBGATE_PREVIOUS_STORAGE_KEY_FILES", ""))},
		IPIntel: IPIntelConfig{DataPath: getEnv("SUBGATE_IPINTEL_PATH", "")},
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Defaults() *Config {
	return &Config{HTTP: HTTPConfig{Listen: "127.0.0.1:8080"}, Database: DatabaseConfig{URL: "file:data/subgate.db", BackupPath: "data/backups"}, Panel: PanelConfig{Timeout: 15 * time.Second, SyncInterval: time.Minute, WebhookPath: "/webhook/panel"}, Auth: AuthConfig{AdminUsername: "admin", SessionTimeout: 24 * time.Hour, PasswordMinLen: 12, LoginRateLimit: 5}}
}

// Validate validates configuration
func (c *Config) Validate() error {
	if c.Health.FailureThreshold == 0 {
		c.Health.FailureThreshold = 3
	}
	if c.Health.SuccessThreshold == 0 {
		c.Health.SuccessThreshold = 2
	}
	if c.Health.MinQuarantine == 0 {
		c.Health.MinQuarantine = time.Minute
	}
	if c.Health.BackoffBase == 0 {
		c.Health.BackoffBase = 30 * time.Second
	}
	if c.Health.BackoffMax == 0 {
		c.Health.BackoffMax = 15 * time.Minute
	}
	if c.Health.FailureThreshold < 1 || c.Health.FailureThreshold > 1000 || c.Health.SuccessThreshold < 1 || c.Health.SuccessThreshold > 1000 {
		return fmt.Errorf("health failure/success thresholds must be 1..1000")
	}
	if c.Health.MinQuarantine < 10*time.Millisecond || c.Health.MinQuarantine > 24*time.Hour || c.Health.BackoffBase < 10*time.Millisecond || c.Health.BackoffBase > c.Health.BackoffMax || c.Health.BackoffMax > 24*time.Hour {
		return fmt.Errorf("health min quarantine and backoff must be 10ms..24h with base <= max")
	}
	if len(c.Health.MaintenanceWindows) > 128 {
		return fmt.Errorf("at most 128 maintenance windows are supported")
	}
	for _, window := range c.Health.MaintenanceWindows {
		if window.NodeID == "" || len(window.NodeID) > 256 || window.From.IsZero() || window.Until.IsZero() || !window.From.Before(window.Until) {
			return fmt.Errorf("maintenance requires node_id and a valid from/until interval")
		}
	}
	if c.Health.Quorum == 0 {
		c.Health.Quorum = 1
	}
	if c.Health.Quorum < 1 || c.Health.Quorum > 64 {
		return fmt.Errorf("health quorum must be 1..64")
	}
	if c.Health.ProbeInterval == 0 {
		c.Health.ProbeInterval = 30 * time.Second
	}
	if c.Health.ProbeTimeout == 0 {
		c.Health.ProbeTimeout = 8 * time.Second
	}
	if c.Health.ProbeInterval < 10*time.Millisecond || c.Health.ProbeInterval > 24*time.Hour || c.Health.ProbeTimeout < 10*time.Millisecond || c.Health.ProbeTimeout > 30*time.Second {
		return fmt.Errorf("health probe interval must be 10ms..24h and timeout 10ms..30s")
	}
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
	if c.Auth.JWTSecret != "" && len(c.Auth.JWTSecret) < 32 {
		return fmt.Errorf("JWT secret must be at least 32 characters")
	}
	for _, raw := range []string{c.Panel.URL, c.Panel.SubscriptionURL} {
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
	if c.Panel.WebhookPath == "" {
		c.Panel.WebhookPath = "/webhook/panel"
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
