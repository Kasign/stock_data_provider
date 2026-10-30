package service

import (
	"errors"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
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
