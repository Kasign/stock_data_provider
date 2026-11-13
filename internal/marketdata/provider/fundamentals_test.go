package provider

import (
	"context"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/baostock"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

type fakeBaoStockFinancialClient struct {
	noDataCategory  string
	publicationDate string
	invalidField    string
	missingField    string
	fieldIssues     []baostock.QuarterlyFinancialFieldIssue
	publicationType string
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
				if metric.SourceField != f.missingField {
					values[metric.SourceField] = "1.25"
				}
				if metric.SourceField == f.invalidField {
					values[metric.SourceField] = "not-a-number"
				}
			}
		}
	}
	if f.invalidField != "roeAvg" && f.missingField != "roeAvg" {
		values["roeAvg"] = "0.074617"
	}
	if f.invalidField != "netProfit" && f.missingField != "netProfit" {
		values["netProfit"] = "28522000000"
	}
	fieldTypes := map[string]string{"pubDate": "string"}
	fieldPreviews := map[string]string{"pubDate": values["pubDate"]}
	if f.publicationType != "" {
		fieldTypes["pubDate"] = f.publicationType
		fieldPreviews["pubDate"] = f.publicationType
	}
	return &baostock.QuarterlyFinancialRecord{
		Code: code, PublicationDate: values["pubDate"], ReportDate: values["statDate"],
		Fields: values, FieldTypes: fieldTypes, FieldPreviews: fieldPreviews, FieldIssues: f.fieldIssues,
	}, nil
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
	var roe, netProfit, totalOperatingRevenue *model.FundamentalMetric
	for i := range got.Sections[0].Metrics {
		metric := &got.Sections[0].Metrics[i]
		switch metric.SourceField {
		case "roeAvg":
			roe = metric
		case "netProfit":
			netProfit = metric
		case "MBRevenue":
			totalOperatingRevenue = metric
		}
	}
	if roe == nil || roe.Value != 0.074617 || roe.Unit != "ratio" {
		t.Fatalf("provider ratio must not be rescaled: %+v", roe)
	}
	if netProfit == nil || netProfit.Value != 28522000000 || netProfit.Unit != "CNY" {
		t.Fatalf("net profit should preserve yuan unit: %+v", netProfit)
	}
	if totalOperatingRevenue == nil || totalOperatingRevenue.Key != "total_operating_revenue" ||
		totalOperatingRevenue.Name != "营业总收入" || totalOperatingRevenue.Value != 1.25 ||
		totalOperatingRevenue.Unit != "CNY" {
		t.Fatalf("BaoStock MBRevenue must retain the verified total-operating-revenue semantics: %+v", totalOperatingRevenue)
	}
	if got.Sections[0].AvailableAt != nil {
		t.Fatalf("date-only pubDate must not be promoted to an exact available_at timestamp: %+v", got.Sections[0])
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

func TestBaostockFundamentalsReportsFieldLevelParseIssues(t *testing.T) {
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	got, err := NewBaostock(fakeBaoStockFinancialClient{
		invalidField: "roeAvg",
		missingField: "netProfit",
		fieldIssues:  []baostock.QuarterlyFinancialFieldIssue{{SourceField: "gpMargin", Code: "invalid_field", ActualType: "object", Preview: "object", RowIndex: 0}},
	}).FetchFundamentals(ctx, "600519.SH", 2026, 2)
	if err != nil {
		t.Fatalf("partial fundamentals should be retained: %v", err)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("invalid/missing fields must degrade the snapshot: %+v", got)
	}
	issues := reporter.Snapshot().Issues
	want := map[string]string{"roe_avg": "invalid_number", "net_profit": "missing_value", "gross_profit_margin": "invalid_field"}
	for field, code := range want {
		found := false
		for _, issue := range issues {
			if issue.Field == field && issue.Code == code && issue.ProviderField != "" &&
				issue.RowIndex != nil && *issue.RowIndex == 0 && issue.Symbol == "600519.SH" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected field-level issue %s/%s, got %+v", field, code, issues)
		}
	}
}

func TestBaostockFundamentalsReportsMissingPublicationDateField(t *testing.T) {
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	_, err := NewBaostock(fakeBaoStockFinancialClient{
		publicationDate: " ", publicationType: "null",
	}).FetchFundamentals(ctx, "600519.SH", 2026, 2)
	if err != nil {
		t.Fatalf("missing publication evidence should preserve partial fundamentals: %v", err)
	}
	found := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Field == "publication_date" && issue.ProviderField == "pubDate" &&
			issue.Code == "missing_value" && issue.ActualType == "null" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected publication-date null evidence, got %+v", reporter.Snapshot().Issues)
	}
}
