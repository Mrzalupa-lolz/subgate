package server

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/subgate/subgate/core/internal/config"
	"github.com/subgate/subgate/core/internal/db"
	"github.com/subgate/subgate/core/internal/ipintel"
	"github.com/subgate/subgate/core/internal/metrics"
	"github.com/subgate/subgate/core/internal/notify"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/core/internal/tsdb"
	"github.com/subgate/subgate/deploy"
	"github.com/subgate/subgate/web"
)

type Server struct {
	config             *config.Config
	db                 *db.DB
	logger             *slog.Logger
	router             chi.Router
	http               *http.Server
	panel              *panel.Client
	intel              *ipintel.Engine
	aead               cipher.AEAD
	previousAEADs      []cipher.AEAD
	ctx                context.Context
	cancel             context.CancelFunc
	wg                 sync.WaitGroup
	syncMu             sync.Mutex
	syncRevision       uint64
	stateMu            sync.RWMutex
	lastSync           time.Time
	panelVersion       panelVersionDiagnostic
	syncError          string
	started            time.Time
	activityReadMu     sync.Mutex
	activityReadBudget activityReadBudget
	loginMu            sync.Mutex
	attempts           map[string]attempt
	liveStore          *tsdb.LiveStore
	metrics            *metrics.Registry
	notify             *notify.Dispatcher
	metricRequests     *metrics.Counter
	metricErrors       *metrics.Counter
	metricLatency      *metrics.Histogram
}

// Version is the Core build version, set by core/cmd from -ldflags.
var Version = "dev"

type attempt struct {
	count int
	start time.Time
}

