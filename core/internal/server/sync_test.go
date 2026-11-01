package server

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/panel"
)

func TestPanelSyncCommitsCompleteSnapshotAndPreservesLastGood(t *testing.T) {
	s := newTestServer(t)
	var failPage atomic.Bool
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/nodes":
			io.WriteString(w, `{"response":[{"uuid":"real-node","name":"Real","address":"192.0.2.1","isConnected":true}]}`)
		case "/api/hosts":
			io.WriteString(w, `{"response":[]}`)
		case "/api/users":
			requests.Add(1)
			offset, err := strconv.Atoi(r.URL.Query().Get("start"))
			if err != nil {
				t.Error(err)
			}
			if offset == 0 {
				io.WriteString(w, `{"response":{"total":2,"users":[{"uuid":"real-user-1","shortUuid":"short1","username":"one","status":"ACTIVE","trafficLimitBytes":0,"expireAt":"2027-01-01T00:00:00Z","userTraffic":{"usedTrafficBytes":0}}]}}`)
			} else if offset == 1 && !failPage.Load() {
				io.WriteString(w, `{"response":{"total":2,"users":[{"uuid":"real-user-2","shortUuid":"short2","username":"two","status":"ACTIVE","trafficLimitBytes":0,"expireAt":"2027-01-01T00:00:00Z","userTraffic":{"usedTrafficBytes":0}}]}}`)
			} else {
				w.WriteHeader(503)
			}
		default:
			t.Errorf("unexpected sync request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer upstream.Close()
	s.config.Demo = false
	s.panel.Close()
	s.panel = panel.NewClient(upstream.URL, "panel-key", s.logger)
	if err := s.syncPanel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("did not paginate actual returned count: %d", requests.Load())
	}
	users, err := s.objects("users")
	if err != nil || len(users) != 2 {
		t.Fatalf("snapshot users: %v %v", users, err)
	}
	nodes, err := s.objects("nodes")
	if err != nil || len(nodes) != 1 {
		t.Fatalf("snapshot nodes: %v %v", nodes, err)
	}
	before := s.lastSync
	failPage.Store(true)
	if err := s.syncPanel(context.Background()); err == nil {
		t.Fatal("failed panel page accepted")
	}
	users, err = s.objects("users")
	if err != nil || len(users) != 2 {
		t.Fatalf("failed pagination erased last good: %v %v", users, err)
	}
	if !s.lastSync.Equal(before) || s.syncError == "" {
		t.Fatalf("sync state wrong: before=%v after=%v error=%s", before, s.lastSync, s.syncError)
	}
}

func TestWebhookHMACReplayPreservesLegacyStorage(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	entry := legacySnapshot{Data: []byte("private subscription snapshot"), StoredAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
	if err := s.Save("snapshot-id", entry); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": time.Now().UTC().Format(time.RFC3339), "data": webhookUserFixture("user-1", "short-1", "ACTIVE")})
	mac := hmac.New(sha256.New, []byte(s.config.Panel.WebhookKey))
	mac.Write(body)
	signature := hex.EncodeToString(mac.Sum(nil))
	call := func(raw []byte, sig string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "http://console.test"+s.config.Panel.WebhookPath, strings.NewReader(string(raw)))
		r.Header.Set("X-Remnawave-Signature", "sha256="+sig)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, r)
		return w
	}
	if w := call(body, "00"); w.Code != 401 {
		t.Fatalf("invalid webhook signature accepted: %d", w.Code)
	}
	if _, err := s.Load("snapshot-id"); err != nil {
		t.Fatalf("invalid webhook deleted snapshot: %v", err)
	}
	if w := call(body, signature); w.Code != 202 {
		t.Fatalf("signed webhook: %d %s", w.Code, w.Body)
	}
	if _, err := s.Load("snapshot-id"); err != nil {
		t.Fatal("webhook changed retired storage")
	}
	if w := call(body, signature); w.Code != 200 || !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatalf("webhook replay: %d %s", w.Code, w.Body)
	}
	oldBody, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": time.Now().Add(-25 * time.Hour).UTC().Format(time.RFC3339), "data": webhookUserFixture("user-1", "short-1", "ACTIVE")})
	mac = hmac.New(sha256.New, []byte(s.config.Panel.WebhookKey))
	mac.Write(oldBody)
	if w := call(oldBody, hex.EncodeToString(mac.Sum(nil))); w.Code != 400 {
		t.Fatalf("old signed event accepted: %d", w.Code)
	}
}

