package panel

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

// A production token's scopes are a second line of defence: the client must
// reject every non-GET before serialization or network access.
func TestPanelReadOnlyRejectsBeforeSending(t *testing.T) {
	var requests atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		io.WriteString(w, `{"response":{"ok":true}}`)
	}))
	defer upstream.Close()
	c := NewClient(upstream.URL, "synthetic-token", slog.Default())
	defer c.Close()
	for _, method := range []string{"POST", "PATCH", "PUT", "DELETE", "HEAD", "OPTIONS", "TRACE", "CONNECT", "get", ""} {
		t.Run(method, func(t *testing.T) {
			_, err := c.Request(context.Background(), method, "/api/users", nil)
			if !errors.Is(err, ErrReadOnly) {
				t.Error("read-only client accepted non-GET")
			}
			if n := requests.Load(); n != 0 {
				t.Errorf("request reached upstream: %d", n)
			}
		})
	}
	if err := c.Action(context.Background(), "nodes", "synthetic-node", "restart"); err == nil {
		t.Error("action bypassed read-only guard")
	}
	if n := requests.Load(); n != 0 {
		t.Fatalf("action reached upstream: %d", n)
	}
	if _, err := c.Request(context.Background(), http.MethodGet, "/api/hosts", nil); err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	if n := requests.Load(); n != 1 {
		t.Fatalf("GET did not reach upstream once: %d", n)
	}
}

type mustNotMarshal struct{ t *testing.T }

func (v mustNotMarshal) MarshalJSON() ([]byte, error) {
	v.t.Fatal("read-only rejection must precede serialization")
	return nil, nil
}

func TestPanelReadOnlyBeforeSerialization(t *testing.T) {
	c := NewClient("https://panel.example.invalid", "synthetic-token", slog.Default())
	_, err := c.Request(context.Background(), "POST", "/api/users", mustNotMarshal{t})
	if !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only rejection: %v", err)
	}
}

func TestPanelExplicitWriteAccess(t *testing.T) {
	var requests atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Error("wrong write method")
		}
		requests.Add(1)
		io.WriteString(w, `{"response":{"ok":true}}`)
	}))
	defer u.Close()
	c := NewClient(u.URL, "synthetic-token", slog.Default(), WithWriteAccess())
	defer c.Close()
	if _, err := c.Request(context.Background(), "POST", "/api/users", nil); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 1 {
		t.Fatal("explicit write did not reach synthetic panel")
	}
}
