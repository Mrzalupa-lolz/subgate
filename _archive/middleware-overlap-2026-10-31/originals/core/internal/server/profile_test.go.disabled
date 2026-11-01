package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/health"
	"github.com/subgate/subgate/core/internal/panel"
	"github.com/subgate/subgate/core/internal/quota"
	"github.com/subgate/subgate/core/internal/routing"
)

func syntheticProfile(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("../routing/testdata/warp-profile.json")
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestGlobalProfileStageAcrossRoutingBranches(t *testing.T) {
	for _, branch := range []string{"missing-user", "no-policy", "no-match", "tier-unavailable", "health", "pin", "quota"} {
		t.Run(branch, func(t *testing.T) {
			s := newTestServer(t)
			profile := syntheticProfile(t)
			if err := s.saveObject("injections", "warp", routing.Injection{ID: "warp", RuleType: "xray-profile", Format: "xray-json", Content: profile, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			// Legacy rules must not start running on newly covered branches.
			_ = s.saveObject("injections", "legacy", routing.Injection{ID: "legacy", RuleType: "append", Content: `{"remarks":"legacy","outbounds":[{"protocol":"freedom"}]}`, Enabled: true})
			_, err := s.db.Exec("DELETE FROM sg_objects WHERE kind IN ('pins','policies','tiers','users','nodes')")
			if err != nil {
				t.Fatal(err)
			}
			if branch != "missing-user" {
				_ = s.saveObject("users", "u", panel.User{ID: "u", ShortUUID: "profile-short", Status: "active"})
			}
			for _, id := range []string{"one", "two"} {
				_ = s.saveObject("nodes", id, panel.Node{ID: id, Status: "online", IPAddress: id + ".example"})
			}
			if branch == "no-match" {
				_ = s.saveObject("policies", "p", map[string]any{"enabled": true, "tier_id": "missing", "conditions": map[string]any{"user_agent": "does-not-match"}})
			}
			if branch == "tier-unavailable" {
				_ = s.saveObject("policies", "p", map[string]any{"enabled": true, "tier_id": "missing"})
			}
			if branch == "health" {
				s.health.RestoreNode(health.NodeHealth{NodeID: "one", Status: health.StatusQuarantined})
			}
			if branch == "pin" || branch == "quota" {
				_ = s.saveObject("pins", "p", map[string]any{"enabled": true, "user_id": "u", "node_id": "one"})
			}
			if branch == "quota" {
				if err := s.quota.Upsert(quota.Rule{ID: "q", NodeID: "one", Scope: "per_user", UserID: "u", LimitBytes: 1, Period: "day", Enabled: true}); err != nil {
					t.Fatal(err)
				}
				// Existing quota accounting reads persisted usage.
				if _, err := s.db.Exec("INSERT INTO sg_usage(node_id,user_id,timestamp,uplink,downlink) VALUES(?,?,?,?,?)", "one", "u", time.Now().UnixMilli(), 0, 2); err != nil {
					t.Fatal(err)
				}
			}
			raw := []byte(`[{"remarks":"one","outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"one.example"}]}}]},{"remarks":"two","outbounds":[{"protocol":"vless","settings":{"vnext":[{"address":"two.example"}]}}]}]`)
			req := httptest.NewRequest("GET", "http://console.test/sub/profile-short/v2ray-json", nil)
			req.SetPathValue("shortUuid", "profile-short")
			req.SetPathValue("clientType", "v2ray-json")
			out, policy, err := s.transformSubscription(req, raw, http.Header{})
			if err != nil {
				t.Fatal(err)
			}
			if branch == "quota" {
				if policy != "quota-exhausted" || !bytes.Equal(out, raw) {
					t.Fatal("quota branch changed", policy)
				}
				return
			}
			var profiles []json.RawMessage
			if json.Unmarshal(out, &profiles) != nil || len(profiles) < 2 {
				t.Fatal("profile missing", policy)
			}
			var expected json.RawMessage = []byte(profile)
			encoded, _ := json.Marshal(expected)
			if !bytes.Equal(profiles[len(profiles)-1], encoded) {
				t.Fatal("injected profile was filtered or modified")
			}
			if branch != "pin" && strings.Contains(string(out), "legacy") {
				t.Fatal("legacy scope expanded")
			}
			if branch == "health" && strings.Contains(string(out), "one.example") {
				t.Fatal("health filtering bypassed")
			}
		})
	}
}

func TestProfileAPIAndGatewayLifecycle(t *testing.T) {
	s := newTestServer(t)
	s.config.Demo = false
	session := loginTest(t, s)
	profile := syntheticProfile(t)
	rule := routing.Injection{Name: "Private WARP", RuleType: "xray-profile", Format: "xray-json", Content: profile, Enabled: true}
	path := "/api/v1/injections/profile"
	if w := perform(s, "GET", path, nil, nil, "", false); w.Code != 401 {
		t.Fatal("detail auth missing", w.Code)
	}
	if w := perform(s, "PUT", path, rule, session, "", false); w.Code != 403 {
		t.Fatal("CSRF missing")
	}
	if w := perform(s, "PUT", path, rule, session, "", true); w.Code != 200 {
		t.Fatal("save failed", w.Code)
	}
	w := perform(s, "GET", path, nil, session, "", false)
	var loaded routing.Injection
	if json.Unmarshal(w.Body.Bytes(), &loaded) != nil || loaded.Content != profile || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("original text not preserved")
	}
	if w := perform(s, "GET", "/api/v1/injections", nil, session, "", false); strings.Contains(w.Body.String(), "SYNTHETIC_PRIVATE") {
		t.Fatal("list leaked content")
	}
	var audit string
	if err := s.db.QueryRow("SELECT coalesce(group_concat(resource_id),'') FROM audit_log WHERE action='injection.upsert'").Scan(&audit); err != nil || strings.Contains(audit, "SYNTHETIC") {
		t.Fatal("unsafe audit")
	}
	original := `[{"remarks":"existing","outbounds":[{"protocol":"freedom"}]}]`
	var requests atomic.Int32
	var status atomic.Int32
	status.Store(200)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/profile-short/v2ray-json" {
			t.Error("wrong upstream format path")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=100")
		w.Header().Set("Content-Length", fmt.Sprint(len(original)))
		w.WriteHeader(int(status.Load()))
		if r.Method != "HEAD" {
			_, _ = w.Write([]byte(original))
		}
	}))
	defer upstream.Close()
	s.gateway.Configure(upstream.URL, nil)
	get := func() *httptest.ResponseRecorder {
		return perform(s, "GET", "/sub/profile-short/v2ray-json", nil, nil, "", false)
	}
	for i := 0; i < 2; i++ {
		w := get()
		var profiles []json.RawMessage
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &profiles) != nil || len(profiles) != 2 {
			t.Fatal("missing/duplicated profile")
		}
		if w.Header().Get("Subscription-Userinfo") == "" || !strings.HasPrefix(w.Header().Get("Content-Type"), "application/json") {
			t.Fatal("metadata lost")
		}
		if i == 1 && w.Header().Get("X-Cache-Status") != "hit" {
			t.Fatal("cache hit not covered")
		}
	}
	if requests.Load() != 1 {
		t.Fatal("upstream cache not used")
	}
	rule.Content = strings.Replace(profile, `"WARP"`, `"WARP edited"`, 1)
	if w := perform(s, "PUT", path, rule, session, "", true); w.Code != 200 {
		t.Fatal("edit failed")
	}
	if w := get(); strings.Count(w.Body.String(), "WARP edited") != 1 || strings.Contains(w.Body.String(), `"remarks":"WARP"`) {
		t.Fatal("edit duplicated profile")
	}
	rule.Enabled = false
	if w := perform(s, "PUT", path, rule, session, "", true); w.Code != 200 {
		t.Fatal("disable failed")
	}
	if w := get(); w.Body.String() != original {
		t.Fatal("disabled rule applied")
	}
	if w := perform(s, "DELETE", path, nil, session, "", true); w.Code != 200 {
		t.Fatal("delete failed")
	}
	if w := get(); w.Body.String() != original {
		t.Fatal("deleted rule applied")
	}
	rule.Enabled = true
	if w := perform(s, "PUT", path, rule, session, "", true); w.Code != 200 {
		t.Fatal("restore failed")
	}
	for _, code := range []int{401, 403, 404, 500} {
		status.Store(int32(code))
		_ = s.gateway.Clear()
		w := get()
		if strings.Contains(w.Body.String(), "WARP") || w.Code == 200 {
			t.Fatal("upstream failure bypassed", code, w.Code)
		}
	}
	status.Store(200)
	_ = s.gateway.Clear()
	w = perform(s, "HEAD", "/sub/profile-short/v2ray-json", nil, nil, "", false)
	if w.Code != 200 || w.Body.Len() != 0 || w.Header().Get("Content-Length") != fmt.Sprint(len(original)) {
		t.Fatal("HEAD changed")
	}
	// Both update and deletion must roll back when the audit fails.
	if _, err := s.db.Exec("DROP TABLE audit_log"); err != nil {
		t.Fatal(err)
	}
	rule.Content = strings.Replace(profile, "WARP", "rolled-back", 1)
	for _, method := range []string{"PUT", "DELETE"} {
		if w := perform(s, method, path, rule, session, "", true); w.Code != 500 {
			t.Fatal("audit failure ignored")
		}
	}
	var stored routing.Injection
	if err := s.object("injections", "profile", &stored); err != nil || stored.Content == rule.Content {
		t.Fatal("audit rollback failed")
	}
}

