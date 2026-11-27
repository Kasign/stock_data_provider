package provider

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	marketdataapp "example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/model"
)

// liveLimitPoolProbeSymbols 是用于验证「未在池内 → none」的候选证券。
// 它们都是主板大盘股，同日既涨停又跌停不可能发生；若某个候选恰好在池内，
// 用例会顺延到下一个候选，而不是放宽断言。
var liveLimitPoolProbeSymbols = []string{"600519.SH", "601398.SH", "600036.SH", "000001.SZ", "000858.SZ"}

type liveLimitPoolReport struct {
	Probe               string   `json:"probe"`
	TradeDate           string   `json:"trade_date"`
	UpPoolTotal         int      `json:"up_pool_total"`
	UpPoolReturned      int      `json:"up_pool_returned"`
	DownPoolTotal       int      `json:"down_pool_total"`
	DownPoolReturned    int      `json:"down_pool_returned"`
	UpProviderDate      string   `json:"up_provider_report_date"`
	DownProviderDate    string   `json:"down_provider_report_date"`
	EmptyPools          []string `json:"empty_pools,omitempty"`
	UpPoolComplete      bool     `json:"up_pool_complete"`
	DownPoolComplete    bool     `json:"down_pool_complete"`
	UpSampleSymbol      string   `json:"up_sample_symbol,omitempty"`
	UpSampleStatus      string   `json:"up_sample_status,omitempty"`
	DownSampleSymbol    string   `json:"down_sample_symbol,omitempty"`
	DownSampleStatus    string   `json:"down_sample_status,omitempty"`
	AbsentSampleSymbol  string   `json:"absent_sample_symbol,omitempty"`
	AbsentSampleStatus  string   `json:"absent_sample_status,omitempty"`
	PayloadQuality      string   `json:"payload_quality"`
	AttemptedDates      []string `json:"attempted_dates"`
	UnusableDatesReason []string `json:"unusable_dates_reason,omitempty"`
}

