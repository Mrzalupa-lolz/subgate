package server

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubscriptionHEADAndMetadata(t *testing.T) {
	body := []byte("opaque synthetic subscription\n")
	responseHeaders := http.Header{
		"Content-Type": {"text/plain"}, "Subscription-Userinfo": {"upload=1; download=2; total=1024"},
		"Profile-Title": {"synthetic"}, "Profile-Update-Interval": {"24"}, "Content-Disposition": {"attachment; filename=synthetic"},
		"Support-Url": {"https://example.invalid/support"}, "Profile-Web-Page-Url": {"https://example.invalid/profile"},
		"Announce": {"synthetic announce"}, "Announce-Url": {"https://example.invalid/announce"},
	}
	for _, route := range []string{"/sub/synthetic123", "/synthetic123", "/sub/synthetic123/mihomo", "/synthetic123/mihomo"} {
		t.Run(route, func(t *testing.T) {
			s := newTestServer(t)
			s.config.Demo = false
			requests := make(chan *http.Request, 16)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.Clone(r.Context())
				for k, v := range responseHeaders {
					w.Header()[k] = v
				}
				w.Header().Set("Content-Length", "30")
				if r.Method != "HEAD" {
					w.Write(body)
				}
			}))
			defer upstream.Close()
			s.gateway.Configure(upstream.URL, nil)
			for _, method := range []string{"HEAD", "GET", "HEAD", "GET"} {
				req := httptest.NewRequest(method, "http://console.test"+route+"?format=a%2Bb&flag=1&flag=2", nil)
				req.Header.Set("User-Agent", "SyntheticClient/1")
				req.Header.Set("Accept-Language", "ru-RU")
				req.Header.Set("X-Hwid", "synthetic-device")
				req.Header.Set("X-Device-Future-Field", "future-device-value")
				req.Header["X-Device-Capabilities"] = []string{"first", "second"}
				req.Header.Set("Authorization", "Bearer client-private-value")
				req.Header.Set("Cookie", "client-private=value")
				out := httptest.NewRecorder()
				s.Handler().ServeHTTP(out, req)
				if out.Code != 200 {
					t.Fatalf("%s status %d", method, out.Code)
				}
				if method == "HEAD" && out.Body.Len() != 0 {
					t.Fatal("HEAD returned a body")
				}
				if method == "GET" && !bytes.Equal(out.Body.Bytes(), body) {
					t.Fatal("all-rules-off bytes changed")
				}
				for k, v := range responseHeaders {
					if out.Header().Get(k) != v[0] {
						t.Errorf("response metadata missing: %s", k)
					}
				}
				if method == "HEAD" && out.Header().Get("Content-Length") != "30" {
					t.Error("HEAD lost content length")
				}
				select {
				case got := <-requests:
					wantPath := strings.Replace(route, "/sub", "", 1)
					if got.Method != method || got.URL.Path != wantPath || got.URL.RawQuery != req.URL.RawQuery {
						t.Errorf("method/path/query changed: %s %s", got.Method, got.URL.String())
					}
					for _, k := range []string{"User-Agent", "Accept-Language", "X-Hwid", "X-Device-Future-Field"} {
						if got.Header.Get(k) != req.Header.Get(k) {
							t.Errorf("request metadata missing: %s", k)
						}
					}
					if values := got.Header.Values("X-Device-Capabilities"); len(values) != 2 || values[0] != "first" || values[1] != "second" {
						t.Error("multi-value device metadata changed")
					}
					if got.Header.Get("Authorization") != "" || got.Header.Get("Cookie") != "" {
						t.Error("client credentials forwarded to panel")
					}
				default:
					t.Fatal("device request was cached or HEAD/GET cache collided")
				}
			}
		})
	}
}

func TestSubscriptionMetadataCacheIsolation(t *testing.T) {
	s := newTestServer(t)
	s.config.Demo = false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, r.Header.Get("Accept-Language")+"/"+r.Header.Get("X-Device-Future-Field"))
	}))
	defer upstream.Close()
	s.gateway.Configure(upstream.URL, nil)
	for _, value := range []string{"first", "second", "first"} {
		req := httptest.NewRequest("GET", "http://console.test/sub/synthetic123", nil)
		req.Header.Set("Accept-Language", value)
		req.Header.Set("X-Device-Future-Field", value)
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, req)
		if out.Code != 200 || out.Body.String() != value+"/"+value {
			t.Fatal("metadata cache variants mixed")
		}
	}
}

func TestSubscriptionRawHeaderCacheIsolation(t *testing.T) {
	s := newTestServer(t)
	s.config.Demo = false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, r.Header.Get("X-Device-Raw")) }))
	defer upstream.Close()
	s.gateway.Configure(upstream.URL, nil)
	for _, raw := range []byte{0xff, 0xfe, 0xff} {
		req := httptest.NewRequest("GET", "http://console.test/sub/synthetic123", nil)
		req.Header.Set("X-Device-Raw", string([]byte{raw}))
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, req)
		if out.Code != 200 || !bytes.Equal(out.Body.Bytes(), []byte{raw}) {
			t.Fatal("raw header cache variants mixed")
		}
	}
}

func TestSubscriptionConnectionMetadataNotForwarded(t *testing.T) {
	s := newTestServer(t)
	s.config.Demo = false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "Announce, Subscription-Userinfo, Content-Length")
		w.Header().Set("Announce", "hop-only")
		w.Header().Set("Subscription-Userinfo", "hop-only")
		w.Header().Set("Content-Length", "4")
		if r.Header.Get("X-Device-Hop") != "" {
			t.Error("request hop metadata forwarded")
		}
		if r.Method != "HEAD" {
			io.WriteString(w, "body")
		}
	}))
	defer upstream.Close()
	s.gateway.Configure(upstream.URL, nil)
	for _, method := range []string{"HEAD", "GET", "HEAD"} {
		req := httptest.NewRequest(method, "http://console.test/sub/synthetic123", nil)
		req.Header.Set("Connection", "X-Device-Hop")
		req.Header.Set("X-Device-Hop", "hop-only")
		out := httptest.NewRecorder()
		s.Handler().ServeHTTP(out, req)
		if out.Code != 200 {
			t.Fatal("hop metadata request failed")
		}
		if out.Header().Get("Announce") != "" || out.Header().Get("Subscription-Userinfo") != "" || out.Header().Get("Content-Length") != "" {
			t.Fatal("response hop metadata forwarded")
		}
	}
}
