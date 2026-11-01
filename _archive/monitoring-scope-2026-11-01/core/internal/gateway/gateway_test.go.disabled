package gateway

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type memoryStore struct {
	mu      sync.Mutex
	entries map[string]CacheEntry
}

func (m *memoryStore) Load(k string) (CacheEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e, ok := m.entries[k]
	if !ok {
		return e, errors.New("not found")
	}
	return e, nil
}
func (m *memoryStore) Save(k string, e CacheEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[k] = e
	return nil
}
func (m *memoryStore) Delete(k string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.entries, k)
	return nil
}
func (m *memoryStore) Clear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = map[string]CacheEntry{}
	return nil
}
func (m *memoryStore) expire() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, e := range m.entries {
		e.ExpiresAt = time.Now().Add(-time.Minute)
		m.entries[k] = e
	}
}

func requestGateway(g *Gateway, id, kind, query, hwid string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "http://subscription.test/sub/"+id+query, nil)
	r.SetPathValue("shortUuid", id)
	r.SetPathValue("clientType", kind)
	r.Header.Set("User-Agent", "test-client")
	if hwid != "" {
		r.Header.Set("X-Hwid", hwid)
	}
	w := httptest.NewRecorder()
	g.HandleRequest(w, r)
	return w
}

func TestOpaquePassthroughAndPersistentFallback(t *testing.T) {
	formats := []struct{ kind, contentType, body string }{
		{"", "text/plain", "dmxlc3M6Ly9wcm90b2NvbC1leGFtcGxl\n"},
		{"sing-box", "application/json", `{"outbounds":[{"type":"future-protocol","opaque":{"extension":true}}]}`},
		{"clash", "application/yaml", "proxies:\n  - name: future\n    type: anytls\n"},
		{"xray", "application/json", `{"outbounds":[{"protocol":"future-xray-extension"}]}`},
	}
	for _, format := range formats {
		t.Run(format.kind, func(t *testing.T) {
			var status atomic.Int32
			status.Store(200)
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				expected := "/opaque-id"
				if format.kind != "" {
					expected += "/" + format.kind
				}
				if r.URL.Path != expected || r.URL.RawQuery != "flag=opaque%2Bvalue" {
					t.Errorf("URL changed: %s", r.URL.String())
				}
				if r.Header.Get("User-Agent") != "test-client" {
					t.Error("client header dropped")
				}
				w.Header().Set("Content-Type", format.contentType)
				w.Header().Set("Subscription-Userinfo", "upload=123; download=456; total=789")
				w.Header().Set("Profile-Title", "base64:dGVzdA==")
				w.Header().Set("Support-Url", "https://help.example.com")
				w.WriteHeader(int(status.Load()))
				io.WriteString(w, format.body)
			}))
			defer upstream.Close()
			store := &memoryStore{entries: map[string]CacheEntry{}}
			g := NewGateway(slog.New(slog.NewTextHandler(io.Discard, nil)))
			g.Configure(upstream.URL, store)
			w := requestGateway(g, "opaque-id", format.kind, "?flag=opaque%2Bvalue", "")
			if w.Code != 200 || w.Body.String() != format.body || w.Header().Get("X-Cache-Status") != "miss" {
				t.Fatalf("passthrough: %d %q %v", w.Code, w.Body.String(), w.Header())
			}
			w = requestGateway(g, "opaque-id", format.kind, "?flag=opaque%2Bvalue", "")
			if w.Header().Get("X-Cache-Status") != "hit" || requests.Load() != 1 {
				t.Fatalf("cache miss: %v requests=%d", w.Header(), requests.Load())
			}
			store.expire()
			status.Store(503)
			// A new gateway reads snapshots after a process restart.
			g = NewGateway(slog.Default())
			g.Configure(upstream.URL, store)
			w = requestGateway(g, "opaque-id", format.kind, "?flag=opaque%2Bvalue", "")
			if w.Code != 200 || w.Body.String() != format.body || w.Header().Get("X-Cache-Status") != "stale" || w.Header().Get("Warning") == "" || w.Header().Get("Content-Type") != format.contentType || w.Header().Get("Subscription-Userinfo") != "upload=123; download=456; total=789" || w.Header().Get("Profile-Title") != "base64:dGVzdA==" || w.Header().Get("Support-Url") != "https://help.example.com" {
				t.Fatalf("snapshot content/headers lost: %d %q %v", w.Code, w.Body.String(), w.Header())
			}
			status.Store(404)
			w = requestGateway(g, "opaque-id", format.kind, "?flag=opaque%2Bvalue", "")
			if w.Code != 404 || w.Header().Get("X-Cache-Status") != "" || len(store.entries) != 0 {
				t.Fatalf("authoritative 4xx served cached credentials: %d %v", w.Code, w.Header())
			}
			status.Store(503)
			w = requestGateway(g, "opaque-id", format.kind, "?flag=opaque%2Bvalue", "")
			if w.Code != 503 {
				t.Fatalf("deleted stale snapshot survived: %d", w.Code)
			}
		})
	}
}

