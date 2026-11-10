package xray

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

// An oversized line must be skipped without stopping collection.
func TestTailSkipsOversizedLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "access.log")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	events := make(chan wire.Activity, 4)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- Tail(ctx, path, events, slog.New(slog.NewTextHandler(io.Discard, nil))) }()
	time.Sleep(150 * time.Millisecond)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	valid := "2026/11/09 19:00:00.000000 from 203.0.113.7:51413 accepted tcp:example.org:443 [REALITY >> DIRECT] email: 42\n"
	if _, err = f.WriteString(strings.Repeat("x", maxLogLine+100) + "\n" + valid); err != nil {
		t.Fatal(err)
	}
	f.Close()
	select {
	case event := <-events:
		if event.UserID != "42" || event.DstHost != "example.org" || event.DstPort != 443 {
			t.Fatalf("unexpected event %+v", event)
		}
	case err := <-done:
		t.Fatalf("tail stopped: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("valid line after oversized line was not delivered")
	}
}
