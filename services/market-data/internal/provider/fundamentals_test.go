package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

type fakeBaoStockFinancialClient struct {
	noDataCategory  string
	publicationDate string
}

func (fakeBaoStockFinancialClient) QueryDailyKLines(context.Context, string, string, string, string) ([]baostock.DailyKLine, error) {
	return nil, nil
}
func (f fakeBaoStockFinancialClient) QueryQuarterlyFinancialData(_ context.Context, code, category string, year, quarter int) (*baostock.QuarterlyFinancialRecord, error) {
	if category == f.noDataCategory {
		return nil, nil
	}
	publicationDate := f.publicationDate
	if publicationDate == "" {
		publicationDate = "2026-08-29"
	}
	values := map[string]string{"code": code, "pubDate": publicationDate, "statDate": "2026-06-30"}
	for _, section := range quarterlyFundamentalMappings {
		if section.Category == category {
			for _, metric := range section.Fields {
				values[metric.SourceField] = "1.25"
			}
		}
	}
	values["roeAvg"], values["netProfit"] = "0.074617", "28522000000"
	return &baostock.QuarterlyFinancialRecord{Code: code, PublicationDate: values["pubDate"], ReportDate: values["statDate"], Fields: values}, nil
}

func TestBaostockFundamentalsPreservesRatioAndDisclosureDate(t *testing.T) {
	got, err := NewBaostock(fakeBaoStockFinancialClient{}).FetchFundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasData || len(got.Sections) != 6 || got.QualityStatus != model.QualityValid {
		t.Fatalf("unexpected fundamentals snapshot: %+v", got)
	}
	if got.Sections[0].ReportDate != "2026-06-30" || got.Sections[0].PublicationDate != "2026-08-29" {
		t.Fatalf("report/publication dates changed: %+v", got.Sections[0])
	}
	var roe, netProfit *model.FundamentalMetric
	for i := range got.Sections[0].Metrics {
		metric := &got.Sections[0].Metrics[i]
		if metric.SourceField == "roeAvg" {
			roe = metric
		}
		if metric.SourceField == "netProfit" {
			netProfit = metric
		}
	}
	if roe == nil || roe.Value != 0.074617 || roe.Unit != "ratio" {
		t.Fatalf("provider ratio must not be rescaled: %+v", roe)
	}
	if netProfit == nil || netProfit.Value != 28522000000 || netProfit.Unit != "CNY" {
		t.Fatalf("net profit should preserve yuan unit: %+v", netProfit)
	}
}

func TestBaostockFundamentalsRejectsFutureReportingPeriod(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	today := now.Format("2006-01-02")
	quarter := int((int(now.Month())-1)/3) + 1
	if fundamentalQuarterEnd(now.Year(), quarter) <= today {
		quarter++
	}
	if quarter > 4 {
		t.Skip("no future quarter end remains in the current year")
	}
	_, err = NewBaostock(fakeBaoStockFinancialClient{}).FetchFundamentals(context.Background(), "600519.SH", now.Year(), quarter)
	if err == nil || !strings.Contains(err.Error(), "report period") || !strings.Contains(err.Error(), "is in the future") {
		t.Fatalf("future reporting period must be rejected before provider queries, got %v", err)
	}
}

func TestBaostockFundamentalsRejectsFuturePublicationDate(t *testing.T) {
	_, err := NewBaostock(fakeBaoStockFinancialClient{publicationDate: "2099-12-31"}).FetchFundamentals(
		context.Background(), "600519.SH", 2026, 2,
	)
	if err == nil || !strings.Contains(err.Error(), "is after China market date") {
		t.Fatalf("future publication date must be rejected, got %v", err)
	}
}

func TestBaostockFundamentalsMissingCategoryIsUnknownNotZeroFilled(t *testing.T) {
	got, err := NewBaostock(fakeBaoStockFinancialClient{noDataCategory: "growth"}).FetchFundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range got.Sections {
		if section.Category == "growth" && (section.HasData || len(section.Metrics) != 0 || section.QualityStatus != model.QualityUnknown) {
			t.Fatalf("missing category must remain unknown: %+v", section)
		}
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("partial snapshot should be degraded: %+v", got)
	}
}
