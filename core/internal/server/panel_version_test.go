package server

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
)

func TestPanelVersionMismatch(t *testing.T) {
	for _, tc := range []struct {
		name, body, wantState, wantVersion string
		status                             int
	}{
		{"approved", `{"response":{"version":"3.4.5"}}`, "compatible", "3.4.5", 200},
		{"mismatch", `{"response":{"version":"3.5.0"}}`, "mismatch", "3.5.0", 200},
		{"scope denied", `{"sensitive":"do-not-log"}`, "unavailable", "", 403},
		{"missing", `{"response":{}}`, "unavailable", "", 200},
		{"malformed type", `{"response":{"version":42}}`, "unavailable", "", 200},
		{"untrusted value", `{"response":{"version":"do-not-log"}}`, "unavailable", "", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newTestServer(t)
			s.config.Demo = false
			logs := &lockedLogBuffer{}
			s.logger = slog.New(slog.NewTextHandler(logs, nil))
			synced := make(chan struct{})
			var once sync.Once
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "GET" {
					t.Error("version check must remain read-only")
				}
				switch r.URL.Path {
				case "/api/system/stats/recap":
					w.WriteHeader(tc.status)
					io.WriteString(w, tc.body)
				case "/api/nodes", "/api/hosts":
					io.WriteString(w, `{"response":[]}`)
					if r.URL.Path == "/api/nodes" {
						once.Do(func() { close(synced) })
					}
				case "/api/users":
					io.WriteString(w, `{"response":{"users":[],"total":0}}`)
				default:
					t.Error("version check used endpoint outside schema")
					w.WriteHeader(404)
				}
			}))
			defer upstream.Close()
			s.panel = panel.NewClient(upstream.URL, "synthetic", s.logger)
			s.wg.Add(1)
			go s.background()
			select {
			case <-synced:
			case <-time.After(3 * time.Second):
				t.Fatal("startup version check prevented synchronization")
			}
			// Version diagnostics are published before initial synchronization.
			out := httptest.NewRecorder()
			s.systemStatus(out, httptest.NewRequest("GET", "/api/v1/system/status", nil))
			var result struct {
				PanelVersion struct{ Expected, Actual, State string } `json:"panel_version"`
			}
			if err := json.Unmarshal(out.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			got := result.PanelVersion
			if got.Expected != "3.4.5" || got.Actual != tc.wantVersion || got.State != tc.wantState {
				t.Fatalf("version diagnostics: %+v", got)
			}
			if tc.wantState == "mismatch" && !strings.Contains(logs.String(), "panel version mismatch") {
				t.Fatal("startup mismatch warning missing")
			}
			if strings.Contains(logs.String(), "do-not-log") || strings.Contains(out.Body.String(), "do-not-log") {
				t.Fatal("upstream private value exposed")
			}
			subscription := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "original-subscription") }))
			defer subscription.Close()
			s.gateway.Configure(subscription.URL, s)
			response := perform(s, "GET", "/sub/synthetic-short-id", nil, nil, "", false)
			if response.Code != 200 || response.Body.String() != "original-subscription" {
				t.Fatal("version diagnostics broke subscription fail-open")
			}
		})
	}
}

type lockedLogBuffer struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *lockedLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}
func (b *lockedLogBuffer) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.data.String() }
