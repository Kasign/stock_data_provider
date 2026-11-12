package service

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
)

func TestAdjustmentFactorsAsOfFallsBackToLatestProviderVisibleRevision(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	symbol := "600519.SH"
	fetchedV1 := time.Now().UTC()
	availableV1 := fetchedV1.Add(-time.Minute)
	v1 := model.AdjustmentFactor{
		Symbol: symbol, TradeDate: "2026-10-29", Factor: 1.1,
		Basis: model.AdjustmentFactorShared, Source: "service-pit-fallback",
		SourceVersion: "v1", AvailableAt: &availableV1,
		FetchedAt: fetchedV1, QualityStatus: model.QualityValid,
	}
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{v1}); err != nil {
		t.Fatalf("save visible v1: %v", err)
	}

	time.Sleep(3 * time.Millisecond)
	fetchedV2 := time.Now().UTC()
	availableV2 := fetchedV2.Add(time.Hour)
	v2 := v1
	v2.Factor = 1.2
	v2.SourceVersion = "v2"
	v2.FetchedAt = fetchedV2
	v2.AvailableAt = &availableV2
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{v2}); err != nil {
		t.Fatalf("save not-yet-visible v2: %v", err)
	}

	service := &DataService{store: store}
	got, err := service.AdjustmentFactorsAsOf(ctx, symbol, "", "", time.Now().UTC())
	if err != nil {
		t.Fatalf("AdjustmentFactorsAsOf: %v", err)
	}
	if len(got) != 1 || got[0].Factor != 1.1 || got[0].SourceVersion != "v1" {
		t.Fatalf("service must return latest provider-visible revision v1, got %+v", got)
	}
}

func TestDailyBarsAsOfIncludesCarryInAdjustmentFactorBeforeRequestedStart(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	const symbol = "600519.SH"
	const source = "daily-bars-asof-carry-in-test"
	const sourceVersion = "carry-in-v1"
	const startDate = "2026-10-29"
	const endDate = "2026-10-29"
	fetchedAt := time.Now().UTC().Add(-time.Minute)
	availableAt := fetchedAt.Add(-time.Minute)
	bar := model.DailyBar{
		Symbol: symbol, TradeDate: startDate, Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10,
		Source: source, SourceVersion: sourceVersion,
		AdjustmentBasis: model.AdjustmentNone, NormalizationVer: normalize.Version,
		FetchedAt: fetchedAt, NormalizedAt: fetchedAt,
		QualityStatus: model.QualityValid,
	}
	if err := store.UpsertBars(ctx, []model.DailyBar{bar}); err != nil {
		t.Fatalf("save raw daily bar: %v", err)
	}
	factor := model.AdjustmentFactor{
		Symbol: symbol, TradeDate: "2026-10-28", Factor: 2,
		Basis: model.AdjustmentFactorBackward, Source: source, SourceVersion: sourceVersion,
		FetchedAt: fetchedAt, AvailableAt: &availableAt, QualityStatus: model.QualityValid,
	}
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{factor}); err != nil {
		t.Fatalf("save carry-in adjustment factor: %v", err)
	}

	service := &DataService{store: store, defaultBars: 200, maxBars: 2000}
	bars, _, err := service.DailyBarsAsOf(ctx, symbol, startDate, endDate, 10, model.AdjustmentBackward, time.Now().UTC())
	if err != nil {
		t.Fatalf("DailyBarsAsOf should use the latest visible factor before the requested range: %v", err)
	}
	if len(bars) != 1 || bars[0].Close != 20 {
		t.Fatalf("expected pre-range factor 2 to adjust raw close 10 to 20, got %+v", bars)
	}
}

func TestCorporateActionsAsOfFallsBackToLatestProviderVisibleRevision(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	symbol := "600519.SH"
	fetchedV1 := time.Now().UTC()
	availableV1 := fetchedV1.Add(-time.Minute)
	v1 := model.CorporateAction{
		Symbol: symbol, ProviderEventID: "pit-fallback-event", EventType: "cash_dividend",
		ExDate: "2026-10-29", Source: "service-pit-fallback", SourceVersion: "v1",
		AvailableAt: &availableV1, FetchedAt: fetchedV1, QualityStatus: model.QualityValid,
	}
	if err := store.SaveCorporateActions(ctx, []model.CorporateAction{v1}); err != nil {
		t.Fatalf("save visible v1: %v", err)
	}

	time.Sleep(3 * time.Millisecond)
	fetchedV2 := time.Now().UTC()
	availableV2 := fetchedV2.Add(time.Hour)
	v2 := v1
	v2.SourceVersion = "v2"
	v2.EventStatus = "revised"
	v2.AvailableAt = &availableV2
	v2.FetchedAt = fetchedV2
	if err := store.SaveCorporateActions(ctx, []model.CorporateAction{v2}); err != nil {
		t.Fatalf("save not-yet-visible v2: %v", err)
	}

	service := &DataService{store: store}
	got, err := service.CorporateActionsAsOf(ctx, symbol, "", "", time.Now().UTC())
	if err != nil {
		t.Fatalf("CorporateActionsAsOf: %v", err)
	}
	if len(got) != 1 || got[0].SourceVersion != "v1" || got[0].EventStatus != "" {
		t.Fatalf("service must return latest provider-visible revision v1, got %+v", got)
	}
}

