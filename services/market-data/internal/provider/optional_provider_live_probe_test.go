package provider

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

type optionalProviderProbeResult struct {
	Provider    string   `json:"provider"`
	Capability  string   `json:"capability"`
	Status      string   `json:"status"`
	ElapsedMS   int64    `json:"elapsed_ms"`
	Rows        int      `json:"rows,omitempty"`
	Quality     string   `json:"quality,omitempty"`
	QualityNote []string `json:"quality_notes,omitempty"`
	Error       string   `json:"error,omitempty"`
}

func optionalProbeDateRange(now time.Time) (string, string) {
	end := now.Format("2006-01-02")
	start := now.AddDate(0, 0, -30).Format("2006-01-02")
	return start, end
}

func logOptionalProviderProbe(t *testing.T, results []optionalProviderProbeResult, failures []string) {
	t.Helper()
	encoded, _ := json.Marshal(results)
	t.Logf("LIVE_OPTIONAL_PROVIDER_RESULT %s", encoded)
	if len(failures) > 0 {
		t.Fatalf("optional provider live acceptance failed: %v", failures)
	}
}

func TestLiveTushareCapabilities(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_TUSHARE_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_TUSHARE_TESTS=1 to call Tushare Pro endpoints")
	}
	token := os.Getenv("MARKET_DATA_TUSHARE_TOKEN")
	if token == "" {
		token = os.Getenv("TUSHARE_TOKEN")
	}
	if token == "" {
		t.Fatal("MARKET_DATA_LIVE_TUSHARE_TESTS=1 requires MARKET_DATA_TUSHARE_TOKEN or TUSHARE_TOKEN")
	}

	endpoint := os.Getenv("MARKET_DATA_TUSHARE_API_URL")
	if endpoint == "" {
		endpoint = os.Getenv("TUSHARE_HTTP_URL")
	}
	client := NewTushareConfigured(25*time.Second, endpoint, token)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	startDate, endDate := optionalProbeDateRange(now)
	const symbol = "600519.SH"
	results := make([]optionalProviderProbeResult, 0, 9)
	failures := make([]string, 0, 9)

	run := func(capability string, call func() (int, model.QualityStatus, []string, error)) {
		started := time.Now()
		rows, quality, notes, err := call()
		item := optionalProviderProbeResult{Provider: "tushare", Capability: capability, Status: "passed", ElapsedMS: time.Since(started).Milliseconds(), Rows: rows, Quality: string(quality), QualityNote: notes}
		if err != nil {
			_, safeMessage := liveProbeErrorSummary(err)
			item.Status, item.Error = "failed", safeMessage
			failures = append(failures, capability+": "+safeMessage)
		}
		results = append(results, item)
	}

	run("stock_basic", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchStockBasic(ctx, symbol)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Symbol != symbol || item.Source != "tushare" || item.FetchedAt.IsZero() {
			return 0, "", nil, errLiveProbeContract("stock_basic identity/source/fetched_at mismatch")
		}
		return 1, item.QualityStatus, item.QualityReasons, nil
	})

	calendarStart := now.AddDate(0, 0, -14).Format("2006-01-02")
	calendar, calendarErr := client.FetchTradingCalendar(ctx, calendarStart, endDate)
	run("trading_calendar", func() (int, model.QualityStatus, []string, error) {
		if calendarErr != nil {
			return 0, "", nil, calendarErr
		}
		if calendar == nil || calendar.Market != "cn_a_share" || calendar.StartDate != calendarStart ||
			calendar.EndDate != endDate || calendar.Source != "tushare" || calendar.FetchedAt.IsZero() {
			return 0, "", nil, errLiveProbeContract("trading_calendar identity/source/date-range/fetched_at mismatch")
		}
		start, startErr := time.Parse("2006-01-02", calendarStart)
		end, endErr := time.Parse("2006-01-02", endDate)
		if startErr != nil || endErr != nil {
			return 0, "", nil, errLiveProbeContract("trading_calendar probe date range invalid")
		}
		expectedDays := int(end.Sub(start)/(24*time.Hour)) + 1
		if calendar.TotalDays != expectedDays || len(calendar.Items) != expectedDays {
			return len(calendar.Items), "", nil, errLiveProbeContract("trading_calendar returned incomplete natural-day coverage")
		}
		return len(calendar.Items), calendar.QualityStatus, calendar.QualityReasons, nil
	})

	tradeDate := endDate
	if calendarErr == nil && calendar != nil {
		for i := len(calendar.Items) - 1; i >= 0; i-- {
			if calendar.Items[i].IsTradingDay {
				tradeDate = calendar.Items[i].Date
				break
			}
		}
	}

	run("daily_bars", func() (int, model.QualityStatus, []string, error) {
		bars, err := client.FetchDailyBars(ctx, symbol, startDate, endDate, 40, model.AdjustmentNone)
		if err != nil {
			return 0, "", nil, err
		}
		if len(bars) == 0 {
			return 0, "", nil, errLiveProbeContract("daily_bars returned no rows in the recent date range")
		}
		if message := liveProbeDailyContractError(bars, "tushare", symbol, model.AdjustmentNone, startDate, endDate, 40); message != "" {
			return len(bars), "", nil, errLiveProbeContract(message)
		}
		return len(bars), bars[len(bars)-1].QualityStatus, bars[len(bars)-1].QualityReasons, nil
	})

	run("security_trading_status", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchSecurityTradingStatus(ctx, symbol, tradeDate)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Symbol != symbol || item.TradeDate != tradeDate || item.Source != "tushare" || item.FetchedAt.IsZero() {
			return 0, "", nil, errLiveProbeContract("security_trading_status identity/source/fetched_at mismatch")
		}
		return 1, item.QualityStatus, item.QualityReasons, nil
	})

	run("fundamentals", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchFundamentals(ctx, symbol, 2025, 4)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Symbol != symbol || item.Year != 2025 || item.Quarter != 4 || item.Source != "tushare" || item.FetchedAt.IsZero() {
			return 0, "", nil, errLiveProbeContract("fundamentals identity/period/source/fetched_at mismatch")
		}
		return len(item.Sections), item.QualityStatus, item.QualityReasons, nil
	})

	run("valuations", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchValuations(ctx, []string{symbol})
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Source != "tushare" || item.FetchedAt.IsZero() || len(item.RequestedSymbols) != 1 || item.RequestedSymbols[0] != symbol {
			return 0, "", nil, errLiveProbeContract("valuations request/source/fetched_at mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})


	run("index_catalog", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchIndexCatalog(ctx, "tushare")
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Tag != "tushare" || item.Source != "tushare" || item.FetchedAt.IsZero() ||
			item.Total != len(item.Items) || len(item.Items) == 0 {
			return 0, "", nil, errLiveProbeContract("index_catalog identity/source/count/fetched_at mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("index_constituents", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchIndexConstituents(ctx, "000300.SH")
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.IndexSymbol != "000300.SH" || item.Source != "tushare" || item.FetchedAt.IsZero() ||
			item.ProviderReportDate == "" || item.Total != len(item.Items) || len(item.Items) == 0 {
			return 0, "", nil, errLiveProbeContract("index_constituents identity/report-date/count/fetched_at mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("index_daily_bars", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchIndexDailyBars(ctx, "000300.SH", startDate, endDate)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.IndexSymbol != "000300.SH" || item.Source != "tushare" || item.FetchedAt.IsZero() ||
			item.StartDate != startDate || item.EndDate != endDate || item.Total != len(item.Items) || len(item.Items) == 0 {
			return 0, "", nil, errLiveProbeContract("index_daily_bars identity/date-range/count/fetched_at mismatch")
		}
		for _, bar := range item.Items {
			if bar.IndexSymbol != "000300.SH" || bar.TradeDate < startDate || bar.TradeDate > endDate ||
				bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 || bar.Low > bar.High ||
				bar.Open < bar.Low || bar.Open > bar.High || bar.Close < bar.Low || bar.Close > bar.High {
				return len(item.Items), "", nil, errLiveProbeContract("index_daily_bars row contract mismatch")
			}
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})
	logOptionalProviderProbe(t, results, failures)
}

func TestLiveHithinkCapabilities(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_HITHINK_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_HITHINK_TESTS=1 to call Hithink Finance endpoints")
	}
	apiKey := os.Getenv("MARKET_DATA_HITHINK_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("HITHINK_FINANCE_API_KEY")
	}
	if apiKey == "" {
		t.Fatal("MARKET_DATA_LIVE_HITHINK_TESTS=1 requires MARKET_DATA_HITHINK_API_KEY or HITHINK_FINANCE_API_KEY")
	}
	baseURL := os.Getenv("MARKET_DATA_HITHINK_BASE_URL")
	client := NewHithinkConfigured(25*time.Second, baseURL, apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	startDate, endDate := optionalProbeDateRange(now)
	const symbol = "600519.SH"
	results := make([]optionalProviderProbeResult, 0, 12)
	failures := make([]string, 0, 12)

	run := func(capability string, call func() (int, model.QualityStatus, []string, error)) {
		started := time.Now()
		rows, quality, notes, err := call()
		item := optionalProviderProbeResult{Provider: "hithink", Capability: capability, Status: "passed", ElapsedMS: time.Since(started).Milliseconds(), Rows: rows, Quality: string(quality), QualityNote: notes}
		if err != nil {
			_, safeMessage := liveProbeErrorSummary(err)
			item.Status, item.Error = "failed", safeMessage
			failures = append(failures, capability+": "+safeMessage)
		}
		results = append(results, item)
	}

	run("quote", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchQuote(ctx, symbol)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Symbol != symbol || item.Source != "hithink" || item.FetchedAt.IsZero() || item.CurrentPrice == nil {
			return 0, "", nil, errLiveProbeContract("quote identity/source/fetched_at/current_price mismatch")
		}
		return 1, item.QualityStatus, item.QualityReasons, nil
	})

	run("daily_bars", func() (int, model.QualityStatus, []string, error) {
		bars, err := client.FetchDailyBars(ctx, symbol, startDate, endDate, 40, model.AdjustmentNone)
		if err != nil {
			return 0, "", nil, err
		}
		if len(bars) == 0 {
			return 0, "", nil, errLiveProbeContract("daily_bars returned no rows in the recent date range")
		}
		if message := liveProbeDailyContractError(bars, "hithink", symbol, model.AdjustmentNone, startDate, endDate, 40); message != "" {
			return len(bars), "", nil, errLiveProbeContract(message)
		}
		return len(bars), bars[len(bars)-1].QualityStatus, bars[len(bars)-1].QualityReasons, nil
	})

	run("index_quotes", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchIndexQuotes(ctx)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Source != "hithink" || item.FetchedAt.IsZero() || len(item.Items) != len(hithinkCoreIndexDefinitions) {
			return 0, "", nil, errLiveProbeContract("core index snapshot coverage mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("hot_rankings_popular", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchHotStocks(ctx, "popular", 20)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.RankingType != "popular" || item.Source != "hithink" || item.FetchedAt.IsZero() || len(item.Items) == 0 {
			return 0, "", nil, errLiveProbeContract("popular ranking identity/source/fetched_at/rows mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("valuations", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchValuations(ctx, []string{symbol})
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Source != "hithink" || item.FetchedAt.IsZero() || len(item.RequestedSymbols) != 1 || item.RequestedSymbols[0] != symbol {
			return 0, "", nil, errLiveProbeContract("valuations request/source/fetched_at mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})


	tradeDate := endDate
	run("trading_calendar", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchTradingCalendar(ctx, startDate, endDate)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Market != "cn_a_share" || item.Source != "hithink" ||
			item.StartDate != startDate || item.EndDate != endDate || item.FetchedAt.IsZero() ||
			item.TotalDays != len(item.Items) || len(item.Items) == 0 {
			return 0, "", nil, errLiveProbeContract("trading_calendar identity/date-range/count/fetched_at mismatch")
		}
		for i := len(item.Items) - 1; i >= 0; i-- {
			if item.Items[i].IsTradingDay {
				tradeDate = item.Items[i].Date
				break
			}
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("limit_up_pool", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchLimitPool(ctx, "up", tradeDate, 20)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.PoolType != "up" || item.TradeDate != tradeDate ||
			item.Source != "hithink" || item.FetchedAt.IsZero() || item.Total < len(item.Items) {
			return 0, "", nil, errLiveProbeContract("limit_up_pool identity/date/count/fetched_at mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("dragon_tiger", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchDragonTiger(ctx, tradeDate, 1, 20)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.TradeDate != tradeDate || item.Source != "hithink" ||
			item.FetchedAt.IsZero() || item.Total < len(item.Items) || item.Returned != len(item.Items) {
			return 0, "", nil, errLiveProbeContract("dragon_tiger identity/date/count/fetched_at mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	indexSymbol := ""
	run("index_catalog_industry", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchIndexCatalog(ctx, "industry")
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Tag != "industry" || item.Source != "hithink" ||
			item.FetchedAt.IsZero() || item.Total != len(item.Items) || len(item.Items) == 0 {
			return 0, "", nil, errLiveProbeContract("index_catalog identity/count/fetched_at mismatch")
		}
		indexSymbol = item.Items[0].Symbol
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("index_constituents", func() (int, model.QualityStatus, []string, error) {
		if indexSymbol == "" {
			return 0, "", nil, errLiveProbeContract("index_constituents skipped because index catalog was unavailable")
		}
		item, err := client.FetchIndexConstituents(ctx, indexSymbol)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.IndexSymbol != indexSymbol || item.Source != "hithink" ||
			item.FetchedAt.IsZero() || item.Total != len(item.Items) || len(item.Items) == 0 {
			return 0, "", nil, errLiveProbeContract("index_constituents identity/count/fetched_at mismatch")
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("index_daily_bars", func() (int, model.QualityStatus, []string, error) {
		if indexSymbol == "" {
			return 0, "", nil, errLiveProbeContract("index_daily_bars skipped because index catalog was unavailable")
		}
		item, err := client.FetchIndexDailyBars(ctx, indexSymbol, startDate, endDate)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.IndexSymbol != indexSymbol || item.Source != "hithink" ||
			item.FetchedAt.IsZero() || item.StartDate != startDate || item.EndDate != endDate ||
			item.Total != len(item.Items) || len(item.Items) == 0 {
			return 0, "", nil, errLiveProbeContract("index_daily_bars identity/date-range/count/fetched_at mismatch")
		}
		for _, bar := range item.Items {
			if bar.IndexSymbol != indexSymbol || bar.TradeDate < startDate || bar.TradeDate > endDate ||
				bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 || bar.Low > bar.High ||
				bar.Open < bar.Low || bar.Open > bar.High || bar.Close < bar.Low || bar.Close > bar.High {
				return len(item.Items), "", nil, errLiveProbeContract("index_daily_bars row contract mismatch")
			}
		}
		return len(item.Items), item.QualityStatus, item.QualityReasons, nil
	})

	run("fundamentals", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchFundamentals(ctx, symbol, 2025, 4)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Symbol != symbol || item.Year != 2025 || item.Quarter != 4 ||
			item.Source != "hithink" || item.FetchedAt.IsZero() {
			return 0, "", nil, errLiveProbeContract("fundamentals identity/period/source/fetched_at mismatch")
		}
		return len(item.Sections), item.QualityStatus, item.QualityReasons, nil
	})
	logOptionalProviderProbe(t, results, failures)
}

func TestLiveXueqiuHotRankings(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_XUEQIU_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_XUEQIU_TESTS=1 to call Xueqiu ranking endpoints")
	}
	cookie := os.Getenv("MARKET_DATA_XUEQIU_COOKIE")
	baseURL := os.Getenv("MARKET_DATA_XUEQIU_BASE_URL")
	client := NewXueqiuConfigured(25*time.Second, baseURL, cookie)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	results := make([]optionalProviderProbeResult, 0, 2)
	failures := make([]string, 0, 2)
	for _, rankingType := range []string{"popular", "surge"} {
		started := time.Now()
		item, err := client.FetchHotStocks(ctx, rankingType, 20)
		result := optionalProviderProbeResult{Provider: "xueqiu", Capability: "hot_rankings_" + rankingType, Status: "passed", ElapsedMS: time.Since(started).Milliseconds()}
		if err != nil {
			_, safeMessage := liveProbeErrorSummary(err)
			result.Status, result.Error = "failed", safeMessage
			failures = append(failures, rankingType+": "+safeMessage)
		} else if item == nil || item.RankingType != rankingType || item.Source != "xueqiu" || item.FetchedAt.IsZero() || len(item.Items) == 0 {
			result.Status, result.Error = "failed", "ranking identity/source/fetched_at/rows mismatch"
			failures = append(failures, rankingType+": "+result.Error)
		} else {
			result.Rows = len(item.Items)
			result.Quality = string(item.QualityStatus)
			result.QualityNote = item.QualityReasons
		}
		results = append(results, result)
	}
	logOptionalProviderProbe(t, results, failures)
}
