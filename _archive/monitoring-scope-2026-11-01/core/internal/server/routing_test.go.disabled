package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/pkg/wire"
)

func TestPerRequestRoutingKeepsUnmappedProtocolsAndHealthyFallback(t *testing.T) {
	s := newTestServer(t)
	raw := []byte("vless://secret@fi.example:443?future=true#fi\nfuture://secret@nl.example:443#nl\nunknown://secret@unmapped.example:443#unknown\n")
	for _, n := range []panel.Node{{ID: "fi", IPAddress: "fi.example", Status: "online"}, {ID: "nl", IPAddress: "nl.example", Status: "online"}} {
		if err := s.saveObject("nodes", n.ID, n); err != nil {
			t.Fatal(err)
		}
	}
	_ = s.saveObject("users", "routing-user", panel.User{ID: "routing-user", ShortUUID: "abc", Status: "active"})
	_ = s.saveObject("tiers", "first", map[string]any{"id": "first", "enabled": true, "strategy": "roundRobin", "node_ids": []string{"fi", "nl"}, "min_healthy": 1})
	_ = s.saveObject("policies", "default", map[string]any{"id": "default", "enabled": true, "tier_id": "first", "conditions": map[string]any{}})
	req := httptest.NewRequest("GET", "http://console.test/sub/abc", nil)
	req.SetPathValue("shortUuid", "abc")
	first, policy, err := s.transformSubscription(req, raw, http.Header{})
	if err != nil || policy != "tier" || !strings.Contains(string(first), "fi.example") || strings.Contains(string(first), "nl.example") || !strings.Contains(string(first), "unmapped.example") {
		t.Fatalf("first %s %s %v", first, policy, err)
	}
	second, _, err := s.transformSubscription(req, raw, http.Header{})
	if err != nil || !strings.Contains(string(second), "nl.example") || strings.Contains(string(second), "fi.example") {
		t.Fatalf("second %s %v", second, err)
	}
	_ = s.saveObject("pins", "pin", map[string]any{"user_id": "routing-user", "node_id": "fi", "enabled": true})
	pinned, policy, err := s.transformSubscription(req, raw, nil)
	if err != nil || policy != "pin" || !strings.Contains(string(pinned), "fi.example") {
		t.Fatal(string(pinned), policy, err)
	}
	_ = s.saveObject("nodes", "fi", panel.Node{ID: "fi", IPAddress: "fi.example", Status: "offline"})
	out, policy, err := s.transformSubscription(req, raw, nil)
	if err != nil || policy != "tier" || strings.Contains(string(out), "fi.example") {
		t.Fatal(string(out), policy, err)
	}
}
func TestTrustedProxyChainCannotBeSpoofed(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "http://console.test", nil)
	req.RemoteAddr = "192.0.2.20:1234"
	req.Header.Set("X-Forwarded-For", "8.8.8.8")
	if got := s.clientIP(req); got != "192.0.2.20" {
		t.Fatal(got)
	}
	s.config.HTTP.TrustedProxies = []string{"192.0.2.0/24"}
	req.Header.Set("X-Forwarded-For", "8.8.8.8, 203.0.113.9, 192.0.2.10")
	if got := s.clientIP(req); got != "203.0.113.9" {
		t.Fatal(got)
	}
}
func TestPolicyInvalidConditionsAndTierReferenceCannotBeDeleted(t *testing.T) {
	s := newTestServer(t)
	cookie := loginTest(t, s)
	w := perform(s, "PUT", "/api/v1/policies/bad", map[string]any{"name": "Bad", "tier_id": "europe", "conditions": map[string]any{"unimplemented": true}}, cookie, "", true)
	if w.Code != 400 {
		t.Fatal(w.Code, w.Body)
	}
	w = perform(s, "PUT", "/api/v1/policies/default", map[string]any{"name": "Default", "tier_id": "europe", "conditions": map[string]any{}, "enabled": true}, cookie, "", true)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body)
	}
	w = perform(s, "DELETE", "/api/v1/tiers/europe", nil, cookie, "", true)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body)
	}
}

func TestQuotaVerticalSlice(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)

	if _, err := s.db.Exec(`INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('nodes','fi',?,unixepoch())`, `{"id":"fi","ip_address":"fi.example","status":"online"}`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec("INSERT INTO nodes(id,name,ip_address) VALUES('fi','Finland','fi.example')"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO sg_objects(kind,id,data,updated_at) VALUES('users','routing-user',?,unixepoch())`, `{"id":"routing-user","short_uuid":"abc","status":"active"}`); err != nil {
		t.Fatal(err)
	}
	_ = s.saveObject("tiers", "first", map[string]any{"id": "first", "enabled": true, "strategy": "roundRobin", "node_ids": []string{"fi"}, "min_healthy": 1})
	_ = s.saveObject("policies", "default", map[string]any{"id": "default", "enabled": true, "tier_id": "first", "conditions": map[string]any{}})
	req := httptest.NewRequest("GET", "http://console.test/sub/abc", nil)
	req.SetPathValue("shortUuid", "abc")
	out, policy, err := s.transformSubscription(req, []byte("vless://one@fi.example:443\n"), nil)
	if err != nil || policy != "tier" || string(out) != "vless://one@fi.example:443\n" {
		t.Fatalf("routing before quota failed: policy=%q err=%v output=%s", policy, err, out)
	}

	w := perform(s, "PUT", "/api/v1/quotas/user-limit", map[string]any{
		"node_id":     "fi",
		"user_id":     "routing-user",
		"scope":       "per_user",
		"limit_bytes": 100,
		"period":      "day",
		"enabled":     true,
	}, session, "", true)
	if w.Code != 200 {
		t.Fatalf("quota save: %d %s", w.Code, w.Body)
	}
	credential, _ := enrollTest(t, s, session, "fi")
	batch := wire.Telemetry{NodeID: "fi", BatchID: "quota-routing", Samples: []wire.Sample{{UserID: "routing-user", Timestamp: time.Now().UTC(), DownlinkDelta: 100}}}
	if w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, credential, false); w.Code != 200 {
		t.Fatalf("quota telemetry: %d %s", w.Code, w.Body)
	}
	out, policy, err = s.transformSubscription(req, []byte("vless://one@fi.example:443\n"), nil)
	if err != nil || policy != "quota-exhausted" || string(out) != "vless://one@fi.example:443\n" {
		t.Fatalf("quota fail-open behavior changed: policy=%q err=%v output=%q", policy, err, out)
	}

	if _, err := s.db.Exec("UPDATE admin_users SET role='viewer' WHERE username='admin'"); err != nil {
		t.Fatal(err)
	}
	if w := perform(s, "DELETE", "/api/v1/quotas/user-limit", nil, session, "", true); w.Code != 200 {
		t.Fatalf("owner quota mutation blocked by legacy role: %d", w.Code)
	}
}
