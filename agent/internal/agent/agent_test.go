package agent

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

func testAgent(t *testing.T, core, path, token string) *Agent {
	t.Helper()
	a, err := NewAgent(&Config{CoreAddress: core, NodeID: "test-node", Token: token, StatsAddress: "127.0.0.1:10085", SpoolFile: path, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Stop(); a.stats.Close(); a.client.CloseIdleConnections() })
	return a
}
func testBatch(id string) wire.Telemetry {
	return wire.Telemetry{BatchID: id, NodeID: "test-node", Samples: []wire.Sample{{UserID: "private-user", Timestamp: time.Now().UTC(), UplinkBps: 100, DownlinkBps: 200}}, Events: []wire.Activity{{UserID: "private-user", Timestamp: time.Now().UTC(), DstHost: "private-destination.example.com", DstPort: 443, Network: "tcp"}}}
}

func TestEncryptedSpoolReplaysSameBatchAfterFailedSendAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.sgb")
	var accepted atomic.Bool
	var mu sync.Mutex
	var received []wire.Telemetry
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer agent-secret-token" {
			t.Error("agent authorization dropped")
		}
		var batch wire.Telemetry
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
		}
		mu.Lock()
		received = append(received, batch)
		mu.Unlock()
		if !accepted.Load() {
			w.WriteHeader(503)
			return
		}
		io.WriteString(w, `{"duplicate":true}`)
	}))
	defer u.Close()
	a := testAgent(t, u.URL, path, "agent-secret-token")
	batch := testBatch("same-batch-on-restart")
	a.pending = []wire.Telemetry{batch}
	a.pendingCreated = time.Now()
	if err := a.deliverPending(); err == nil {
		t.Fatal("failed Core response acknowledged")
	}
	if len(a.pending) != 1 {
		t.Fatal("failed request removed pending batch")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"agent-secret-token", "private-user", "private-destination", "same-batch-on-restart"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("spool exposes %s", secret)
		}
	}
	if !bytes.HasPrefix(raw, []byte(spoolMagic)) {
		t.Fatal("missing authenticated spool format")
	}
	restarted := testAgent(t, u.URL, path, "agent-secret-token")
	if len(restarted.pending) != 1 || restarted.pending[0].BatchID != batch.BatchID {
		t.Fatalf("restart lost pending identity: %+v", restarted.pending)
	}
	accepted.Store(true)
	if err := restarted.deliverPending(); err != nil {
		t.Fatal(err)
	}
	if len(restarted.pending) != 0 {
		t.Fatal("acknowledged batch retained")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("acknowledged spool remains: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 || received[0].BatchID != received[1].BatchID {
		t.Fatalf("restart duplicated with a new identity: %+v", received)
	}
	first, _ := json.Marshal(received[0])
	second, _ := json.Marshal(received[1])
	if !bytes.Equal(first, second) {
		t.Fatal("retry changed telemetry payload")
	}
}

func TestSpoolRejectsCorruptionWrongTokenAndNode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.sgb")
	a := testAgent(t, "http://127.0.0.1:8080", path, "original-token")
	batch := testBatch("batch")
	if err := a.persistSpool([]wire.Telemetry{batch}, time.Now()); err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, token, node string
		corrupt           bool
	}{{"wrong token", "replacement-token", "test-node", false}, {"wrong node", "original-token", "other-node", false}, {"tampered ciphertext", "original-token", "test-node", true}} {
		t.Run(tc.name, func(t *testing.T) {
			raw := append([]byte(nil), original...)
			if tc.corrupt {
				raw[len(raw)-1] ^= 1
			}
			if err := os.WriteFile(path, raw, 0600); err != nil {
				t.Fatal(err)
			}
			_, err := NewAgent(&Config{CoreAddress: "http://127.0.0.1:8080", NodeID: tc.node, Token: tc.token, StatsAddress: "127.0.0.1:10085", SpoolFile: path})
			if err == nil {
				t.Fatal("unauthenticated spool accepted")
			}
			if strings.Contains(err.Error(), tc.token) || strings.Contains(err.Error(), "private-user") {
				t.Fatal("spool error leaks credentials or payload")
			}
			after, e := os.ReadFile(path)
			if e != nil || !bytes.Equal(raw, after) {
				t.Fatal("corrupt spool overwritten or discarded")
			}
		})
	}
}

