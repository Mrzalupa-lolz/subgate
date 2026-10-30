package enrollment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/subgate/subgate/agent/internal/agent"
)

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
