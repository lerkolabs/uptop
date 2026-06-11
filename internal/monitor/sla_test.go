package monitor

import (
	"math"
	"testing"
	"time"

	"gitea.lerkolabs.com/lerkolabs/uptop/internal/models"
)

func TestComputeSLA_NoChanges_CurrentlyUp(t *testing.T) {
	r := ComputeSLA(nil, "UP", 24*time.Hour)
	if r.UptimePct != 100 {
		t.Errorf("expected 100%% uptime, got %.2f%%", r.UptimePct)
	}
	if r.Downtime != 0 {
		t.Errorf("expected 0 downtime, got %v", r.Downtime)
	}
}

func TestComputeSLA_NoChanges_CurrentlyDown(t *testing.T) {
	r := ComputeSLA(nil, "DOWN", 24*time.Hour)
	if r.UptimePct != 0 {
		t.Errorf("expected 0%% uptime, got %.2f%%", r.UptimePct)
	}
}

func TestComputeSLA_SingleOutage(t *testing.T) {
	now := time.Now()
	// DOWN 2 hours ago, recovered 1 hour ago → 1 hour downtime in 24h window
	changes := []models.StateChange{
		{ToStatus: "UP", ChangedAt: now.Add(-1 * time.Hour)},
		{ToStatus: "DOWN", FromStatus: "UP", ChangedAt: now.Add(-2 * time.Hour)},
	}

	r := ComputeSLA(changes, "UP", 24*time.Hour)

	if r.OutageCount != 1 {
		t.Errorf("expected 1 outage, got %d", r.OutageCount)
	}

	expectedDowntime := 1 * time.Hour
	if absDuration(r.Downtime-expectedDowntime) > time.Minute {
		t.Errorf("expected ~1h downtime, got %v", r.Downtime)
	}

	expectedPct := float64(23) / float64(24) * 100
	if math.Abs(r.UptimePct-expectedPct) > 0.5 {
		t.Errorf("expected ~%.1f%% uptime, got %.2f%%", expectedPct, r.UptimePct)
	}

	if r.LongestOut < 55*time.Minute || r.LongestOut > 65*time.Minute {
		t.Errorf("expected longest outage ~1h, got %v", r.LongestOut)
	}
}

func TestComputeSLA_CurrentlyDown(t *testing.T) {
	now := time.Now()
	// Went down 3 hours ago, still down
	changes := []models.StateChange{
		{ToStatus: "DOWN", FromStatus: "UP", ChangedAt: now.Add(-3 * time.Hour)},
	}

	r := ComputeSLA(changes, "DOWN", 24*time.Hour)

	if r.OutageCount != 1 {
		t.Errorf("expected 1 outage, got %d", r.OutageCount)
	}

	expectedDowntime := 3 * time.Hour
	if absDuration(r.Downtime-expectedDowntime) > time.Minute {
		t.Errorf("expected ~3h downtime, got %v", r.Downtime)
	}
}

func TestComputeSLA_MultipleOutages(t *testing.T) {
	now := time.Now()
	// Two outages: 6h-5h ago and 2h-1h ago
	changes := []models.StateChange{
		{ToStatus: "UP", ChangedAt: now.Add(-1 * time.Hour)},
		{ToStatus: "DOWN", FromStatus: "UP", ChangedAt: now.Add(-2 * time.Hour)},
		{ToStatus: "UP", ChangedAt: now.Add(-5 * time.Hour)},
		{ToStatus: "DOWN", FromStatus: "UP", ChangedAt: now.Add(-6 * time.Hour)},
	}

	r := ComputeSLA(changes, "UP", 24*time.Hour)

	if r.OutageCount != 2 {
		t.Errorf("expected 2 outages, got %d", r.OutageCount)
	}

	expectedDowntime := 2 * time.Hour
	if absDuration(r.Downtime-expectedDowntime) > time.Minute {
		t.Errorf("expected ~2h downtime, got %v", r.Downtime)
	}

	if r.MTTR < 55*time.Minute || r.MTTR > 65*time.Minute {
		t.Errorf("expected MTTR ~1h, got %v", r.MTTR)
	}
}

func TestComputeSLA_LateNotDown(t *testing.T) {
	now := time.Now()
	// LATE for 2 hours is not downtime
	changes := []models.StateChange{
		{ToStatus: "UP", ChangedAt: now.Add(-1 * time.Hour)},
		{ToStatus: "LATE", FromStatus: "UP", ChangedAt: now.Add(-3 * time.Hour)},
	}

	r := ComputeSLA(changes, "UP", 24*time.Hour)

	if r.OutageCount != 0 {
		t.Errorf("expected 0 outages for LATE, got %d", r.OutageCount)
	}
	if r.UptimePct != 100 {
		t.Errorf("expected 100%% uptime (LATE is not down), got %.2f%%", r.UptimePct)
	}
}

func TestComputeDailyBreakdown(t *testing.T) {
	// Use a fixed time well past midnight so the outage always falls within today's window.
	now := time.Date(2026, 6, 4, 15, 0, 0, 0, time.UTC)
	changes := []models.StateChange{
		{ToStatus: "UP", ChangedAt: now.Add(-1 * time.Hour)},
		{ToStatus: "DOWN", FromStatus: "UP", ChangedAt: now.Add(-2 * time.Hour)},
	}

	days := ComputeDailyBreakdown(changes, "UP", 7, now)

	if len(days) != 7 {
		t.Fatalf("expected 7 days, got %d", len(days))
	}

	// Today should have less than 100% uptime
	if days[0].UptimePct >= 100 {
		t.Errorf("expected today < 100%%, got %.2f%%", days[0].UptimePct)
	}
}

func TestIsBroken(t *testing.T) {
	if !models.StatusDown.IsBroken() {
		t.Error("DOWN should be broken")
	}
	if !models.StatusSSLExp.IsBroken() {
		t.Error("SSL EXP should be broken")
	}
	if models.StatusUp.IsBroken() {
		t.Error("UP should not be broken")
	}
	if models.StatusLate.IsBroken() {
		t.Error("LATE should not be broken")
	}
	if models.StatusStale.IsBroken() {
		t.Error("STALE should not be broken")
	}
	if models.StatusPending.IsBroken() {
		t.Error("PENDING should not be broken")
	}
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
