package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

type liveTushareIntradaySuspensionReport struct {
	GeneratedAt string                                `json:"generated_at"`
	Items       []liveTushareIntradaySuspensionSample `json:"items"`
}

type liveTushareIntradaySuspensionSample struct {
	Symbol          string `json:"symbol"`
	TradeDate       string `json:"trade_date"`
	OfficialURL     string `json:"official_url"`
	OfficialTiming  string `json:"official_timing"`
	ProviderStatus  string `json:"provider_status,omitempty"`
	ProviderTiming  string `json:"provider_timing,omitempty"`
	ProviderType    string `json:"provider_type,omitempty"`
	TimingMatches   bool   `json:"timing_matches_official"`
	DailyBarRows    int    `json:"daily_bar_rows"`
	DailyBarQuality string `json:"daily_bar_quality,omitempty"`
	DailyBarError   string `json:"daily_bar_error,omitempty"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
}
// TestLiveTushareIntradaySuspensionSamples checks official exchange intraday
// suspension announcements against Tushare suspend_d and the same-day Pytdx bar.
// The test performs at most four suspend_d requests and stops immediately on 40203.
func TestLiveTushareIntradaySuspensionSamples(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_TUSHARE_SUSPENSION_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_TUSHARE_SUSPENSION_TESTS=1 to query Tushare intraday suspension evidence")
	}
	token := strings.TrimSpace(os.Getenv("MARKET_DATA_TUSHARE_TOKEN"))
	if token == "" {
		token = strings.TrimSpace(os.Getenv("TUSHARE_TOKEN"))
	}
	if token == "" {
		t.Skip("Tushare intraday suspension probe skipped: token is not configured")
	}

	samples := []liveTushareIntradaySuspensionSample{
		{Symbol: "603072.SH", TradeDate: "2025-01-03", OfficialTiming: "09:47-09:57", OfficialURL: "https://www.sse.com.cn/disclosure/announcement/general/mainsuspension/c/c_20250103_10767682.shtml"},
		{Symbol: "301595.SZ", TradeDate: "2025-05-19", OfficialTiming: "13:02:08-13:12:09", OfficialURL: "https://www.szse.cn/disclosure/notice/temp/t20250519_613632.html"},
		{Symbol: "001390.SZ", TradeDate: "2025-05-29", OfficialTiming: "09:56:24-10:06:25", OfficialURL: "https://www.szse.cn/disclosure/notice/temp/t20250529_613857.html"},
		{Symbol: "603400.SH", TradeDate: "2025-06-25", OfficialTiming: "14:27-14:37", OfficialURL: "https://www.sse.com.cn/disclosure/announcement/general/mainsuspension/c/c_20250625_10783012.shtml"},
	}
	report := liveTushareIntradaySuspensionReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Items:       make([]liveTushareIntradaySuspensionSample, 0, len(samples)),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	endpoint := strings.TrimSpace(os.Getenv("MARKET_DATA_TUSHARE_API_URL"))
	client := NewTushareConfigured(15*time.Second, endpoint, token)
	pytdx := NewPytdx(20 * time.Second)
	var failures []string

	writeReport := func() {
		encoded, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Errorf("marshal Tushare suspension report: %v", err)
			return
		}
		t.Logf("LIVE_TUSHARE_INTRADAY_SUSPENSION_REPORT_BEGIN\n%s\nLIVE_TUSHARE_INTRADAY_SUSPENSION_REPORT_END", encoded)
		if path := strings.TrimSpace(os.Getenv("LIVE_TUSHARE_SUSPENSION_REPORT_PATH")); path != "" {
			if err := os.WriteFile(path, append(encoded, '\n'), 0o644); err != nil {
				t.Errorf("write Tushare suspension report: %v", err)
			}
		}
	}

	for _, sample := range samples {
		item := sample
		compactDate := strings.ReplaceAll(item.TradeDate, "-", "")
		status, timing, suspendType, _, err := client.fetchSuspensionOnDate(ctx, item.Symbol, compactDate, true)
		if err != nil {
			item.Status = "provider_error"
			item.Error = strings.ReplaceAll(err.Error(), token, "[redacted]")
			report.Items = append(report.Items, item)
			writeReport()
			if strings.Contains(err.Error(), "40203") {
				t.Skipf("Tushare suspend_d permission blocked (40203); stopped after one request: %s", item.Error)
			}
			t.Fatalf("Tushare suspend_d query failed for %s/%s: %s", item.Symbol, item.TradeDate, item.Error)
		}
		item.ProviderStatus = string(status)
		item.ProviderTiming = timing
		item.ProviderType = suspendType
		item.TimingMatches = liveSuspensionTimingMatches(timing, item.OfficialTiming)
		if status != model.SuspensionStatusIntraday || strings.TrimSpace(timing) == "" || !strings.Contains(suspendType, "S") || !item.TimingMatches {
			item.Status = "status_mismatch"
			item.Error = fmt.Sprintf("expected intraday suspension with official timing %q; got status=%q timing=%q type=%q", item.OfficialTiming, status, timing, suspendType)
			failures = append(failures, item.Symbol+"/"+item.TradeDate+": "+item.Error)
		}

		bars, barErr := pytdx.FetchDailyBars(ctx, item.Symbol, item.TradeDate, item.TradeDate, 5, model.AdjustmentNone)
		if barErr != nil {
			item.DailyBarError = barErr.Error()
			if item.Status == "" {
				item.Status = "daily_bar_error"
			}
			failures = append(failures, item.Symbol+"/"+item.TradeDate+": Pytdx daily bar: "+barErr.Error())
		} else {
			item.DailyBarRows = len(bars)
			if len(bars) == 1 {
				item.DailyBarQuality = string(bars[0].QualityStatus)
			}
			if len(bars) != 1 {
				item.Status = "daily_bar_missing"
				item.Error = fmt.Sprintf("expected one same-day daily bar, got %d", len(bars))
				failures = append(failures, item.Symbol+"/"+item.TradeDate+": "+item.Error)
			}
		}
		if item.Status == "" {
			item.Status = "matched"
		}
		report.Items = append(report.Items, item)
	}
	writeReport()
	if len(failures) > 0 {
		t.Errorf("Tushare intraday suspension evidence mismatches: %s", strings.Join(failures, " | "))
	}
}

func liveSuspensionTimingMatches(actual, expected string) bool {
	actual = strings.TrimSpace(actual)
	expected = strings.TrimSpace(expected)
	if actual == "" || expected == "" {
		return false
	}
	actualParts := strings.SplitN(actual, "-", 2)
	expectedParts := strings.SplitN(expected, "-", 2)
	if len(actualParts) != 2 || len(expectedParts) != 2 {
		return false
	}
	parseClock := func(value string) (time.Duration, bool) {
		for _, layout := range []string{"15:04:05", "15:04"} {
			parsed, err := time.Parse(layout, strings.TrimSpace(value))
			if err == nil {
				return time.Duration(parsed.Hour())*time.Hour + time.Duration(parsed.Minute())*time.Minute + time.Duration(parsed.Second())*time.Second, true
			}
		}
		return 0, false
	}
	for index := 0; index < 2; index++ {
		got, okGot := parseClock(actualParts[index])
		want, okWant := parseClock(expectedParts[index])
		if !okGot || !okWant {
			return false
		}
		diff := got - want
		if diff < 0 {
			diff = -diff
		}
		if diff > time.Minute {
			return false
		}
	}
	return true
}
