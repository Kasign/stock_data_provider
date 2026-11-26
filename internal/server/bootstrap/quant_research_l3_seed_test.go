package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/platform/security"
)

// This file builds the real PIT seed rows consumed by TestQuantResearchMariaDBRedisL3.
// Every row is written through production repository APIs and must satisfy the same
// PIT/exact-value/authorization gates that Data applies to live provider reads. No
// snapshot is forged and no gate is relaxed: the bars are a synthetic but internally
// legal trading-day series used as a test fixture (see doc/quant/14 D1).

const (
	l3SeedSymbol      = "600036.SH"
	l3SeedSource      = "eastmoney"
	l3SeedSourceVer   = "eastmoney-stock-api-v1"
	l3SeedStart       = "2026-09-01"
	l3SeedEnd         = "2026-10-30"
	l3SeedSMAStart    = "2026-10-01"
	l3SeedSMATrading  = 6 // factor limit=6 and Quant minimum_coverage=1 share this calendar
	l3SeedStatusDate  = "2026-11-25"
	l3SeedBarsCount   = 36 // 2026-09-01..09-30 plus 2026-10-25..10-30, matching both calendars
	l3SeedPolicyDaily = "policy-l3-daily-v1"
	l3SeedPolicyFacts = "policy-l3-facts-v1"
)

// l3SeedCloses is 61 two-decimal closes generated deterministically so the series is
// reproducible across runs. Values are realistic magnitudes; exact-value evidence
// binds every close as an observed decimal token (not a float reconstruction).
func l3SeedCloses(count int) []string {
	closes := make([]string, count)
	base := 3800 // 38.00 CNY in cents
	for i := 0; i < count; i++ {
		cents := base + (i%17)*7 - (i%5)*3
		closes[i] = fmt.Sprintf("%d.%02d", cents/100, cents%100)
	}
	return closes
}

// l3SeedTradingDates returns the synthetic exchange trading days shared by the
// trend and sma/factor windows: 2026-09-01..09-30 plus 2026-10-25..10-30.
// The sma/factor calendar marks only the final six of these dates as trading
// days; bars exist only on this same set. This keeps calendar, coverage
// evidence and bars mutually coherent while respecting Quant's 100% coverage
// policy and the factor limit of six. Trend sees 36 bars, so its 61-bar
// threshold is a legitimate degradation with a persisted result_id, not a
// Stage failure. This is a legal synthetic fixture (doc/quant/14 D1).
func l3SeedTradingDates(t *testing.T, count int) []string {
	t.Helper()
	start, err := time.Parse("2006-01-02", l3SeedStart)
	if err != nil {
		t.Fatal(err)
	}
	smaStart, err := time.Parse("2006-01-02", l3SeedSMAStart)
	if err != nil {
		t.Fatal(err)
	}
	last, err := time.Parse("2006-01-02", l3SeedEnd)
	if err != nil {
		t.Fatal(err)
	}
	dates := make([]string, 0, count)
	for day := start; !day.After(last) && len(dates) < count; day = day.AddDate(0, 0, 1) {
		if !day.Before(smaStart) && day.Before(smaStart.AddDate(0, 0, 24)) {
			continue
		}
		dates = append(dates, day.Format("2006-01-02"))
	}
	if len(dates) != count {
		t.Fatalf("seed trading day count = %d, want %d", len(dates), count)
	}
	if parsed, _ := time.Parse("2006-01-02", dates[len(dates)-1]); parsed.After(last) {
		t.Fatalf("seed trading days overshoot %s: last=%s", l3SeedEnd, dates[len(dates)-1])
	}
	return dates
}

func l3SeedFraction(v int) *int { return &v }

func l3SeedCloseEvidence(token string) *model.DailyBarDecimalEvidence {
	two := 2
	return &model.DailyBarDecimalEvidence{
		EvidenceKind:         model.DailyBarPrecisionEvidenceObservedToken,
		SourceToken:          token,
		SourceFractionDigits: l3SeedFraction(two),
	}
}

