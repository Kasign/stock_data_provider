package service

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
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
	invalidProvider := &adjustmentFactorFixtureProvider{name: "refresh-factor-invalid", values: []model.AdjustmentFactor{
		{Symbol: "688009.SH", TradeDate: "2026-11-06", Factor: 0, Basis: model.AdjustmentFactorShared,
			Source: "refresh-factor-invalid", SourceVersion: "fixture-v1", FetchedAt: now},
	}}
	validProvider := &adjustmentFactorFixtureProvider{name: valid.Source, values: []model.AdjustmentFactor{valid}}
	svc := &DataService{
		providers: []provider.NamedProvider{emptyProvider, invalidProvider, validProvider},
		store: store,
	}
	got, err := svc.RefreshAdjustmentFactors(ctx, valid.Symbol, valid.TradeDate, valid.TradeDate)
	if err != nil {
		t.Fatalf("RefreshAdjustmentFactors should continue after empty/invalid batches: %v", err)
	}
	if len(got) != 1 || got[0].Source != valid.Source || got[0].Factor != valid.Factor {
		t.Fatalf("unexpected fallback result: %+v", got)
	}
	if emptyProvider.calls != 1 || invalidProvider.calls != 1 || validProvider.calls != 1 {
		t.Fatalf("unexpected provider call counts: empty=%d invalid=%d valid=%d",
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
		SourceVersion: "fixture-v1", FetchedAt: now, QualityStatus: model.QualityValid,
	}
	// 先写入同一身份但不同 revision，确保后续刷新在 Repository 唯一身份/事务约束处失败。
	existing := valid
	existing.Factor = 1.222
	existing.SourceVersion = "existing-v1"
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{existing}); err != nil {
		t.Fatalf("seed existing factor: %v", err)
	}
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
}
