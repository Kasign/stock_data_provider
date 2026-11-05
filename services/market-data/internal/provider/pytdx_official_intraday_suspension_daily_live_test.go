package provider

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

type pytdxOfficialSuspensionDailyReport struct {
	GeneratedAt string                             `json:"generated_at"`
	Items       []pytdxOfficialSuspensionDailyItem `json:"items"`
}

type pytdxOfficialSuspensionDailyItem struct {
	Symbol         string   `json:"symbol"`
	TradeDate      string   `json:"trade_date"`
	OfficialURL    string   `json:"official_url"`
	Rows           int      `json:"rows"`
	QualityStatus  string   `json:"quality_status,omitempty"`
	QualityReasons []string `json:"quality_reasons,omitempty"`
	Close          float64  `json:"close,omitempty"`
	Volume         *int64   `json:"volume,omitempty"`
	Amount         *float64 `json:"amount,omitempty"`
	Status         string   `json:"status"`
	Error          string   `json:"error,omitempty"`
}// TestLivePytdxOfficialIntradaySuspensionDailyBars verifies that public daily
// K-line endpoints return a full-day candle for exchange-announced intraday
// suspensions. It deliberately does not call Tushare or consume API credits.
func TestLivePytdxOfficialIntradaySuspensionDailyBars(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_PYTDX_OFFICIAL_SUSPENSION_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_PYTDX_OFFICIAL_SUSPENSION_TESTS=1 to query Pytdx daily bars for official intraday-suspension samples")
	}
	samples := []pytdxOfficialSuspensionDailyItem{
		{Symbol: "603072.SH", TradeDate: "2025-01-03", OfficialURL: "https://www.sse.com.cn/disclosure/announcement/general/mainsuspension/c/c_20250103_10767682.shtml"},
		{Symbol: "301595.SZ", TradeDate: "2025-05-19", OfficialURL: "https://www.szse.cn/disclosure/notice/temp/t20250519_613632.html"},
		{Symbol: "001390.SZ", TradeDate: "2025-05-29", OfficialURL: "https://www.szse.cn/disclosure/notice/temp/t20250529_613857.html"},
		{Symbol: "603400.SH", TradeDate: "2025-06-25", OfficialURL: "https://www.sse.com.cn/disclosure/announcement/general/mainsuspension/c/c_20250625_10783012.shtml"},
	}
	report := pytdxOfficialSuspensionDailyReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Items:       make([]pytdxOfficialSuspensionDailyItem, 0, len(samples)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client := NewPytdx(20 * time.Second)
	var failures []string
	for _, sample := range samples {
		item := sample
		bars, err := client.FetchDailyBars(ctx, item.Symbol, item.TradeDate, item.TradeDate, 5, model.AdjustmentNone)
		if err != nil {
			item.Status = "provider_error"
			item.Error = err.Error()
			failures = append(failures, item.Symbol+"/"+item.TradeDate+": "+item.Error)
			report.Items = append(report.Items, item)
			continue
		}
		item.Rows = len(bars)
		if len(bars) != 1 {
			item.Status = "daily_bar_missing_or_duplicated"
			item.Error = "expected exactly one daily bar for official intraday suspension date"
			failures = append(failures, item.Symbol+"/"+item.TradeDate+": "+item.Error)
			report.Items = append(report.Items, item)
			continue
		}
		bar := bars[0]
		item.QualityStatus = string(bar.QualityStatus)
		item.QualityReasons = append([]string(nil), bar.QualityReasons...)
		item.Close = bar.Close
		item.Volume = bar.Volume
		item.Amount = bar.Amount
		if bar.Symbol != item.Symbol || bar.TradeDate != item.TradeDate || bar.AdjustmentBasis != model.AdjustmentNone {
			item.Status = "contract_mismatch"
			item.Error = "returned daily bar identity/date/adjustment contract mismatch"
			failures = append(failures, item.Symbol+"/"+item.TradeDate+": "+item.Error)
		} else {
			item.Status = "returned"
		}
		report.Items = append(report.Items, item)
	}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("marshal Pytdx official suspension daily-bar report: %v", err)
	}
	t.Logf("LIVE_PYTDX_OFFICIAL_SUSPENSION_DAILY_REPORT_BEGIN\n%s\nLIVE_PYTDX_OFFICIAL_SUSPENSION_DAILY_REPORT_END", encoded)
	if path := strings.TrimSpace(os.Getenv("LIVE_PYTDX_OFFICIAL_SUSPENSION_REPORT_PATH")); path != "" {
		if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
			t.Errorf("write Pytdx official suspension daily-bar report: %v", err)
		}
	}
	if len(failures) > 0 {
		t.Errorf("Pytdx official suspension daily-bar probe failed: %s", strings.Join(failures, " | "))
	}
}