func TestWebhookRefreshesUserBeforeDeadline(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	if err := s.saveObject("users", "user-1", panel.User{ID: "user-1", ShortUUID: "short-1", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": time.Now().UTC().Format(time.RFC3339), "data": webhookUserFixture("user-1", "short-1", "DISABLED")})
	mac := hmac.New(sha256.New, []byte(s.config.Panel.WebhookKey))
	mac.Write(body)
	r := httptest.NewRequest("POST", "http://console.test"+s.config.Panel.WebhookPath, strings.NewReader(string(body)))
	r.Header.Set("X-Remnawave-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	w := httptest.NewRecorder()
	start := time.Now()
	s.Handler().ServeHTTP(w, r)
	if w.Code != 202 || time.Since(start) > time.Second {
		t.Fatalf("webhook refresh response: %d latency=%v", w.Code, time.Since(start))
	}
	var user panel.User
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if err := s.object("users", "user-1", &user); err == nil && user.Status == "disabled" && user.TrafficLimitBytes == 123 && user.UsedTrafficBytes == 12 && user.ShortUUID == "short-1" {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("user state was not refreshed: %+v", user)
}

func TestPanelSyncRejectsOversizedTotalWithoutErasingSnapshot(t *testing.T) {
	s := newTestServer(t)
	before, err := s.objects("users")
	if err != nil || len(before) == 0 {
		t.Fatal("demo snapshot missing", err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/nodes" {
			io.WriteString(w, `{"response":[]}`)
			return
		}
		io.WriteString(w, `{"response":{"total":1000001,"users":[]}}`)
	}))
	defer upstream.Close()
	s.config.Demo = false
	s.panel.Close()
	s.panel = panel.NewClient(upstream.URL, "test", s.logger)
	if err := s.syncPanel(context.Background()); err == nil {
		t.Fatal("oversized incomplete snapshot accepted")
	}
	after, err := s.objects("users")
	if err != nil || len(after) != len(before) {
		t.Fatal("invalid pagination erased snapshot", err)
	}
}

func signedWebhook(s *Server, body []byte) *httptest.ResponseRecorder {
	mac := hmac.New(sha256.New, []byte(s.config.Panel.WebhookKey))
	mac.Write(body)
	r := httptest.NewRequest("POST", "http://console.test"+s.config.Panel.WebhookPath, strings.NewReader(string(body)))
	r.Header.Set("X-Remnawave-Signature", hex.EncodeToString(mac.Sum(nil)))
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}

func TestWebhookRejectsMalformedUserWithoutMutation(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	for _, data := range []any{nil, map[string]any{"uuid": "user-1"}, map[string]any{"uuid": "user-1", "shortUuid": "short-1"}, map[string]any{"uuid": "user-1", "shortUuid": "short-1", "status": "ACTIVE", "trafficLimitBytes": "bad"}} {
		body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "data": data})
		if w := signedWebhook(s, body); w.Code != 400 {
			t.Fatalf("malformed user accepted: %d", w.Code)
		}
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_webhooks").Scan(&count); err != nil || count != 0 {
		t.Fatalf("malformed event recorded: %d %v", count, err)
	}
}

func TestWebhookUnknownEventDoesNotUpsertUser(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	body, _ := json.Marshal(map[string]any{"event": "future.event", "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "data": map[string]any{"uuid": "future-user", "shortUuid": "future-short", "status": "ACTIVE"}})
	if w := signedWebhook(s, body); w.Code != 202 {
		t.Fatalf("unknown event: %d", w.Code)
	}
	var user panel.User
	if err := s.object("users", "future-user", &user); err == nil {
		t.Fatal("unknown event mutated users")
	}
}

func TestWebhookDuringPanelFetchStaysFresh(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	s.config.Demo = false
	started, release := make(chan struct{}), make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/nodes":
			close(started)
			<-release
			io.WriteString(w, `{"response":[]}`)
		case "/api/users":
			io.WriteString(w, `{"response":{"total":1,"users":[{"uuid":"user-1","shortUuid":"short-1","username":"fixture","status":"ACTIVE","trafficLimitBytes":123,"expireAt":"2027-01-01T00:00:00Z","userTraffic":{"usedTrafficBytes":12}}]}}`)
		default:
			io.WriteString(w, `{"response":[]}`)
		}
	}))
	defer upstream.Close()
	unblock := func() { close(release) }
	defer func() {
		select {
		case <-release:
		default:
			unblock()
		}
	}()
	s.panel.Close()
	s.panel = panel.NewClient(upstream.URL, "test", s.logger)
	synced := make(chan error, 1)
	go func() { synced <- s.syncPanel(context.Background()) }()
	<-started
	body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "data": webhookUserFixture("user-1", "short-1", "DISABLED")})
	webhookDone := make(chan *httptest.ResponseRecorder, 1)
	go func() { webhookDone <- signedWebhook(s, body) }()
	select {
	case w := <-webhookDone:
		if w.Code != 202 {
			t.Fatalf("webhook: %d", w.Code)
		}
	case <-time.After(time.Second):
		t.Fatal("panel fetch blocked webhook freshness")
	}
	unblock()
	if err := <-synced; err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatal("obsolete sync committed")
	}
	var user panel.User
	if err := s.object("users", "user-1", &user); err != nil || user.Status != "disabled" {
		t.Fatalf("sync erased webhook: %v %+v", err, user)
	}
}

