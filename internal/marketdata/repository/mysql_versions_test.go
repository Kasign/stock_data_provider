package repository

import (
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestDailyBarRevisionHashIgnoresFetchTimestamps(t *testing.T) {
	fetched := time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC)
	normalized := fetched.Add(time.Second)
	volume := int64(10000)
	amount := 125000.0
	change := 1.25
	bar := model.DailyBar{
		Symbol: "600519.SH", TradeDate: "2026-10-27", Period: "1d",
		Open: 12.1, High: 12.8, Low: 12.0, Close: 12.5,
		Volume: &volume, Amount: &amount, ChangePercent: &change,
		AdjustmentBasis: model.AdjustmentNone, AdjustmentMethod: "none",
		Source: "fixture", SourceVersion: "fixture-v1",
		FetchedAt: fetched, NormalizedAt: normalized,
		VolumeUnit: "shares", AmountUnit: "CNY",
		QualityStatus: model.QualityValid, NormalizationVer: "v1",
	}

	first, err := dailyBarRevisionHash(bar)
	if err != nil {
		t.Fatal(err)
	}
	bar.FetchedAt = fetched.Add(30 * time.Minute)
	bar.NormalizedAt = normalized.Add(30 * time.Minute)
	second, err := dailyBarRevisionHash(bar)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("fetch timestamps alone must not create a new content revision")
	}

	bar.Close += 0.01
	third, err := dailyBarRevisionHash(bar)
	if err != nil {
		t.Fatal(err)
	}
	if third == first {
		t.Fatal("a changed close price must produce a new content revision")
	}
}

func TestShouldRecordRevisionComparesOnlyLatestVersion(t *testing.T) {
	var latest string
	hasLatest := false
	for _, revision := range []struct {
		hash string
		want bool
	}{
		{hash: "A", want: true},
		{hash: "A", want: false},
		{hash: "B", want: true},
		{hash: "A", want: true}, // 内容恢复为旧值时，也必须写入新的观察版本。
		{hash: "A", want: false},
	} {
		got := shouldRecordRevision(latest, hasLatest, revision.hash)
		if got != revision.want {
			t.Fatalf("shouldRecordRevision(previous=%q, next=%q)=%v want %v", latest, revision.hash, got, revision.want)
		}
		if got {
			latest, hasLatest = revision.hash, true
		}
	}
}

func TestFundamentalsRevisionHashIgnoresFetchTimestamps(t *testing.T) {
	fetched := time.Date(2026, 10, 28, 2, 0, 0, 0, time.UTC)
	snapshot := model.FundamentalsSnapshot{
		Symbol: "600519.SH", Year: 2026, Quarter: 2, HasData: true,
		Sections: []model.FundamentalSection{{
			Category: "profitability", ReportDate: "2026-06-30", PublicationDate: "2026-08-29",
			HasData: true, QualityStatus: model.QualityValid,
			Metrics: []model.FundamentalMetric{{Key: "roe", Name: "净资产收益率", Value: 0.12, Unit: "ratio", SourceField: "roe"}},
		}},
		Source: "tushare", SourceVersion: "fixture-v1", FetchedAt: fetched, QualityStatus: model.QualityValid,
	}
	first, err := fundamentalsRevisionHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.FetchedAt = fetched.Add(time.Hour)
	second, err := fundamentalsRevisionHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("fetch timestamps alone must not create a new fundamentals revision")
	}
	snapshot.Sections[0].Metrics[0].Value = 0.13
	third, err := fundamentalsRevisionHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("a changed financial metric must create a new fundamentals revision")
	}
}

func TestValuationRevisionHashIgnoresObservationTimestamps(t *testing.T) {
	fetched := time.Date(2026, 11, 10, 2, 0, 0, 0, time.UTC)
	observed := fetched.Add(time.Second)
	value := 25.0
	snapshot := model.ValuationSnapshot{
		RequestedSymbols: []string{"600519.SH"}, Total: 1,
		Items: []model.ValuationItem{{
			Symbol: "600519.SH", Code: "600519", PETTM: &value,
			Source: "eastmoney", SourceVersion: "fixture-v1",
			FetchedAt: fetched, QualityStatus: model.QualityDegraded,
		}},
		Source: "eastmoney", SourceVersion: "fixture-v1",
		FetchedAt: fetched, SystemObservedAt: &observed,
		QualityStatus: model.QualityDegraded,
	}
	first, err := valuationRevisionHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.FetchedAt = fetched.Add(time.Hour)
	snapshot.Items[0].FetchedAt = fetched.Add(time.Hour)
	observed = fetched.Add(2 * time.Hour)
	second, err := valuationRevisionHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("fetch/observation timestamps alone must not create a valuation content revision")
	}
	changed := 26.0
	snapshot.Items[0].PETTM = &changed
	third, err := valuationRevisionHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("a changed valuation metric must create a new revision")
	}
}