// seedQuantResearchL3Bars writes PIT bar revisions satisfying the Data gates in
// internal/marketdata/service/data.go:4310-4320 and exact-value evidence required by
// the Close Series Snapshot Creator.
func seedQuantResearchL3Bars(t *testing.T, ctx context.Context, store *repository.Store, observedAt time.Time) {
	t.Helper()
	dates := l3SeedTradingDates(t, l3SeedBarsCount)
	closes := l3SeedCloses(l3SeedBarsCount)
	rows := make([]model.DailyBar, 0, len(dates))
	for i, date := range dates {
		closeToken := closes[i]
		var close float64
		_, _ = fmt.Sscanf(closeToken, "%f", &close)
		fetchedAt := observedAt.Add(-3 * time.Hour)
		normalizedAt := observedAt.Add(-2 * time.Hour)
		sysObserved := observedAt.Add(-time.Hour)
		volume := int64(1000000)
		amount := close * float64(volume)
		rows = append(rows, model.DailyBar{
			Symbol: l3SeedSymbol, TradeDate: date, Period: "1d",
			Open: close - 0.1, High: close + 0.2, Low: close - 0.2, Close: close,
			AdjustmentBasis: model.AdjustmentNone,
			Source:          l3SeedSource, SourceVersion: l3SeedSourceVer,
			Volume: &volume, VolumeUnit: "shares", Amount: &amount, AmountUnit: "CNY",
			FetchedAt: fetchedAt, NormalizedAt: normalizedAt, SystemObservedAt: &sysObserved,
			QualityStatus:    model.QualityValid,
			NormalizationVer: normalize.Version,
			ExactValues: &model.DailyBarExactValues{
				Open: fmt.Sprintf("%.2f", close-0.1), High: fmt.Sprintf("%.2f", close+0.2),
				Low: fmt.Sprintf("%.2f", close-0.2), Close: closeToken,
				RawVolume:     "1000000",
				CloseEvidence: l3SeedCloseEvidence(closeToken),
				OpenEvidence:  l3SeedCloseEvidence(fmt.Sprintf("%.2f", close-0.1)),
				HighEvidence:  l3SeedCloseEvidence(fmt.Sprintf("%.2f", close+0.2)),
				LowEvidence:   l3SeedCloseEvidence(fmt.Sprintf("%.2f", close-0.2)),
			},
		})
	}
	if err := store.UpsertBars(ctx, rows); err != nil {
		t.Fatalf("seed L3 PIT bars: %v", err)
	}
}

// seedQuantResearchL3Calendar writes valid trading calendar snapshots for the
// exact query windows used by quant.research Stages. Calendar snapshots are
// looked up by an exact start/end match, so sma20/factor (2026-10-01..10-30)
// and trend (2026-09-01..10-30) each need their own immutable ref.
func seedQuantResearchL3Calendar(t *testing.T, ctx context.Context, store *repository.Store, observedAt time.Time) {
	t.Helper()
	allDates := l3SeedTradingDates(t, l3SeedBarsCount)
	start, err := time.Parse("2006-01-02", l3SeedStart)
	if err != nil {
		t.Fatal(err)
	}
	smaStart, err := time.Parse("2006-01-02", l3SeedSMAStart)
	if err != nil {
		t.Fatal(err)
	}
	end, err := time.Parse("2006-01-02", l3SeedEnd)
	if err != nil {
		t.Fatal(err)
	}
	trendWindowDates := make([]string, 0, 60)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		trendWindowDates = append(trendWindowDates, day.Format("2006-01-02"))
	}
	smaWindowDates := make([]string, 0, 30)
	for day := smaStart; !day.After(end); day = day.AddDate(0, 0, 1) {
		smaWindowDates = append(smaWindowDates, day.Format("2006-01-02"))
	}
	// Quant requires len(bars)/ExpectedTradingDays >= 1 while sma20/factor share
	// one calendar and factor's limit is 6. Mark only the final six dates as
	// trading days so factor computes a real value and sma20 legitimately
	// degrades to insufficient_data (20-bar window vs 6 available bars).
	smaTrading := map[string]bool{}
	for _, date := range smaWindowDates[len(smaWindowDates)-l3SeedSMATrading:] {
		smaTrading[date] = true
	}
	seedTrading := map[string]bool{}
	for _, date := range allDates {
		seedTrading[date] = true
	}
	windows := []struct {
		dates         []string
		trading       map[string]bool
		sourceVersion string
	}{
		{dates: trendWindowDates, trading: seedTrading, sourceVersion: "l3-seed-calendar-trend-v1"},
		{dates: smaWindowDates, trading: smaTrading, sourceVersion: "l3-seed-calendar-sma-v1"},
	}
	for _, window := range windows {
		items := make([]model.TradingCalendarDay, 0, len(window.dates))
		tradingDays := 0
		for _, date := range window.dates {
			isTrading := window.trading == nil || window.trading[date]
			items = append(items, model.TradingCalendarDay{Date: date, IsTradingDay: isTrading})
			if isTrading {
				tradingDays++
			}
		}
		snapshot := &model.TradingCalendarSnapshot{
			Market: "cn_a_share", StartDate: window.dates[0], EndDate: window.dates[len(window.dates)-1],
			TotalDays: len(items), TradingDays: tradingDays, Items: items,
			Source: "l3-seed-calendar", SourceVersion: window.sourceVersion,
			FetchedAt: observedAt.Add(-3 * time.Hour), QualityStatus: model.QualityValid,
		}
		if _, _, err := store.SaveTradingCalendarSnapshot(ctx, snapshot, observedAt.Add(-time.Hour)); err != nil {
			t.Fatalf("seed L3 trading calendar %s: %v", window.sourceVersion, err)
		}
	}
}

