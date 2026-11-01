package server

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/subgate/subgate/core/internal/config"
	"github.com/subgate/subgate/core/internal/db"
	"github.com/subgate/subgate/pkg/wire"
)

// A populated mid-minute window, twenty enrolled nodes, fifty accounts per node.
// The timed path includes HTTP auth, JSON, journal encryption, analysis, SQLite
// commit and response. SQLite is in-memory: this is not a disk/network SLA.
func BenchmarkTelemetry20Nodes1000Accounts(b *testing.B) {
	b.Run("50_event_batch", func(b *testing.B) { benchmarkTelemetryFleet(b, 1) })
	b.Run("1000_event_batch", func(b *testing.B) { benchmarkTelemetryFleet(b, 20) })
}

func benchmarkTelemetryFleet(b *testing.B, eventsPerAccount int) {
	cfg := config.Defaults()
	cfg.Demo = true
	cfg.Database.URL = ":memory:"
	cfg.Auth.AdminPassword = "synthetic-benchmark-password"
	database, err := db.New(":memory:")
	if err != nil {
		b.Fatal(err)
	}
	if err := db.RunMigrations(database); err != nil {
		database.Close()
		b.Fatal(err)
	}
	s, err := New(cfg, database, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		database.Close()
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Shutdown(context.Background()); database.Close() })
	now := time.Now().UTC()
	start := now.Truncate(time.Minute).Add(-time.Minute).Add(30 * time.Second)
	for node := 0; node < 20; node++ {
		id := fmt.Sprintf("bench-node-%d", node)
		if _, err := s.db.Exec("INSERT INTO nodes(id,name,ip_address,status) VALUES(?,?,?,'online')", id, id, "192.0.2.10"); err != nil {
			b.Fatal(err)
		}
		if _, err := s.db.Exec("INSERT INTO agents(node_id,token_hash,status) VALUES(?,?,'online')", id, hash("bench-token-"+id)); err != nil {
			b.Fatal(err)
		}
		seed := wire.Telemetry{NodeID: id}
		for user := 0; user < 50; user++ {
			for n := 0; n < 300; n++ {
				seed.Events = append(seed.Events, wire.Activity{UserID: fmt.Sprintf("account-%d", node*50+user), Timestamp: start.Add(time.Duration(n) * time.Millisecond), DstHost: "192.0.2.10", DstPort: 443, Network: "tcp", Result: "accepted"})
			}
		}
		tx, err := s.db.BeginTx()
		if err != nil {
			b.Fatal(err)
		}
		err = s.analyzeTelemetry(tx, seed, now)
		if err == nil {
			err = tx.Commit()
		} else {
			tx.Rollback()
		}
		s.db.TxUnlock()
		if err != nil {
			b.Fatal(err)
		}
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		node := i % 20
		id := fmt.Sprintf("bench-node-%d", node)
		batch := wire.Telemetry{NodeID: id, BatchID: fmt.Sprintf("bench-%d", i)}
		for user := 0; user < 50; user++ {
			for n := 0; n < eventsPerAccount; n++ {
				batch.Events = append(batch.Events, wire.Activity{UserID: fmt.Sprintf("account-%d", node*50+user), Timestamp: start.Add(time.Second + time.Duration(i)*time.Microsecond + time.Duration(n)*time.Nanosecond), DstHost: "192.0.2.10", DstPort: 443, Network: "tcp", Result: "accepted"})
			}
		}
		w := perform(s, "POST", "/api/v1/agents/telemetry", batch, nil, "bench-token-"+id, false)
		if w.Code != 200 {
			b.Fatal(w.Code, w.Body)
		}
	}
	b.ReportMetric(float64(b.N*50*eventsPerAccount)/b.Elapsed().Seconds(), "events/s")
}
