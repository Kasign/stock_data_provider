package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestMariaDBRevisionPersistenceAndPointInTimeQueries(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		t.Fatalf("first migration: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		t.Fatalf("idempotent migration: %v", err)
	}

	symbol := fmt.Sprintf("T%014d", time.Now().UnixNano()%100000000000000)
	availableAt := time.Now().UTC().Add(-time.Minute)
	fetchedAt := time.Now().UTC()
	factorV1 := model.AdjustmentFactor{
		Symbol: symbol, TradeDate: "2026-10-29", Factor: 1.25,
		Source: "integration-fixture", SourceVersion: "v1",
		AvailableAt: &availableAt, FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded,
	}
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{factorV1}); err != nil {
		store.Close()
		t.Fatalf("save first factor revision: %v", err)
	}
	firstAsOf := time.Now().UTC()
	// 相同内容重复采集不得制造新版本。
	factorDuplicate := factorV1
	factorDuplicate.FetchedAt = fetchedAt.Add(time.Second)
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{factorDuplicate}); err != nil {
		store.Close()
		t.Fatalf("save duplicate factor: %v", err)
	}
	firstFactors, err := store.ListAdjustmentFactorsAsOf(ctx, symbol, "", "", firstAsOf)
	if err != nil || len(firstFactors) != 1 || firstFactors[0].Factor != 1.25 {
		store.Close()
		t.Fatalf("first factor revision should remain stable: rows=%+v err=%v", firstFactors, err)
	}
	if firstFactors[0].SystemObservedAt == nil {
		store.Close()
		t.Fatal("factor query must restore system observation timestamp")
	}

	time.Sleep(3 * time.Millisecond)
	factorV2 := factorV1
	factorV2.Factor = 1.5
	factorV2.SourceVersion = "v2"
	factorV2.FetchedAt = time.Now().UTC()
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{factorV2}); err != nil {
		store.Close()
		t.Fatalf("save revised factor: %v", err)
	}
	secondAsOf := time.Now().UTC()
	secondFactors, err := store.ListAdjustmentFactorsAsOf(ctx, symbol, "", "", secondAsOf)
	if err != nil || len(secondFactors) != 1 || secondFactors[0].Factor != 1.5 || secondFactors[0].SourceVersion != "v2" {
		store.Close()
		t.Fatalf("latest factor revision should be returned: rows=%+v err=%v", secondFactors, err)
	}
	historicalFactors, err := store.ListAdjustmentFactorsAsOf(ctx, symbol, "", "", firstAsOf)
	if err != nil || len(historicalFactors) != 1 || historicalFactors[0].Factor != 1.25 {
		store.Close()
		t.Fatalf("as_of before revision must restore v1: rows=%+v err=%v", historicalFactors, err)
	}

	// 新版本已被系统观察，但 Provider 声明它在 as_of 之后才可用；
	// PIT 必须回退到此前已可用的旧版本，而不是将整条记录过滤掉。
	fallbackSymbol := fmt.Sprintf("F%014d", time.Now().UnixNano()%100000000000000)
	fallbackFetched := time.Now().UTC()
	fallbackAvailable := fallbackFetched.Add(-time.Minute)
	fallbackFactor := model.AdjustmentFactor{
		Symbol: fallbackSymbol, TradeDate: "2026-10-29", Factor: 1.1,
		Source: "integration-fallback", SourceVersion: "v1",
		AvailableAt: &fallbackAvailable, FetchedAt: fallbackFetched, QualityStatus: model.QualityValid,
	}
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{fallbackFactor}); err != nil {
		store.Close()
		t.Fatalf("save available fallback factor: %v", err)
	}
	time.Sleep(3 * time.Millisecond)
	fallbackFactor.Factor = 1.2
	fallbackFactor.SourceVersion = "v2"
	fallbackFactor.FetchedAt = time.Now().UTC()
	futureAvailable := fallbackFactor.FetchedAt.Add(time.Hour)
	fallbackFactor.AvailableAt = &futureAvailable
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{fallbackFactor}); err != nil {
		store.Close()
		t.Fatalf("save not-yet-available factor revision: %v", err)
	}
	fallbackAsOf := time.Now().UTC()
	fallbackFactors, err := store.ListAdjustmentFactorsAsOf(ctx, fallbackSymbol, "", "", fallbackAsOf)
	if err != nil || len(fallbackFactors) != 1 || fallbackFactors[0].Factor != 1.1 || fallbackFactors[0].SourceVersion != "v1" {
		store.Close()
		t.Fatalf("PIT must fall back to latest provider-available factor revision: rows=%+v err=%v", fallbackFactors, err)
	}

	// 同一证券、交易日和来源允许 forward/backward 两种独立因子口径并存；
	// BaoStock 未提供 AvailableAt 时仍应保存原始观测，但 PIT 服务层不得将其视为可见。
	basisSymbol := fmt.Sprintf("B%014d", time.Now().UnixNano()%100000000000000)
	basisFetchedAt := time.Now().UTC()
	forwardFactor := model.AdjustmentFactor{
		Symbol: basisSymbol, TradeDate: "2026-10-30", Factor: 0.95,
		Basis: model.AdjustmentFactorForward, Source: "baostock", SourceVersion: "baostock-public-tcp-v1",
		FetchedAt: basisFetchedAt, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_available_at_not_exposed_by_query_adjust_factor"},
	}
	backwardFactor := forwardFactor
	backwardFactor.Basis = model.AdjustmentFactorBackward
	backwardFactor.Factor = 5.4
	if err := store.SaveAdjustmentFactors(ctx, []model.AdjustmentFactor{forwardFactor, backwardFactor}); err != nil {
		store.Close()
		t.Fatalf("save independent BaoStock factor bases: %v", err)
	}
	var persistedBasisCount int
	if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_data_adjustment_factor_versions