func TestTelemetryBatchBoundsAndExplicitOverflowCount(t *testing.T) {
	now := time.Now().UTC()
	samples := make([]wire.Sample, 11000)
	for i := range samples {
		samples[i] = wire.Sample{UserID: "user", Timestamp: now, UplinkBps: 1, DownlinkBps: 2}
	}
	events := make([]wire.Activity, 1200)
	for i := range events {
		events[i] = wire.Activity{UserID: "user", Timestamp: now, DstHost: strings.Repeat("<", 1024), DstPort: 443, Network: "tcp"}
	}
	batches, dropped, expired, err := splitTelemetry("test-node", samples, events, now)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 0 || expired != 0 {
		t.Fatalf("valid measurements discarded: %d %d", dropped, expired)
	}
	var sampleCount, eventCount int
	identities := map[string]bool{}
	for _, batch := range batches {
		raw, _ := json.Marshal(batch)
		if len(raw) > maxBatchBytes || len(batch.Samples) > maxBatchSamples || len(batch.Events) > maxBatchEvents {
			t.Fatalf("unbounded Core batch: bytes=%d samples=%d events=%d", len(raw), len(batch.Samples), len(batch.Events))
		}
		if identities[batch.BatchID] {
			t.Fatal("split batches share identity")
		}
		identities[batch.BatchID] = true
		sampleCount += len(batch.Samples)
		eventCount += len(batch.Events)
	}
	if sampleCount != 11000 || eventCount != 1200 || len(batches) < 4 {
		t.Fatalf("split lost values: samples=%d events=%d batches=%d", sampleCount, eventCount, len(batches))
	}
	overflow := make([]wire.Sample, 20000)
	for i := range overflow {
		overflow[i] = wire.Sample{UserID: strings.Repeat("<", 256), Timestamp: now, UplinkBps: 1}
	}
	batches, dropped, expired, err = splitTelemetry("test-node", overflow, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	totalBytes := 0
	for _, batch := range batches {
		raw, _ := json.Marshal(batch)
		totalBytes += len(raw)
	}
	if dropped == 0 || telemetryCount(batches)+dropped != uint64(len(overflow)) || totalBytes > maxSpoolBytes || expired != 0 {
		t.Fatalf("unbounded/lost queue: bytes=%d count=%d dropped=%d expired=%d", totalBytes, telemetryCount(batches), dropped, expired)
	}
}

func TestExpiredSpoolIsCountedAndNotReplayed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.sgb")
	a := testAgent(t, "http://127.0.0.1:8080", path, "token")
	batch := testBatch("expired")
	old := time.Now().Add(-25 * time.Hour)
	batch.Samples[0].Timestamp = old
	batch.Events[0].Timestamp = old
	if err := a.persistSpool([]wire.Telemetry{batch}, old); err != nil {
		t.Fatal(err)
	}
	restarted := testAgent(t, "http://127.0.0.1:8080", path, "token")
	if len(restarted.pending) != 0 || restarted.expired != 2 {
		t.Fatalf("expired spool replayed or not counted: pending=%d expired=%d", len(restarted.pending), restarted.expired)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("expired file remains")
	}
	batches, dropped, expired, err := splitTelemetry("test-node", batch.Samples, batch.Events, time.Now())
	if err != nil || len(batches) != 0 || dropped != 0 || expired != 2 {
		t.Fatalf("expired input batch: %+v %d %d %v", batches, dropped, expired, err)
	}
}

func TestAcknowledgementPersistenceFailureKeepsIdempotency(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.sgb")
	var sabotaged atomic.Bool
	var mu sync.Mutex
	var ids []string
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch wire.Telemetry
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
		}
		mu.Lock()
		ids = append(ids, batch.BatchID)
		mu.Unlock()
		if sabotaged.CompareAndSwap(false, true) {
			if err := os.Remove(path); err != nil {
				t.Error(err)
			}
			if err := os.Mkdir(path, 0700); err != nil {
				t.Error(err)
			}
			if err := os.WriteFile(filepath.Join(path, "prevent-remove"), []byte("x"), 0600); err != nil {
				t.Error(err)
			}
		}
		io.WriteString(w, `{"ok":true}`)
	}))
	defer u.Close()
	a := testAgent(t, u.URL, path, "token")
	a.pending = []wire.Telemetry{testBatch("ack-id")}
	a.pendingCreated = time.Now()
	if err := a.deliverPending(); err == nil {
		t.Fatal("ack persistence failure hidden")
	}
	if len(a.pending) != 1 || a.Health()["spool_error"] == "" {
		t.Fatal("ack failure lost pending batch or diagnostics")
	}
	if err := os.Remove(filepath.Join(path, "prevent-remove")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := a.deliverPending(); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(ids) != 2 || ids[0] != ids[1] {
		t.Fatalf("ack failure replay changed id: %v", ids)
	}
}

func TestSpoolFailurePreventsUnpersistedDelivery(t *testing.T) {
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer u.Close()
	a := testAgent(t, u.URL, filepath.Join(t.TempDir(), "pending.sgb"), "token")
	if err := os.Mkdir(a.config.SpoolFile, 0700); err != nil {
		t.Fatal(err)
	}
	a.pending = []wire.Telemetry{testBatch("unsaved")}
	a.pendingCreated = time.Now()
	if err := a.deliverPending(); err == nil {
		t.Fatal("unwritable spool accepted")
	}
	if calls.Load() != 0 || len(a.pending) != 1 || a.Health()["spool_error"] == "" {
		t.Fatalf("unpersisted batch sent or lost: calls=%d pending=%d", calls.Load(), len(a.pending))
	}
}

