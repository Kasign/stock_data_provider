package service

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
)

func TestFundamentalsAsOfRequiresRevisionStorage(t *testing.T) {
	svc := &DataService{}
	_, err := svc.FundamentalsAsOf(nil, "600519.SH", 2025, 4, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrFundamentalsAsOfUnavailable) {
		t.Fatalf("expected explicit unavailable-storage error, got %v", err)
	}
}

func TestFilterFundamentalsSnapshotAsOfExcludesUnprovenPublications(t *testing.T) {
	asOf := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	snapshot := &model.FundamentalsSnapshot{
		Symbol: "600519.SH", Year: 2025, Quarter: 4, HasData: true, QualityStatus: model.QualityDegraded,
		Sections: []model.FundamentalSection{
			{Category: "profitability", PublicationDate: "2026-08-19", HasData: true, QualityStatus: model.QualityDegraded,
				Metrics: []model.FundamentalMetric{{Key: "roe", Name: "净资产收益率", Value: 0.12, Unit: "ratio", SourceField: "roe"}}},
			{Category: "growth", PublicationDate: "2026-08-20", HasData: true, QualityStatus: model.QualityDegraded,
				Metrics: []model.FundamentalMetric{{Key: "growth", Name: "营收增长率", Value: 0.08, Unit: "ratio", SourceField: "growth"}}},
			{Category: "cash_flow", PublicationDate: "", HasData: true, QualityStatus: model.QualityDegraded,
				Metrics: []model.FundamentalMetric{{Key: "cash", Name: "现金流", Value: 1, Unit: "CNY", SourceField: "cash"}}},
		},
	}
	filtered, usable := filterFundamentalsSnapshotAsOf(snapshot, asOf)
	if !usable || filtered == nil {
		t.Fatal("previously published section should keep partial PIT result usable")
	}
	if !filtered.Sections[0].HasData || len(filtered.Sections[0].Metrics) != 1 {
		t.Fatal("section published before as_of should remain available")
	}
	if filtered.Sections[1].HasData || len(filtered.Sections[1].Metrics) != 0 {
		t.Fatal("same-day disclosure must be excluded when publication time is unavailable")
	}
	if filtered.Sections[2].HasData || len(filtered.Sections[2].Metrics) != 0 {
		t.Fatal("undated disclosure must be excluded from strict PIT")
	}
	if filtered.QualityStatus != model.QualityDegraded {
		t.Fatalf("excluded sections must make PIT quality degraded, got %s", filtered.QualityStatus)
	}
}

func TestFilterFundamentalsSnapshotAsOfUsesExactAvailableAt(t *testing.T) {
	asOf := time.Date(2026, 8, 20, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	before := time.Date(2026, 8, 20, 9, 59, 59, 0, time.FixedZone("CST", 8*60*60)).UTC()
	after := time.Date(2026, 8, 20, 10, 0, 1, 0, time.FixedZone("CST", 8*60*60)).UTC()
	snapshot := &model.FundamentalsSnapshot{
		Symbol: "600519.SH", Year: 2025, Quarter: 4, HasData: true, QualityStatus: model.QualityDegraded,
		Sections: []model.FundamentalSection{
			{Category: "profitability", ReportDate: "2025-12-31", PublicationDate: "2026-08-20", AvailableAt: &before, HasData: true, QualityStatus: model.QualityDegraded,
				Metrics: []model.FundamentalMetric{{Key: "roe", Name: "净资产收益率", Value: 0.12, Unit: "ratio", SourceField: "roe"}}},
			{Category: "growth", ReportDate: "2025-12-31", PublicationDate: "2026-08-20", AvailableAt: &after, HasData: true, QualityStatus: model.QualityDegraded,
				Metrics: []model.FundamentalMetric{{Key: "growth", Name: "营收增长率", Value: 0.08, Unit: "ratio", SourceField: "growth"}}},
		},
	}
	filtered, usable := filterFundamentalsSnapshotAsOf(snapshot, asOf)
	if !usable || filtered == nil {
		t.Fatal("section available before as_of should keep the PIT result usable")
	}
	if !filtered.Sections[0].HasData || len(filtered.Sections[0].Metrics) != 1 {
		t.Fatal("same-day section available before as_of should remain available")
	}
	if filtered.Sections[1].HasData || len(filtered.Sections[1].Metrics) != 0 {
		t.Fatal("same-day section available after as_of must be excluded")
	}
}

func TestFundamentalsAsOfReadsHistoricalRevisionFromMariaDB(t *testing.T) {
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
	const year, quarter = 2025, 4
	const source = "fundamentals-pit-integration"
	publicationDate := "2026-03-20"
	availableAt := time.Date(2026, 3, 20, 15, 0, 0, 0, marketLocation())

	makeSnapshot := func(value float64) *model.FundamentalsSnapshot {
		categories := []string{"profitability", "operating", "growth", "solvency", "cash_flow", "dupont"}
		sections := make([]model.FundamentalSection, 0, len(categories))
		for _, category := range categories {
			section := model.FundamentalSection{
				Category: category, ReportDate: "2025-12-31",
				HasData: false, Metrics: []model.FundamentalMetric{},
				QualityStatus: model.QualityUnknown,
			}
			if category == "profitability" {
				section.PublicationDate = publicationDate
				section.AvailableAt = &availableAt
				section.HasData = true
				section.QualityStatus = model.QualityDegraded
				section.Metrics = []model.FundamentalMetric{{
					Key: "roe", Name: "净资产收益率", Value: value,
					Unit: "ratio", SourceField: "roe",
				}}
			}
			sections = append(sections, section)
		}
		return &model.FundamentalsSnapshot{
			Symbol: symbol, Year: year, Quarter: quarter, HasData: true,
			Sections: sections, Source: source, SourceVersion: "integration-v1",
			FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
		}
	}

	if err := store.SaveFundamentals(ctx, makeSnapshot(0.12)); err != nil {
		t.Fatalf("save first fundamentals revision: %v", err)
	}
	firstAsOf := time.Now().UTC()

	// 同一报告期、同一来源出现修订后，较早 as_of 必须仍返回当时已观察的版本。
	time.Sleep(3 * time.Millisecond)
	if err := store.SaveFundamentals(ctx, makeSnapshot(0.15)); err != nil {
		t.Fatalf("save revised fundamentals snapshot: %v", err)
	}
	latestAsOf := time.Now().UTC()

	service := &DataService{store: store}
	historical, err := service.FundamentalsAsOf(ctx, symbol, year, quarter, firstAsOf)
	if err != nil {
		t.Fatalf("read historical fundamentals revision: %v", err)
	}
	if historical.Sections[0].Metrics[0].Value != 0.12 {
		t.Fatalf("historical as_of must restore first observed value 0.12, got %+v", historical.Sections[0])
	}
	if historical.SystemObservedAt == nil || historical.SystemObservedAt.After(firstAsOf) {
		t.Fatalf("historical result must restore its system observation timestamp: %+v", historical.SystemObservedAt)
	}

	latest, err := service.FundamentalsAsOf(ctx, symbol, year, quarter, latestAsOf)
	if err != nil {
		t.Fatalf("read latest fundamentals revision: %v", err)
	}
	if latest.Sections[0].Metrics[0].Value != 0.15 {
		t.Fatalf("latest as_of must return revised value 0.15, got %+v", latest.Sections[0])
	}
}
