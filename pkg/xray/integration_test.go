package xray

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"
)

// Opt-in test on a real node. Counter values are never reset and user
// identities are never printed; only the number of observed counters is shown.
func TestLiveStatsService(t *testing.T) {
	address := os.Getenv("SUBGATE_TEST_XRAY_SERVER")
	if address == "" {
		t.Skip("set SUBGATE_TEST_XRAY_SERVER=auto to verify a real node")
	}
	client, err := NewStatsClient(address, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stats, err := client.QueryStats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range stats {
		if sample.UplinkBytes < 0 || sample.DownlinkBytes < 0 || sample.UplinkDelta != 0 || sample.DownlinkDelta != 0 {
			t.Fatal("invalid initial live counter values")
		}
	}
	t.Logf("live Xray StatsService query succeeded; users with counters: %d", len(stats))
}
