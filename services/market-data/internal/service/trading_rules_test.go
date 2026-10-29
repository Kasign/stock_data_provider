package service

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestResolvePriceLimitRuleMainboardRiskWarningChangeDate(t *testing.T) {
	before, err := ResolvePriceLimitRule("000001.SZ", "2026-07-05",
		model.RiskWarningApplied, 2000, true, model.PriceLimitExceptionNone)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != model.PriceLimitStatusLimited || before.LimitPercent == nil || *before.LimitPercent != 5 {
		t.Fatalf("expected 5%% limit before effective date, got %+v", before)
	}
	if before.RuleEffectiveTo != "2026-07-05" {
		t.Fatalf("expected the previous rule to end on 2026-07-05, got %+v", before)
	}

	after, err := ResolvePriceLimitRule("000001.SZ", "2026-07-06",
		model.RiskWarningApplied, 2000, true, model.PriceLimitExceptionNone)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != model.PriceLimitStatusLimited || after.LimitPercent == nil || *after.LimitPercent != 10 {
		t.Fatalf("expected 10%% limit from 2026-07-06, got %+v", after)
	}
	if after.RuleEffectiveFrom != "2026-07-06" {
		t.Fatalf("effective date missing from current risk-warning rule: %+v", after)
	}
}

func TestResolvePriceLimitRuleBoardRates(t *testing.T) {
	cases := []struct {
		name, symbol, date string
		risk               model.RiskWarningStatus
		want               float64
	}{
		{name: "mainboard normal before cutover", symbol: "600519.SH", date: "2026-07-05", risk: model.RiskWarningNormal, want: 10},
		{name: "ChiNext before reform", symbol: "300750.SZ", date: "2020-08-21", risk: model.RiskWarningUnknown, want: 10},
		{name: "ChiNext after reform", symbol: "300750.SZ", date: "2020-08-24", risk: model.RiskWarningUnknown, want: 20},
		{name: "STAR", symbol: "688981.SH", date: "2026-07-06", risk: model.RiskWarningUnknown, want: 20},
		{name: "Beijing Exchange", symbol: "430047.BJ", date: "2026-07-06", risk: model.RiskWarningUnknown, want: 30},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolvePriceLimitRule(tc.symbol, tc.date, tc.risk, 2000, true, model.PriceLimitExceptionNone)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != model.PriceLimitStatusLimited || got.LimitPercent == nil || *got.LimitPercent != tc.want {
				t.Fatalf("got %+v; want limited at %.0f%%", got, tc.want)
			}
		})
	}
}

func TestResolvePriceLimitRuleIPOAndSpecialNoLimitEvents(t *testing.T) {
	ipo, err := ResolvePriceLimitRule("600519.SH", "2026-07-06",
		model.RiskWarningNormal, 3, true, model.PriceLimitExceptionNone)
	if err != nil {
		t.Fatal(err)
	}
	if ipo.Status != model.PriceLimitStatusNoLimit || ipo.LimitPercent != nil ||
		ipo.NoLimitReason != "ipo_listing_day_without_price_limit" {
		t.Fatalf("expected first-five-day IPO exemption, got %+v", ipo)
	}

	relisting, err := ResolvePriceLimitRule("600519.SH", "2026-07-06",
		model.RiskWarningUnknown, 0, false, model.PriceLimitExceptionRelistingFirst)
	if err != nil {
		t.Fatal(err)
	}
	if relisting.Status != model.PriceLimitStatusNoLimit || relisting.LimitPercent != nil {
		t.Fatalf("explicit relisting-first-day event must override ordinary percentage, got %+v", relisting)
	}
	if relisting.QualityStatus != model.QualityValid {
		t.Fatalf("explicit event should resolve the rule result, got %+v", relisting)
	}
}

func TestResolvePriceLimitRuleReturnsUnknownForUnverifiedInputs(t *testing.T) {
	cases := []struct {
		name      string
		risk      model.RiskWarningStatus
		day       int
		known     bool
		exception model.PriceLimitException
		date      string
	}{
		{name: "unknown special event", risk: model.RiskWarningNormal, day: 2000, known: true, exception: model.PriceLimitExceptionUnknown, date: "2026-07-06"},
		{name: "unknown historical risk status", risk: model.RiskWarningUnknown, day: 2000, known: true, exception: model.PriceLimitExceptionNone, date: "2026-07-05"},
		{name: "unknown listing day", risk: model.RiskWarningNormal, day: 0, known: false, exception: model.PriceLimitExceptionNone, date: "2026-07-06"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolvePriceLimitRule("600519.SH", tc.date, tc.risk, tc.day, tc.known, tc.exception)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != model.PriceLimitStatusUnknown || got.QualityStatus != model.QualityUnknown || got.LimitPercent != nil {
				t.Fatalf("unverified inputs must not produce a usable percentage, got %+v", got)
			}
		})
	}
}

