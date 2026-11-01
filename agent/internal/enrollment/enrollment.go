package enrollment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/subgate/subgate/agent/internal/agent"
)

// EnrollToFile verifies local storage before consuming the one-time token and
// replaces the credential atomically, preserving the old file on failure.
func EnrollToFile(core, token, node, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".enroll-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	raw, err := Enroll(core, token, node)
	if err != nil {
		return err
	}
	if _, err = f.WriteString(raw); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), path)
	}
	if err != nil {
		return fmt.Errorf("credential received but not saved; issue a new enrollment token and retry: %w", err)
	}
	return nil
}

func Enroll(core, token, node string) (string, error) {
	base, err := agent.NormalizeCore(core)
	if err != nil {
		return "", err
	}
	raw, _ := json.Marshal(map[string]string{"token": token, "node_id": node})
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Post(base+"/api/v1/agents/enroll", "application/json", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 201 {
		return "", fmt.Errorf("enrollment returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Token  string `json:"token"`
		NodeID string `json:"node_id"`
	}
	if err = json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&result); err != nil {
		return "", err
	}
	if result.Token == "" || result.NodeID != node {
		return "", fmt.Errorf("invalid enrollment response")
	}
	return result.Token, nil
}