func TestWebhookUserPersistenceRollsBackReplayMarker(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	if _, err := s.db.Exec("CREATE TRIGGER fail_webhook_user BEFORE INSERT ON sg_objects WHEN NEW.kind='users' BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "data": webhookUserFixture("user-1", "short-1", "DISABLED")})
	if w := signedWebhook(s, body); w.Code != 500 {
		t.Fatalf("failed write accepted: %d", w.Code)
	}
	if _, err := s.db.Exec("DROP TRIGGER fail_webhook_user"); err != nil {
		t.Fatal(err)
	}
	if w := signedWebhook(s, body); w.Code != 202 {
		t.Fatalf("retry swallowed by replay marker: %d %s", w.Code, w.Body)
	}
}

func webhookUserFixture(id, short, status string) map[string]any {
	return map[string]any{"uuid": id, "shortUuid": short, "username": "fixture", "email": nil, "status": status, "trafficLimitBytes": 123, "expireAt": "2027-01-01T00:00:00Z", "userTraffic": map[string]any{"usedTrafficBytes": 12}}
}

func TestWebhookRejectsPartialUserProjection(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	for _, field := range []string{"username", "email", "trafficLimitBytes", "expireAt", "userTraffic"} {
		for _, null := range []bool{false, true} {
			if field == "email" && null {
				continue
			}
			data := webhookUserFixture("user-1", "short-1", "DISABLED")
			if null {
				data[field] = nil
			} else {
				delete(data, field)
			}
			body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": time.Now().UTC().Format(time.RFC3339Nano), "data": data})
			if w := signedWebhook(s, body); w.Code != 400 {
				t.Errorf("field %s null=%v accepted: %d", field, null, w.Code)
			}
		}
	}
}

func TestWebhookPreservesCanonicalNumericIdentityAndEventOrder(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	if err := s.saveObject("users", "42", panel.User{ID: "42", ShortUUID: "native-short", Status: "active"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, update := range []struct {
		at     time.Time
		status string
	}{{now, "DISABLED"}, {now.Add(-time.Minute), "ACTIVE"}} {
		data := webhookUserFixture("native-uuid", "native-short", update.status)
		data["id"] = 42
		body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": update.at.Format(time.RFC3339Nano), "data": data})
		if w := signedWebhook(s, body); w.Code != 202 {
			t.Fatalf("native webhook: %d %s", w.Code, w.Body)
		}
	}
	var count int
	if err := s.db.QueryRow("SELECT COUNT(*) FROM sg_objects WHERE kind='users' AND json_extract(data,'$.short_uuid')=?", "native-short").Scan(&count); err != nil || count != 1 {
		t.Fatalf("duplicate identity: %d %v", count, err)
	}
	var user panel.User
	if err := s.object("users", "42", &user); err != nil || user.Status != "disabled" {
		t.Fatalf("canonical state regressed: %+v %v", user, err)
	}
}

func TestWebhookOlderThanCompletedSyncCannotRegressUser(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	now := time.Now().UTC()
	event := func(at time.Time, status string) []byte {
		body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": at.Format(time.RFC3339Nano), "data": webhookUserFixture("user-1", "short-1", status)})
		return body
	}
	if w := signedWebhook(s, event(now.Add(-time.Minute), "ACTIVE")); w.Code != 202 {
		t.Fatal(w.Code)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/users" {
			data, _ := json.Marshal(map[string]any{"response": map[string]any{"total": 1, "users": []any{webhookUserFixture("user-1", "short-1", "DISABLED")}}})
			w.Write(data)
		} else {
			io.WriteString(w, `{"response":[]}`)
		}
	}))
	defer upstream.Close()
	s.config.Demo = false
	s.panel.Close()
	s.panel = panel.NewClient(upstream.URL, "test", s.logger)
	if err := s.syncPanel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w := signedWebhook(s, event(now.Add(-30*time.Second), "ACTIVE")); w.Code != 202 {
		t.Fatal(w.Code)
	}
	var user panel.User
	if err := s.object("users", "user-1", &user); err != nil || user.Status != "disabled" {
		t.Fatalf("old webhook undid sync: %+v %v", user, err)
	}
}

func TestWebhookOlderThanCompletedSyncCannotResurrectDeletedUser(t *testing.T) {
	s := newTestServer(t)
	s.config.Panel.WebhookKey = "webhook-test-secret"
	now := time.Now().UTC()
	event := func(at time.Time) []byte {
		body, _ := json.Marshal(map[string]any{"event": "user.updated", "timestamp": at.Format(time.RFC3339Nano), "data": webhookUserFixture("user-1", "short-1", "ACTIVE")})
		return body
	}
	if w := signedWebhook(s, event(now.Add(-time.Minute))); w.Code != 202 {
		t.Fatal(w.Code)
	}
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
	if err := s.syncPanel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if w := signedWebhook(s, event(now.Add(-30*time.Second))); w.Code != 202 {
		t.Fatal(w.Code)
	}
	var user panel.User
	if err := s.object("users", "user-1", &user); err == nil {
		t.Fatal("old webhook resurrected user deleted by sync")
	}
}