func New(cfg *config.Config, database *db.DB, logger *slog.Logger) (*Server, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	registry := metrics.NewRegistry()
	quietLocation := time.UTC
	if cfg.Telegram.QuietHours.Timezone != "" {
		if loaded, err := time.LoadLocation(cfg.Telegram.QuietHours.Timezone); err == nil {
			quietLocation = loaded
		}
	}
	var senders []notify.Sender
	if cfg.Telegram.Enabled && cfg.Telegram.Token != "" {
		senders = append(senders, &notify.Telegram{Token: cfg.Telegram.Token, ChatIDs: cfg.Telegram.AllowedUserIDs, Logger: logger})
	}
	notifyQuiet := notify.QuietHours{Enabled: cfg.Telegram.QuietHours.Enabled, StartHour: cfg.Telegram.QuietHours.StartHour, EndHour: cfg.Telegram.QuietHours.EndHour, Location: quietLocation}
	s := &Server{config: cfg, db: database, logger: logger, router: chi.NewRouter(), ctx: ctx, cancel: cancel, started: time.Now(), attempts: map[string]attempt{}, liveStore: tsdb.NewLiveStore(100000), metrics: registry, notify: notify.NewDispatcher(logger, cfg.Telegram.AlertCooldown, notifyQuiet, senders...)}
	s.metricRequests = registry.MustCounter("subgate_http_requests_total", "HTTP requests handled by SubGate")
	s.metricErrors = registry.MustCounter("subgate_http_errors_total", "HTTP requests that returned an error")
	s.metricLatency = registry.MustHistogram("subgate_http_request_duration_seconds", "HTTP request duration", []float64{0.005, 0.01, 0.025, 0.1, 0.5, 1, 5})
	if err := s.initialize(); err != nil {
		cancel()
		return nil, err
	}
	var panelOptions []panel.Option
	if cfg.Panel.AllowWrites {
		panelOptions = append(panelOptions, panel.WithWriteAccess())
	}
	s.panel = panel.NewClient(cfg.Panel.URL, cfg.Panel.APIToken, logger, panelOptions...)
	s.panel.SetTimeout(cfg.Panel.Timeout)
	s.panelVersion = initialPanelVersionDiagnostic()
	s.intel = ipintel.NewEngine(logger)
	if cfg.IPIntel.DataPath != "" {
		if err := s.intel.LoadPrefixes(cfg.IPIntel.DataPath); err != nil {
			logger.Warn("IP dataset unavailable", "error", err)
		}
	}
	s.routes()
	return s, nil
}
func (s *Server) initialize() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS sg_objects(kind TEXT NOT NULL,id TEXT NOT NULL,data TEXT NOT NULL,updated_at INTEGER NOT NULL,PRIMARY KEY(kind,id));
 CREATE TABLE IF NOT EXISTS sg_sessions(token_hash TEXT PRIMARY KEY,admin_id INTEGER NOT NULL,expires_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS sg_enrollment(token_hash TEXT PRIMARY KEY,node_id TEXT NOT NULL,expires_at INTEGER NOT NULL,used INTEGER NOT NULL DEFAULT 0);
 CREATE TABLE IF NOT EXISTS sg_monitor_states(id TEXT PRIMARY KEY,payload BLOB NOT NULL,updated_at INTEGER NOT NULL);
 CREATE TABLE IF NOT EXISTS sg_shaper_actions(id TEXT PRIMARY KEY,node_id TEXT NOT NULL,user_id TEXT NOT NULL,status TEXT NOT NULL,created_at INTEGER NOT NULL,expires_at INTEGER NOT NULL,payload BLOB NOT NULL,report BLOB);
 CREATE INDEX IF NOT EXISTS sg_shaper_node_user ON sg_shaper_actions(node_id,user_id,expires_at);
 CREATE TABLE IF NOT EXISTS sg_analysis_windows(id TEXT PRIMARY KEY,minute INTEGER NOT NULL,payload BLOB NOT NULL);
 CREATE INDEX IF NOT EXISTS sg_analysis_windows_time ON sg_analysis_windows(minute);
 CREATE TABLE IF NOT EXISTS sg_analysis_seen(id TEXT PRIMARY KEY,minute INTEGER NOT NULL);
 CREATE INDEX IF NOT EXISTS sg_analysis_seen_time ON sg_analysis_seen(minute);
 CREATE TABLE IF NOT EXISTS sg_incidents(id TEXT PRIMARY KEY,updated_at INTEGER NOT NULL,status TEXT NOT NULL,payload BLOB NOT NULL,review BLOB);
 CREATE INDEX IF NOT EXISTS sg_incidents_time ON sg_incidents(updated_at DESC);
 CREATE TABLE IF NOT EXISTS sg_activity(id INTEGER PRIMARY KEY AUTOINCREMENT,node_id TEXT NOT NULL,user_id TEXT NOT NULL,timestamp INTEGER NOT NULL,payload BLOB NOT NULL);
 CREATE INDEX IF NOT EXISTS sg_activity_user ON sg_activity(user_id,timestamp DESC);
 CREATE INDEX IF NOT EXISTS sg_activity_time ON sg_activity(timestamp DESC);
 CREATE TABLE IF NOT EXISTS sg_speed(node_id TEXT NOT NULL,user_id TEXT NOT NULL,timestamp INTEGER NOT NULL,uplink INTEGER NOT NULL,downlink INTEGER NOT NULL,PRIMARY KEY(node_id,user_id,timestamp));
 CREATE INDEX IF NOT EXISTS sg_speed_time ON sg_speed(timestamp);
 CREATE TABLE IF NOT EXISTS sg_webhooks(key TEXT PRIMARY KEY,timestamp INTEGER NOT NULL);`)
	if err != nil {
		return err
	}
	if _, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS sg_speed_meta(node_id TEXT NOT NULL,user_id TEXT NOT NULL,timestamp INTEGER NOT NULL,interval_ms INTEGER NOT NULL DEFAULT 1000,scope TEXT NOT NULL DEFAULT 'user',gap INTEGER NOT NULL DEFAULT 0,warming INTEGER NOT NULL DEFAULT 0,uplink_delta INTEGER NOT NULL DEFAULT 0,downlink_delta INTEGER NOT NULL DEFAULT 0,received_at INTEGER NOT NULL,PRIMARY KEY(node_id,user_id,timestamp));
 CREATE INDEX IF NOT EXISTS sg_speed_meta_time ON sg_speed_meta(timestamp);
 CREATE INDEX IF NOT EXISTS sg_speed_meta_node_scope_time ON sg_speed_meta(node_id,scope,timestamp);
 CREATE INDEX IF NOT EXISTS sg_speed_user_time ON sg_speed(user_id,timestamp);
 CREATE TABLE IF NOT EXISTS sg_usage(node_id TEXT NOT NULL,user_id TEXT NOT NULL,timestamp INTEGER NOT NULL,uplink INTEGER NOT NULL,downlink INTEGER NOT NULL,PRIMARY KEY(node_id,user_id,timestamp));
 CREATE INDEX IF NOT EXISTS sg_usage_node_time ON sg_usage(node_id,timestamp);`); err != nil {
		return err
	}
	if _, err = s.db.Exec(`CREATE INDEX IF NOT EXISTS sg_users_username ON sg_objects(json_extract(data,'$.username')) WHERE kind='users';
 CREATE INDEX IF NOT EXISTS sg_users_email ON sg_objects(json_extract(data,'$.email')) WHERE kind='users';
 CREATE INDEX IF NOT EXISTS sg_users_short_uuid ON sg_objects(json_extract(data,'$.short_uuid')) WHERE kind='users';`); err != nil {
		return err
	}
	// The encryption key is generated once and survives restarts. Back it up with the database.
	keyPath := filepath.Join(filepath.Dir(strings.TrimPrefix(strings.Split(strings.TrimPrefix(s.config.Database.URL, "sqlite:"), "?")[0], "file:")), "subgate.key")
	var key []byte
	if s.config.Storage.EncryptionKeyFile != "" {
		key, err = readStorageKey(s.config.Storage.EncryptionKeyFile)
		if err != nil {
			return err
		}
	} else if s.config.Database.URL == ":memory:" || strings.Contains(s.config.Database.URL, "mode=memory") {
		key = make([]byte, 32)
		if _, err = rand.Read(key); err != nil {
			return err
		}
	} else {
		key, err = readStorageKey(keyPath)
		if errors.Is(err, os.ErrNotExist) {
			key = make([]byte, 32)
			if _, err = rand.Read(key); err != nil {
				return err
			}
			f, e := os.OpenFile(keyPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if e != nil {
				return e
			}
			_, err = f.Write(key)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		} else if err != nil {
			return err
		}
	}
	if err = s.initializeStorageCiphers(key); err != nil {
		return err
	}
	if err = s.bootstrapAdmin(); err != nil {
		return err
	}
	if s.config.Demo {
		return s.seedDemo()
	}
	return nil
}
func (s *Server) routes() {
	r := s.router
	r.Use(middleware.RequestID, middleware.Recoverer)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			started := time.Now()
			if s.metricRequests != nil {
				s.metricRequests.Inc()
			}
			defer func() {
				if s.metricLatency != nil {
					s.metricLatency.Observe(time.Since(started).Seconds())
				}
			}()
			next.ServeHTTP(w, r)
		})
	})
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Referrer-Policy", "no-referrer")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'")
			next.ServeHTTP(w, r)
		})
	})
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		ok := s.db.Health() == nil
		code := 200
		if !ok {
			code = 503
		}
		writeJSON(w, code, map[string]any{"status": map[bool]string{true: "ok", false: "error"}[ok], "database": ok})
	})
	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]any{"version": Version, "demo": s.config.Demo})
	})
	r.Get("/metrics", s.metricsEndpoint)
	r.Get("/install-agent.sh", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/x-shellscript; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(deploy.AgentInstaller)
	})
	r.Post("/api/v1/auth/login", s.login)
	r.Post("/api/v1/agents/enroll", s.enroll)
	r.Group(func(r chi.Router) {
		r.Use(s.agentAuth)
		r.Post("/api/v1/agents/heartbeat", s.heartbeat)
		r.Post("/api/v1/agents/monitor", s.monitorReport)
		r.Get("/api/v1/agents/shaper", s.shaperCommands)
		r.Put("/api/v1/agents/shaper-results", s.shaperResults)
		r.Post("/api/v1/agents/telemetry", s.telemetry)
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(s.authenticate)
		r.Get("/auth/me", s.me)
		r.Post("/auth/logout", s.logout)
		r.Get("/system/status", s.systemStatus)
		r.Get("/nodes", s.listNodes)
		r.Get("/nodes/{id}", s.getNode)
		r.Post("/nodes/{id}/actions/{action}", s.nodeAction)
		r.Get("/users", s.listUsers)
		r.Get("/users/{id}", s.getUser)
		r.Post("/panel/sync", s.syncHandler)
		r.Get("/live", s.live)
		r.Get("/live/stream", s.liveStream)
		r.Get("/live/export", s.exportLive)
		r.Get("/users/{id}/traffic", s.userTraffic)
		r.Get("/traffic", s.traffic)
		r.Get("/activity", s.activity)
		r.Get("/activity/export", s.exportActivity)
		r.Delete("/activity/{user}", s.purgeActivity)
		r.Get("/agents", s.agents)
		r.Post("/agents/enrollment-tokens", s.createEnrollment)
		r.Delete("/agents/{id}", s.revokeAgent)
		r.Get("/settings", s.settings)
		r.Get("/audit", s.auditList)
		r.Get("/incidents", s.incidents)
		r.Put("/incidents/{id}/review", s.reviewIncident)
		r.Get("/analysis/policy", s.getAnalysisPolicy)
		r.Put("/analysis/policy", s.putAnalysisPolicy)
		r.Get("/analysis/shaper", s.listShaper)
		r.Get("/analysis/monitor", s.listMonitor)
		r.Delete("/analysis/shaper/{id}", s.cancelShaper)
		r.Get("/ipintel/lookup", s.lookup)
		r.Post("/ipintel/reload", s.reloadIntel)
		r.Get("/backups", s.backups)
		r.Post("/backups", s.createBackup)
		r.Get("/backups/{name}", s.downloadBackup)
		r.Post("/notifications/test", s.notificationTest)
		for _, kind := range []string{"alerts"} {
			kind := kind
			r.Get("/"+kind, func(w http.ResponseWriter, r *http.Request) { s.listObjects(w, r, kind) })
			r.Put("/"+kind+"/{id}", func(w http.ResponseWriter, r *http.Request) { s.putObject(w, r, kind) })
			r.Delete("/"+kind+"/{id}", func(w http.ResponseWriter, r *http.Request) { s.deleteObject(w, r, kind) })
		}
	})
	assets, _ := fs.Sub(web.Assets, ".")
	files := http.FileServer(http.FS(assets))
	r.Get("/", files.ServeHTTP)
	r.Get("/app.js", files.ServeHTTP)
	r.Get("/styles.css", files.ServeHTTP)
	r.Get("/console", func(w http.ResponseWriter, r *http.Request) { r.URL.Path = "/"; files.ServeHTTP(w, r) })
}
func (s *Server) Handler() http.Handler { return s.router }
func (s *Server) Listen(addr string) error {
	s.startBackground()
	s.http = &http.Server{Addr: addr, Handler: s.router, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
	var err error
	if s.config.HTTP.TLS.Enabled {
		err = s.http.ListenAndServeTLS(s.config.HTTP.TLS.Cert, s.config.HTTP.TLS.Key)
	} else {
		err = s.http.ListenAndServe()
	}
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
func (s *Server) Shutdown(ctx context.Context) error {
	s.cancel()
	var err error
	if s.http != nil {
		err = s.http.Shutdown(ctx)
	}
	s.wg.Wait()
	s.panel.Close()
	if s.intel != nil {
		_ = s.intel.Close()
	}
	return err
}
func writeJSON(w http.ResponseWriter, code int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": msg})
}
func decode(w http.ResponseWriter, r *http.Request, value any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		fail(w, 400, "invalid request JSON")
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		fail(w, 400, "request must contain one JSON object")
		return false
	}
	return true
}
func hash(v string) string { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func (s *Server) encrypt(raw []byte, aad string) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.aead.Seal(nonce, nonce, raw, []byte(aad)), nil
}
func (s *Server) decrypt(raw []byte, aad string) ([]byte, error) {
	n := s.aead.NonceSize()
	if len(raw) < n {
		return nil, fmt.Errorf("invalid ciphertext")
	}
	if plain, err := s.aead.Open(nil, raw[:n], raw[n:], []byte(aad)); err == nil {
		return plain, nil
	}
	for _, previous := range s.previousAEADs {
		if plain, err := previous.Open(nil, raw[:n], raw[n:], []byte(aad)); err == nil {
			return plain, nil
		}
	}
	return nil, fmt.Errorf("storage integrity/key verification failed")
}