func TestProfileAPIValidationAndRuleLimit(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	for _, content := range []string{"", "  ", "{}", "[]", `{"outbounds":[{"type":"direct"}]}`, `{"outbounds":[{"protocol":"freedom"}]} {}`, strings.Repeat("a", (256<<10)+1)} {
		w := perform(s, "PUT", "/api/v1/injections/invalid", routing.Injection{RuleType: "xray-profile", Format: "xray-json", Content: content}, session, "", true)
		if w.Code != 400 {
			t.Fatal("invalid content accepted", w.Code)
		}
	}
	rule := routing.Injection{RuleType: "xray-profile", Format: "xray-json", Content: syntheticProfile(t), Enabled: true}
	for i := 0; i < 100; i++ {
		if err := s.saveObject("injections", fmt.Sprintf("rule-%03d", i), rule); err != nil {
			t.Fatal(err)
		}
	}
	if w := perform(s, "PUT", "/api/v1/injections/overflow", rule, session, "", true); w.Code != 400 {
		t.Fatal("rule count limit ignored", w.Code)
	}
	if w := perform(s, "PUT", "/api/v1/injections/rule-000", rule, session, "", true); w.Code != 200 {
		t.Fatal("editing at limit blocked", w.Code)
	}
}

func TestProfileOrderContextAndDamagedStorage(t *testing.T) {
	s := newTestServer(t)
	for _, id := range []string{"z-last", "a-first"} {
		profile := strings.Replace(syntheticProfile(t), `"WARP"`, fmt.Sprintf(`%q`, id), 1)
		if err := s.saveObject("injections", id, routing.Injection{ID: id, RuleType: "xray-profile", Format: "xray-json", Content: profile, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.saveObject("injections", "broken", routing.Injection{RuleType: "xray-profile", Format: "xray-json", Content: "invalid synthetic profile", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"remarks":"source","outbounds":[{"protocol":"freedom"}]}`)
	req := httptest.NewRequest("GET", "http://console.test/sub/unsynchronized/v2ray-json", nil)
	req.SetPathValue("shortUuid", "unsynchronized")
	req.Header.Set("User-Agent", "Happ Incy")
	for _, kind := range []string{"", "singbox", "mihomo"} {
		req.SetPathValue("clientType", kind)
		out, _, err := s.transformSubscription(req, raw, nil)
		if err != nil || !bytes.Equal(out, raw) {
			t.Fatal("UA or incompatible context enabled profiles")
		}
	}
	req.SetPathValue("clientType", "v2ray-json")
	out, _, err := s.transformSubscription(req, raw, nil)
	var profiles []struct {
		Remarks string `json:"remarks"`
	}
	if err != nil || json.Unmarshal(out, &profiles) != nil || len(profiles) != 3 || profiles[1].Remarks != "a-first" || profiles[2].Remarks != "z-last" {
		t.Fatal("profile order/corrupt storage handling failed")
	}
}