func TestSaveAdjustmentFactorsRejectsDuplicateIdentityInBatch(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	now := time.Now().UTC()
	available := now.Add(-time.Minute)
	first := model.AdjustmentFactor{
		Symbol: "688001.SH", TradeDate: "2026-11-06", Factor: 1.1,
		Basis: model.AdjustmentFactorShared, Source: "batch-atomicity-test",
		SourceVersion: "v1", FetchedAt: now, AvailableAt: &available,
		QualityStatus: model.QualityValid,
	}
	duplicate := first
	duplicate.Factor = 1.2
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{first, duplicate}); err == nil {
		t.Fatal("duplicate identity in one refresh batch must be rejected")
	}

	values, err := store.ListAdjustmentFactorsAsOf(ctx, first.Symbol, first.TradeDate, first.TradeDate, time.Now().UTC())
	if err != nil {
		t.Fatalf("query adjustment factors after rejected batch: %v", err)
	}
	if len(values) != 0 {
		t.Fatalf("rejected batch must not persist any revision, got %+v", values)
	}
}

type adjustmentFactorFixtureProvider struct {
	name   string
	values []model.AdjustmentFactor
	err    error
	calls  int
}

func (p *adjustmentFactorFixtureProvider) Name() string { return p.name }
func (p *adjustmentFactorFixtureProvider) FetchAdjustmentFactors(context.Context, string, string, string) ([]model.AdjustmentFactor, error) {
	p.calls++
	return append([]model.AdjustmentFactor(nil), p.values...), p.err
}

var _ provider.AdjustmentFactorProvider = (*adjustmentFactorFixtureProvider)(nil)

func TestRefreshAdjustmentFactorsFallsBackFromEmptyAndInvalidProviderBatch(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	now := time.Now().UTC()
	valid := model.AdjustmentFactor{
		Symbol: "688009.SH", TradeDate: "2026-11-06", Factor: 1.234,
		Basis: model.AdjustmentFactorShared, Source: "refresh-factor-fallback-valid",
		SourceVersion: "fixture-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"available_at_not_provided"},
	}
	emptyProvider := &adjustmentFactorFixtureProvider{name: "refresh-factor-empty"}
	duplicateA := valid
	duplicateA.Source = "refresh-factor-duplicate"
	duplicateA.Factor = 1.01
	duplicateB := duplicateA
	duplicateB.Factor = 1.02
	invalidProvider := &adjustmentFactorFixtureProvider{name: duplicateA.Source, values: []model.AdjustmentFactor{duplicateA, duplicateB}}
	validProvider := &adjustmentFactorFixtureProvider{name: valid.Source, values: []model.AdjustmentFactor{valid}}
	svc := &DataService{
		providers: []provider.NamedProvider{emptyProvider, invalidProvider, validProvider},
		store:     store,
	}
	got, err := svc.RefreshAdjustmentFactors(ctx, valid.Symbol, valid.TradeDate, valid.TradeDate)
	if err != nil {
		t.Fatalf("RefreshAdjustmentFactors should continue after empty/duplicate-identity batches: %v", err)
	}
	if len(got) != 1 || got[0].Source != valid.Source || got[0].Factor != valid.Factor {
		t.Fatalf("unexpected fallback result: %+v", got)
	}
	if emptyProvider.calls != 1 || invalidProvider.calls != 1 || validProvider.calls != 1 {
		t.Fatalf("unexpected provider call counts: empty=%d duplicate=%d valid=%d",
			emptyProvider.calls, invalidProvider.calls, validProvider.calls)
	}
	persisted, err := store.ListAdjustmentFactorsAsOf(ctx, valid.Symbol, valid.TradeDate, valid.TradeDate, time.Now().UTC())
	if err != nil {
		t.Fatalf("query persisted adjustment factors: %v", err)
	}
	if len(persisted) != 1 || persisted[0].Source != valid.Source || persisted[0].Factor != valid.Factor {
		t.Fatalf("only valid fallback batch should be persisted: %+v", persisted)
	}
}

func TestRefreshAdjustmentFactorsStopsOnPersistenceFailure(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	now := time.Now().UTC()
	valid := model.AdjustmentFactor{
		Symbol: "688010.SH", TradeDate: "2026-11-06", Factor: 1.111,
		Basis: model.AdjustmentFactorShared, Source: "refresh-factor-persist-failure",
		SourceVersion: "fixture-v1", FetchedAt: now, QualityStatus: model.QualityStatus("unsupported_quality"),
	}
	// Service 的 Provider 合同校验通过，但 Repository 必须拒绝未知质量状态；该持久化错误不能触发 Provider fallback。
	p := &adjustmentFactorFixtureProvider{name: valid.Source, values: []model.AdjustmentFactor{valid}}
	svc := &DataService{providers: []provider.NamedProvider{p}, store: store}
	got, err := svc.RefreshAdjustmentFactors(ctx, valid.Symbol, valid.TradeDate, valid.TradeDate)
	if err == nil {
		t.Fatalf("persistence conflict should fail refresh, got %+v", got)
	}
	if got != nil {
		t.Fatalf("persistence failure must not return an unpersisted batch: %+v", got)
	}
	if p.calls != 1 {
		t.Fatalf("persistence failure must not trigger provider fallback/retry, calls=%d", p.calls)
	}
	persisted, queryErr := store.ListAdjustmentFactorsAsOf(ctx, valid.Symbol, valid.TradeDate, valid.TradeDate, time.Now().UTC())
	if queryErr != nil {
		t.Fatalf("query after rejected persistence: %v", queryErr)
	}
	if len(persisted) != 0 {
		t.Fatalf("rejected refresh must not persist a partial revision: %+v", persisted)
	}
}