func TestResolvePriceLimitRuleRejectsContradictoryAndInvalidInputs(t *testing.T) {
	cases := []struct {
		name      string
		symbol    string
		date      string
		risk      model.RiskWarningStatus
		day       int
		known     bool
		exception model.PriceLimitException
	}{
		{name: "known listing day must be positive", symbol: "600519.SH", date: "2026-07-06", risk: model.RiskWarningNormal, day: 0, known: true, exception: model.PriceLimitExceptionNone},
		{name: "negative listing day rejected", symbol: "600519.SH", date: "2026-07-06", risk: model.RiskWarningNormal, day: -1, known: true, exception: model.PriceLimitExceptionNone},
		{name: "unsupported risk status rejected", symbol: "600519.SH", date: "2026-07-06", risk: "suspended", day: 100, known: true, exception: model.PriceLimitExceptionNone},
		{name: "unsupported exception rejected", symbol: "600519.SH", date: "2026-07-06", risk: model.RiskWarningNormal, day: 100, known: true, exception: "maybe"},
		{name: "invalid date rejected", symbol: "600519.SH", date: "2026-02-30", risk: model.RiskWarningNormal, day: 100, known: true, exception: model.PriceLimitExceptionNone},
		{name: "unsupported board rejected", symbol: "900001.SH", date: "2026-07-06", risk: model.RiskWarningNormal, day: 100, known: true, exception: model.PriceLimitExceptionNone},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got, err := ResolvePriceLimitRule(tc.symbol, tc.date, tc.risk, tc.day, tc.known, tc.exception); err == nil || got != nil {
				t.Fatalf("expected invalid input to return an error and no snapshot, got snapshot=%+v err=%v", got, err)
			}
		})
	}
}

func TestResolvePriceLimitRuleWithEvidenceRequiresSourceBackedEvents(t *testing.T) {
	base := model.PriceLimitEventEvidence{
		Symbol: "600519.SH", Status: model.PriceLimitExceptionRelistingFirst,
		EffectiveFrom: "2026-07-06", EffectiveTo: "2026-07-06",
		Source: "exchange notice", SourceURL: "https://example.com/notice/123",
		AnnouncementID: "notice-123", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	cases := []struct {
		name        string
		evidence    *model.PriceLimitEventEvidence
		date        string
		wantStatus  model.PriceLimitStatus
		wantQuality model.QualityStatus
	}{
		{name: "complete event", evidence: &base, date: "2026-07-06", wantStatus: model.PriceLimitStatusNoLimit, wantQuality: model.QualityValid},
		{name: "confirmed no event with date coverage", evidence: func() *model.PriceLimitEventEvidence {
			v := base
			v.Status = model.PriceLimitExceptionNone
			v.EffectiveFrom = "2026-07-01"
			v.EffectiveTo = "2026-07-31"
			v.CoverageConfirmed = true
			return &v
		}(), date: "2026-07-06", wantStatus: model.PriceLimitStatusLimited, wantQuality: model.QualityValid},
		{name: "confirmed no event outside coverage", evidence: func() *model.PriceLimitEventEvidence {
			v := base
			v.Status = model.PriceLimitExceptionNone
			v.EffectiveFrom = "2026-07-07"
			v.EffectiveTo = "2026-07-31"
			v.CoverageConfirmed = true
			return &v
		}(), date: "2026-07-06", wantStatus: model.PriceLimitStatusUnknown, wantQuality: model.QualityUnknown},
		{name: "event outside effective date", evidence: &base, date: "2026-07-07", wantStatus: model.PriceLimitStatusUnknown, wantQuality: model.QualityUnknown},
		{name: "missing evidence", evidence: nil, date: "2026-07-06", wantStatus: model.PriceLimitStatusUnknown, wantQuality: model.QualityUnknown},
		{name: "future evidence observation time", evidence: func() *model.PriceLimitEventEvidence {
			v := base
			v.ObservedAt = time.Now().UTC().Add(24 * time.Hour)
			return &v
		}(), date: "2026-07-06", wantStatus: model.PriceLimitStatusUnknown, wantQuality: model.QualityUnknown},
		{name: "missing announcement", evidence: func() *model.PriceLimitEventEvidence { v := base; v.AnnouncementID = ""; return &v }(), date: "2026-07-06", wantStatus: model.PriceLimitStatusUnknown, wantQuality: model.QualityUnknown},
		{name: "unconfirmed no event", evidence: func() *model.PriceLimitEventEvidence {
			v := base
			v.Status = model.PriceLimitExceptionNone
			v.CoverageConfirmed = false
			return &v
		}(), date: "2026-07-06", wantStatus: model.PriceLimitStatusUnknown, wantQuality: model.QualityUnknown},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ResolvePriceLimitRuleWithEvidence("600519.SH", tc.date,
				model.RiskWarningNormal, 100, true, tc.evidence)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != tc.wantStatus || got.QualityStatus != tc.wantQuality {
				t.Fatalf("got status=%s quality=%s reasons=%v; want status=%s quality=%s",
					got.Status, got.QualityStatus, got.QualityReasons, tc.wantStatus, tc.wantQuality)
			}
		})
	}
}

