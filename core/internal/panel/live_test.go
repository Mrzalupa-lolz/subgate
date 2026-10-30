package panel

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"testing"
)

// Opt-in test for the isolated synthetic panel. Never accept production URLs.
func TestLivePanelReadContract(t *testing.T) {
	base := os.Getenv("SUBGATE_SYNTHETIC_PANEL_URL")
	if base == "" {
		t.Skip("isolated synthetic panel not configured")
	}
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		t.Fatal("synthetic panel must use loopback HTTP")
	}
	token := os.Getenv("SUBGATE_SYNTHETIC_PANEL_TOKEN")
	if token == "" {
		t.Fatal("synthetic panel token is missing")
	}
	c := NewClient(base, token, slog.New(slog.NewTextHandler(io.Discard, nil)))
	defer c.Close()
	c.httpClient.Transport = syntheticPanelTransport{http.DefaultTransport}
	version, err := c.Version(context.Background())
	if err != nil || version != "3.4.5" {
		t.Fatal("synthetic panel version contract failed")
	}
	users, total, err := c.ListUsers(0, 10)
	if err != nil {
		t.Fatal("synthetic panel read/decode failed")
	}
	if total < 1 || len(users) < 1 || users[0].ID == "" || users[0].ShortUUID == "" || users[0].Username == "" || users[0].Status == "" || users[0].ExpireAt == "" {
		t.Fatal("synthetic panel contract assertions failed")
	}
}

type syntheticPanelTransport struct{ upstream http.RoundTripper }

func (s syntheticPanelTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header.Set("X-Forwarded-Proto", "https")
	clone.Header.Set("X-Forwarded-For", "127.0.0.1")
	clone.Host = "localhost"
	return s.upstream.RoundTrip(clone)
}
