package provider

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

// 本测试支持 quote-only、daily-only、index-only 模式；线上复测通过 [include_tickflow] 加对应能力标记显式触发。 daily-only 仅请求一只股票的 8 条未复权日 K。
// quote-only 会记录认证头是否存在及必要报价字段，不输出 API Key 值或完整响应体。
// TestLiveTickFlowCapabilities 是 TickFlow 已实现但默认关闭能力的独立真实请求验收入口。
// 仅在显式设置 MARKET_DATA_LIVE_TICKFLOW_TESTS=1 且提供 API Key 时执行，避免普通 CI 依赖外部服务。
func TestLiveTickFlowCapabilities(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_TICKFLOW_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_TICKFLOW_TESTS=1 to call TickFlow live endpoints")
	}

	apiKey := os.Getenv("MARKET_DATA_TICKFLOW_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("TICKFLOW_API_KEY")
	}
	if apiKey == "" {
		t.Fatal("MARKET_DATA_LIVE_TICKFLOW_TESTS=1 requires MARKET_DATA_TICKFLOW_API_KEY or TICKFLOW_API_KEY")
	}

	baseURL := os.Getenv("MARKET_DATA_TICKFLOW_API_URL")
	if baseURL == "" {
		baseURL = os.Getenv("TICKFLOW_BASE_URL")
	}
	freeURL := os.Getenv("MARKET_DATA_TICKFLOW_FREE_API_URL")
	if freeURL == "" {
		freeURL = os.Getenv("TICKFLOW_FREE_BASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client := NewTickFlowConfigured(25*time.Second, baseURL, freeURL, apiKey)
	type result struct {
		Capability   string                      `json:"capability"`
		Status       string                      `json:"status"`
		ElapsedMS    int64                       `json:"elapsed_ms"`
		Quality      string                      `json:"quality,omitempty"`
		QualityNotes []string                    `json:"quality_notes,omitempty"`
		Rows         int                         `json:"rows,omitempty"`
		Error        string                      `json:"error,omitempty"`
		HTTP         []liveProbeHTTPResponse     `json:"http_responses,omitempty"`
		QuoteFields  *tickflowQuoteRawDiagnostic `json:"quote_fields,omitempty"`
		Validation   *validation.Report          `json:"validation,omitempty"`
		DailyRaw     *tickflowDailyRawDiagnostic `json:"daily_raw,omitempty"`
	}

	results := make([]result, 0, 4)
	indexOnly := os.Getenv("MARKET_DATA_LIVE_TICKFLOW_INDEX_ONLY") == "1"
	quoteOnly := os.Getenv("MARKET_DATA_LIVE_TICKFLOW_QUOTE_ONLY") == "1"
	dailyOnly := os.Getenv("MARKET_DATA_LIVE_TICKFLOW_DAILY_ONLY") == "1"
	selectedModes := 0
	for _, enabled := range []bool{indexOnly, quoteOnly, dailyOnly} {
		if enabled {
			selectedModes++
		}
	}
	if selectedModes > 1 {
		t.Fatal("TickFlow live probe modes are mutually exclusive")
	}
	failures := make([]string, 0, 4)
	run := func(name string, call func() (int, string, []string, error)) {
		started := time.Now()
		rows, quality, notes, err := call()
		item := result{Capability: name, Status: "passed", ElapsedMS: time.Since(started).Milliseconds(), Rows: rows, Quality: quality, QualityNotes: notes}
		if err != nil {
			_, safeMessage := liveProbeErrorSummary(err)
			item.Status, item.Error = "failed", safeMessage
			failures = append(failures, name+": "+safeMessage)
		}
		results = append(results, item)
	}

	runQuote := func() {
		capture := &liveProbeCapture{}
		client.client.Transport = capture.transport()
		run("quote", func() (int, string, []string, error) {
			item, err := client.FetchQuote(ctx, "600519.SH")
			if err != nil {
				return 0, "", nil, err
			}
			if item == nil || item.Symbol != "600519.SH" || item.Source != "tickflow" || item.FetchedAt.IsZero() || item.CurrentPrice == nil {
				return 0, "", nil, errLiveProbeContract("quote identity/source/time/price mismatch")
			}
			return 1, string(item.QualityStatus), item.QualityReasons, nil
		})
		if len(results) > 0 {
			httpResponses := capture.snapshot()
			if len(httpResponses) > 0 {
				var envelope struct {
					Data []tickflowQuoteRawDiagnostic `json:"data"`
				}
				if json.Unmarshal([]byte(httpResponses[0].BodySample), &envelope) == nil && len(envelope.Data) > 0 {
					results[len(results)-1].QuoteFields = &envelope.Data[0]
				}
				for i := range httpResponses {
					httpResponses[i].BodySample = ""
				}
			}
			results[len(results)-1].HTTP = httpResponses
		}
	}
	if quoteOnly || (!indexOnly && !dailyOnly) {
		runQuote()
	}
	if dailyOnly {
		capture := &liveProbeCapture{}
		client.client.Transport = capture.transport()
		var dailyValidation *validation.Report
		run("daily_bars_none", func() (int, string, []string, error) {
			location, err := time.LoadLocation("Asia/Shanghai")
			if err != nil {
				location = time.FixedZone("CST", 8*60*60)
			}
			end := time.Now().In(location).AddDate(0, 0, -1)
			start := end.AddDate(0, 0, -30)
			reporter := validation.NewReporter(validation.DefaultMaxIssues)
			dailyCtx := validation.WithReporter(ctx, reporter)
			bars, err := client.FetchDailyBars(dailyCtx, "600519.SH", start.Format("2006-01-02"), end.Format("2006-01-02"), 8, model.AdjustmentNone)
			report := reporter.Snapshot()
			dailyValidation = &report
			if err != nil {
				return 0, "", nil, err
			}
			if len(bars) == 0 || len(bars) > 8 {
				return len(bars), "", nil, errLiveProbeContract("daily K-line row count is empty or exceeds requested limit")
			}
			quality := model.QualityValid
			notes := make([]string, 0)
			previousDate := ""
			for _, bar := range bars {
				if bar.Symbol != "600519.SH" || bar.Source != "tickflow" || bar.AdjustmentBasis != model.AdjustmentNone || bar.TradeDate <= previousDate {
					return len(bars), "", nil, errLiveProbeContract("daily K-line identity, adjustment or chronological order mismatch")
				}
				previousDate = bar.TradeDate
				switch bar.QualityStatus {
				case model.QualityValid:
				case model.QualityDegraded:
					quality = model.QualityDegraded
					for _, reason := range bar.QualityReasons {
						notes = appendTickflowUniqueReason(notes, reason)
					}
				default:
					return len(bars), "", nil, errLiveProbeContract("daily K-line quality is invalid or unknown")
				}
			}
			return len(bars), string(quality), notes, nil
		})
		if len(results) > 0 {
			results[len(results)-1].Validation = dailyValidation
			httpResponses := capture.snapshot()
			if len(httpResponses) > 0 {
				results[len(results)-1].DailyRaw = summarizeTickFlowDailyRawResponse(httpResponses[0].BodySample, httpResponses[0].BodyTruncated)
				for index := range httpResponses {
					httpResponses[index].BodySample = ""
				}
			}
			results[len(results)-1].HTTP = httpResponses
		}
	}
	if dailyOnly {
		if len(results) == 0 {
			t.Fatal("TickFlow daily-only probe did not produce a result")
		}
		result := results[len(results)-1]
		if result.Validation == nil || result.DailyRaw == nil || result.DailyRaw.BodyTruncated ||
			result.DailyRaw.TimestampCount != 8 || result.DailyRaw.OpenCount != 8 ||
			result.DailyRaw.HighCount != 8 || result.DailyRaw.LowCount != 8 ||
			result.DailyRaw.CloseCount != 8 || result.DailyRaw.VolumeCount != 8 ||
			result.DailyRaw.AmountCount != 8 {
			t.Fatalf("TickFlow daily-only probe must retain complete structured validation and raw field diagnostics: %+v", result)
		}
	}
	if !quoteOnly && !dailyOnly {
		run("index_quotes", func() (int, string, []string, error) {
			item, err := client.FetchIndexQuotes(ctx)
			if err != nil {
				return 0, "", nil, err
			}
			if item == nil || item.Source != "tickflow" || item.FetchedAt.IsZero() || len(item.Items) != len(tickflowCoreIndexDefinitions) {
				return 0, "", nil, errLiveProbeContract("core index snapshot coverage mismatch")
			}
			return len(item.Items), string(item.QualityStatus), item.QualityReasons, nil
		})
	}

	if !indexOnly && !quoteOnly && !dailyOnly {
		run("market_breadth", func() (int, string, []string, error) {
			item, err := client.FetchMarketBreadth(ctx)
			if err != nil {
				return 0, "", nil, err
			}
			if item == nil || item.Market != "cn_a_share" || item.Source != "tickflow" ||
				item.TotalSecurities <= 0 || item.CoveredSecurities != item.TotalSecurities ||
				item.AdvancingCount+item.DecliningCount+item.UnchangedCount+item.UnclassifiedCount != item.CoveredSecurities {
				return 0, "", nil, errLiveProbeContract("market breadth universe/coverage/count contract mismatch")
			}
			return item.CoveredSecurities, string(item.QualityStatus), item.QualityReasons, nil
		})

		run("sector_rankings_industry", func() (int, string, []string, error) {
			item, err := client.FetchSectorRankings(ctx, "industry", 10)
			if err != nil {
				return 0, "", nil, err
			}
			if item == nil || item.Category != "industry" || item.Source != "tickflow" ||
				item.Total <= 0 || len(item.Items) == 0 || item.FetchedAt.IsZero() {
				return 0, "", nil, errLiveProbeContract("industry ranking identity/coverage mismatch")
			}
			// 当前 TickFlow 行业涨幅由申万一级成分股行情聚合计算，预期为 degraded，不得标记为源端完整排名。
			if item.QualityStatus != model.QualityDegraded {
				return 0, "", nil, errLiveProbeContract("constituent-derived industry ranking must remain degraded")
			}
			return len(item.Items), string(item.QualityStatus), item.QualityReasons, nil
		})
	}
	encoded, _ := json.Marshal(results)
	t.Logf("LIVE_TICKFLOW_RESULT %s", encoded)
	if len(failures) > 0 {
		t.Fatalf("TickFlow live capability acceptance failed: %v", failures)
	}
}

type tickflowQuoteRawDiagnostic struct {
	Symbol    string          `json:"symbol,omitempty"`
	LastPrice json.RawMessage `json:"last_price,omitempty"`
	PrevClose json.RawMessage `json:"prev_close,omitempty"`
	Open      json.RawMessage `json:"open,omitempty"`
	High      json.RawMessage `json:"high,omitempty"`
	Low       json.RawMessage `json:"low,omitempty"`
	Volume    json.RawMessage `json:"volume,omitempty"`
	Amount    json.RawMessage `json:"amount,omitempty"`
	Timestamp json.RawMessage `json:"timestamp,omitempty"`
}

func errLiveProbeContract(message string) error {
	return &liveProbeContractError{message: message}
}

type liveProbeContractError struct {
	message string
}

func (e *liveProbeContractError) Error() string { return e.message }