func TestDeviceIsolationAndInvalidIdentity(t *testing.T) {
	var requests atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		io.WriteString(w, r.Header.Get("X-Hwid"))
	}))
	defer u.Close()
	g := NewGateway(slog.Default())
	g.Configure(u.URL, nil)
	for _, device := range []string{"device-a", "device-b", "device-a"} {
		w := requestGateway(g, "id", "", "", device)
		if w.Code != 200 || !bytes.Equal(w.Body.Bytes(), []byte(device)) || w.Header().Get("X-Cache-Status") != "miss" {
			t.Fatalf("device cache reused: %d %q", w.Code, w.Body.String())
		}
	}
	if requests.Load() != 3 {
		t.Fatalf("HWID authorization skipped: %d requests", requests.Load())
	}
	for _, id := range []string{"", "../secrets", "a/b", "token%00", string(bytes.Repeat([]byte("a"), 129))} {
		if w := requestGateway(g, id, "", "", ""); w.Code != 400 {
			t.Errorf("invalid identity accepted: %q => %d", id, w.Code)
		}
	}
}

func TestExpiredSnapshotNeverFallsBack(t *testing.T) {
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer u.Close()
	store := &memoryStore{entries: map[string]CacheEntry{}}
	g := NewGateway(slog.Default())
	g.Configure(u.URL, store)
	// Seed using the normal key computation and fetch, then age the persisted entry.
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "old credentials") }))
	defer good.Close()
	g.Configure(good.URL, store)
	requestGateway(g, "id", "", "", "")
	for k, e := range store.entries {
		e.StoredAt = time.Now().Add(-6 * time.Minute)
		e.ExpiresAt = e.StoredAt
		store.entries[k] = e
	}
	g = NewGateway(slog.Default())
	g.Configure(u.URL, store)
	w := requestGateway(g, "id", "", "", "")
	if w.Code != 503 || w.Header().Get("Retry-After") != "30" {
		t.Fatalf("expired snapshot exposed: %d %q", w.Code, w.Body.String())
	}
}

func TestStaleSnapshotServesWhileRefreshRunsInBackground(t *testing.T) {
	var calls atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			io.WriteString(w, "initial")
			return
		}
		close(started)
		<-release
		io.WriteString(w, "refreshed")
	}))
	defer u.Close()
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	store := &memoryStore{entries: map[string]CacheEntry{}}
	g := NewGateway(slog.Default())
	g.Configure(u.URL, store)
	if w := requestGateway(g, "swr", "", "", ""); w.Body.String() != "initial" {
		t.Fatal(w.Body.String())
	}
	store.mu.Lock()
	for key, entry := range store.entries {
		entry.StoredAt = time.Now().Add(-31 * time.Second)
		entry.ExpiresAt = entry.StoredAt.Add(30 * time.Second)
		store.entries[key] = entry
	}
	store.mu.Unlock()
	g.Configure(u.URL, store)
	startedAt := time.Now()
	w := requestGateway(g, "swr", "", "", "")
	if w.Code != 200 || w.Body.String() != "initial" || w.Header().Get("X-Cache-Status") != "stale" {
		t.Fatalf("stale response: %d %q %v", w.Code, w.Body.String(), w.Header())
	}
	if time.Since(startedAt) > 100*time.Millisecond {
		t.Fatalf("stale response waited for panel: %v", time.Since(startedAt))
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("background refresh did not start")
	}
	unblock()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if w = requestGateway(g, "swr", "", "", ""); w.Body.String() == "refreshed" && w.Header().Get("X-Cache-Status") == "hit" {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("background refresh did not update snapshot")
}

