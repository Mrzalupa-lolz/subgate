package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/subgate/subgate/core/internal/config"
	"github.com/subgate/subgate/core/internal/db"
	"github.com/subgate/subgate/core/internal/panel"
)

func TestPanelWriteConfiguration(t *testing.T) {
	for _, allow := range []bool{false, true} {
		name := "default read-only"
		if allow {
			name = "explicit write access"
		}
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				io.WriteString(w, `{"response":{"ok":true}}`)
			}))
			defer u.Close()
			cfg := config.Defaults()
			cfg.Demo = true
			cfg.Database.URL = ":memory:"
			cfg.Database.BackupPath = t.TempDir()
			cfg.Auth.AdminPassword = "synthetic-test-password"
			cfg.Panel.URL = u.URL
			cfg.Panel.AllowWrites = allow
			database, err := db.New(":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			if err = db.RunMigrations(database); err != nil {
				t.Fatal(err)
			}
			s, err := New(cfg, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}
			defer s.Shutdown(context.Background())
			_, err = s.panel.Request(context.Background(), "POST", "/api/users", nil)
			if allow {
				if err != nil || calls.Load() != 1 {
					t.Fatalf("explicit write not applied: %v, calls=%d", err, calls.Load())
				}
			} else if !errors.Is(err, panel.ErrReadOnly) || calls.Load() != 0 {
				t.Fatalf("default sent a write: %v, calls=%d", err, calls.Load())
			}
		})
	}
}
