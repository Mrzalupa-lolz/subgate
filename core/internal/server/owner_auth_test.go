package server

import (
	"bytes"
	"encoding/json"
	"github.com/subgate/subgate/pkg/wire"
	"strings"
	"testing"
	"time"
)

// Q6 replaces mandatory TOTP/RBAC with one password-authenticated owner.
func TestSingleOwnerPasswordLogin(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.db.Exec("UPDATE admin_users SET role='viewer',totp_secret='synthetic-unused' WHERE username='admin'"); err != nil {
		t.Fatal(err)
	}
	session := loginTest(t, s)
	if w := perform(s, "GET", "/api/v1/auth/me", nil, session, "", false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	if _, err := s.db.Exec("INSERT INTO admin_users(username,password_hash,role) VALUES('extra',?,'superadmin')", passwordHash("synthetic-extra-password")); err != nil {
		t.Fatal(err)
	}
	if err := s.bootstrapAdmin(); err == nil {
		t.Error("ambiguous multiple-account installation accepted")
	}
	if w := perform(s, "GET", "/api/v1/auth/me", nil, session, "", false); w.Code != 401 {
		t.Errorf("legacy extra account left owner session authorized: %d", w.Code)
	}
	if w := perform(s, "POST", "/api/v1/auth/login", map[string]string{"username": "extra", "password": "synthetic-extra-password"}, nil, "", false); w.Code != 401 {
		t.Errorf("extra account login accepted: %d", w.Code)
	}
}

func TestOwnerActivityAccess(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.db.Exec("UPDATE admin_users SET role='viewer'"); err != nil {
		t.Fatal(err)
	}
	session := loginTest(t, s)
	me := perform(s, "GET", "/api/v1/auth/me", nil, session, "", false)
	var identity map[string]any
	if err := json.Unmarshal(me.Body.Bytes(), &identity); err != nil {
		t.Fatal(err)
	}
	if _, ok := identity["Role"]; ok {
		t.Error("single owner still exposed as an RBAC role")
	}
	if _, ok := identity["role"]; ok {
		t.Error("single owner still exposed as an RBAC role")
	}
	credential, _ := enrollTest(t, s, session, "demo-node-1")
	batch := wire.Telemetry{BatchID: "owner-activity", NodeID: "demo-node-1", Events: []wire.Activity{{UserID: "1", Timestamp: time.Now().UTC(), DstHost: "owner.example.invalid", DstPort: 443}}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatal(w.Code)
	}
	for _, path := range []string{"/api/v1/activity?user=1", "/api/v1/activity/export?user=1"} {
		if w := perform(s, "GET", path, nil, nil, "", false); w.Code != 401 {
			t.Errorf("anonymous %s: %d", path, w.Code)
		}
		if w := perform(s, "GET", path, nil, session, "", false); w.Code != 200 || !strings.Contains(w.Body.String(), "owner.example.invalid") {
			t.Errorf("owner %s: %d %s", path, w.Code, w.Body)
		}
	}
	for _, path := range []string{"/api/v1/activity/1", "/api/v1/backups"} {
		method := "DELETE"
		if path == "/api/v1/backups" {
			method = "POST"
		}
		if w := perform(s, method, path, nil, nil, "", true); w.Code != 401 {
			t.Errorf("anonymous mutation %s: %d", path, w.Code)
		}
		if w := perform(s, method, path, nil, session, "", false); w.Code != 403 {
			t.Errorf("missing CSRF %s: %d", path, w.Code)
		}
	}
	w := perform(s, "POST", "/api/v1/backups", nil, session, "", true)
	var backup struct {
		Name string `json:"name"`
	}
	if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &backup) != nil || backup.Name == "" {
		t.Fatalf("owner backup: %d %s", w.Code, w.Body)
	}
	if w = perform(s, "GET", "/api/v1/backups/"+backup.Name, nil, nil, "", false); w.Code != 401 {
		t.Fatal("anonymous backup download", w.Code)
	}
	if w = perform(s, "GET", "/api/v1/backups/"+backup.Name, nil, session, "", false); w.Code != 200 || !bytes.HasPrefix(w.Body.Bytes(), []byte("SUBGATE1")) {
		t.Fatal("owner backup download", w.Code)
	}
	if w = perform(s, "DELETE", "/api/v1/activity/1", nil, session, "", true); w.Code != 200 {
		t.Fatal("owner purge", w.Code)
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_activity WHERE user_id='1'").Scan(&count); err != nil || count != 0 {
		t.Fatal("purge did not erase activity", count, err)
	}
}
