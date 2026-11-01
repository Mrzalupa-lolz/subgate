package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"
)

// The bridge supplies proxy headers required by the isolated HTTP panel. It
// never sends panel API credentials to the public subscription route.
func TestLiveSubscriptionMetadata(t *testing.T) {
	base := os.Getenv("SUBGATE_SYNTHETIC_PANEL_URL")
	if base == "" {
		t.Skip("synthetic panel not configured")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("synthetic panel must use loopback HTTP")
	}
	short := os.Getenv("SUBGATE_SYNTHETIC_SHORT_UUID")
	if short == "" {
		t.Fatal("synthetic subscription identity missing")
	}
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	bridge := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		req, err := http.NewRequestWithContext(r.Context(), r.Method, base+"/api/sub"+r.URL.RequestURI(), nil)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		req.Header = r.Header.Clone()
		req.Header.Set("X-Forwarded-Proto", "https")
		req.Header.Set("X-Forwarded-For", "127.0.0.1")
		req.Host = "localhost"
		resp, err := client.Do(req)
		if err != nil {
			w.WriteHeader(502)
			return
		}
		defer resp.Body.Close()
		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.Header().Del("Connection")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	defer bridge.Close()
	s := newTestServer(t)
	s.config.Demo = false
	s.gateway.Configure(bridge.URL, nil)
	for _, method := range []string{"HEAD", "GET"} {
		direct, err := http.NewRequest(method, bridge.URL+"/"+short, nil)
		if err != nil {
			t.Fatal("synthetic request failed")
		}
		direct.Header.Set("User-Agent", "SyntheticClient/1")
		direct.Header.Set("Accept-Language", "ru")
		upstream, err := client.Do(direct)
		if err != nil {
			t.Fatal("synthetic origin unavailable")
		}
		body, err := io.ReadAll(upstream.Body)
		upstream.Body.Close()
		if err != nil || upstream.StatusCode != 200 {
			t.Fatal("synthetic origin read failed")
		}
		request := httptest.NewRequest(method, "http://console.test/sub/"+short, nil)
		request.Header = direct.Header.Clone()
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, request)
		if out.Code != 200 || !bytes.Equal(out.Body.Bytes(), body) {
			t.Fatal("synthetic subscription bytes/status changed")
		}
		for _, name := range []string{"Content-Type", "Profile-Title", "Profile-Update-Interval", "Subscription-Userinfo", "Support-Url", "Content-Disposition", "Profile-Web-Page-Url"} {
			if out.Header().Get(name) != upstream.Header.Get(name) {
				t.Fatal("synthetic subscription metadata changed")
			}
		}
		if method == "HEAD" && out.Header().Get("Content-Length") != upstream.Header.Get("Content-Length") {
			t.Fatal("synthetic HEAD length changed")
		}
	}
}