func TestValuationRequestFingerprintValidatesAndPreservesRequestOrder(t *testing.T) {
	first, err := valuationRequestFingerprint([]string{"600519.SH", "000858.SZ"})
	if err != nil {
		t.Fatal(err)
	}
	reordered, err := valuationRequestFingerprint([]string{"000858.SZ", "600519.SH"})
	if err != nil {
		t.Fatal(err)
	}
	if first == reordered {
		t.Fatal("request fingerprint must preserve requested symbol order")
	}
	if _, err := valuationRequestFingerprint([]string{"600519.SH", "600519.SH"}); err == nil {
		t.Fatal("duplicate symbols must be rejected by the repository fingerprint")
	}
	if _, err := valuationRequestFingerprint(nil); err == nil {
		t.Fatal("empty valuation request must be rejected")
	}
}

func TestFundamentalsRevisionHashNormalizesEquivalentOffsetInstants(t *testing.T) {
	availableAtOffset := time.Date(2026, 8, 29, 10, 0, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))
	snapshot := model.FundamentalsSnapshot{
		Symbol: "600519.SH", Year: 2026, Quarter: 2, HasData: true,
		Sections: []model.FundamentalSection{{
			Category: "profitability", ReportDate: "2026-06-30", PublicationDate: "2026-08-29",
			AvailableAt: &availableAtOffset, HasData: true, QualityStatus: model.QualityValid,
			Metrics: []model.FundamentalMetric{{Key: "roe", Name: "净资产收益率", Value: 0.12, Unit: "ratio", SourceField: "roe"}},
		}},
		Source: "fixture", SourceVersion: "fixture-v1", FetchedAt: availableAtOffset, QualityStatus: model.QualityValid,
	}
	first, err := fundamentalsRevisionHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	availableAtUTC := availableAtOffset.UTC()
	snapshot.Sections[0].AvailableAt = &availableAtUTC
	second, err := fundamentalsRevisionHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("equivalent instants with different offsets must produce the same fundamentals revision hash")
	}
}

func TestMarketDataMigrationDefinitionsExposeImmutableVersionedHistory(t *testing.T) {
	migrations := MigrationDefinitions()
	if len(migrations) != 9 {
		t.Fatalf("migration count = %d, want V1-V6, authorization policy V7, decisions/revocations V8 and publication audit V9", len(migrations))
	}
	for index := 0; index < 3; index++ {
		if migrations[index].Version != uint64(index+1) || len(migrations[index].Statements) == 0 || len(migrations[index].Operations) != 0 {
			t.Fatalf("published migration %d changed unexpectedly: %+v", index+1, migrations[index])
		}
	}
	compatibility := migrations[3]
	if compatibility.Version != 4 || len(compatibility.Statements) != 0 || len(compatibility.Operations) != 4 {
		t.Fatalf("compatibility migration must contain four idempotent operations only: %+v", compatibility)
	}
	calendar := migrations[4]
	if calendar.Version != 5 || len(calendar.Statements) != 1 || len(calendar.Operations) != 0 {
		t.Fatalf("calendar snapshot migration V5 must contain one additive table creation: %+v", calendar)
	}
	dailyBarsSnapshot := migrations[5]
	if dailyBarsSnapshot.Version != 6 || len(dailyBarsSnapshot.Statements) != 1 || len(dailyBarsSnapshot.Operations) != 0 ||
		!strings.Contains(dailyBarsSnapshot.Statements[0], "market_data_daily_bars_close_series_snapshots") ||
		!strings.Contains(dailyBarsSnapshot.Statements[0], "PRIMARY KEY (snapshot_ref)") {
		t.Fatalf("daily-bars Snapshot migration V6 must add one immutable query-level Snapshot table: %+v", dailyBarsSnapshot)
	}
	policy := migrations[6]
	if policy.Version != 7 || len(policy.Statements) != 2 || len(policy.Operations) != 0 ||
		!strings.Contains(policy.Statements[0], "market_data_authorization_policy_versions") ||
		!strings.Contains(policy.Statements[1], "market_data_current_authorization_policy_versions") {
		t.Fatalf("Data authorization policy migration V7 must add immutable versions and a current pointer: %+v", policy)
	}
	decisions := migrations[7]
	if decisions.Version != 8 || len(decisions.Statements) != 2 || len(decisions.Operations) != 0 ||
		!strings.Contains(decisions.Statements[0], "market_data_authorization_decisions") ||
		!strings.Contains(decisions.Statements[1], "market_data_authorization_revocations") ||
		!strings.Contains(decisions.Statements[1], "FOREIGN KEY (authorization_ref)") {
		t.Fatalf("Data authorization decision migration V8 must add immutable decisions and append-only revocations: %+v", decisions)
	}
	publicationAudit := migrations[8]
	if publicationAudit.Version != 9 || len(publicationAudit.Statements) != 1 || len(publicationAudit.Operations) != 0 ||
		!strings.Contains(publicationAudit.Statements[0], "market_data_authorization_policy_publications") ||
		!strings.Contains(publicationAudit.Statements[0], "previous_policy_version") ||
		!strings.Contains(publicationAudit.Statements[0], "payload_hash") ||
		!strings.Contains(publicationAudit.Statements[0], "request_id") {
		t.Fatalf("Data policy publication audit migration V9 must add append-only policy change evidence: %+v", publicationAudit)
	}
}
