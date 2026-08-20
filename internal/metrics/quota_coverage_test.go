package metrics

import (
	"strconv"
	"testing"
	"time"

	"github.com/onllm-dev/onwatch/v2/internal/api"
	"github.com/onllm-dev/onwatch/v2/internal/store"
)

// TestScrapeZai_ExportsDeclaredButUnconsumedQuota covers the case in issue #112:
// the plan reports a token percentage while leaving usage at zero, so gating the
// series on consumption hid the only number that actually moved.
func TestScrapeZai_ExportsDeclaredButUnconsumedQuota(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	reset := now.Add(3 * time.Hour)

	// Field shape taken from the report: usage stays at 0 while the percentage
	// climbs, and the limit is Unit*Number rather than a token count.
	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt:          now,
		TokensLimit:         6,
		TokensUsage:         0,
		TokensPercentage:    68,
		TokensNextResetTime: &reset,
		TimeLimit:           5,
		TimeUsage:           4000,
		TimePercentage:      0,
	}); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	tokenLabels := map[string]string{"provider": "zai", "quota_type": "tokens", "account_id": "default"}
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", tokenLabels, 68)
	// Without this, burn-rate prediction on the token quota is impossible.
	assertGaugeValue(t, families, "onwatch_quota_reset_timestamp_seconds", tokenLabels, float64(reset.Unix()))

	// The time quota is declared too, so it stays exported at its real value.
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent",
		map[string]string{"provider": "zai", "quota_type": "time", "account_id": "default"}, 0)
}

// TestScrapeZai_OmitsQuotaThePlanDoesNotHave verifies the gate still suppresses
// a quota the provider says nothing about, rather than exporting a flat zero
// that would read as a permanently healthy quota.
func TestScrapeZai_OmitsQuotaThePlanDoesNotHave(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	if _, err := s.InsertZaiSnapshot(&api.ZaiSnapshot{
		CapturedAt: time.Now().UTC(),
		// Only a time quota is declared; every tokens axis is zero.
		TimeLimit:      5,
		TimeUsage:      120,
		TimePercentage: 40,
	}); err != nil {
		t.Fatalf("InsertZaiSnapshot: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	if hasGaugeMetric(families, "onwatch_quota_utilization_percent",
		map[string]string{"provider": "zai", "quota_type": "tokens", "account_id": "default"}) {
		t.Error("tokens series exported for a plan that declares no token quota")
	}
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent",
		map[string]string{"provider": "zai", "quota_type": "time", "account_id": "default"}, 40)
}

// TestScrapeMiniMax_ExportsWeeklyQuota verifies the weekly window is exported.
// The store already reads it, but it was never scraped, so the window that
// burns over days could not be alerted on.
func TestScrapeMiniMax_ExportsWeeklyQuota(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	acct, err := s.GetOrCreateProviderAccount("minimax", "primary")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}

	now := time.Now().UTC()
	fiveHourReset := now.Add(2 * time.Hour)
	weeklyReset := now.Add(96 * time.Hour)

	if _, err := s.InsertMiniMaxSnapshot(&api.MiniMaxSnapshot{
		CapturedAt: now,
		Models: []api.MiniMaxModelQuota{{
			ModelName:         "MiniMax-M2",
			Total:             100,
			Remain:            70,
			Used:              30,
			UsedPercent:       30,
			ResetAt:           &fiveHourReset,
			HasWeeklyQuota:    true,
			WeeklyTotal:       1000,
			WeeklyRemain:      200,
			WeeklyUsed:        800,
			WeeklyUsedPercent: 80,
			WeeklyResetAt:     &weeklyReset,
		}},
	}, acct.ID); err != nil {
		t.Fatalf("InsertMiniMaxSnapshot: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	accountID := strconv.FormatInt(acct.ID, 10)

	// The existing per-model window must keep its label untouched.
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent",
		map[string]string{"provider": "minimax", "quota_type": "MiniMax-M2", "account_id": accountID}, 30)

	weeklyLabels := map[string]string{"provider": "minimax", "quota_type": "weekly_MiniMax-M2", "account_id": accountID}
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent", weeklyLabels, 80)
	assertGaugeValue(t, families, "onwatch_quota_reset_timestamp_seconds", weeklyLabels, float64(weeklyReset.Unix()))
}

// TestScrapeMiniMax_OmitsWeeklyWhenAccountHasNone verifies accounts without a
// weekly window emit no weekly series. A flat zero would read as a permanently
// healthy account.
func TestScrapeMiniMax_OmitsWeeklyWhenAccountHasNone(t *testing.T) {
	s, err := store.New(":memory:")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	defer s.Close()

	acct, err := s.GetOrCreateProviderAccount("minimax", "legacy")
	if err != nil {
		t.Fatalf("GetOrCreateProviderAccount: %v", err)
	}

	now := time.Now().UTC()
	if _, err := s.InsertMiniMaxSnapshot(&api.MiniMaxSnapshot{
		CapturedAt: now,
		Models: []api.MiniMaxModelQuota{{
			ModelName:      "MiniMax-M2",
			Total:          100,
			Remain:         55,
			Used:           45,
			UsedPercent:    45,
			HasWeeklyQuota: false,
		}},
	}, acct.ID); err != nil {
		t.Fatalf("InsertMiniMaxSnapshot: %v", err)
	}

	m := New()
	m.Scrape(s, time.Minute)
	families, err := m.Gather().Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}

	accountID := strconv.FormatInt(acct.ID, 10)
	assertGaugeValue(t, families, "onwatch_quota_utilization_percent",
		map[string]string{"provider": "minimax", "quota_type": "MiniMax-M2", "account_id": accountID}, 45)

	if hasGaugeMetric(families, "onwatch_quota_utilization_percent",
		map[string]string{"provider": "minimax", "quota_type": "weekly_MiniMax-M2", "account_id": accountID}) {
		t.Error("weekly series exported for an account with no weekly quota")
	}
}

// TestZaiQuotaDeclared documents which signals count as "the provider said
// something about this quota".
func TestZaiQuotaDeclared(t *testing.T) {
	tests := []struct {
		name       string
		limit      int
		usage      float64
		current    float64
		remaining  float64
		percentage int
		want       bool
	}{
		{name: "nothing declared", want: false},
		{name: "percentage only (issue #112)", percentage: 68, want: true},
		{name: "limit only", limit: 6, want: true},
		{name: "usage only", usage: 4000, want: true},
		{name: "current value only", current: 12, want: true},
		{name: "remaining only", remaining: 40, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := zaiQuotaDeclared(tt.limit, tt.usage, tt.current, tt.remaining, tt.percentage)
			if got != tt.want {
				t.Errorf("zaiQuotaDeclared() = %v, want %v", got, tt.want)
			}
		})
	}
}
