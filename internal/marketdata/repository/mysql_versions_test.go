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

func TestMarketDataMigrationDefinitionsExposeCompleteBaseline(t *testing.T) {
	migrations := MigrationDefinitions()
	// V1 是已部署库的历史基线，修改任一语句都会改变 checksum 并让启动校验失败；
	// 后续结构变更只能追加版本。V2 为共享快照表增加 payload_kind 分流列。
	if len(migrations) != 2 || migrations[0].Version != 1 || len(migrations[0].Statements) != 17 ||
		migrations[1].Version != 2 || len(migrations[1].Statements) != 1 {
		t.Fatalf("migration baseline = %#v, want V1 with seventeen tables plus V2 with one statement", migrations)
	}
	schema := strings.Join(migrations[0].Statements, "\n")
	for _, table := range []string{
		"market_data_provider_probe_runs", "market_data_sentiment_snapshots", "market_data_quotes",
		"market_data_daily_bars", "market_data_daily_bar_versions", "market_data_fundamental_versions",
		"market_data_security_trading_status_versions", "market_data_adjustment_factor_versions",
		"market_data_corporate_action_versions", "market_data_valuation_versions",
		"market_data_trading_calendar_snapshots", "market_data_daily_bars_close_series_snapshots",
		"market_data_authorization_policy_versions", "market_data_current_authorization_policy_versions",
		"market_data_authorization_decisions", "market_data_authorization_revocations",
		"market_data_authorization_policy_publications",
	} {
		if !strings.Contains(schema, "CREATE TABLE "+table+" (") {
			t.Errorf("baseline is missing table %s", table)
		}
	}
	for _, fragment := range []string{
		"source_version VARCHAR(128) NOT NULL DEFAULT ''",
		"idx_market_adjustment_factor_version_asof",
		"volume_ratio DECIMAL(16,8) NULL",
		"turnover_rate DECIMAL(16,8) NULL",
		"idx_market_bar_revision_identity_asof",
		"idx_market_fundamental_identity_asof",
		"FOREIGN KEY (policy_type, policy_ref, policy_version)",
		"FOREIGN KEY (authorization_ref) REFERENCES market_data_authorization_decisions",
	} {
		if !strings.Contains(schema, fragment) {
			t.Errorf("baseline is missing final schema fragment %q", fragment)
		}
	}
	for _, forbidden := range []string{"IF NOT EXISTS", "ALTER TABLE", "UPDATE market_data_", "uq_market_bar_revision_content", "uq_market_fundamental_revision_content"} {
		if strings.Contains(strings.ToUpper(schema), strings.ToUpper(forbidden)) {
			t.Errorf("fresh baseline contains obsolete migration fragment %q", forbidden)
		}
	}
}
