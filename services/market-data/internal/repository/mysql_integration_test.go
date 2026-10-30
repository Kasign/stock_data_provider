package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
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
	if err := reopened.Close(); err != nil {
		t.Fatalf("close MariaDB before unavailable check: %v", err)
	}
	if _, err := reopened.ListAdjustmentFactorsAsOf(ctx, symbol, "", "", secondAsOf); err == nil {
		t.Fatal("closed database connection must return an explicit storage error")
	}
}
