package analysis

import (
	"encoding/json"
	"github.com/subgate/subgate/pkg/wire"
	"testing"
	"time"
)

func TestOverloadContinuityBaselineAndRecovery(t *testing.T) {
	start := time.Unix(1800000000, 0).UTC().Truncate(time.Minute)
	var s LoadState
	p := DefaultPolicy()
	minute := func(index int, mbps int, loss uint64) *LoadWindow {
		t.Helper()
		var completed *LoadWindow
		for tick := 1; tick <= 6; tick++ {
			r := wire.Resources{Timestamp: start.Add(time.Duration(index*60+tick*10) * time.Second), IntervalMS: 10000, CapacityMbps: 1000, RXBytes: uint64(mbps) * 1250000, Dropped: loss}
			if w := s.Add(r, p); w != nil {
				completed = w
			}
		}
		return completed
	}
	minute(0, 100, 0)
	minute(1, 100, 0)
	minute(2, 100, 0)
	if s.Baseline != 100 || s.BaselineWindows != 2 {
		t.Fatalf("missing historic baseline: %+v", s)
	}
	minute(3, 900, 0)
	minute(4, 900, 0)
	w := minute(5, 900, 0)
	if s.Active || w.Active {
		t.Fatal("activated before three complete high windows")
	}
	// Serialize mid-run: restart must preserve the evidence and streak.
	raw, _ := json.Marshal(s)
	s = LoadState{}
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	w = minute(6, 600, 0)
	if !w.Active || !s.Active || w.Baseline <= 0 {
		t.Fatalf("sustained load not detected: %+v %+v", w, s)
	}
	minute(7, 600, 0)
	if !s.Active {
		t.Fatal("recovered without two finalized low windows")
	}
	minute(8, 600, 0)
	if s.Active {
		t.Fatal("hysteresis did not recover")
	}
	minute(9, 900, 1)
	minute(10, 900, 0)
	if s.Active || s.Streak != 0 {
		t.Fatal("loss allowed a high-confidence streak")
	}
	// A sparse minute and a capacity change must not inherit overload evidence.
	s.Add(wire.Resources{Timestamp: start.Add(20 * time.Minute), IntervalMS: 10000, CapacityMbps: 100, RXBytes: 1125000000}, p)
	if s.Active || s.Streak != 0 || s.BaselineWindows != 0 {
		t.Fatal("gap or capacity change retained evidence")
	}
}

func TestOverloadDuplicateAndPartialSamples(t *testing.T) {
	var s LoadState
	start := time.Unix(1800000000, 0).UTC()
	r := wire.Resources{Timestamp: start.Add(10 * time.Second), IntervalMS: 10000, CapacityMbps: 1000, RXBytes: 1125000000}
	s.Add(r, DefaultPolicy())
	s.Add(r, DefaultPolicy())
	if s.CoverageMS != 10000 {
		t.Fatal("duplicate sample counted")
	}
	r.Timestamp = start.Add(15 * time.Second)
	s.Add(r, DefaultPolicy())
	if s.CoverageMS != 15000 {
		t.Fatal("overlapping samples counted twice", s.CoverageMS)
	}
	r.Timestamp = start.Add(70 * time.Second)
	w := s.Add(r, DefaultPolicy())
	if w.Complete || s.Active {
		t.Fatal("sparse minute treated as complete")
	}
}
