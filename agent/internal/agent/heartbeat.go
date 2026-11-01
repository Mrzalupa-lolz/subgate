package agent

import (
	"fmt"
	"os"
	"time"
)

func writeHeartbeat(path string) error {
	if err := os.WriteFile(path+".new", []byte(time.Now().UTC().Format(time.RFC3339Nano)), 0600); err != nil {
		return err
	}
	return os.Rename(path+".new", path)
}

// CheckHeartbeat reports readiness only after Core accepts a heartbeat.
func CheckHeartbeat(path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	at, err := time.Parse(time.RFC3339Nano, string(raw))
	if err != nil || time.Since(at) < 0 || time.Since(at) > 90*time.Second {
		return fmt.Errorf("no successful Core heartbeat in the last 90 seconds")
	}
	return nil
}
