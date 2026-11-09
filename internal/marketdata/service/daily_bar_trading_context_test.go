package service

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func dailyBarContextStatus(date, listingDate string, listingKnown bool, suspension model.SuspensionStatus) *model.SecurityTradingStatusSnapshot {
	return &model.SecurityTradingStatusSnapshot{
		TradeDate: date, ListingDate: listingDate, ListingStatusKnown: listingKnown,
		TradingDayKnown: true, IsTradingDay: true, SuspensionStatus: suspension,
	}
}

func hasDailyBarContextReason(reasons []string, expected string) bool {
	for _, reason := range reasons {
		if reason == expected {
			return true
		}
	}
	return false
}

func TestAssessDailyBarTradingContextDistinguishesMissingSuspendedAndPreListing(t *testing.T) {
	calendar := &model.TradingCalendarSnapshot{Items: []model.TradingCalendarDay{
		{Date: "2026-10-20", IsTradingDay: true},
		{Date: "2026-10-21", IsTradingDay: true},
		{Date: "2026-10-22", IsTradingDay: false},
		{Date: "2026-10-23", IsTradingDay: true},
	}}
	statuses := map[string]*model.SecurityTradingStatusSnapshot{
		"2026-10-20": dailyBarContextStatus("2026-10-20", "2026-10-21", true, model.SuspensionStatusNoEvent),
		"2026-10-21": dailyBarContextStatus("2026-10-21", "2026-10-21", true, model.SuspensionStatusSuspended),
		"2026-10-23": dailyBarContextStatus("2026-10-23", "2026-10-21", true, model.SuspensionStatusNoEvent),
	}
	result := assessDailyBarTradingContext(calendar, nil, statuses, nil)
	if result.QualityStatus != model.QualityDegraded {
		t.Fatalf("confirmed missing bar should degrade quality, got %s", result.QualityStatus)
	}
	if !hasDailyBarContextReason(result.Reasons, "daily_bar_excluded_before_listing:2026-10-20") ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_excluded_suspended:2026-10-21") ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_missing_on_confirmed_trading_day:2026-10-23") {
		t.Fatalf("expected distinct pre-listing, suspension and missing-bar reasons, got %v", result.Reasons)
	}
}

func TestAssessDailyBarTradingContextKeepsUnknownStateNotComparable(t *testing.T) {
	calendar := &model.TradingCalendarSnapshot{Items: []model.TradingCalendarDay{
		{Date: "2026-10-20", IsTradingDay: true},
		{Date: "2026-10-21", IsTradingDay: true},
	}}
	result := assessDailyBarTradingContext(calendar, nil,
		map[string]*model.SecurityTradingStatusSnapshot{
			"2026-10-20": dailyBarContextStatus("2026-10-20", "2026-10-01", true, model.SuspensionStatusUnknown),
		},
		map[string]error{"2026-10-21": errors.New("provider timeout")})
	if result.QualityStatus != model.QualityDegraded ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_context_suspension_status_unknown:2026-10-20") ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_context_security_status_unavailable:2026-10-21") {
		t.Fatalf("unknown states must remain not-comparable, got %+v", result)
	}
}

func TestAssessDailyBarTradingContextAcceptsNonTradingDayAndReturnedBar(t *testing.T) {
	calendar := &model.TradingCalendarSnapshot{QualityStatus: model.QualityValid, Items: []model.TradingCalendarDay{
		{Date: "2026-10-20", IsTradingDay: true},
		{Date: "2026-10-21", IsTradingDay: false},
	}}
	result := assessDailyBarTradingContext(calendar, []model.DailyBar{{TradeDate: "2026-10-20"}}, nil, nil)
	if result.QualityStatus != model.QualityValid || len(result.Reasons) != 0 {
		t.Fatalf("present bar and explicit non-trading day should pass context check, got %+v", result)
	}
}

func TestAssessDailyBarTradingContextFlagsBarOnNonTradingDate(t *testing.T) {
	calendar := &model.TradingCalendarSnapshot{Items: []model.TradingCalendarDay{
		{Date: "2026-10-20", IsTradingDay: false},
	}}
	result := assessDailyBarTradingContext(calendar, []model.DailyBar{{TradeDate: "2026-10-20"}}, nil, nil)
	if result.QualityStatus != model.QualityDegraded ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_returned_on_non_trading_date:2026-10-20") {
		t.Fatalf("bar on non-trading date must be reported, got %+v", result)
	}
}

