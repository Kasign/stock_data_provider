package service

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/repository"
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

	symbol := fmt.Sprintf("S%014d", time.Now().UnixNano()%100000000000000)
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

	symbol := fmt.Sprintf("A%014d", time.Now().UnixNano()%100000000000000)
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
