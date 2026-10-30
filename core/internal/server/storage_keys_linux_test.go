package server

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestStorageKeyRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "synthetic-fifo")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, err := readStorageKey(path); done <- err }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted as key")
		}
	case <-time.After(250 * time.Millisecond):
		// Unblock the buggy reader before reporting failure.
		f, err := os.OpenFile(path, os.O_WRONLY|syscall.O_NONBLOCK, 0)
		if err == nil {
			f.Close()
		}
		<-done
		t.Fatal("key reader blocked on FIFO")
	}
}
