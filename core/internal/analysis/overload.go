package analysis

import (
	"github.com/subgate/subgate/pkg/wire"
	"math"
	"time"
)

// LoadState keeps finalized minute windows, continuity and a historic EWMA.
// Missing coverage, packet loss and capacity changes break the streak.
type LoadState struct {
	Minute          int64     `json:"minute"`
	Last            time.Time `json:"last"`
	CoverageMS      int64     `json:"coverage_ms"`
	Bits            float64   `json:"bits"`
	CPU             float64   `json:"cpu"`
	Dropped         uint64    `json:"dropped"`
	Capacity        int       `json:"capacity"`
	Streak          int       `json:"streak"`
	Recovery        int       `json:"recovery"`
	Active          bool      `json:"active"`
	Baseline        float64   `json:"baseline"`
	BaselineWindows int       `json:"baseline_windows"`
	First           time.Time `json:"first"`
}

type LoadWindow struct {
	Minute   int64
	Mbps     float64
	Baseline float64
	CPU      float64
	Percent  float64
	Active   bool
	Complete bool
}

func (s *LoadState) Add(r wire.Resources, p Policy) *LoadWindow {
	p = p.Normalized()
	if !r.Timestamp.After(s.Last) {
		return nil
	}
	minute := r.Timestamp.Add(-time.Millisecond).Unix() / 60
	var completed *LoadWindow
	if s.Minute != 0 && minute != s.Minute {
		mbps := s.Bits / float64(max(s.CoverageMS, 1)) / 1000
		percent := mbps / float64(max(s.Capacity, 1)) * 100
		good := s.CoverageMS >= 45000 && s.Dropped == 0 && minute == s.Minute+1
		if !good {
			s.Streak, s.Recovery, s.Active = 0, 0, false
		} else if percent >= float64(p.OverloadPercent) {
			s.Streak++
			s.Recovery = 0
		} else if percent <= float64(p.RecoveryPercent) {
			s.Streak = 0
			s.Recovery++
			if s.Recovery >= 2 {
				s.Active = false
			}
		} else {
			s.Streak = 0
			s.Recovery = 0
		}
		if s.Streak >= p.SustainedMinutes {
			s.Active = true
		}
		completed = &LoadWindow{s.Minute, mbps, s.Baseline, s.CPU / float64(max(s.CoverageMS, 1)), percent, s.Active, good}
		if good && !s.Active {
			if s.BaselineWindows == 0 {
				s.Baseline = mbps
			} else {
				s.Baseline = .9*s.Baseline + .1*mbps
			}
			s.BaselineWindows++
		}
		s.CoverageMS = 0
		s.Bits = 0
		s.CPU = 0
		s.Dropped = 0
	}
	if s.Capacity != 0 && s.Capacity != r.CapacityMbps {
		s.Streak = 0
		s.Active = false
		s.Baseline = 0
		s.BaselineWindows = 0
	}
	if !s.Last.IsZero() && r.Timestamp.Sub(s.Last) > 20*time.Second {
		s.Streak = 0
		s.Active = false
	}
	s.Minute = minute
	s.Capacity = r.CapacityMbps
	if s.First.IsZero() {
		s.First = r.Timestamp
	}
	// Clip a sample crossing a UTC minute; never credit overlapping intervals.
	start := r.Timestamp.Add(-time.Duration(r.IntervalMS) * time.Millisecond)
	boundary := time.Unix(minute*60, 0)
	if start.Before(boundary) {
		start = boundary
	}
	if start.Before(s.Last) {
		start = s.Last
	}
	duration := r.Timestamp.Sub(start).Milliseconds()
	if duration > 0 {
		s.CoverageMS += duration
		s.Bits += float64(max(r.RXBytes, r.TXBytes)) * 8 * float64(duration) / float64(r.IntervalMS)
		s.CPU += r.CPUPercent * float64(duration)
		s.Dropped += r.Dropped
	}
	s.Last = r.Timestamp
	return completed
}

func ValidResources(r wire.Resources) bool {
	return r.IntervalMS >= 1000 && r.IntervalMS <= 15000 && r.CapacityMbps > 0 && r.CapacityMbps <= 1000000 && r.RXBytes <= 1e15 && r.TXBytes <= 1e15 && !math.IsNaN(r.CPUPercent) && !math.IsInf(r.CPUPercent, 0) && r.CPUPercent >= 0 && r.CPUPercent <= 100
}
