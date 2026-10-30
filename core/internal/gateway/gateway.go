package gateway

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

type CacheEntry struct {
	Data      []byte
	Headers   http.Header
	ExpiresAt time.Time
	StoredAt  time.Time
}
type Store interface {
	Load(key string) (CacheEntry, error)
	Save(key string, entry CacheEntry) error
	Delete(key string) error
	Clear() error
}
type requestLock struct {
	mu   sync.Mutex
	refs int
}
type Gateway struct {
	mu                    sync.Mutex
	storeMu               sync.Mutex
	cache                 map[string]CacheEntry
	locks                 map[string]*requestLock
	generation            uint64
	policyVersion         string
	refreshing            map[string]uint64
	transformSlots        chan struct{}
	upstream              string
	client                *http.Client
	logger                *slog.Logger
	store                 Store
	requests, hits, stale uint64
	Transform             func(*http.Request, []byte, http.Header) ([]byte, string, error)
}

var identity = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

func NewGateway(logger *slog.Logger) *Gateway {
	return &Gateway{transformSlots: make(chan struct{}, 32), cache: map[string]CacheEntry{}, locks: map[string]*requestLock{}, refreshing: map[string]uint64{}, logger: logger, client: &http.Client{Timeout: 12 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (g *Gateway) Configure(upstream string, store Store) {
	g.storeMu.Lock()
	defer g.storeMu.Unlock()
	g.mu.Lock()
	defer g.mu.Unlock()
	g.upstream = strings.TrimRight(upstream, "/")
	g.store = store
	g.generation++
	g.cache = map[string]CacheEntry{}
}

// SetPolicyVersion changes the cache namespace and prevents in-flight saves
// from repopulating snapshots belonging to an earlier policy.
func (g *Gateway) SetPolicyVersion(version string) error {
	g.storeMu.Lock()
	defer g.storeMu.Unlock()
	g.mu.Lock()
	if g.policyVersion == version {
		g.mu.Unlock()
		return nil
	}
	g.policyVersion = version
	g.generation++
	g.cache = map[string]CacheEntry{}
	store := g.store
	g.mu.Unlock()
	if store != nil {
		return store.Clear()
	}
	return nil
}

func (g *Gateway) Clear() error {
	// Serialize persistent snapshots with saves, so an old fetch cannot recreate
	// a snapshot after a webhook, agent revocation, or policy update cleared it.
	g.storeMu.Lock()
	defer g.storeMu.Unlock()
	g.mu.Lock()
	g.generation++
	g.cache = map[string]CacheEntry{}
	store := g.store
	g.mu.Unlock()
	if store != nil {
		return store.Clear()
	}
	return nil
}
func (g *Gateway) acquire(key string) *requestLock {
	g.mu.Lock()
	lock := g.locks[key]
	if lock == nil {
		lock = &requestLock{}
		g.locks[key] = lock
	}
	lock.refs++
	g.requests++
	g.mu.Unlock()
	lock.mu.Lock()
	return lock
}
func (g *Gateway) release(key string, lock *requestLock) {
	lock.mu.Unlock()
	g.mu.Lock()
	lock.refs--
	if lock.refs == 0 {
		delete(g.locks, key)
	}
	g.mu.Unlock()
}
func (g *Gateway) snapshot(key string) (CacheEntry, bool, uint64, string) {
	g.storeMu.Lock()
	defer g.storeMu.Unlock()
	g.mu.Lock()
	entry, exists := g.cache[key]
	generation, upstream, store := g.generation, g.upstream, g.store
	g.mu.Unlock()
	if !exists && store != nil {
		if saved, err := store.Load(key); err == nil {
			entry = saved
			exists = true
		}
	}
	return entry, exists, generation, upstream
}
func (g *Gateway) save(key string, entry CacheEntry, generation uint64) {
	g.storeMu.Lock()
	defer g.storeMu.Unlock()
	g.mu.Lock()
	if generation != g.generation {
		g.mu.Unlock()
		return
	}
	if len(g.cache) >= 1024 {
		var oldestKey string
		var oldest time.Time
		for k, e := range g.cache {
			if oldestKey == "" || e.StoredAt.Before(oldest) {
				oldestKey = k
				oldest = e.StoredAt
			}
		}
		delete(g.cache, oldestKey)
	}
	g.cache[key] = entry
	store := g.store
	g.mu.Unlock()
	if store != nil {
		if err := store.Save(key, entry); err != nil {
			g.logger.Warn("subscription snapshot failed", "error", err)
		}
	}
}
func (g *Gateway) invalidate(key string, generation uint64) {
	g.storeMu.Lock()
	defer g.storeMu.Unlock()
	g.mu.Lock()
	if generation != g.generation {
		g.mu.Unlock()
		return
	}
	delete(g.cache, key)
	store := g.store
	g.mu.Unlock()
	if store != nil {
		if err := store.Delete(key); err != nil {
			g.logger.Warn("subscription snapshot deletion failed", "error", err)
		}
	}
}
func (g *Gateway) Statistics() map[string]any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return map[string]any{"requests": g.requests, "cache_hits": g.hits, "fallbacks": g.stale, "cache_entries": len(g.cache)}
}

var forwarded = []string{"Accept", "Accept-Language", "User-Agent", "X-Hwid", "X-Device-Os", "X-Device-Model", "X-Ver-Os", "X-Device-Type"}
var responseHeaders = []string{"Content-Type", "Subscription-Userinfo", "Profile-Title", "Profile-Update-Interval", "Support-Url", "Profile-Web-Page-Url", "Content-Disposition", "Announce", "Announce-Url"}

func connectionHeaders(source http.Header) map[string]bool {
	blocked := make(map[string]bool)
	for _, value := range source.Values("Connection") {
		for _, name := range strings.Split(value, ",") {
			blocked[http.CanonicalHeaderKey(strings.TrimSpace(name))] = true
		}
	}
	return blocked
}

func hashField(h hash.Hash, data []byte) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(data)))
	h.Write(size[:])
	h.Write(data)
}

