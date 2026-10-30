// Package youtube provides a bounded regional reachability diagnostic. It is
// intentionally a normal HTTP probe, so operators can point it at an internal
// mirror in tests or deployments that restrict public YouTube access.
package youtube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type Verdict string

const (
	OK      Verdict = "ok"
	Blocked Verdict = "blocked"
	Suspect Verdict = "suspect"
	Unknown Verdict = "unknown"
)

type Config struct {
	Endpoint string
	VideoID  string
	Timeout  time.Duration
	MaxBody  int64
	Client   *http.Client
}

type Result struct {
	Verdict    Verdict `json:"verdict"`
	Region     string  `json:"region,omitempty"`
	Status     int     `json:"status"`
	DurationMS int64   `json:"duration_ms"`
	Error      string  `json:"error,omitempty"`
}

var regionPattern = regexp.MustCompile(`(?i)(?:GL|contentRegion|regionCode|"country"\s*:\s*")[:=\s"]+([A-Z]{2})`)

func ParseRegion(body string) string {
	if match := regionPattern.FindStringSubmatch(body); len(match) > 1 {
		return strings.ToUpper(match[1])
	}
	return ""
}

func Check(ctx context.Context, cfg Config) Result {
	if cfg.Endpoint == "" {
		id := cfg.VideoID
		if id == "" {
			id = "dQw4w9WgXcQ"
		}
		cfg.Endpoint = "https://www.youtube.com/watch?v=" + id
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.MaxBody <= 0 || cfg.MaxBody > 4<<20 {
		cfg.MaxBody = 1 << 20
	}
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: cfg.Timeout}
	}
	started := time.Now()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, cfg.Endpoint, nil)
	if err != nil {
		return Result{Verdict: Unknown, DurationMS: time.Since(started).Milliseconds(), Error: err.Error()}
	}
	request.Header.Set("User-Agent", "SubGate/1 regional-check")
	response, err := client.Do(request)
	if err != nil {
		return Result{Verdict: Suspect, DurationMS: time.Since(started).Milliseconds(), Error: err.Error()}
	}
	defer response.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(response.Body, cfg.MaxBody))
	result := Result{Status: response.StatusCode, Region: ParseRegion(string(body)), DurationMS: time.Since(started).Milliseconds()}
	if readErr != nil {
		result.Error = readErr.Error()
	}
	switch {
	case response.StatusCode == http.StatusOK && readErr == nil && !strings.Contains(strings.ToLower(string(body)), "video unavailable"):
		result.Verdict = OK
	case response.StatusCode == http.StatusForbidden || response.StatusCode == http.StatusUnavailableForLegalReasons || strings.Contains(strings.ToLower(string(body)), "not available in your country"):
		result.Verdict = Blocked
	default:
		result.Verdict = Suspect
	}
	return result
}

func (r Result) Err() error {
	if r.Error == "" {
		return nil
	}
	return fmt.Errorf("youtube check: %s", r.Error)
}