func TestStopCancelsPendingDelivery(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer u.Close()
	defer close(release)
	a := testAgent(t, u.URL, "", "token")
	a.pending = []wire.Telemetry{testBatch("stop-id")}
	a.pendingCreated = time.Now()
	done := make(chan error, 1)
	go func() { done <- a.deliverPending() }()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("delivery did not start")
	}
	a.Stop()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled delivery acknowledged")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stop blocked on delivery")
	}
	if len(a.pending) != 1 {
		t.Fatal("cancelled delivery lost pending batch")
	}
}

func TestPermanentTelemetryRejectionDropsOnlyRejectedBatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.sgb")
	var calls atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(400) }))
	defer u.Close()
	a := testAgent(t, u.URL, path, "token")
	a.pending = []wire.Telemetry{testBatch("bad-batch"), testBatch("next-batch")}
	a.pendingCreated = time.Now()
	if err := a.deliverPending(); err != nil {
		t.Fatal(err)
	}
	if len(a.pending) != 1 || a.pending[0].BatchID != "next-batch" || a.dropped != 2 || a.pendingBatches != 1 {
		t.Fatalf("permanent 400 queue handling: pending=%+v dropped=%d batches=%d", a.pending, a.dropped, a.pendingBatches)
	}
	if a.deliveryError == "" {
		t.Fatal("permanent rejection diagnostic missing")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("remaining pending spool removed on first rejection: %v", err)
	}
}

func TestRevokedCredentialPreservesPendingForReenrollment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pending.sgb")
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer u.Close()
	a := testAgent(t, u.URL, path, "token")
	a.pending = []wire.Telemetry{testBatch("revoked-batch")}
	a.pendingCreated = time.Now()
	if err := a.deliverPending(); err == nil {
		t.Fatal("401 accepted")
	}
	if len(a.pending) != 1 || a.pending[0].BatchID != "revoked-batch" {
		t.Fatal("401 lost pending batch")
	}
	if !strings.Contains(a.deliveryError, "re-enroll") {
		t.Fatalf("401 diagnostic: %s", a.deliveryError)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("401 removed durable pending: %v", err)
	}
}

// Regression: the retention clock of a drained queue must not expire batches
// produced more than 24h after the agent started.
func TestDrainedQueueRestartsRetentionClock(t *testing.T) {
	var received atomic.Int32
	u := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received.Add(1)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer u.Close()
	a := testAgent(t, u.URL, filepath.Join(t.TempDir(), "spool"), "agent-secret-token")
	// State left behind by a queue that was drained a day ago.
	a.pendingCreated = time.Now().Add(-25 * time.Hour)
	a.appendPending(testBatch("after-a-day"), time.Now())
	if err := a.deliverPending(); err != nil {
		t.Fatal(err)
	}
	if received.Load() != 1 || len(a.pending) != 0 || a.expired != 0 {
		t.Fatalf("fresh batch was not delivered: received=%d pending=%d expired=%d", received.Load(), len(a.pending), a.expired)
	}
	if !a.pendingCreated.IsZero() {
		t.Fatal("drained queue kept its retention clock")
	}
	first, second := testBatch("oldest"), testBatch("newest")
	a.appendPending(first, time.Now().Add(-time.Minute))
	a.appendPending(second, time.Now())
	if err := a.deliverPending(); err != nil {
		t.Fatal(err)
	}
	if len(a.pending) != 1 || !a.pendingCreated.Equal(a.pending[0].SentAt) {
		t.Fatalf("retention clock does not follow the oldest batch: %v %v", a.pendingCreated, a.pending)
	}
}

func TestThinIdleKeepsContiguousCoverage(t *testing.T) {
	a := &Agent{}
	start := time.Unix(1_800_000_000, 0).UTC()
	sample := func(sec int, bps int64) wire.Sample {
		delta := bps / 8
		return wire.Sample{UserID: "u", Scope: "user", Timestamp: start.Add(time.Duration(sec) * time.Second), IntervalMS: 1000, UplinkBps: bps, UplinkDelta: delta}
	}
	var sent []wire.Sample
	for sec := 0; sec < 40; sec++ {
		bps := int64(0)
		if sec < 3 || sec == 30 {
			bps = 8000
		}
		sent = append(sent, a.thinIdle([]wire.Sample{sample(sec, bps)})...)
	}
	if len(sent) >= 40 || len(sent) < 10 {
		t.Fatalf("idle samples not thinned: %d sent", len(sent))
	}
	for i := 1; i < len(sent); i++ {
		gap := sent[i].Timestamp.Sub(sent[i-1].Timestamp).Milliseconds()
		if sent[i].IntervalMS != gap {
			t.Fatalf("sample %d interval %d does not cover gap %d: %+v", i, sent[i].IntervalMS, gap, sent[i])
		}
		if sent[i].IntervalMS < 1000 || sent[i].IntervalMS > 15000 {
			t.Fatalf("interval outside analysis bounds: %+v", sent[i])
		}
	}
	active := 0
	for _, s := range sent {
		if s.UplinkBps > 0 {
			active++
		}
	}
	if active != 4 {
		t.Fatalf("active samples lost: %d", active)
	}
	// A vanished series (Xray restart) is forgotten.
	a.thinIdle(nil)
	if len(a.series) != 0 {
		t.Fatal("series state leaked")
	}
}