func TestPolicyVersionSeparatesCacheEntries(t *testing.T) {
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			io.WriteString(w, "v1")
		} else {
			io.WriteString(w, "v2")
		}
	}))
	defer u.Close()
	g := NewGateway(slog.Default())
	g.Configure(u.URL, nil)
	g.SetPolicyVersion("policy-v1")
	if w := requestGateway(g, "versioned", "", "", ""); w.Body.String() != "v1" {
		t.Fatal(w.Body.String())
	}
	g.SetPolicyVersion("policy-v2")
	if w := requestGateway(g, "versioned", "", "", ""); w.Body.String() != "v2" {
		t.Fatal(w.Body.String())
	}
	if calls.Load() != 2 {
		t.Fatalf("policy version reused cache: %d", calls.Load())
	}
}

func waitForGatewayRefs(t *testing.T, g *Gateway, key string, refs int) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(2 * time.Millisecond)
	defer tick.Stop()
	for {
		g.mu.Lock()
		lock := g.locks[key]
		ready := lock != nil && lock.refs == refs
		g.mu.Unlock()
		if ready {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("requests did not share lock: wanted refs=%d", refs)
		case <-tick.C:
		}
	}
}

func TestConcurrentRequestsCoalesceAndReleaseIdleLocks(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		io.WriteString(w, "shared subscription")
	}))
	defer u.Close()
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	g := NewGateway(slog.Default())
	g.Configure(u.URL, nil)
	const workers = 12
	results := make(chan *httptest.ResponseRecorder, workers)
	go func() { results <- requestGateway(g, "shared", "", "", "") }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream request did not start")
	}
	g.mu.Lock()
	var key string
	for k := range g.locks {
		key = k
	}
	g.mu.Unlock()
	for i := 1; i < workers; i++ {
		go func() { results <- requestGateway(g, "shared", "", "", "") }()
	}
	waitForGatewayRefs(t, g, key, workers)
	unblock()
	for i := 0; i < workers; i++ {
		select {
		case w := <-results:
			if w.Code != 200 || w.Body.String() != "shared subscription" {
				t.Errorf("shared response: %d %s", w.Code, w.Body)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("coalesced request blocked")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("same cache key fetched %d times", calls.Load())
	}
	g.mu.Lock()
	locks := len(g.locks)
	g.mu.Unlock()
	if locks != 0 {
		t.Fatalf("completed requests leaked %d locks", locks)
	}
}

func TestActiveLockSurvivesMoreThan4096DistinctKeys(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-release
		io.WriteString(w, "subscription")
	}))
	defer u.Close()
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	g := NewGateway(slog.Default())
	g.Configure(u.URL, nil)
	results := make(chan *httptest.ResponseRecorder, 2)
	go func() { results <- requestGateway(g, "held-key", "", "", "") }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("upstream did not start")
	}
	g.mu.Lock()
	var activeKey string
	for k := range g.locks {
		activeKey = k
	}
	g.mu.Unlock()
	// Hold unrelated keys as if thousands of distinct requests were in flight.
	held := make([]*requestLock, 4097)
	for i := range held {
		held[i] = g.acquire(fmt.Sprintf("other-%d", i))
	}
	go func() { results <- requestGateway(g, "held-key", "", "", "") }()
	waitForGatewayRefs(t, g, activeKey, 2)
	if calls.Load() != 1 {
		t.Errorf("active lock replaced: %d upstream calls", calls.Load())
	}
	for i, lock := range held {
		g.release(fmt.Sprintf("other-%d", i), lock)
	}
	unblock()
	for i := 0; i < 2; i++ {
		select {
		case w := <-results:
			if w.Code != 200 {
				t.Errorf("response: %d", w.Code)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("active request blocked")
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("same key used concurrent locks: %d calls", calls.Load())
	}
	g.mu.Lock()
	locks := len(g.locks)
	g.mu.Unlock()
	if locks != 0 {
		t.Fatalf("lock entries retained after completion: %d", locks)
	}
}

func TestClearDoesNotResurrectInFlightSnapshot(t *testing.T) {
	for _, upstreamStatus := range []int{200, 503} {
		t.Run(fmt.Sprint(upstreamStatus), func(t *testing.T) {
			started := make(chan struct{})
			release := make(chan struct{})
			var calls atomic.Int32
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					close(started)
					<-release
					w.WriteHeader(upstreamStatus)
					io.WriteString(w, "old subscription")
					return
				}
				io.WriteString(w, "fresh subscription")
			}))
			defer u.Close()
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			store := &memoryStore{entries: map[string]CacheEntry{}}
			g := NewGateway(slog.Default())
			g.Configure(u.URL, store)
			if upstreamStatus == 503 {
				// Persist an expired snapshot under the real request key before a failing fetch.
				good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "revoked subscription") }))
				g.Configure(good.URL, store)
				requestGateway(g, "id", "", "", "")
				good.Close()
				store.expire()
				g.Configure(u.URL, store)
			}
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- requestGateway(g, "id", "", "", "") }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("upstream did not start")
			}
			if err := g.Clear(); err != nil {
				t.Fatal(err)
			}
			unblock()
			select {
			case w := <-result:
				if upstreamStatus == 503 && w.Code != 503 {
					t.Fatalf("invalidated stale credentials served: %d %q", w.Code, w.Body.String())
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request blocked after clear")
			}
			g.mu.Lock()
			cacheCount := len(g.cache)
			g.mu.Unlock()
			store.mu.Lock()
			storedCount := len(store.entries)
			store.mu.Unlock()
			if cacheCount != 0 || storedCount != 0 {
				t.Fatalf("in-flight request resurrected cache: memory=%d persisted=%d", cacheCount, storedCount)
			}
			w := requestGateway(g, "id", "", "", "")
			if w.Code != 200 || w.Body.String() != "fresh subscription" || w.Header().Get("X-Cache-Status") != "miss" || calls.Load() != 2 {
				t.Fatalf("cleared key reused obsolete response: %d %q %v calls=%d", w.Code, w.Body.String(), w.Header(), calls.Load())
			}
		})
	}
}