// relevantRequestHeaders is used both for forwarding and the cache key, so
// future device metadata cannot share a cache entry with another variant.
func relevantRequestHeaders(source http.Header) http.Header {
	headers := make(http.Header)
	blocked := connectionHeaders(source)
	for name, values := range source {
		canonical := http.CanonicalHeaderKey(name)
		allowed := strings.HasPrefix(strings.ToLower(canonical), "x-device-")
		for _, candidate := range forwarded {
			allowed = allowed || canonical == candidate
		}
		if allowed && !blocked[canonical] {
			headers[canonical] = append([]string(nil), values...)
		}
	}
	return headers
}

func (g *Gateway) HandleRequest(w http.ResponseWriter, r *http.Request) {
	id, kind := r.PathValue("shortUuid"), r.PathValue("clientType")
	if !identity.MatchString(id) || (kind != "" && !identity.MatchString(kind)) {
		http.Error(w, "invalid subscription identifier", 400)
		return
	}
	path := "/" + url.PathEscape(id)
	if kind != "" {
		path += "/" + url.PathEscape(kind)
	}
	g.mu.Lock()
	version, requestGeneration := g.policyVersion, g.generation
	g.mu.Unlock()
	h := sha256.New()
	hashField(h, []byte(version))
	hashField(h, []byte(r.Method))
	hashField(h, []byte(path))
	hashField(h, []byte(r.URL.RawQuery))
	headers := relevantRequestHeaders(r.Header)
	names := make([]string, 0, len(headers))
	for name := range headers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		hashField(h, []byte(name))
		var count [8]byte
		binary.BigEndian.PutUint64(count[:], uint64(len(headers[name])))
		h.Write(count[:])
		for _, value := range headers[name] {
			hashField(h, []byte(value))
		}
	}
	key := hex.EncodeToString(h.Sum(nil))
	lock := g.acquire(key)
	defer g.release(key, lock)
	cached, exists, generation, upstream := g.snapshot(key)
	if generation != requestGeneration {
		// Namespace changed while waiting for a coalesced request. Do not save
		// any response under the superseded key.
		generation = requestGeneration
		exists = false
	}
	if exists && time.Now().Before(cached.ExpiresAt) {
		g.mu.Lock()
		g.hits++
		g.mu.Unlock()
		g.serve(w, r, cached, "hit")
		return
	}
	if upstream == "" {
		http.Error(w, "subscription upstream is not configured", 503)
		return
	}
	target := upstream + path
	if r.URL.RawQuery != "" {
		target += "?" + r.URL.RawQuery
	}
	// Only ordinary cacheable responses use SWR. HWID and Set-Cookie
	// responses still require authoritative validation on every request.
	if exists && cached.ExpiresAt.After(cached.StoredAt) && time.Since(cached.StoredAt) < 5*time.Minute && r.Header.Get("X-Hwid") == "" {
		g.mu.Lock()
		current := generation == g.generation
		if current {
			g.stale++
		}
		g.mu.Unlock()
		if current {
			g.refresh(key, generation, cached.StoredAt, r.Method, target, headers)
			g.serve(w, r, cached, "stale")
			return
		}
	}
	entry, status, body, err := g.fetch(r.Context(), r.Method, target, headers)
	if err == nil && status >= 400 && status < 500 {
		g.invalidate(key, generation)
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		if r.Method != http.MethodHead {
			_, _ = w.Write(body)
		}
		return
	}
	if err == nil && status == http.StatusOK {
		g.save(key, entry, generation)
		g.serve(w, r, entry, "miss")
		return
	}
	// Fallback is bounded and never used after an authoritative 4xx.
	g.mu.Lock()
	current := generation == g.generation
	if exists && current && time.Since(cached.StoredAt) < 5*time.Minute {
		g.stale++
		g.mu.Unlock()
		g.serve(w, r, cached, "stale")
		return
	}
	g.mu.Unlock()
	w.Header().Set("Retry-After", "30")
	http.Error(w, "subscription upstream unavailable", 503)
}

