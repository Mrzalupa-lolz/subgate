package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewareOwnedAPIsAreInactive(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	for _, endpoint := range []struct{ method, path string }{
		{"GET", "/api/v1/injections"},
		{"GET", "/api/v1/injections/legacy"},
		{"PUT", "/api/v1/injections/legacy"},
		{"DELETE", "/api/v1/injections/legacy"},
		{"POST", "/api/v1/users/1/actions/disable"},
		{"POST", "/api/v1/users/1/actions/enable"},
		{"POST", "/api/v1/users/1/actions/reset-traffic"},
	} {
		if w := perform(s, endpoint.method, endpoint.path, nil, session, "", true); w.Code != http.StatusNotFound {
			t.Errorf("archived route %s %s returned %d", endpoint.method, endpoint.path, w.Code)
		}
	}
	for _, path := range []string{"/api/v1/users", "/api/v1/users/1", "/api/v1/quotas"} {
		if w := perform(s, "GET", path, nil, session, "", false); w.Code != http.StatusOK {
			t.Errorf("retained route %s returned %d", path, w.Code)
		}
	}
}

func TestLegacyInjectionsRemainStoredWithoutExecution(t *testing.T) {
	s := newTestServer(t)
	if err := s.saveObject("users", "routing-user", map[string]any{"id": "routing-user", "short_uuid": "abc"}); err != nil {
		t.Fatal(err)
	}
	if err := s.saveObject("nodes", "fi", map[string]any{"id": "fi", "ip_address": "fi.example", "status": "online"}); err != nil {
		t.Fatal(err)
	}
	if err := s.saveObject("nodes", "other", map[string]any{"id": "other", "ip_address": "other.example", "status": "online"}); err != nil {
		t.Fatal(err)
	}
	if err := s.saveObject("pins", "routing-pin", map[string]any{"user_id": "routing-user", "node_id": "fi", "enabled": true}); err != nil {
		t.Fatal(err)
	}
	for id, rule := range map[string]map[string]any{
		"legacy-uri":     {"rule_type": "append", "content": "vless://extra@extra.example:443", "enabled": true},
		"legacy-profile": {"rule_type": "xray-profile", "format": "xray-json", "content": `{"outbounds":[{"protocol":"freedom"}]}`, "enabled": true},
	} {
		if err := s.saveObject("injections", id, rule); err != nil {
			t.Fatal(err)
		}
	}
	for _, input := range []struct{ kind, body string }{
		{"", "vless://one@fi.example:443\nvless://two@other.example:443\n"},
		{"v2ray-json", `[{"outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"fi.example","port":443}]}}]}]`},
	} {
		req := httptest.NewRequest("GET", "http://console.test/sub/abc", nil)
		req.SetPathValue("shortUuid", "abc")
		req.SetPathValue("clientType", input.kind)
		out, policy, err := s.transformSubscription(req, []byte(input.body), nil)
		if err != nil || policy != "pin" || strings.Contains(string(out), "extra.example") || strings.Contains(string(out), "freedom") || strings.Contains(string(out), "other.example") {
			t.Fatalf("archived injections affected active routing: %s %s %v", policy, out, err)
		}
	}
	var count int
	if err := s.db.QueryRow("SELECT count(*) FROM sg_objects WHERE kind='injections'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("archived rules were removed: count=%d err=%v", count, err)
	}
}
