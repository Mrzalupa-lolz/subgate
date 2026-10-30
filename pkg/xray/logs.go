package xray

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

var accessPattern = regexp.MustCompile(`(?:from\s+)?((?:tcp:|udp:)?\S+)\s+(accepted|rejected)\s+(tcp|udp):(\S+)(?:\s+\[([^\]]+)\])?`)
var userPattern = regexp.MustCompile(`\bemail:\s*(\S+)`)

func ParseLine(line string) *wire.Activity {
	if len(line) > 1<<20 || line == "" {
		return nil
	}
	var event wire.Activity
	if strings.HasPrefix(strings.TrimSpace(line), "{") {
		if json.Unmarshal([]byte(line), &event) == nil && event.UserID != "" && event.DstHost != "" {
			if event.Timestamp.IsZero() {
				event.Timestamp = time.Now()
			}
			return &event
		}
		return nil
	}
	m := accessPattern.FindStringSubmatch(line)
	if len(m) == 0 {
		return nil
	}
	user := userPattern.FindStringSubmatch(line)
	if len(user) < 2 {
		return nil
	}
	event.UserID = user[1]
	event.Result = m[2]
	event.Network = m[3]
	event.Timestamp = time.Now()
	if len(line) >= 19 {
		for _, format := range []string{"2006/01/02 15:04:05", "2006-01-02 15:04:05"} {
			if ts, err := time.ParseInLocation(format, line[:19], time.Local); err == nil {
				event.Timestamp = ts
				break
			}
		}
	}
	src := strings.TrimPrefix(strings.TrimPrefix(m[1], "tcp:"), "udp:")
	if host, _, err := net.SplitHostPort(src); err == nil {
		event.SrcIP = host
	} else if net.ParseIP(src) != nil {
		event.SrcIP = src
	}
	host, port, err := net.SplitHostPort(m[4])
	if err != nil {
		return nil
	}
	event.DstHost = host
	event.DstPort, _ = strconv.Atoi(port)
	if len(m) > 5 {
		tags := strings.Split(m[5], " -> ")
		if len(tags) != 2 {
			tags = strings.Split(m[5], " >> ")
		}
		event.InboundTag = strings.TrimSpace(tags[0])
		if len(tags) == 2 {
			event.OutboundTag = strings.TrimSpace(tags[1])
		}
	}
	return &event
}

// Tail follows append, rename rotation and copytruncate. Partial lines are kept
// until completed; scanning is restarted after EOF rather than using Scanner.
func Tail(ctx context.Context, path string, events chan<- wire.Activity, logger *slog.Logger) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { f.Close() }()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	offset := info.Size()
	if _, err = f.Seek(offset, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReaderSize(f, 64<<10)
	partial := ""
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			for {
				part, e := reader.ReadString('\n')
				offset += int64(len(part))
				partial += part
				if len(partial) > 1<<20 {
					return fmt.Errorf("Xray log line exceeds 1 MiB")
				}
				if strings.HasSuffix(partial, "\n") {
					if event := ParseLine(strings.TrimSpace(partial)); event != nil {
						select {
						case events <- *event:
						case <-ctx.Done():
							return nil
						}
					}
					partial = ""
				}
				if e != nil {
					if !errors.Is(e, io.EOF) {
						return e
					}
					break
				}
			}
			current, e := os.Stat(path)
			if e != nil {
				if os.IsNotExist(e) {
					continue
				}
				return e
			}
			openInfo, e := f.Stat()
			if e != nil {
				return e
			}
			if !os.SameFile(openInfo, current) || current.Size() < offset {
				replacement, e := os.Open(path)
				if e != nil {
					continue
				}
				f.Close()
				f = replacement
				reader = bufio.NewReaderSize(f, 64<<10)
				offset = 0
				partial = ""
				logger.Info("Xray access log reopened after rotation")
			}
		}
	}
}
