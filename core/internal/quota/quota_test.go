package quota

import (
	"math"
	"testing"
	"time"
)

func TestCalendarQuotaResetsAtConfiguredPeriodBoundary(t *testing.T) {
	now := time.Date(2026, time.January, 31, 23, 30, 0, 0, time.UTC)
	m := New()
	m.SetClock(func() time.Time { return now })
	if err := m.Upsert(Rule{ID: "monthly", NodeID: "node", UserID: "user", Scope: "per_user", LimitBytes: 100, Period: PeriodMonth, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if allowed, _ := m.Consume("node", "user", 100); !allowed {
		t.Fatal("initial quota consumption was denied")
	}
	now = time.Date(2026, time.February, 1, 0, 1, 0, 0, time.UTC)
	checks := m.Check("node", "user", 1)
	if len(checks) != 1 || checks[0].Bytes != 0 || !checks[0].Allowed {
		t.Fatalf("quota did not reset at month boundary: %+v", checks)
	}
}

func TestRollingQuotaRetainsSamplesAcrossCalendarBoundary(t *testing.T) {
	now := time.Date(2026, time.January, 31, 23, 30, 0, 0, time.UTC)
	m := New()
	m.SetClock(func() time.Time { return now })
	if err := m.Upsert(Rule{ID: "rolling", NodeID: "node", Scope: "per_node", LimitBytes: 100, Period: PeriodRolling, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if allowed, _ := m.Consume("node", "user", 80); !allowed {
		t.Fatal("initial rolling quota consumption was denied")
	}
	now = now.Add(24 * time.Hour)
	checks := m.Check("node", "other-user", 21)
	if len(checks) != 1 || checks[0].Bytes != 80 || checks[0].Allowed {
		t.Fatalf("rolling sample was lost across calendar boundary: %+v", checks)
	}
	now = now.Add(30 * 24 * time.Hour)
	checks = m.Check("node", "other-user", 100)
	if len(checks) != 1 || checks[0].Bytes != 0 || !checks[0].Allowed {
		t.Fatalf("expired rolling sample was retained: %+v", checks)
	}
}

func TestRecordAccountsDeliveredBytesAfterLimitAndIsIdempotenceCallerControlled(t *testing.T) {
	m := New()
	if err := m.Upsert(Rule{ID: "recorded", NodeID: "node", UserID: "user", Scope: "per_user", LimitBytes: 100, Period: PeriodDay, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	m.Record("node", "user", 70)
	if remaining := m.Remaining("node", "user"); remaining != 30 {
		t.Fatalf("first observation not recorded: remaining=%d", remaining)
	}
	m.Record("node", "user", 50)
	if remaining := m.Remaining("node", "user"); remaining != 0 {
		t.Fatalf("over-limit observation not retained: remaining=%d", remaining)
	}
}

func TestQuotaRejectsInvalidRules(t *testing.T) {
	m := New()
	for _, rule := range []Rule{
		{ID: "", NodeID: "node", Scope: "per_node", LimitBytes: 1},
		{ID: "bad-scope", NodeID: "node", Scope: "global", LimitBytes: 1},
		{ID: "missing-user", NodeID: "node", Scope: "per_user", LimitBytes: 1},
		{ID: "negative-grace", NodeID: "node", Scope: "per_node", LimitBytes: 1, GraceBytes: -1},
		{ID: "overflowing-grace", NodeID: "node", Scope: "per_node", LimitBytes: math.MaxInt64, GraceBytes: 1},
		{ID: "invalid-zone", NodeID: "node", Scope: "per_node", LimitBytes: 1, Timezone: "not/a/timezone"},
	} {
		if err := m.Upsert(rule); err == nil {
			t.Fatalf("invalid rule accepted: %+v", rule)
		}
	}
}

func TestRollingRecordedUsageSaturatesWithoutWrapping(t *testing.T) {
	m := New()
	if err := m.Upsert(Rule{ID: "rolling", NodeID: "node", Scope: "per_node", LimitBytes: math.MaxInt64, Period: PeriodRolling, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	m.Record("node", "user", math.MaxInt64)
	m.Record("node", "user", 1)
	checks := m.Check("node", "user", 1)
	if len(checks) != 1 || checks[0].Bytes != math.MaxInt64 || checks[0].Allowed || checks[0].Remaining != 0 {
		t.Fatalf("recorded usage wrapped: %+v", checks)
	}
}

func TestCalendarQuotaTimezones(t *testing.T) {
	now := time.Date(2026, time.March, 8, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		zone       string
		start, end time.Time
	}{
		{"Europe/Moscow", time.Date(2026, time.March, 7, 21, 0, 0, 0, time.UTC), time.Date(2026, time.March, 8, 21, 0, 0, 0, time.UTC)},
		// The DST transition makes this local calendar day 23 hours long.
		{"America/New_York", time.Date(2026, time.March, 8, 5, 0, 0, 0, time.UTC), time.Date(2026, time.March, 9, 4, 0, 0, 0, time.UTC)},
	} {
		t.Run(tc.zone, func(t *testing.T) {
			m := New()
			m.SetClock(func() time.Time { return now })
			if err := m.Upsert(Rule{ID: "local-day", NodeID: "node", Scope: "per_node", LimitBytes: 100, Period: PeriodDay, Timezone: tc.zone, Enabled: true}); err != nil {
				t.Fatal(err)
			}
			checks := m.Check("node", "user", 0)
			if len(checks) != 1 || !checks[0].PeriodStart.Equal(tc.start) || !checks[0].PeriodEnd.Equal(tc.end) {
				t.Fatalf("incorrect local day: %+v; want %s to %s", checks, tc.start, tc.end)
			}
		})
	}
}
