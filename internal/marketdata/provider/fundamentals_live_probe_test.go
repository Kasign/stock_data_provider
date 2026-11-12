package provider

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/baostock"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestLiveProviderProbeQuarterlyFundamentals(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_PROVIDER_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_PROVIDER_TESTS=1 to query live BaoStock quarterly financial endpoints")
	}
	client := baostock.NewClient(baostock.Config{ConnectTimeout: 8 * time.Second, RequestTimeout: 20 * time.Second})
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("BaoStock fundamentals cleanup warning: %v", err)
		}
	}()
	p := NewBaostock(client)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx = validation.WithReporter(ctx, reporter)
	started := time.Now()
	snapshot, err := p.FetchFundamentals(ctx, "600519.SH", 2026, 2)
	result := map[string]any{
		"provider": "baostock", "capability": "fundamentals", "symbol": "600519.SH",
		"year": 2026, "quarter": 2, "elapsed_ms": time.Since(started).Milliseconds(),
		"status": "failed",
	}
	if snapshot != nil {
		result["fundamentals"] = snapshot
		result["quality_status"] = snapshot.QualityStatus
		result["quality_reasons"] = snapshot.QualityReasons
	}
	result["validation"] = reporter.Snapshot()
	if err != nil {
		kind, message := liveProbeErrorSummary(err)
		result["error_kind"], result["error"] = kind, message
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
		t.Fatalf("BaoStock quarterly fundamentals failed: %s (%s)", kind, message)
	}
	if snapshot == nil || snapshot.Symbol != "600519.SH" || snapshot.Year != 2026 || snapshot.Quarter != 2 ||
		snapshot.Source != "baostock" || snapshot.FetchedAt.IsZero() || len(snapshot.Sections) != 6 || !snapshot.HasData {
		result["status"], result["error"] = "invalid", "snapshot identity/period/coverage mismatch"
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
		t.Fatal("BaoStock quarterly fundamentals identity/period/coverage mismatch")
	}
	wantDate := "2026-06-30"
	seen := map[string]bool{}
	for _, section := range snapshot.Sections {
		if seen[section.Category] {
			t.Fatalf("duplicate fundamentals category %q", section.Category)
		}
		seen[section.Category] = true
		if section.HasData && section.ReportDate != wantDate {
			t.Fatalf("category %s report_date=%s want=%s", section.Category, section.ReportDate, wantDate)
		}
		if section.HasData && section.PublicationDate != "" {
			if _, parseErr := time.Parse("2006-01-02", section.PublicationDate); parseErr != nil {
				t.Fatalf("category %s invalid publication date %q", section.Category, section.PublicationDate)
			}
			if section.PublicationDate != "2026-08-15" {
				t.Fatalf("category %s publication_date=%s want=2026-08-15", section.Category, section.PublicationDate)
			}
		}
		if section.AvailableAt != nil {
			t.Fatalf("category %s date-only pubDate must not be promoted to an exact available_at timestamp: %v", section.Category, section.AvailableAt)
		}
	}
	// 固定对账基准：贵州茅台 2026 年半年度报告，2026-08-15 披露。
	// 报告中的营业总收入、净利润、经营现金流及期末总股本均以人民币元/股数列示。
	// epsTTM 与 roeAvg 不直接对比报告中的半年度基本 EPS 和加权平均 ROE，二者口径不同。
	revenue := 90703260964.48
	totalRevenue := 92278072083.21
	netProfit := 46033330566.78
	operatingCashFlow := 70690750119.06
	assertBaoStockLiveFundamentalMetric(t, snapshot, "MBRevenue", "total_operating_revenue", "营业总收入", "CNY", totalRevenue, 0.01)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "netProfit", "net_profit", "净利润", "CNY", netProfit, 0.01)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "totalShare", "total_shares", "总股本", "shares", 1250081601, 0.5)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "npMargin", "net_profit_margin", "销售净利率", "ratio", netProfit/revenue, 0.00001)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "gpMargin", "gross_profit_margin", "销售毛利率", "ratio", (revenue-9473762565.88)/revenue, 0.00001)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "CFOToOR", "operating_cash_to_revenue", "经营现金流/营业收入", "ratio", operatingCashFlow/revenue, 0.00001)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "CFOToGr", "operating_cash_to_total_revenue", "经营现金流/营业总收入", "ratio", operatingCashFlow/totalRevenue, 0.00001)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "CFOToNP", "operating_cash_to_net_profit", "经营现金流/净利润", "ratio", operatingCashFlow/netProfit, 0.00001)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "YOYNI", "net_income_yoy", "净利润同比变化", "ratio", 46033330566.78/46986681449.24-1, 0.0001)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "YOYPNI", "parent_net_income_yoy", "归母净利润同比变化", "ratio", 44516880421.86/45402962298.10-1, 0.0001)
	assertBaoStockLiveFundamentalMetric(t, snapshot, "YOYEPSBasic", "basic_eps_yoy", "基本每股收益同比变化", "ratio", 35.57/36.18-1, 0.0001)
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("unexpected live fundamentals quality %q", snapshot.QualityStatus)
	}
	result["status"] = "returned"
	if snapshot.QualityStatus == model.QualityDegraded {
		result["status"] = "returned_degraded"
	} else if snapshot.QualityStatus == model.QualityUnknown {
		result["status"] = "returned_unknown"
	}
	encoded, _ := json.Marshal(result)
	t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
}

func assertBaoStockLiveFundamentalMetric(
	t *testing.T,
	snapshot *model.FundamentalsSnapshot,
	sourceField, key, name, unit string,
	want, tolerance float64,
) {
	t.Helper()
	var found *model.FundamentalMetric
	for sectionIndex := range snapshot.Sections {
		for metricIndex := range snapshot.Sections[sectionIndex].Metrics {
			metric := &snapshot.Sections[sectionIndex].Metrics[metricIndex]
			if metric.SourceField != sourceField {
				continue
			}
			if found != nil {
				t.Fatalf("duplicate BaoStock source field %s", sourceField)
			}
			found = metric
		}
	}
	if found == nil {
		t.Fatalf("BaoStock source field %s is missing", sourceField)
	}
	if found.Key != key || found.Name != name || found.Unit != unit {
		t.Fatalf("BaoStock source field %s mapping mismatch: got key=%s name=%s unit=%s", sourceField, found.Key, found.Name, found.Unit)
	}
	if math.IsNaN(found.Value) || math.IsInf(found.Value, 0) || math.Abs(found.Value-want) > tolerance {
		t.Fatalf("BaoStock source field %s value=%0.12f want=%0.12f tolerance=%0.12f", sourceField, found.Value, want, tolerance)
	}
}
