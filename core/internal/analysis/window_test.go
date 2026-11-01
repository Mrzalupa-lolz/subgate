package analysis

import (
	"fmt"
	"testing"
	"time"

	"github.com/subgate/subgate/pkg/wire"
)

func TestFingerprintIsUnambiguousAndNormalizesTime(t *testing.T) {
	a := wire.Activity{Timestamp: time.Unix(1800000000, 0), SrcIP: "a\x00b", DstHost: "c"}
	b := a
	b.SrcIP, b.DstHost = "a", "b\x00c"
	if Fingerprint(a) == Fingerprint(b) {
		t.Fatal("tuple boundary collision")
	}
	b = a
	b.Timestamp = a.Timestamp.In(time.FixedZone("offset", 3600))
	if Fingerprint(a) != Fingerprint(b) {
		t.Fatal("same instant produced distinct fingerprint")
	}
}

func TestWindowSeparatesEvidence(t *testing.T) {
	var w Window
	p := DefaultPolicy()
	p.Ports = 16
	start := time.Unix(1800000000, 0)
	for i := 1; i <= 16; i++ {
		e := wire.Activity{Timestamp: start.Add(time.Duration(i) * time.Millisecond), DstHost: "[::ffff:192.0.2.1]", DstPort: i, Result: "accepted"}
		w.Add(e)
	}
	if w.Count != 16 || len(w.IPs) != 1 {
		t.Fatalf("duplicate or mapped IP counted twice: %+v", w)
	}
	signals := w.Signals(p)
	if len(signals) != 1 || signals[0].Kind != "port_breadth" || signals[0].Value != 16 {
		t.Fatalf("wrong evidence: %+v", signals)
	}
	// A browser visiting many domains on HTTPS is not port fanout.
	var browser Window
	for i := 0; i < 1000; i++ {
		browser.Add(wire.Activity{Timestamp: start, DstHost: fmt.Sprintf("site-%d.example", i), DstPort: 443})
	}
	if signals := browser.Signals(p); len(signals) != 0 {
		t.Fatalf("browser labelled: %+v", signals)
	}
}

func TestWindowBoundedAndTruncationVisible(t *testing.T) {
	var w Window
	for i := 0; i < MaxEvents+100; i++ {
		w.Add(wire.Activity{Timestamp: time.Unix(1800000000, int64(i)), DstHost: "192.0.2.1", DstPort: 443})
	}
	if w.Count != MaxEvents || !w.Truncated {
		t.Fatalf("unbounded or silent truncation: %d %v", w.Count, w.Truncated)
	}
	signals := w.Signals(DefaultPolicy())
	if len(signals) != 1 || !signals[0].Truncated {
		t.Fatalf("quality omitted: %+v", signals)
	}
}

func BenchmarkWindowBatch(b *testing.B) {
	events := make([]wire.Activity, 1000)
	for i := range events {
		events[i] = wire.Activity{Timestamp: time.Unix(1800000000, int64(i)), DstHost: "192.0.2.1", DstPort: i + 1}
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var w Window
		for _, e := range events {
			w.Add(e)
		}
		w.Signals(DefaultPolicy())
	}
}