WHERE symbol=? AND trade_date=? AND source=?`, basisSymbol, "2026-10-30", "baostock").Scan(&persistedBasisCount); err != nil || persistedBasisCount != 2 {
		store.Close()
		t.Fatalf("forward/backward factor bases must both persist: count=%d err=%v", persistedBasisCount, err)
	}
	basisRows, err := store.ListAdjustmentFactorsAsOf(ctx, basisSymbol, "2026-10-30", "2026-10-30", time.Now().UTC())
	if err != nil || len(basisRows) != 0 {
		store.Close()
		t.Fatalf("BaoStock factors without available_at must remain excluded from PIT queries: rows=%+v err=%v", basisRows, err)
	}

	cashV1 := 0.5
	action := model.CorporateAction{
		Symbol: symbol, ProviderEventID: "integration-dividend-1",
		EventType: "cash_dividend", EventStatus: "implementation",
		AnnouncementDate: "2026-08-01", ExDate: "2026-10-29",
		CashDividendCNY: &cashV1, Source: "integration-fixture",
		SourceVersion: "v1", AvailableAt: &availableAt,
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
	}
	if err := store.SaveCorporateActions(ctx, []model.CorporateAction{action}); err != nil {
		store.Close()
		t.Fatalf("save first corporate-action revision: %v", err)
	}
	actionFirstAsOf := time.Now().UTC()
	cashV2 := 0.8
	action.CashDividendCNY = &cashV2
	action.SourceVersion = "v2"
	action.FetchedAt = time.Now().UTC()
	time.Sleep(3 * time.Millisecond)
	if err := store.SaveCorporateActions(ctx, []model.CorporateAction{action}); err != nil {
		store.Close()
		t.Fatalf("save revised corporate action: %v", err)
	}
	actionSecondAsOf := time.Now().UTC()
	oldActions, err := store.ListCorporateActionsAsOf(ctx, symbol, "", "", actionFirstAsOf)
	if err != nil || len(oldActions) != 1 || oldActions[0].CashDividendCNY == nil || *oldActions[0].CashDividendCNY != 0.5 {
		store.Close()
		t.Fatalf("corporate-action as_of must restore v1: rows=%+v err=%v", oldActions, err)
	}
	newActions, err := store.ListCorporateActionsAsOf(ctx, symbol, "", "", actionSecondAsOf)
	if err != nil || len(newActions) != 1 || newActions[0].CashDividendCNY == nil || *newActions[0].CashDividendCNY != 0.8 {
		store.Close()
		t.Fatalf("corporate-action query must return v2: rows=%+v err=%v", newActions, err)
	}

	// 公司行动采用相同的 PIT 可见性规则：未到供应商可用时间的修订不能遮蔽旧版本。
	actionFallback := model.CorporateAction{
		Symbol: symbol, ProviderEventID: "integration-future-availability",
		EventType: "cash_dividend", EventStatus: "implementation",
		AnnouncementDate: "2026-08-02", ExDate: "2026-10-30",
		Source: "integration-fallback", SourceVersion: "v1",
		AvailableAt: &fallbackAvailable, FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid,
	}
	actionFallback.CashDividendCNY = &cashV1
	if err := store.SaveCorporateActions(ctx, []model.CorporateAction{actionFallback}); err != nil {
		store.Close()
		t.Fatalf("save available corporate-action fallback: %v", err)
	}
	time.Sleep(3 * time.Millisecond)
	actionFallback.SourceVersion = "v2"
	actionFallback.CashDividendCNY = &cashV2
	actionFallback.FetchedAt = time.Now().UTC()
	actionFutureAvailable := actionFallback.FetchedAt.Add(time.Hour)
	actionFallback.AvailableAt = &actionFutureAvailable
	if err := store.SaveCorporateActions(ctx, []model.CorporateAction{actionFallback}); err != nil {
		store.Close()
		t.Fatalf("save not-yet-available corporate-action revision: %v", err)
	}
	actionFallbackAsOf := time.Now().UTC()
	actionFallbackRows, err := store.ListCorporateActionsAsOf(ctx, symbol, "", "", actionFallbackAsOf)
	if err != nil {
		store.Close()
		t.Fatalf("query corporate-action fallback: %v", err)
	}
	var selectedFallback *model.CorporateAction
	for i := range actionFallbackRows {
		if actionFallbackRows[i].ProviderEventID == "integration-future-availability" {
			selectedFallback = &actionFallbackRows[i]
			break
		}
	}
	if selectedFallback == nil || selectedFallback.SourceVersion != "v1" ||
		selectedFallback.CashDividendCNY == nil || *selectedFallback.CashDividendCNY != cashV1 {
		store.Close()
		t.Fatalf("PIT must fall back to latest provider-available corporate-action revision: rows=%+v", actionFallbackRows)
	}

	barVolume := int64(1000)
	barAmount := 10000.0
	barFetchedAt := time.Now().UTC()
	makeIntegrationBar := func(tradeDate string) model.DailyBar {
		return model.DailyBar{
			Symbol: symbol, TradeDate: tradeDate, Period: "1d", AdjustmentBasis: model.AdjustmentNone,
			Source: "integration-fixture", SourceVersion: "integration-bars-v1",
			Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &barVolume, Amount: &barAmount,
			VolumeUnit: "shares", AmountUnit: "CNY", FetchedAt: barFetchedAt, NormalizedAt: barFetchedAt,
			QualityStatus: model.QualityDegraded, NormalizationVer: "4",
		}
	}
	if err := store.UpsertBars(ctx, []model.DailyBar{
		makeIntegrationBar("2026-10-19"), makeIntegrationBar("2026-10-20"),
		makeIntegrationBar("2026-10-21"), makeIntegrationBar("2026-10-22"),
	}); err != nil {
		t.Fatalf("save daily-bar range fixtures: %v", err)
	}
	boundedBars, err := store.ListBars(ctx, symbol, "2026-10-20", "2026-10-21", model.AdjustmentNone, 0)
	if err != nil || len(boundedBars) != 2 || boundedBars[0].TradeDate != "2026-10-21" || boundedBars[1].TradeDate != "2026-10-20" {
		t.Fatalf("current daily-bar query must use inclusive date bounds: bars=%+v err=%v", boundedBars, err)
	}
	barsAsOf, err := store.ListBarsAsOf(ctx, symbol, "2026-10-20", "2026-10-21", model.AdjustmentNone, time.Now().UTC().Add(time.Second), 0)
	if err != nil || len(barsAsOf) != 2 {
		t.Fatalf("PIT daily-bar query must use the same inclusive date bounds: bars=%+v err=%v", barsAsOf, err)
	}
	barsBeforeObservation, err := store.ListBarsAsOf(ctx, symbol, "2026-10-20", "2026-10-21", model.AdjustmentNone, barFetchedAt.Add(-time.Second), 0)
	if err != nil || len(barsBeforeObservation) != 0 {
		t.Fatalf("PIT daily-bar query must exclude revisions recorded after as_of: bars=%+v err=%v", barsBeforeObservation, err)
	}

	if err := store.Close(); err != nil {
		t.Fatalf("close MariaDB connection: %v", err)
	}
	reopened, err := OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("reopen MariaDB after connection restart: %v", err)
	}
	defer reopened.Close()
	persisted, err := reopened.ListAdjustmentFactorsAsOf(ctx, symbol, "", "", secondAsOf)
	if err != nil || len(persisted) != 1 || persisted[0].Factor != 1.5 {
		t.Fatalf("revision must persist across connection restart: rows=%+v err=%v", persisted, err)
	}
	probeRun := &model.ProviderProbeRun{
		RunID: fmt.Sprintf("probe-%d", time.Now().UnixNano()),
		Scope: "all", Status: "completed", Persisted: true,
		StartedAt: time.Now().UTC().Add(-time.Second), FinishedAt: time.Now().UTC(),
		RequestedCapabilities: []string{"daily_bars"},
		RequestedChecks:       []string{"cross_provider"},
		CheckResults: []model.ProviderProbeCheckResult{{
			Check: "cross_provider", Status: "skipped",
			AdjustmentBasis: "backward_adjusted",
			LeftProvider:    "eastmoney", RightProvider: "baostock",
			LeftAdjustmentMethod:   "eastmoney_fqt_2",
			RightAdjustmentMethod:  "baostock_pct_change",
			LeftAdjustmentRefDate:  "2026-11-03",
			RightAdjustmentRefDate: "2026-11-03",
			NotComparableCount:     1,
			FieldComparisons: []model.ProviderProbeFieldComparison{{
				Field: "close", Status: "not_comparable",
				Reason: "adjustment_method_mismatch",
			}},
			CheckedAt: time.Now().UTC(),
		}},
		Items: []model.ProviderProbeItem{{
			Provider: "eastmoney", Capability: "daily_bars",
			Status: "returned", AdjustmentBasis: model.AdjustmentBackward,
			QualityStatus: model.QualityValid, CheckedAt: time.Now().UTC(),
		}},
	}
	if err := reopened.SaveProviderProbeRun(ctx, probeRun); err != nil {
		t.Fatalf("save provider-probe run: %v", err)
	}
	probeHistory, err := reopened.ListProviderProbeRuns(ctx, 10)
	if err != nil {
		t.Fatalf("read provider-probe history: %v", err)
	}
	assertProbeAdjustmentEvidence := func(items []model.ProviderProbeRun) {
		t.Helper()
		for _, item := range items {
			if item.RunID != probeRun.RunID {
				continue
			}
			if len(item.CheckResults) != 1 {
				t.Fatalf("persisted probe check results were lost: %+v", item)
			}
			check := item.CheckResults[0]
			if check.LeftAdjustmentMethod != "eastmoney_fqt_2" ||
				check.RightAdjustmentMethod != "baostock_pct_change" ||
				check.LeftAdjustmentRefDate != "2026-11-03" ||
				check.RightAdjustmentRefDate != "2026-11-03" ||
				len(check.FieldComparisons) != 1 ||
				check.FieldComparisons[0].Reason != "adjustment_method_mismatch" {
				t.Fatalf("persisted adjustment evidence changed: %+v", check)
			}
			if len(item.Items) != 1 || item.Items[0].AdjustmentBasis != model.AdjustmentBackward {
				t.Fatalf("persisted provider probe items changed: %+v", item.Items)
			}
			return
		}
		t.Fatalf("provider-probe run %q was not returned in history: %+v", probeRun.RunID, items)
	}
	assertProbeAdjustmentEvidence(probeHistory)

	if err := reopened.Close(); err != nil {
		t.Fatalf("close MariaDB before unavailable check: %v", err)
	}
	probeReopened, err := OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("reopen MariaDB for provider-probe history: %v", err)
	}
	persistedProbeHistory, err := probeReopened.ListProviderProbeRuns(ctx, 10)
	if err != nil {
		probeReopened.Close()
		t.Fatalf("read provider-probe history after connection restart: %v", err)
	}
	assertProbeAdjustmentEvidence(persistedProbeHistory)
	if err := probeReopened.Close(); err != nil {
		t.Fatalf("close MariaDB after provider-probe history verification: %v", err)
	}
	if _, err := reopened.ListAdjustmentFactorsAsOf(ctx, symbol, "", "", secondAsOf); err == nil {
		t.Fatal("closed database connection must return an explicit storage error")
	}
}
