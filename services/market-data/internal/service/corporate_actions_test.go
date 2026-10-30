package service

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestValidServiceDateRange(t *testing.T) {
	cases := []struct {
		start, end string
		want bool
	}{
		{"2026-01-01", "2026-10-30", true},
		{"2026-10-30", "2026-01-01", false},
		{"20260101", "2026-10-30", false},
		{"", "2026-10-30", true},
		{"2026-01-01", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		if got := validServiceDateRange(tc.start, tc.end); got != tc.want {
			t.Errorf("validServiceDateRange(%q, %q) = %t, want %t", tc.start, tc.end, got, tc.want)
		}
	}
}


func TestAdjustmentFactorPITVisibilityRequiresAllThreeTimes(t *testing.T) {
	asOf := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	before, after := asOf.Add(-time.Minute), asOf.Add(time.Minute)
	base := model.AdjustmentFactor{FetchedAt: before, AvailableAt: &before, SystemObservedAt: &before}
	if !adjustmentFactorVisibleAsOf(base, asOf) { t.Fatal("complete pre-as-of factor should be visible") }
	missingAvailable := base; missingAvailable.AvailableAt = nil
	if adjustmentFactorVisibleAsOf(missingAvailable, asOf) { t.Fatal("factor without available_at must be invisible") }
	observedLater := base; observedLater.SystemObservedAt = &after
	if adjustmentFactorVisibleAsOf(observedLater, asOf) { t.Fatal("factor observed after as_of must be invisible") }
	fetchedLater := base; fetchedLater.FetchedAt = after
	if adjustmentFactorVisibleAsOf(fetchedLater, asOf) { t.Fatal("factor fetched after as_of must be invisible") }
}

func TestCorporateActionPITVisibilityRequiresAllThreeTimes(t *testing.T) {
	asOf := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	before, after := asOf.Add(-time.Minute), asOf.Add(time.Minute)
	base := model.CorporateAction{FetchedAt: before, AvailableAt: &before, SystemObservedAt: &before}
	if !corporateActionVisibleAsOf(base, asOf) { t.Fatal("complete pre-as-of action should be visible") }
	missingAvailable := base; missingAvailable.AvailableAt = nil
	if corporateActionVisibleAsOf(missingAvailable, asOf) { t.Fatal("action without available_at must be invisible") }
	observedLater := base; observedLater.SystemObservedAt = &after
	if corporateActionVisibleAsOf(observedLater, asOf) { t.Fatal("action observed after as_of must be invisible") }
	fetchedLater := base; fetchedLater.FetchedAt = after
	if corporateActionVisibleAsOf(fetchedLater, asOf) { t.Fatal("action fetched after as_of must be invisible") }
}