func l3SeedPublishPolicy(
	t *testing.T, ctx context.Context, store *repository.Store,
	policyRef, policyVersion string, allowedPurpose string, effectiveAt time.Time,
) {
	t.Helper()
	policy := authorization.DataAccessPolicy{
		SchemaVersion: authorization.DataAccessPolicySchemaVersion,
		Status:        authorization.PolicyApproved,
		// The real Worker's Quant Stage executes as principal:service:quant
		// (researchflow_runtime.go:387-391 from the "quant" credential).
		AllowedSubjects: []string{"principal:service:quant"},
		AllowedPurposes: []string{allowedPurpose},
		RequiredScopes:  []string{security.ScopeDataRead},
		EffectiveAt:     effectiveAt.UTC(),
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	version := authorization.PolicyVersion{
		PolicyType: authorization.DataAccessPolicyType, PolicyRef: policyRef,
		PolicyVersion: policyVersion, Payload: payload,
		PayloadHash: hex.EncodeToString(sum[:]), CreatedAt: effectiveAt.UTC(),
	}
	audit := app.DataAuthorizationPolicyPublicationAudit{
		ActorRef: "principal:owner", RequestID: "l3-seed-policy-" + policyVersion,
		OccurredAt: effectiveAt.UTC(),
	}
	if err := store.PublishDataAuthorizationPolicyVersion(ctx, "", version, audit); err != nil {
		t.Fatalf("seed L3 authorization policy %s: %v", policyRef, err)
	}
}

// seedQuantResearchL3StatusFacts writes a valid trading-status revision so the risk
// Stage can create an authorized facts Snapshot. The risk decision itself stays
// reject because price_limit and tradable_volume remain unavailable (doc/quant/14 D2).
func seedQuantResearchL3StatusFacts(t *testing.T, ctx context.Context, store *repository.Store, observedAt time.Time) {
	t.Helper()
	observed := observedAt.Add(-time.Hour)
	snapshot := &model.SecurityTradingStatusSnapshot{
		Symbol: l3SeedSymbol, TradeDate: l3SeedStatusDate,
		RiskWarningStatus:       model.RiskWarningNormal,
		SecurityName:            "招商银行",
		NameEffectiveFrom:       "1991-01-01",
		DelistingStatus:         model.DelistingStatusNormal,
		ListingDate:             "1991-01-01",
		ListingStatusKnown:      true,
		ListingTradingDayNumber: 1,
		IsTradingDay:            true,
		TradingDayKnown:         true,
		SuspensionStatus:        model.SuspensionStatusNoEvent,
		Source:                  l3SeedSource, SourceVersion: l3SeedSourceVer,
		FetchedAt: observed.Add(-time.Hour), SystemObservedAt: &observed,
		QualityStatus: model.QualityValid,
	}
	if err := store.SaveSecurityTradingStatus(ctx, snapshot); err != nil {
		t.Fatalf("seed L3 trading status facts: %v", err)
	}
}

// seedQuantResearchL3 writes all real PIT seeds into the isolated L3 database.
func seedQuantResearchL3(t *testing.T, ctx context.Context, store *repository.Store) {
	t.Helper()
	observedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	seedQuantResearchL3Bars(t, ctx, store, observedAt)
	seedQuantResearchL3Calendar(t, ctx, store, observedAt)
	l3SeedPublishPolicy(t, ctx, store, authorization.DailyBarsV2PolicyRef, l3SeedPolicyDaily, "quant_analysis", observedAt.Add(-24*time.Hour))
	l3SeedPublishPolicy(t, ctx, store, authorization.SecurityStatusFactsPolicyRef, l3SeedPolicyFacts, "risk_review", observedAt.Add(-24*time.Hour))
	seedQuantResearchL3StatusFacts(t, ctx, store, observedAt)
}

var _ = marketdatabootstrap.Dependencies{}