// refresh is coalesced per key and bounded across all keys. Its context is
// independent from the already served request, but has a fixed deadline.
func (g *Gateway) refresh(key string, generation uint64, storedAt time.Time, method, target string, headers http.Header) {
	g.mu.Lock()
	if _, active := g.refreshing[key]; active || len(g.refreshing) >= 32 || generation != g.generation {
		g.mu.Unlock()
		return
	}
	g.refreshing[key] = generation
	g.mu.Unlock()
	go func() {
		defer func() {
			g.mu.Lock()
			delete(g.refreshing, key)
			g.mu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		entry, status, _, err := g.fetch(ctx, method, target, headers)
		if err != nil {
			return
		}
		// Foreground requests may have replaced or revoked this snapshot while
		// the fetch ran. Serialize commit and compare the original snapshot.
		lock := g.acquire(key)
		defer g.release(key, lock)
		current, exists, currentGeneration, _ := g.snapshot(key)
		if !exists || currentGeneration != generation || !current.StoredAt.Equal(storedAt) {
			return
		}
		if status == http.StatusOK {
			g.save(key, entry, generation)
		}
		if status >= 400 && status < 500 {
			g.invalidate(key, generation)
		}
	}()
}

func (g *Gateway) fetch(ctx context.Context, method, target string, headers http.Header) (CacheEntry, int, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, target, nil)
	if err != nil {
		return CacheEntry{}, 0, nil, err
	}
	req.Header = headers.Clone()
	resp, err := g.client.Do(req)
	if err != nil {
		return CacheEntry{}, 0, nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 {
		// Status is authoritative even if the error body is truncated.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return CacheEntry{}, resp.StatusCode, body, nil
	}
	if resp.StatusCode != http.StatusOK {
		return CacheEntry{}, resp.StatusCode, nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (8<<20)+1))
	if err != nil {
		return CacheEntry{}, 0, nil, err
	}
	if len(data) > 8<<20 {
		return CacheEntry{}, 0, nil, errors.New("subscription exceeds response limit")
	}
	now := time.Now()
	entry := CacheEntry{Data: data, Headers: make(http.Header), StoredAt: now, ExpiresAt: now.Add(30 * time.Second)}
	blocked := connectionHeaders(resp.Header)
	for _, name := range responseHeaders {
		if !blocked[name] && len(resp.Header.Values(name)) != 0 {
			entry.Headers[name] = append([]string(nil), resp.Header.Values(name)...)
		}
	}
	if method == http.MethodHead {
		entry.Data = nil
		if length := resp.Header.Get("Content-Length"); length != "" && !blocked["Content-Length"] {
			entry.Headers.Set("Content-Length", length)
		}
	}
	if headers.Get("X-Hwid") != "" || resp.Header.Get("Set-Cookie") != "" {
		entry.ExpiresAt = entry.StoredAt
	}
	return entry, resp.StatusCode, nil, nil
}

func serve(w http.ResponseWriter, e CacheEntry, status string) {
	for k, v := range e.Headers {
		w.Header()[k] = append([]string(nil), v...)
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Cache-Status", status)
	if status == "stale" {
		w.Header().Set("Warning", `110 - "Response is stale"`)
	}
	w.WriteHeader(200)
	_, _ = w.Write(e.Data)
}

type transformResult struct {
	data    []byte
	headers http.Header
	policy  string
	err     error
}

func (g *Gateway) transform(r *http.Request, entry CacheEntry) transformResult {
	select {
	case g.transformSlots <- struct{}{}:
	default:
		return transformResult{err: errors.New("transform capacity reached")}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 300*time.Millisecond)
	defer cancel()
	request := r.Clone(ctx)
	data, headers := append([]byte(nil), entry.Data...), entry.Headers.Clone()
	results := make(chan transformResult, 1)
	go func() {
		result := transformResult{headers: headers}
		defer func() {
			if recover() != nil {
				result = transformResult{err: errors.New("transform panic")}
			}
			results <- result
			<-g.transformSlots
		}()
		result.data, result.policy, result.err = g.Transform(request, data, headers)
	}()
	select {
	case result := <-results:
		if ctx.Err() != nil {
			return transformResult{err: errors.New("transform deadline exceeded")}
		}
		return result
	case <-ctx.Done():
		return transformResult{err: errors.New("transform deadline exceeded")}
	}
}

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request, e CacheEntry, status string) {
	if g.Transform != nil && r.Method != http.MethodHead {
		result := g.transform(r, e)
		if result.err == nil {
			if !bytes.Equal(e.Data, result.data) {
				for _, name := range []string{"Content-Length", "Content-Encoding", "Etag", "Content-Md5", "Digest", "Content-Digest", "Repr-Digest"} {
					result.headers.Del(name)
				}
			}
			e.Data, e.Headers = result.data, result.headers
			if result.policy != "" {
				w.Header().Set("X-SubGate-Policy", result.policy)
			}
		} else {
			w.Header().Set("X-SubGate-Policy", "bypass")
			g.logger.Warn("subscription transform bypassed")
		}
	}
	serve(w, e, status)
}

// ValidateOpaque checks JSON syntax without imposing a protocol allowlist.
func ValidateOpaque(data []byte) bool { return json.Valid(data) }
