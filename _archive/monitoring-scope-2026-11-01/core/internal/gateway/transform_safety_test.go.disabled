package gateway

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestChangedBodyHeadersAndRawCache(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Subscription-Userinfo", "upload=1; download=2; total=100")
		_, _ = io.WriteString(w, "original")
	}))
	defer upstream.Close()
	g := NewGateway(slog.New(slog.NewTextHandler(io.Discard, nil)))
	g.Configure(upstream.URL, nil)
	g.Transform = func(r *http.Request, data []byte, headers http.Header) ([]byte, string, error) {
		headers.Set("Content-Type", "application/json")
		for _, key := range []string{"Content-Length", "Content-Encoding", "Etag", "Content-Md5", "Digest", "Content-Digest", "Repr-Digest"} {
			headers.Set(key, "obsolete")
		}
		return []byte(`["changed"]`), "profile", nil
	}
	for i := 0; i < 2; i++ {
		out := requestGateway(g, "synthetic", "v2ray-json", "", "")
		if out.Body.String() != `["changed"]` || out.Header().Get("Subscription-Userinfo") == "" || out.Header().Get("Content-Type") != "application/json" {
			t.Fatal("body or metadata incorrect")
		}
		for _, key := range []string{"Content-Length", "Content-Encoding", "Etag", "Content-Md5", "Digest", "Content-Digest", "Repr-Digest"} {
			if out.Header().Get(key) != "" {
				t.Fatal("obsolete header retained", key)
			}
		}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	for _, entry := range g.cache {
		if !bytes.Equal(entry.Data, []byte("original")) || entry.Headers.Get("Content-Type") != "text/plain" {
			t.Fatal("source cache changed")
		}
	}
}

func TestTransformPanicAndDeadlineFailOpen(t *testing.T) {
	for _, mode := range []string{"panic", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				io.WriteString(w, "original-subscription")
			}))
			defer upstream.Close()
			g := NewGateway(slog.New(slog.NewTextHandler(io.Discard, nil)))
			g.Configure(upstream.URL, nil)
			release := make(chan struct{})
			finished := make(chan struct{})
			g.Transform = func(r *http.Request, data []byte, headers http.Header) ([]byte, string, error) {
				data[0] = 'X'
				headers.Set("Content-Type", "bad/type")
				if mode == "panic" {
					panic("synthetic private panic")
				}
				defer close(finished)
				<-release
				data[1] = 'Y'
				headers.Set("Late", "mutation")
				return data, "late", nil
			}
			started := time.Now()
			returned := make(chan *httptest.ResponseRecorder, 1)
			escaped := make(chan bool, 1)
			go func() {
				defer func() {
					if recover() != nil {
						escaped <- true
					}
				}()
				returned <- requestGateway(g, "synthetic", "", "", "")
			}()
			var out *httptest.ResponseRecorder
			select {
			case <-escaped:
				t.Fatal("transform panic escaped gateway")
			case out = <-returned:
			case <-time.After(500 * time.Millisecond):
				close(release)
				<-finished
				t.Fatal("transform exceeded 300ms budget")
			}
			elapsed := time.Since(started)
			if mode == "deadline" {
				if elapsed < 250*time.Millisecond || elapsed > 450*time.Millisecond {
					t.Fatalf("deadline duration %s", elapsed)
				}
				close(release)
				<-finished
			}
			if out.Code != 200 || out.Body.String() != "original-subscription" || out.Header().Get("Content-Type") != "text/plain" || out.Header().Get("X-SubGate-Policy") != "bypass" || out.Header().Get("Late") != "" {
				t.Fatal("fail-open original bytes/headers changed")
			}
			g.mu.Lock()
			for _, entry := range g.cache {
				if string(entry.Data) != "original-subscription" || entry.Headers.Get("Content-Type") != "text/plain" {
					t.Error("late transform changed raw cache")
				}
			}
			g.mu.Unlock()
		})
	}
}

func TestTransformWorkersBoundedAndContextCancelled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "original") }))
	defer upstream.Close()
	g := NewGateway(slog.New(slog.NewTextHandler(io.Discard, nil)))
	g.Configure(upstream.URL, nil)
	release := make(chan struct{})
	started := make(chan struct{}, 32)
	cancelled := make(chan struct{}, 32)
	finished := make(chan struct{}, 32)
	g.Transform = func(r *http.Request, data []byte, headers http.Header) ([]byte, string, error) {
		started <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
		<-release
		finished <- struct{}{}
		return data, "", nil
	}
	results := make(chan *httptest.ResponseRecorder, 32)
	for i := 0; i < 32; i++ {
		go func(id int) { results <- requestGateway(g, "synthetic"+strconv.Itoa(id), "", "", "") }(i)
	}
	for i := 0; i < 32; i++ {
		select {
		case <-results:
		case <-time.After(2 * time.Second):
			t.Fatal("worker request did not bypass")
		}
	}
	if len(started) != 32 {
		t.Fatal("transform workers were not started")
	}
	for i := 0; i < 32; i++ {
		select {
		case <-cancelled:
		case <-time.After(time.Second):
			t.Fatal("transform context did not cancel")
		}
	}
	begin := time.Now()
	out := requestGateway(g, "synthetic", "", "", "")
	if time.Since(begin) > 100*time.Millisecond || out.Body.String() != "original" || out.Header().Get("X-SubGate-Policy") != "bypass" {
		t.Fatal("saturated transforms did not bypass immediately")
	}
	close(release)
	for i := 0; i < 32; i++ {
		<-finished
	}
}