func TestTransformCannotMutateRawCachedSubscription(t *testing.T) {
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		io.WriteString(w, "raw subscription")
	}))
	defer u.Close()
	store := &memoryStore{entries: map[string]CacheEntry{}}
	g := NewGateway(slog.Default())
	g.Configure(u.URL, store)
	var transforms int
	g.Transform = func(r *http.Request, data []byte, headers http.Header) ([]byte, string, error) {
		transforms++
		if string(data) != "raw subscription" || headers.Get("Content-Type") != "text/plain" {
			t.Errorf("transform receives cached transformed data: %q %v", data, headers)
		}
		data[0] = 'X'
		headers.Set("Content-Type", "application/json")
		return append(data, []byte(fmt.Sprint(transforms))...), "tier", nil
	}
	for _, status := range []string{"miss", "hit"} {
		w := requestGateway(g, "id", "", "", "")
		if w.Header().Get("X-Cache-Status") != status || w.Header().Get("X-SubGate-Policy") != "tier" || w.Header().Get("Content-Type") != "application/json" || !strings.HasPrefix(w.Body.String(), "Xaw subscription") {
			t.Fatalf("transformed response: %d %q %v", w.Code, w.Body.String(), w.Header())
		}
	}
	store.mu.Lock()
	for _, entry := range store.entries {
		if string(entry.Data) != "raw subscription" || entry.Headers.Get("Content-Type") != "text/plain" {
			t.Errorf("persisted snapshot mutated by transform: %q %v", entry.Data, entry.Headers)
		}
	}
	store.mu.Unlock()
	g.Transform = func(r *http.Request, data []byte, headers http.Header) ([]byte, string, error) {
		data[0] = '!'
		headers.Set("Content-Type", "bad/type")
		return data, "tier", errors.New("transform failed")
	}
	w := requestGateway(g, "id", "", "", "")
	if w.Body.String() != "raw subscription" || w.Header().Get("Content-Type") != "text/plain" || w.Header().Get("X-SubGate-Policy") != "bypass" {
		t.Fatalf("failed transform modified opaque fallback: %q %v", w.Body.String(), w.Header())
	}
}

func expireForSWR(g *Gateway) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for key, entry := range g.cache {
		entry.StoredAt = time.Now().Add(-31 * time.Second)
		entry.ExpiresAt = entry.StoredAt.Add(30 * time.Second)
		g.cache[key] = entry
	}
}