func TestAssessDailyBarTradingContextDoesNotExcludeMissingBarDuringIntradaySuspension(t *testing.T) {
	calendar := &model.TradingCalendarSnapshot{Items: []model.TradingCalendarDay{
		{Date: "2026-10-20", IsTradingDay: true},
	}}
	statuses := map[string]*model.SecurityTradingStatusSnapshot{
		"2026-10-20": dailyBarContextStatus("2026-10-20", "2026-10-01", true, model.SuspensionStatusIntraday),
	}
	result := assessDailyBarTradingContext(calendar, nil, statuses, nil)
	if result.QualityStatus != model.QualityDegraded ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_missing_during_intraday_suspension:2026-10-20") ||
		hasDailyBarContextReason(result.Reasons, "daily_bar_excluded_suspended:2026-10-20") {
		t.Fatalf("intraday suspension must not mask a missing full-day bar: %+v", result)
	}
}

func TestAssessDailyBarTradingContextMissingCalendarIsUnknown(t *testing.T) {
	result := assessDailyBarTradingContext(nil, nil, nil, nil)
	if result.QualityStatus != model.QualityDegraded ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_unavailable") {
		t.Fatalf("missing calendar must not be interpreted as a clean result, got %+v", result)
	}
}

func TestAssessDailyBarTradingContextDoesNotUseConflictingCalendarToConfirmMissingBars(t *testing.T) {
	calendar := &model.TradingCalendarSnapshot{
		QualityReasons: []string{"provider_trading_calendar_sources_disagree"},
		Items:          []model.TradingCalendarDay{{Date: "2026-10-20", IsTradingDay: true}},
	}
	statuses := map[string]*model.SecurityTradingStatusSnapshot{
		"2026-10-20": dailyBarContextStatus("2026-10-20", "2026-10-01", true, model.SuspensionStatusNoEvent),
	}
	result := assessDailyBarTradingContext(calendar, nil, statuses, nil)
	if result.QualityStatus != model.QualityDegraded ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_sources_disagree") ||
		hasDailyBarContextReason(result.Reasons, "daily_bar_missing_on_confirmed_trading_day:2026-10-20") {
		t.Fatalf("conflicting calendar must not confirm provider missing data: %+v", result)
	}
}

func TestAssessDailyBarTradingContextHistoricalCalendarTimestampGapIsWarningOnly(t *testing.T) {
	calendar := &model.TradingCalendarSnapshot{
		StartDate: "2025-07-07", EndDate: "2025-07-07",
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		Items:          []model.TradingCalendarDay{{Date: "2025-07-07", IsTradingDay: true}},
	}
	result := assessDailyBarTradingContext(calendar, []model.DailyBar{{TradeDate: "2025-07-07"}}, nil, nil)
	if result.QualityStatus != model.QualityValid ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_snapshot_time_unverified") ||
		hasDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_degraded") {
		t.Fatalf("historical date coverage must remain usable while exposing missing source timestamp: %+v", result)
	}
}

func TestAssessDailyBarTradingContextCurrentCalendarTimestampGapDegrades(t *testing.T) {
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	calendar := &model.TradingCalendarSnapshot{
		StartDate: today, EndDate: today,
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		Items:          []model.TradingCalendarDay{{Date: today, IsTradingDay: true}},
	}
	result := assessDailyBarTradingContext(calendar, []model.DailyBar{{TradeDate: today}}, nil, nil)
	if result.QualityStatus != model.QualityDegraded ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_degraded") {
		t.Fatalf("current calendar without source timestamp must remain degraded: %+v", result)
	}
}

func TestAssessDailyBarTradingContextHistoricalCalendarInferenceStillDegrades(t *testing.T) {
	calendar := &model.TradingCalendarSnapshot{
		StartDate: "2025-07-07", EndDate: "2025-07-07",
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_returns_trading_days_only_non_trading_dates_inferred"},
		Items:          []model.TradingCalendarDay{{Date: "2025-07-07", IsTradingDay: true}},
	}
	result := assessDailyBarTradingContext(calendar, []model.DailyBar{{TradeDate: "2025-07-07"}}, nil, nil)
	if result.QualityStatus != model.QualityDegraded ||
		!hasDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_degraded") {
		t.Fatalf("inferred historical calendar dates must remain degraded: %+v", result)
	}
}