// TestLiveEastmoneyFullLimitPoolCompleteness 是 price_limit 的真实来源证据：
// 它调用 Eastmoney 公开的全量涨跌停池，验证完整性、跨池互斥、涨跌方向，并把
// 真实响应喂给 BuildLimitPoolFactsPayload，确认 none / limit_up / limit_down
// 三个语义分支在真实数据上都成立。没有真实响应时本用例 SKIP，不得以夹具代替。
func TestLiveEastmoneyFullLimitPoolCompleteness(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_LIMIT_POOL_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_LIMIT_POOL_TESTS=1 to call the real Eastmoney limit-pool endpoints")
	}

	report := liveLimitPoolReport{Probe: "eastmoney-full-limit-pool-completeness"}
	client := NewEastmoney(20*time.Second, "", "")
	dates := liveLimitPoolCandidateDates()
	if len(dates) == 0 {
		t.Fatal("no candidate trade date for the live limit-pool probe")
	}

	var up, down *model.LimitPoolSnapshot
	var chosen string
	for _, date := range dates {
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		// 先取跌停池：它通常更小，两个池都为空时可以立即换下一个候选日。
		downSnapshot, downErr := client.FetchFullLimitPool(ctx, "down", date)
		if downErr != nil {
			cancel()
			report.AttemptedDates = append(report.AttemptedDates, date)
			report.UnusableDatesReason = append(report.UnusableDatesReason, date+": "+downErr.Error())
			continue
		}
		upSnapshot, upErr := client.FetchFullLimitPool(ctx, "up", date)
		cancel()
		if upErr != nil {
			report.AttemptedDates = append(report.AttemptedDates, date)
			report.UnusableDatesReason = append(report.UnusableDatesReason, date+": "+upErr.Error())
			continue
		}
		report.AttemptedDates = append(report.AttemptedDates, date)
		if upSnapshot == nil || downSnapshot == nil {
			report.UnusableDatesReason = append(report.UnusableDatesReason, date+": reader returned a nil snapshot")
			continue
		}
		// 两个池都没有真实行时无法区分「零涨跌停」与「日期不可用」，换下一个候选日。
		if !upSnapshot.HasData && !downSnapshot.HasData {
			report.UnusableDatesReason = append(report.UnusableDatesReason, date+": both pools returned no rows")
			continue
		}
		up, down, chosen = upSnapshot, downSnapshot, date
		break
	}
	if up == nil || down == nil {
		t.Fatalf("no trade date yielded a usable full limit-pool pair within %d candidates: %v",
			len(dates), report.UnusableDatesReason)
	}

	// 1) 真实响应必须自洽：源端总量与取回行数一致，且不是下界估计。
	//    这是 none 结论可信的前提，也是 FetchFullLimitPool 存在的理由。
	for _, pool := range []*model.LimitPoolSnapshot{up, down} {
		if pool.Total != len(pool.Items) {
			t.Fatalf("%s pool: total=%d items=%d, full fetch must return the whole pool", pool.PoolType, pool.Total, len(pool.Items))
		}
		if pool.TotalIsLowerBound {
			t.Fatalf("%s pool reported a lower-bound total; none cannot be asserted from it", pool.PoolType)
		}
		if pool.ProviderReturned != pool.Total {
			t.Fatalf("%s pool: provider returned=%d total=%d, want equal", pool.PoolType, pool.ProviderReturned, pool.Total)
		}
		if pool.TradeDate != chosen {
			t.Fatalf("%s pool trade_date=%q, want requested %q", pool.PoolType, pool.TradeDate, chosen)
		}
		if pool.Total == 0 && pool.HasData {
			t.Fatalf("%s pool claims data with zero rows; an empty pool must not claim data", pool.PoolType)
		}
		// 真实行为：请求历史交易日时源端 qdate 仍返回「最新交易日」，并不回显请求日。
		// 因此 requested trade date 才是权威（Provider 自己声明了该质量原因）；
		// 这里只要求 qdate 可解析且不早于请求日，绝不把它当作交易日身份。
		if pool.ProviderReportDate == "" {
			t.Fatalf("%s pool has no provider report date", pool.PoolType)
		}
		reportDate, parseErr := time.Parse("2006-01-02", pool.ProviderReportDate)
		if parseErr != nil {
			t.Fatalf("%s pool provider report date %q is not canonical", pool.PoolType, pool.ProviderReportDate)
		}
		if reportDate.Before(mustLiveLimitPoolDate(t, chosen)) {
			t.Fatalf("%s pool provider report date %q precedes the requested session %q",
				pool.PoolType, pool.ProviderReportDate, chosen)
		}
		if pool.Source == "" || pool.SourceVersion == "" || pool.FetchedAt.IsZero() {
			t.Fatalf("%s pool is missing source provenance: %+v", pool.PoolType, pool)
		}
	}

	// 2) 池内条目身份与方向必须可信：证券符号唯一，涨停池只应有上涨证券、
	//    跌停池只应有下跌证券（可捕获上下池接反）。
	upSymbols := liveLimitPoolSymbolSet(t, up)
	downSymbols := liveLimitPoolSymbolSet(t, down)
	for _, item := range up.Items {
		if item.ChangePercent != nil && *item.ChangePercent <= 0 {
			t.Fatalf("limit-up pool contains a non-positive change percent: %s=%v", item.Symbol, *item.ChangePercent)
		}
	}
	for _, item := range down.Items {
		if item.ChangePercent != nil && *item.ChangePercent >= 0 {
			t.Fatalf("limit-down pool contains a non-negative change percent: %s=%v", item.Symbol, *item.ChangePercent)
		}
	}

	// 3) 同日不可能既涨停又跌停：两个池必须互斥。
	for symbol := range upSymbols {
		if _, present := downSymbols[symbol]; present {
			t.Fatalf("%s appears in both limit-up and limit-down pools on %s", symbol, chosen)
		}
	}

	// 4) 真实数据必须驱动三个语义分支。none 是最危险的分支（会让门禁放行），
	//    必须在真实完整池上验证，而不是只用夹具。
	var absentSymbol string
	for _, candidate := range liveLimitPoolProbeSymbols {
		if _, inUp := upSymbols[candidate]; inUp {
			continue
		}
		if _, inDown := downSymbols[candidate]; inDown {
			continue
		}
		absentSymbol = candidate
		break
	}
	if absentSymbol == "" {
		t.Fatalf("every probe candidate is in a limit pool on %s; extend liveLimitPoolProbeSymbols", chosen)
	}
	absentPayload, err := marketdataapp.BuildLimitPoolFactsPayload(up, down, absentSymbol)
	if err != nil {
		t.Fatalf("build payload for a security outside both pools: %v", err)
	}
	if absentPayload.LimitStatus != marketdataapp.LimitPoolStatusNone {
		t.Fatalf("security %s outside both pools got status=%q, want %q",
			absentSymbol, absentPayload.LimitStatus, marketdataapp.LimitPoolStatusNone)
	}
	if !absentPayload.UpPoolComplete || !absentPayload.DownPoolComplete {
		t.Fatalf("none payload complete=%v/%v, want both complete", absentPayload.UpPoolComplete, absentPayload.DownPoolComplete)
	}
	if err := absentPayload.Validate(); err != nil {
		t.Fatalf("none payload failed validation: %v", err)
	}
	// 有池为零事件时，"未在池内"的结论依赖兄弟池存活性推断，质量必须降级。
	if up.Total == 0 || down.Total == 0 {
		if absentPayload.QualityStatus != marketdataapp.QualityDegraded {
			t.Fatalf("payload quality=%q, want degraded when one pool reported zero events", absentPayload.QualityStatus)
		}
	}

	report.TradeDate = chosen
	report.UpPoolTotal, report.UpPoolReturned = up.Total, up.ProviderReturned
	report.DownPoolTotal, report.DownPoolReturned = down.Total, down.ProviderReturned
	report.UpProviderDate, report.DownProviderDate = up.ProviderReportDate, down.ProviderReportDate
	if up.Total == 0 {
		report.EmptyPools = append(report.EmptyPools, "up")
	}
	if down.Total == 0 {
		report.EmptyPools = append(report.EmptyPools, "down")
	}
	report.UpPoolComplete, report.DownPoolComplete = absentPayload.UpPoolComplete, absentPayload.DownPoolComplete
	report.AbsentSampleSymbol, report.AbsentSampleStatus = absentSymbol, string(absentPayload.LimitStatus)
	report.PayloadQuality = string(absentPayload.QualityStatus)

	if len(up.Items) > 0 {
		upSymbol := up.Items[0].Symbol
		payload, upProbeErr := marketdataapp.BuildLimitPoolFactsPayload(up, down, upSymbol)
		if upProbeErr != nil {
			t.Fatalf("build payload for a limit-up security: %v", upProbeErr)
		}
		if payload.LimitStatus != marketdataapp.LimitPoolStatusUp {
			t.Fatalf("limit-up payload status=%q, want %q", payload.LimitStatus, marketdataapp.LimitPoolStatusUp)
		}
		report.UpSampleSymbol, report.UpSampleStatus = upSymbol, string(payload.LimitStatus)
	}
	if len(down.Items) > 0 {
		downSymbol := down.Items[0].Symbol
		downPayload, downProbeErr := marketdataapp.BuildLimitPoolFactsPayload(up, down, downSymbol)
		if downProbeErr != nil {
			t.Fatalf("build payload for a limit-down security: %v", downProbeErr)
		}
		if downPayload.LimitStatus != marketdataapp.LimitPoolStatusDown {
			t.Fatalf("limit-down payload status=%q, want %q", downPayload.LimitStatus, marketdataapp.LimitPoolStatusDown)
		}
		report.DownSampleSymbol, report.DownSampleStatus = downSymbol, string(downPayload.LimitStatus)
	}

	encoded, marshalErr := json.Marshal(report)
	if marshalErr != nil {
		t.Fatalf("marshal live limit-pool report: %v", marshalErr)
	}
	t.Logf("LIVE_LIMIT_POOL_REPORT_BEGIN\n%s\nLIVE_LIMIT_POOL_REPORT_END", encoded)
	if outputPath := strings.TrimSpace(os.Getenv("LIVE_LIMIT_POOL_REPORT_PATH")); outputPath != "" {
		if writeErr := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); writeErr != nil {
			t.Fatalf("write live limit-pool report: %v", writeErr)
		}
	}
}

