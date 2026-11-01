package agent

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHeartbeatReadiness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "health")
	if CheckHeartbeat(path) == nil {
		t.Fatal("missing heartbeat accepted")
	}
	if err := writeHeartbeat(path); err != nil {
		t.Fatal(err)
	}
	if err := CheckHeartbeat(path); err != nil {
		t.Fatal(err)
	}
	for _, at := range []time.Time{time.Now().Add(-91 * time.Second), time.Now().Add(time.Hour)} {
		os.WriteFile(path, []byte(at.Format(time.RFC3339Nano)), 0600)
		if CheckHeartbeat(path) == nil {
			t.Fatal("stale or future heartbeat accepted")
		}
	}
}