func TestResolvePriceLimitRuleWithEvidenceRejectsMismatchedSymbol(t *testing.T) {
	evidence := &model.PriceLimitEventEvidence{
		Symbol: "000001.SZ", Status: model.PriceLimitExceptionRelistingFirst,
		EffectiveFrom: "2026-07-06", EffectiveTo: "2026-07-06",
		Source: "exchange notice", SourceURL: "https://example.com/notice/123",
		AnnouncementID: "notice-123", ObservedAt: time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
	}
	if got, err := ResolvePriceLimitRuleWithEvidence("600519.SH", "2026-07-06",
		model.RiskWarningNormal, 100, true, evidence); err == nil || got != nil {
		t.Fatalf("expected mismatched event identity to be rejected, got snapshot=%+v err=%v", got, err)
	}
}

func TestResolvePriceLimitRuleFromTradingStatusRequiresMatchingVerifiedTradingDate(t *testing.T) {
	base := model.SecurityTradingStatusSnapshot{
		Symbol: "600519.SH", TradeDate: "2026-07-06",
		RiskWarningStatus:       model.RiskWarningNormal,
		DelistingStatus:         model.DelistingStatusNormal,
		ListingTradingDayNumber: 100, ListingStatusKnown: true,
		IsTradingDay: true, TradingDayKnown: true,
		Source: "fixture-provider", SourceVersion: "fixture-v1",
		FetchedAt:     time.Now().UTC(),
		QualityStatus: model.QualityValid,
	}
	evidence := &model.PriceLimitEventEvidence{
		Symbol: "600519.SH", Status: model.PriceLimitExceptionNone,
		EffectiveFrom: "2026-07-01", EffectiveTo: "2026-07-31",
		Source: "exchange calendar", SourceURL: "https://example.com/calendar",
		ObservedAt:        time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC),
		CoverageConfirmed: true,
	}

	t.Run("verified trading day uses status and event evidence", func(t *testing.T) {
		got, err := ResolvePriceLimitRuleFromTradingStatus("600519.SH", "2026-07-06", &base, evidence)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != model.PriceLimitStatusLimited || got.LimitPercent == nil || *got.LimitPercent != 10 {
			t.Fatalf("expected verified 10%% rule, got %+v", got)
		}
	})

	t.Run("unknown trading calendar cannot use event evidence to claim a rule", func(t *testing.T) {
		status := base
		status.TradingDayKnown = false
		got, err := ResolvePriceLimitRuleFromTradingStatus("600519.SH", "2026-07-06", &status, evidence)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != model.PriceLimitStatusUnknown || got.LimitPercent != nil || got.QualityStatus != model.QualityUnknown {
			t.Fatalf("unknown calendar must keep result unknown, got %+v", got)
		}
	})

	t.Run("known non-trading day is not assigned a price-limit rule", func(t *testing.T) {
		status := base
		status.IsTradingDay = false
		got, err := ResolvePriceLimitRuleFromTradingStatus("600519.SH", "2026-07-06", &status, evidence)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != model.PriceLimitStatusUnknown || got.LimitPercent != nil {
			t.Fatalf("non-trading date must not produce a price-limit result, got %+v", got)
		}
	})

	t.Run("missing source provenance cannot produce a rule", func(t *testing.T) {
		status := base
		status.Source = ""
		got, err := ResolvePriceLimitRuleFromTradingStatus("600519.SH", "2026-07-06", &status, evidence)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != model.PriceLimitStatusUnknown || got.LimitPercent != nil ||
			!hasRuleReason(got.QualityReasons, "security_trading_status_source_provenance_missing_or_invalid") {
			t.Fatalf("missing source provenance must fail closed, got %+v", got)
		}
	})

	t.Run("future source fetch time cannot produce a rule", func(t *testing.T) {
		status := base
		status.FetchedAt = time.Now().UTC().Add(24 * time.Hour)
		got, err := ResolvePriceLimitRuleFromTradingStatus("600519.SH", "2026-07-06", &status, evidence)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != model.PriceLimitStatusUnknown || got.LimitPercent != nil ||
			!hasRuleReason(got.QualityReasons, "security_trading_status_source_provenance_missing_or_invalid") {
			t.Fatalf("future source fetch time must fail closed, got %+v", got)
		}
	})

	t.Run("mismatched status date is rejected", func(t *testing.T) {
		status := base
		status.TradeDate = "2026-07-07"
		if got, err := ResolvePriceLimitRuleFromTradingStatus("600519.SH", "2026-07-06", &status, evidence); err == nil || got != nil {
			t.Fatalf("expected date mismatch to fail, got snapshot=%+v err=%v", got, err)
		}
	})
}

func hasRuleReason(reasons []string, expected string) bool {
	for _, reason := range reasons {
		if reason == expected {
			return true
		}
	}
	return false
}
