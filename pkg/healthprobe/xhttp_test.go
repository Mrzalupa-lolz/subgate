package healthprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestXHTTPOptionsRequiresConfiguredRoute(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    int
		cors    bool
		success bool
	}{
		{"preflight", 200, true, true},
		{"plain200", 200, false, false},
		{"redirect", 302, true, false},
		{"not-found", 404, true, false},
		{"oversize-header", 200, true, false},
		{"wrong-echo", 200, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "OPTIONS" || r.URL.Path != "/probe" {
					t.Error("not a configured-path preflight")
				}
				if tc.cors {
					w.Header().Set("Access-Control-Allow-Origin", "*")
					w.Header().Set("Access-Control-Allow-Methods", r.Header.Get("Access-Control-Request-Method"))
					w.Header().Set("Access-Control-Allow-Headers", r.Header.Get("Access-Control-Request-Headers"))
				}
				if tc.name == "wrong-echo" {
					w.Header().Set("Access-Control-Allow-Methods", "GET")
				}
				if tc.name == "oversize-header" {
					w.Header().Set("X-Padding", strings.Repeat("x", 20000))
				}
				w.Header().Set("Location", "http://127.0.0.1:1/")
				w.WriteHeader(tc.code)
			}))
			defer srv.Close()
			target := httpTarget(t, srv.URL)
			target.Inbounds[0].Network = "xhttp"
			r := Run(context.Background(), target, time.Second)
			if r.Verified != "xhttp-options" || r.Success != tc.success {
				t.Fatal(r)
			}
		})
	}
}

func TestXHTTPOptionsDeadlineAndUnverifiableSecurity(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer srv.Close()
	target := httpTarget(t, srv.URL)
	target.Inbounds[0].Network = "xhttp"
	start := time.Now()
	r := Run(context.Background(), target, 30*time.Millisecond)
	if r.Success || r.Verified != "xhttp-options" || time.Since(start) > time.Second {
		t.Fatal("preflight deadline ignored", r)
	}
	for _, security := range []string{"reality", "future-security"} {
		target.Inbounds[0].Security = security
		r = Run(context.Background(), target, time.Second)
		if r.Success || r.Verified != "unverified" {
			t.Fatal("unknown security guessed", r)
		}
	}
}

func TestXHTTPOptionsPreservesLiteralPath(t *testing.T) {
	for _, path := range []string{"/literal%25/", "/literal%/", "/literal#fragment/"} {
		t.Run(path, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != path || r.URL.RawQuery != "" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				w.Header().Set("Access-Control-Allow-Origin", "*")
				w.Header().Set("Access-Control-Allow-Methods", "POST")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			}))
			defer srv.Close()
			target := httpTarget(t, srv.URL)
			target.Inbounds[0].Network = "xhttp"
			target.Inbounds[0].HTTP.Path = path
			if r := Run(context.Background(), target, time.Second); !r.Success || r.Verified != "xhttp-options" {
				t.Fatal("literal XHTTP path changed", r)
			}
		})
	}
}
