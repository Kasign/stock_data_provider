package provider

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

type pytdxOfficialSuspensionDailyReport struct {
	GeneratedAt       string                                `json:"generated_at"`
	Items              []pytdxOfficialSuspensionDailyItem    `json:"items"`
	FullDaySuspensions []pytdxFullDaySuspensionWindowItem    `json:"full_day_suspensions,omitempty"`
}

type pytdxFullDaySuspensionWindowItem struct {
	Symbol                 string   `json:"symbol"`
	SuspensionStartDate    string   `json:"suspension_start_date"`
	ResumeDate             string   `json:"resume_date"`
	WindowStartDate        string   `json:"window_start_date"`
	WindowEndDate          string   `json:"window_end_date"`
	OfficialSuspensionURL  string   `json:"official_suspension_url"`
	OfficialResumptionURL  string   `json:"official_resumption_url"`
	ObservedDates          []string `json:"observed_dates,omitempty"`
	PreSuspensionRows      int      `json:"pre_suspension_rows"`
	SuspensionWindowRows   int      `json:"suspension_window_rows"`
	ResumeRows             int      `json:"resume_rows"`
	ResumeQualityStatus    string   `json:"resume_quality_status,omitempty"`
	ResumeQualityReasons   []string `json:"resume_quality_reasons,omitempty"`
	Status                 string   `json:"status"`
	Error                  string   `json:"error,omitempty"`
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
}

// TestLivePytdxOfficialIntradaySuspensionDailyBars verifies that public daily
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
		GeneratedAt:       time.Now().UTC().Format(time.RFC3339),
		Items:              make([]pytdxOfficialSuspensionDailyItem, 0, len(samples)),
		FullDaySuspensions: make([]pytdxFullDaySuspensionWindowItem, 0, 1),
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
	// 新大正 002968.SZ：深交所公告确认 2025-09-15 开市起停牌，
	// 公司 2025-09-29 复牌公告确认当日开市起恢复交易。
	// 使用跨越停牌区间的窗口请求，避免对空单日区间把 Provider 的 empty-result 错误误判为网络故障。
	fullDaySample := pytdxFullDaySuspensionWindowItem{
		Symbol:                "002968.SZ",
		SuspensionStartDate:   "2025-09-15",
		ResumeDate:            "2025-09-29",
		WindowStartDate:       "2025-09-12",
		WindowEndDate:         "2025-09-30",
		OfficialSuspensionURL: "https://www.szse.cn/disclosure/notice/temp/t20250915_616077.html",
		OfficialResumptionURL: "https://disc.static.szse.cn/download/disc/disk03/finalpage/2025-09-29/8a38844e-6ccc-475f-a8f6-b808a5be55e1.PDF",
	}
	bars, fullDayErr := client.FetchDailyBars(ctx, fullDaySample.Symbol, fullDaySample.WindowStartDate, fullDaySample.WindowEndDate, 50, model.AdjustmentNone)
	if fullDayErr != nil {
		fullDaySample.Status = "provider_error"
		fullDaySample.Error = fullDayErr.Error()
		failures = append(failures, fullDaySample.Symbol+": "+fullDaySample.Error)
	} else {
		for _, bar := range bars {
			fullDaySample.ObservedDates = append(fullDaySample.ObservedDates, bar.TradeDate)
			switch {
			case bar.TradeDate == "2025-09-12":
				fullDaySample.PreSuspensionRows++
			case bar.TradeDate >= fullDaySample.SuspensionStartDate && bar.TradeDate < fullDaySample.ResumeDate:
				fullDaySample.SuspensionWindowRows++
			case bar.TradeDate == fullDaySample.ResumeDate:
				fullDaySample.ResumeRows++
				fullDaySample.ResumeQualityStatus = string(bar.QualityStatus)
				fullDaySample.ResumeQualityReasons = append([]string(nil), bar.QualityReasons...)
			}
		}
		switch {
		case fullDaySample.PreSuspensionRows != 1:
			fullDaySample.Status = "pre_suspension_bar_missing_or_duplicated"
			fullDaySample.Error = "expected exactly one daily bar on 2025-09-12 before suspension"
		case fullDaySample.SuspensionWindowRows != 0:
			fullDaySample.Status = "daily_bar_returned_during_full_day_suspension"
			fullDaySample.Error = "Pytdx returned daily bars during the officially announced full-day suspension window"
		case fullDaySample.ResumeRows != 1:
			fullDaySample.Status = "resume_daily_bar_missing_or_duplicated"
			fullDaySample.Error = "expected exactly one daily bar on the official resumption date 2025-09-29"
		default:
			fullDaySample.Status = "suspension_gap_and_resume_verified"
		}
		if fullDaySample.Error != "" {
			failures = append(failures, fullDaySample.Symbol+": "+fullDaySample.Error)
		}
	}
	report.FullDaySuspensions = append(report.FullDaySuspensions, fullDaySample)

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
