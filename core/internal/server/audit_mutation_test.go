package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/gateway"
	"github.com/subgate/subgate/core/internal/panel"
)

func TestEnrollmentAndSyncAuditFailures(t *testing.T) {
	t.Run("issue token", func(t *testing.T) {
		s := newTestServer(t)
		session := loginTest(t, s)
		if _, err := s.db.Exec("CREATE TRIGGER audit_fixture BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
			t.Fatal(err)
		}
		w := perform(s, "POST", "/api/v1/agents/enrollment-tokens", map[string]string{"node_id": "demo-node-1"}, session, "", true)
		if w.Code != 500 {
			t.Errorf("unaudited token issued: %d", w.Code)
		}
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_enrollment").Scan(&count); err != nil || count != 0 {
			t.Fatalf("token persisted without audit: %d %v", count, err)
		}
	})
	t.Run("enroll", func(t *testing.T) {
		s := newTestServer(t)
		session := loginTest(t, s)
		w := perform(s, "POST", "/api/v1/agents/enrollment-tokens", map[string]string{"node_id": "demo-node-1"}, session, "", true)
		if w.Code != 201 {
			t.Fatal(w.Code)
		}
		var issued struct {
			Token string `json:"token"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &issued); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec("CREATE TRIGGER audit_fixture BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
			t.Fatal(err)
		}
		w = perform(s, "POST", "/api/v1/agents/enroll", map[string]string{"node_id": "demo-node-1", "token": issued.Token}, nil, "", false)
		if w.Code != 500 {
			t.Errorf("unaudited enrollment succeeded: %d", w.Code)
		}
		var used, count int
		if err := s.db.QueryRow("SELECT used FROM sg_enrollment WHERE token_hash=?", hash(issued.Token)).Scan(&used); err != nil || used != 0 {
			t.Errorf("token consumed without audit: %d %v", used, err)
		}
		if err := s.db.QueryRow("SELECT COUNT(*) FROM agents").Scan(&count); err != nil || count != 0 {
			t.Errorf("agent persisted without audit: %d %v", count, err)
		}
		if _, err := s.db.Exec("DROP TRIGGER audit_fixture"); err != nil {
			t.Fatal(err)
		}
		if w = perform(s, "POST", "/api/v1/agents/enroll", map[string]string{"node_id": "demo-node-1", "token": issued.Token}, nil, "", false); w.Code != 201 {
			t.Fatalf("retry blocked: %d", w.Code)
		}
		if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='agent.enroll' AND status='success' AND admin_id IS NULL AND resource_id='demo-node-1'").Scan(&count); err != nil || count != 1 {
			t.Errorf("success outcome missing: %d %v", count, err)
		}
	})
	t.Run("manual sync", func(t *testing.T) {
		s := newTestServer(t)
		session := loginTest(t, s)
		before, err := s.objects("users")
		if err != nil {
			t.Fatal(err)
		}
		// A complete, changed synthetic snapshot exercises the persisted mutation.
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/users" {
				io.WriteString(w, `{"response":{"total":0,"users":[]}}`)
			} else {
				io.WriteString(w, `{"response":[]}`)
			}
		}))
		defer upstream.Close()
		s.config.Demo = false
		s.panel.Close()
		s.panel = panel.NewClient(upstream.URL, "test", s.logger)
		entry := gateway.CacheEntry{Data: []byte("last-good"), StoredAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
		if err := s.Save("audit-snapshot", entry); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec("CREATE TRIGGER audit_fixture BEFORE INSERT ON audit_log WHEN NEW.status='success' BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
			t.Fatal(err)
		}
		w := perform(s, "POST", "/api/v1/panel/sync", nil, session, "", true)
		if w.Code < 500 {
			t.Errorf("unaudited sync succeeded: %d", w.Code)
		}
		after, err := s.objects("users")
		if err != nil || len(after) != len(before) {
			t.Errorf("audit failure changed snapshot: %d -> %d %v", len(before), len(after), err)
		}
		if _, err := s.Load("audit-snapshot"); err != nil {
			t.Error("audit failure cleared last-good cache", err)
		}
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='panel.sync' AND status='failure'").Scan(&count); err != nil || count != 1 {
			t.Errorf("failure outcome missing: %d %v", count, err)
		}
		if _, err := s.db.Exec("DROP TRIGGER audit_fixture"); err != nil {
			t.Fatal(err)
		}
		w = perform(s, "POST", "/api/v1/panel/sync", nil, session, "", true)
		if w.Code != 200 {
			t.Fatalf("sync retry: %d %s", w.Code, w.Body)
		}
		after, err = s.objects("users")
		if err != nil || len(after) != 0 {
			t.Error("successful sync not committed", err)
		}
		if err := s.db.QueryRow("SELECT COUNT(*) FROM audit_log WHERE action='panel.sync' AND status='success'").Scan(&count); err != nil || count != 1 {
			t.Errorf("success outcome missing: %d %v", count, err)
		}
	})
}

func TestBackgroundSyncDoesNotRequireAdminAudit(t *testing.T) {
	s := newTestServer(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/users" {
			io.WriteString(w, `{"response":{"total":0,"users":[]}}`)
		} else {
			io.WriteString(w, `{"response":[]}`)
		}
	}))
	defer upstream.Close()
	s.config.Demo = false
	s.panel.Close()
	s.panel = panel.NewClient(upstream.URL, "test", s.logger)
	if _, err := s.db.Exec("CREATE TRIGGER audit_fixture BEFORE INSERT ON audit_log BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	if err := s.syncPanel(context.Background()); err != nil {
		t.Fatal(err)
	}
	users, err := s.objects("users")
	if err != nil || len(users) != 0 {
		t.Fatalf("background snapshot did not commit: %d %v", len(users), err)
	}
}
