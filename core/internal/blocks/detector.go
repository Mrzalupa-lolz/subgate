// Package blocks implements the regional 16-20 KiB transport block check.
// The probe deliberately reads a bounded amount of data and treats a read
// timeout in the characteristic stall window as evidence, rather than trying
// to infer censorship from a single connection failure.
package blocks

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

type Verdict string

const (
	OK      Verdict = "ok"
	Suspect Verdict = "suspect"
	Blocked Verdict = "blocked"
)

type Config struct {
	Address     string
	ServerName  string
	TLS         bool
	Timeout     time.Duration
	MaxBytes    int
	StallMin    int
	StallMax    int
	StallWindow time.Duration
}

type Result struct {
	Verdict   Verdict       `json:"verdict"`
	Address   string        `json:"address,omitempty"`
	Bytes     int           `json:"bytes"`
	StallByte int           `json:"stall_bytes,omitempty"`
	Duration  time.Duration `json:"duration"`
	Error     string        `json:"error,omitempty"`
}

func (c *Config) defaults() {
	if c.Timeout <= 0 {
		c.Timeout = 8 * time.Second
	}
	if c.MaxBytes <= 0 || c.MaxBytes > 1<<20 {
		c.MaxBytes = 64 << 10
	}
	if c.StallMin <= 0 {
		c.StallMin = 14 << 10
	}
	if c.StallMax <= 0 {
		c.StallMax = 24 << 10
	}
	if c.StallMax < c.StallMin {
		c.StallMin, c.StallMax = c.StallMax, c.StallMin
	}
	if c.StallWindow <= 0 {
		c.StallWindow = 1200 * time.Millisecond
	}
}

// Classify converts bounded reader observations to a verdict. A timeout in
// the configured byte window is the strong blocked signal; other failures are
// retained as suspect so callers can require corroboration from another view.
func Classify(bytesRead int, stalled bool, stallDuration time.Duration, readErr error, cfg Config) Verdict {
	cfg.defaults()
	inWindow := bytesRead >= cfg.StallMin && bytesRead <= cfg.StallMax
	if inWindow && stalled && stallDuration >= cfg.StallWindow {
		return Blocked
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		if inWindow && stalled {
			return Suspect
		}
		return Suspect
	}
	return OK
}

// Probe opens the configured endpoint and reads at most MaxBytes.
func Probe(ctx context.Context, cfg Config) (Result, error) {
	cfg.defaults()
	if strings.TrimSpace(cfg.Address) == "" {
		return Result{Verdict: Suspect}, fmt.Errorf("probe address is required")
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	dialer := &net.Dialer{Timeout: cfg.Timeout}
	var conn net.Conn
	var err error
	if cfg.TLS {
		serverName := cfg.ServerName
		if serverName == "" {
			host, _, splitErr := net.SplitHostPort(cfg.Address)
			if splitErr == nil {
				serverName = host
			}
		}
		conn, err = (&tls.Dialer{NetDialer: dialer, Config: &tls.Config{ServerName: serverName, MinVersion: tls.VersionTLS12}}).DialContext(ctx, "tcp", cfg.Address)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", cfg.Address)
	}
	if err != nil {
		result := Result{Verdict: Suspect, Address: cfg.Address, Duration: time.Since(started), Error: err.Error()}
		return result, err
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(cfg.Timeout))
	buf := make([]byte, 4096)
	bytesRead := 0
	stallStarted := time.Time{}
	var readErr error
	for bytesRead < cfg.MaxBytes {
		want := len(buf)
		if remaining := cfg.MaxBytes - bytesRead; remaining < want {
			want = remaining
		}
		n, e := conn.Read(buf[:want])
		bytesRead += n
		if e != nil {
			readErr = e
			if n == 0 && (errors.Is(e, osTimeout{}) || isTimeout(e)) {
				stallStarted = time.Now()
			}
			break
		}
	}
	stalled := isTimeout(readErr) || !stallStarted.IsZero()
	stallDuration := time.Duration(0)
	if stalled {
		stallDuration = time.Since(started)
	}
	result := Result{Address: cfg.Address, Bytes: bytesRead, Duration: time.Since(started), Verdict: Classify(bytesRead, stalled, stallDuration, readErr, cfg)}
	if stalled {
		result.StallByte = bytesRead
	}
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		result.Error = readErr.Error()
	}
	if result.Verdict == Blocked || result.Verdict == Suspect {
		return result, nil
	}
	return result, nil
}

// Detector is a reusable probe with defaults and an optional clock-independent
// configuration. New callers should use Detector.Check for a named target.
type Detector struct{ Config Config }

func New(cfg Config) *Detector { cfg.defaults(); return &Detector{Config: cfg} }
func (d *Detector) Check(ctx context.Context, address string) (Result, error) {
	cfg := d.Config
	cfg.Address = address
	return Probe(ctx, cfg)
}

func isTimeout(err error) bool {
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// osTimeout is kept as a tiny sentinel for tests that use a timeout-shaped
// reader without importing net internals.
type osTimeout struct{}

func (osTimeout) Error() string   { return "timeout" }
func (osTimeout) Timeout() bool   { return true }
func (osTimeout) Temporary() bool { return true }
