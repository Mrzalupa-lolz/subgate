package server

import (
	"bufio"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The live stream must outlive http.Server.WriteTimeout.
func TestLiveStreamOutlivesServerWriteTimeout(t *testing.T) {
	s := newTestServer(t)
	session := loginTest(t, s)
	ts := httptest.NewUnstartedServer(s.Handler())
	ts.Config.WriteTimeout = time.Second
	ts.Start()
	defer ts.Close()
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/live/stream", nil)
	req.AddCookie(session)
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("stream status %d", resp.StatusCode)
	}
	deadline := time.Now().Add(2500 * time.Millisecond)
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	events := 0
	for time.Now().Before(deadline) && scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "event: live") {
			events++
		}
	}
	if events < 3 {
		t.Fatalf("stream ended after %d events: %v", events, scanner.Err())
	}
}