func mustLiveLimitPoolDate(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		t.Fatalf("candidate trade date %q is not canonical: %v", value, err)
	}
	return parsed
}

// liveLimitPoolSymbolSet 校验池内每条证券身份可用且不重复，返回符号集合。
func liveLimitPoolSymbolSet(t *testing.T, pool *model.LimitPoolSnapshot) map[string]struct{} {
	t.Helper()
	symbols := make(map[string]struct{}, len(pool.Items))
	for index, item := range pool.Items {
		symbol := strings.TrimSpace(item.Symbol)
		if symbol == "" || symbol != item.Symbol {
			t.Fatalf("%s pool row %d has an unusable symbol %q", pool.PoolType, index, item.Symbol)
		}
		if _, duplicate := symbols[symbol]; duplicate {
			t.Fatalf("%s pool row %d repeats symbol %s", pool.PoolType, index, symbol)
		}
		symbols[symbol] = struct{}{}
	}
	return symbols
}

// liveLimitPoolCandidateDates 返回候选交易日：显式配置优先，否则从最近一个已收盘
// 工作日往前取 10 个工作日。不复用交易日历，避免真实探针依赖另一个数据源。
func liveLimitPoolCandidateDates() []string {
	loc := providerMarketLocation()
	now := providerNowUTC().In(loc)
	if configured := strings.TrimSpace(os.Getenv("LIVE_LIMIT_POOL_TRADE_DATE")); configured != "" {
		return []string{configured}
	}
	latest := now
	// 收盘前当日池不完整，必须退到上一个工作日。
	if now.Hour() < 15 || (now.Hour() == 15 && now.Minute() < 30) {
		latest = now.AddDate(0, 0, -1)
	}
	dates := make([]string, 0, 10)
	for offset := 0; offset < 30 && len(dates) < 10; offset++ {
		day := latest.AddDate(0, 0, -offset)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		dates = append(dates, day.Format("2006-01-02"))
	}
	return dates
}