func waitForRefresh(t *testing.T, g *Gateway) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		g.mu.Lock()
		done := len(g.refreshing) == 0
		g.mu.Unlock()
		if done {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("refresh did not finish")
}

func TestSWRRefreshCoalescesAndClearPreventsResurrection(t *testing.T) {
	for _, status := range []int{200, 404} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var calls atomic.Int32
			started, release := make(chan struct{}), make(chan struct{})
			u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					io.WriteString(w, "cached")
					return
				}
				close(started)
				<-release
				w.WriteHeader(status)
				io.WriteString(w, "refreshed")
			}))
			defer u.Close()
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			store := &memoryStore{entries: map[string]CacheEntry{}}
			g := NewGateway(slog.Default())
			g.Configure(u.URL, store)
			requestGateway(g, "swr-clear", "", "", "")
			expireForSWR(g)
			if w := requestGateway(g, "swr-clear", "", "", ""); w.Body.String() != "cached" || w.Header().Get("X-Cache-Status") != "stale" {
				t.Fatal("no stale response")
			}
			select {
			case <-started:
			case <-time.After(time.Second):
				t.Fatal("refresh not started")
			}
			for i := 0; i < 10; i++ {
				if w := requestGateway(g, "swr-clear", "", "", ""); w.Header().Get("X-Cache-Status") != "stale" {
					t.Fatal("stale request blocked")
				}
			}
			if calls.Load() != 2 {
				t.Fatalf("duplicate refreshes: %d", calls.Load())
			}
			if status == 200 {
				if err := g.Clear(); err != nil {
					t.Fatal(err)
				}
			}
			unblock()
			waitForRefresh(t, g)
			g.mu.Lock()
			memoryCount := len(g.cache)
			g.mu.Unlock()
			store.mu.Lock()
			persistedCount := len(store.entries)
			store.mu.Unlock()
			if memoryCount != 0 || persistedCount != 0 {
				t.Fatalf("revoked snapshot retained: %d %d", memoryCount, persistedCount)
			}
		})
	}
}

func TestAuthoritative4xxWithTruncatedBodyInvalidatesSnapshot(t *testing.T) {
	var revoked atomic.Bool
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if revoked.Load() {
			w.Header().Set("Content-Length", "1000")
			w.WriteHeader(404)
			io.WriteString(w, "gone")
			return
		}
		io.WriteString(w, "credentials")
	}))
	defer u.Close()
	g := NewGateway(slog.Default())
	g.Configure(u.URL, nil)
	requestGateway(g, "revoke", "", "", "")
	expireForSWR(g)
	revoked.Store(true)
	requestGateway(g, "revoke", "", "", "")
	waitForRefresh(t, g)
	w := requestGateway(g, "revoke", "", "", "")
	if w.Code != 404 || strings.Contains(w.Body.String(), "credentials") {
		t.Fatalf("revoked stale credentials survived: %d %s", w.Code, w.Body)
	}
}

func TestLateSWRRefreshCannotUndoAuthoritativeForeground(t *testing.T) {
	var calls atomic.Int32
	started, release := make(chan struct{}), make(chan struct{})
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1:
			io.WriteString(w, "cached")
		case 2:
			close(started)
			<-release
			io.WriteString(w, "revoked")
		default:
			w.WriteHeader(404)
		}
	}))
	defer u.Close()
	unblock := sync.OnceFunc(func() { close(release) })
	defer unblock()
	g := NewGateway(slog.Default())
	g.Configure(u.URL, nil)
	requestGateway(g, "late-refresh", "", "", "")
	expireForSWR(g)
	requestGateway(g, "late-refresh", "", "", "")
	<-started
	g.mu.Lock()
	for k, e := range g.cache {
		e.StoredAt = time.Now().Add(-6 * time.Minute)
		g.cache[k] = e
	}
	g.mu.Unlock()
	if w := requestGateway(g, "late-refresh", "", "", ""); w.Code != 404 {
		t.Fatalf("authoritative status: %d", w.Code)
	}
	unblock()
	waitForRefresh(t, g)
	if w := requestGateway(g, "late-refresh", "", "", ""); w.Code != 404 {
		t.Fatalf("late refresh resurrected credentials: %d %s", w.Code, w.Body)
	}
}
