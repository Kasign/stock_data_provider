package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

const liveProbeBodySampleLimit = 2048

var liveProbeURLQuery = regexp.MustCompile(`(?i)(https?://[^\s"?]+)\?[^\s"']*`)

func liveProbeErrorSummary(err error) (string, string) {
	if err == nil {
		return "unknown", "provider returned an unspecified error"
	}
	kind := "provider_error"
	var networkErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		kind = "timeout"
	case errors.Is(err, context.Canceled):
		kind = "canceled"
	case errors.As(err, &networkErr) && networkErr.Timeout():
		kind = "timeout"
	default:
		lower := strings.ToLower(err.Error())
		switch {
		case strings.Contains(lower, "i/o timeout"),
			strings.Contains(lower, "timed out"),
			strings.Contains(lower, "timeout"):
			kind = "timeout"
		case strings.Contains(lower, "network is unreachable"),
			strings.Contains(lower, "no route to host"),
			strings.Contains(lower, "connection refused"),
			strings.Contains(lower, "dial tcp"):
			kind = "network_connect"
		case strings.Contains(lower, "eof"):
			kind = "connection_closed"
		case strings.Contains(lower, "http status"):
			kind = "http_status"
		case strings.Contains(lower, "decode"),
			strings.Contains(lower, "no parsable"),
			strings.Contains(lower, "empty"):
			kind = "empty_or_unparseable_response"
		}
	}
	message := liveProbeURLQuery.ReplaceAllString(err.Error(), `$1?[query redacted]`)
	if len(message) > 500 {
		message = message[:500] + "…"
	}
	return kind, message
}

type liveProbeReport struct {
	GeneratedAt string            `json:"generated_at"`
	Runner      string            `json:"runner"`
	QuoteSymbol string            `json:"quote_symbol"`
	DailySymbol string            `json:"daily_symbol"`
	StartDate   string            `json:"start_date"`
	EndDate     string            `json:"end_date"`
	Results     []liveProbeResult `json:"results"`
	Failures    []string          `json:"failures,omitempty"`
}

type liveProbeResult struct {
	Provider         string                           `json:"provider"`
	Capability       string                           `json:"capability"`
	SectorType       string                           `json:"sector_type,omitempty"`
	SectorRankings   *model.SectorRankingSnapshot     `json:"sector_rankings,omitempty"`
	IndexQuotes      *model.IndexQuoteSnapshot        `json:"index_quotes,omitempty"`
	MarketBreadth    *model.MarketBreadthSnapshot     `json:"market_breadth,omitempty"`
	LimitPool        *model.LimitPoolSnapshot         `json:"limit_pool,omitempty"`
	DragonTiger      *model.DragonTigerSnapshot       `json:"dragon_tiger,omitempty"`
	Announcements    *model.StockAnnouncementSnapshot `json:"announcements,omitempty"`
	StockBoards      *model.StockBoardSnapshot        `json:"stock_boards,omitempty"`
	TradingCalendar  *model.TradingCalendarSnapshot   `json:"trading_calendar,omitempty"`
	PoolType         string                           `json:"pool_type,omitempty"`
	Symbol           string                           `json:"symbol,omitempty"`
	Adjustment       model.AdjustmentBasis            `json:"adjustment,omitempty"`
	Status           string                           `json:"status"`
	ErrorKind        string                           `json:"error_kind,omitempty"`
	Error            string                           `json:"error,omitempty"`
	ElapsedMS        int64                            `json:"elapsed_ms"`
	Quote            *model.Quote                     `json:"quote,omitempty"`
	StockBasic       *model.StockBasic                `json:"stock_basic,omitempty"`
	StockSearch      *model.StockSearchSnapshot       `json:"stock_search,omitempty"`
	BarCount         int                              `json:"bar_count,omitempty"`
	SampleBars       []model.DailyBar                 `json:"sample_bars,omitempty"`
	ChipDistribution *model.ChipDistribution          `json:"chip_distribution,omitempty"`
	CapitalFlow      *model.CapitalFlow               `json:"capital_flow,omitempty"`
	HTTP             []liveProbeHTTPResponse          `json:"http_responses,omitempty"`
}

type liveProbeHTTPResponse struct {
	Endpoint          string   `json:"endpoint"`
	StatusCode        int      `json:"status_code,omitempty"`
	ContentType       string   `json:"content_type,omitempty"`
	TransportError    string   `json:"transport_error,omitempty"`
	RedirectEndpoint  string   `json:"redirect_endpoint,omitempty"`
	RedirectQueryKeys []string `json:"redirect_query_keys,omitempty"`
	BodyBytesObserved int64    `json:"body_bytes_observed,omitempty"`
	BodyTruncated     bool     `json:"body_truncated,omitempty"`
	BodySample        string   `json:"body_sample,omitempty"`
	rawSample         []byte
}

type liveProbeCapture struct {
	mu        sync.Mutex
	responses []*liveProbeHTTPResponse
}

func (c *liveProbeCapture) add(response *liveProbeHTTPResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responses = append(c.responses, response)
}

func (c *liveProbeCapture) snapshot() []liveProbeHTTPResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]liveProbeHTTPResponse, 0, len(c.responses))
	for _, response := range c.responses {
		copyOfResponse := *response
		copyOfResponse.rawSample = nil
		out = append(out, copyOfResponse)
	}
	return out
}

func (c *liveProbeCapture) transport() http.RoundTripper {
	return liveProbeCaptureTransport{capture: c, base: http.DefaultTransport}
}

type liveProbeCaptureTransport struct {
	capture *liveProbeCapture
	base    http.RoundTripper
}

func (t liveProbeCaptureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	// Never record RawQuery: provider URLs may include public tokens or request parameters.
	record := &liveProbeHTTPResponse{Endpoint: request.URL.Host + request.URL.EscapedPath()}
	t.capture.add(record)

	response, err := t.base.RoundTrip(request)
	if err != nil {
		_, message := liveProbeErrorSummary(err)
		record.TransportError = message
		return nil, err
	}
	record.StatusCode = response.StatusCode
	record.ContentType = response.Header.Get("Content-Type")
	if location := strings.TrimSpace(response.Header.Get("Location")); location != "" {
		if parsed, parseErr := url.Parse(location); parseErr == nil {
			redirectURL := request.URL.ResolveReference(parsed)
			record.RedirectEndpoint = redirectURL.Host + redirectURL.EscapedPath()
			for key := range redirectURL.Query() {
				record.RedirectQueryKeys = append(record.RedirectQueryKeys, key)
			}
			sort.Strings(record.RedirectQueryKeys)
		}
	}
	response.Body = &liveProbeCaptureBody{ReadCloser: response.Body, record: record}
	return response, nil
}

type liveProbeCaptureBody struct {
	io.ReadCloser
	record *liveProbeHTTPResponse
}

func (b *liveProbeCaptureBody) Read(buffer []byte) (int, error) {
	n, err := b.ReadCloser.Read(buffer)
	if n > 0 {
		b.record.BodyBytesObserved += int64(n)
		remaining := liveProbeBodySampleLimit - len(b.record.rawSample)
		if remaining > 0 {
			take := n
			if take > remaining {
				take = remaining
			}
			b.record.rawSample = append(b.record.rawSample, buffer[:take]...)
		}
	}
	return n, err
}

func (b *liveProbeCaptureBody) Close() error {
	err := b.ReadCloser.Close()
	b.record.BodySample = strings.ToValidUTF8(string(b.record.rawSample), "�")
	b.record.BodyTruncated = b.record.BodyBytesObserved > int64(len(b.record.rawSample))
	return err
}

func liveProbeDailyContractError(bars []model.DailyBar, providerName, symbol string, basis model.AdjustmentBasis, startDate, endDate string, limit int) string {
	if limit > 0 && len(bars) > limit {
		return fmt.Sprintf("provider returned %d bars above requested limit %d", len(bars), limit)
	}
	seenDates := make(map[string]struct{}, len(bars))
	for index, bar := range bars {
		if bar.Source != providerName || bar.Symbol != symbol {
			return fmt.Sprintf("bar %d identity mismatch: symbol=%q source=%q", index, bar.Symbol, bar.Source)
		}
		if bar.AdjustmentBasis != basis {
			return fmt.Sprintf("bar %d adjustment mismatch: requested=%q got=%q", index, basis, bar.AdjustmentBasis)
		}
		if _, err := time.Parse("2006-01-02", bar.TradeDate); err != nil {
			return fmt.Sprintf("bar %d has invalid trade date %q", index, bar.TradeDate)
		}
		if bar.TradeDate < startDate || bar.TradeDate > endDate {
			return fmt.Sprintf("bar %d trade date %s falls outside requested range %s..%s", index, bar.TradeDate, startDate, endDate)
		}
		if index > 0 && bar.TradeDate < bars[index-1].TradeDate {
			return fmt.Sprintf("bars are not sorted ascending at index %d: %s follows %s", index, bar.TradeDate, bars[index-1].TradeDate)
		}
		if _, exists := seenDates[bar.TradeDate]; exists {
			return fmt.Sprintf("duplicate trade date %s", bar.TradeDate)
		}
		seenDates[bar.TradeDate] = struct{}{}
		if bar.Volume == nil || *bar.Volume < 0 {
			return fmt.Sprintf("bar %s has missing or negative volume", bar.TradeDate)
		}
		prices := []float64{bar.Open, bar.High, bar.Low, bar.Close}
		for _, value := range prices {
			if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
				return fmt.Sprintf("bar %s has invalid OHLC value", bar.TradeDate)
			}
		}
		if bar.Low > bar.High || bar.Open < bar.Low || bar.Open > bar.High || bar.Close < bar.Low || bar.Close > bar.High {
			return fmt.Sprintf("bar %s violates OHLC range invariant", bar.TradeDate)
		}
		if bar.Amount == nil && bar.QualityStatus == model.QualityValid {
			return fmt.Sprintf("bar %s has no amount but quality is marked valid", bar.TradeDate)
		}
		if bar.Amount == nil && bar.RawAmountField != "" {
			return fmt.Sprintf("bar %s has no amount but raw amount field metadata is present", bar.TradeDate)
		}
	}
	return ""
}

func TestLiveProbeDailyContractRejectsLimitAndUnsortedBars(t *testing.T) {
	volume := int64(1000)
	makeBar := func(date string) model.DailyBar {
		return model.DailyBar{
			Symbol: "600036.SH", TradeDate: date, Period: "1d",
			Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: &volume, VolumeUnit: "shares", AmountUnit: "unknown",
			AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney",
			QualityStatus: model.QualityDegraded,
		}
	}
	older, newer := makeBar("2026-10-20"), makeBar("2026-10-21")
	const startDate, endDate = "2026-10-19", "2026-10-22"

	if got := liveProbeDailyContractError([]model.DailyBar{older, newer}, "eastmoney", "600036.SH", model.AdjustmentForward, startDate, endDate, 1); !strings.Contains(got, "above requested limit") {
		t.Fatalf("expected limit violation, got %q", got)
	}
	if got := liveProbeDailyContractError([]model.DailyBar{newer, older}, "eastmoney", "600036.SH", model.AdjustmentForward, startDate, endDate, 8); !strings.Contains(got, "not sorted ascending") {
		t.Fatalf("expected unsorted bar violation, got %q", got)
	}
	if got := liveProbeDailyContractError([]model.DailyBar{older, newer}, "eastmoney", "600036.SH", model.AdjustmentForward, startDate, endDate, 8); got != "" {
		t.Fatalf("valid ordered bars should pass the shared contract, got %q", got)
	}
}

func TestLiveProbeCaptureRedactsRedirectQueryValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://redirect.example.invalid/path?token=secret&symbol=600036&mode=full", http.StatusFound)
	}))
	defer server.Close()

	capture := &liveProbeCapture{}
	client := &http.Client{
		Transport:     capture.transport(),
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(server.URL + "/quote")
	if err != nil {
		t.Fatalf("GET redirect fixture: %v", err)
	}
	_ = resp.Body.Close()

	responses := capture.snapshot()
	if len(responses) != 1 {
		t.Fatalf("captured response count=%d, want 1", len(responses))
	}
	got := responses[0]
	if got.StatusCode != http.StatusFound {
		t.Fatalf("status=%d, want %d", got.StatusCode, http.StatusFound)
	}
	if got.RedirectEndpoint != "redirect.example.invalid/path" {
		t.Fatalf("redirect endpoint=%q", got.RedirectEndpoint)
	}
	if strings.Join(got.RedirectQueryKeys, ",") != "mode,symbol,token" {
		t.Fatalf("redirect query keys=%v", got.RedirectQueryKeys)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "600036") {
		t.Fatalf("redirect query values leaked into probe report: %s", encoded)
	}
}

func TestLiveProbeErrorSummaryRedactsURLQuery(t *testing.T) {
	err := fmt.Errorf(`tencent request: Get "https://example.invalid/quote?token=secret&symbol=600519": EOF`)
	kind, message := liveProbeErrorSummary(err)
	if kind != "connection_closed" {
		t.Fatalf("error kind = %q, want connection_closed", kind)
	}
	if strings.Contains(message, "secret") || strings.Contains(message, "symbol=600519") {
		t.Fatalf("provider URL query parameters leaked: %q", message)
	}
	if !strings.Contains(message, "https://example.invalid/quote?[query redacted]") {
		t.Fatalf("sanitized endpoint path missing: %q", message)
	}
}

func TestLiveProbeErrorSummaryClassifiesNetworkTimeout(t *testing.T) {
	err := fmt.Errorf("BaoStock login: receive response: %w", &net.OpError{
		Op:  "read",
		Net: "tcp",
		Err: os.ErrDeadlineExceeded,
	})
	kind, message := liveProbeErrorSummary(err)
	if kind != "timeout" {
		t.Fatalf("error kind = %q, want timeout", kind)
	}
	if !strings.Contains(message, "timeout") {
		t.Fatalf("timeout symptom was not preserved: %q", message)
	}
}

func TestLiveProbeErrorSummaryClassifiesNetworkUnavailable(t *testing.T) {
	kind, message := liveProbeErrorSummary(fmt.Errorf("dial tcp 61.129.129.199:443: connect: Network unreachable"))
	if kind != "network_connect" {
		t.Fatalf("error kind = %q, want network_connect", kind)
	}
	if !strings.Contains(message, "Network unreachable") {
		t.Fatalf("network symptom was not preserved: %q", message)
	}
}

func liveProbeTradingCalendarContractError(snapshot *model.TradingCalendarSnapshot, startDate, endDate string) string {
	if snapshot == nil {
		return "provider returned an empty trading-calendar snapshot"
	}
	start, startErr := time.Parse("2006-01-02", startDate)
	end, endErr := time.Parse("2006-01-02", endDate)
	if startErr != nil || endErr != nil || start.After(end) {
		return "invalid trading-calendar probe range"
	}
	expected := int(end.Sub(start)/(24*time.Hour)) + 1
	if snapshot.Market != "cn_a_share" || snapshot.StartDate != startDate || snapshot.EndDate != endDate {
		return "trading-calendar market/date-range identity mismatch"
	}
	if snapshot.TotalDays != expected || len(snapshot.Items) != expected {
		return fmt.Sprintf("trading-calendar coverage mismatch: total=%d items=%d expected=%d", snapshot.TotalDays, len(snapshot.Items), expected)
	}
	tradingDays := 0
	for index, item := range snapshot.Items {
		wantDate := start.AddDate(0, 0, index).Format("2006-01-02")
		if item.Date != wantDate {
			return fmt.Sprintf("trading-calendar row %d date mismatch: got %s want %s", index, item.Date, wantDate)
		}
		if item.IsTradingDay {
			tradingDays++
		}
	}
	if tradingDays != snapshot.TradingDays {
		return fmt.Sprintf("trading-calendar trading-days mismatch: got %d calculated %d", snapshot.TradingDays, tradingDays)
	}
	if snapshot.Source != "baostock" || snapshot.FetchedAt.IsZero() {
		return "trading-calendar source or fetched_at is missing"
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Sprintf("unexpected trading-calendar quality %q", snapshot.QualityStatus)
	}
	return ""
}

func TestLiveTradingCalendarContractRejectsGaps(t *testing.T) {
	snapshot := &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: "2026-10-22", EndDate: "2026-10-24",
		TotalDays: 3, TradingDays: 2, Source: "baostock", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded,
		Items: []model.TradingCalendarDay{
			{Date: "2026-10-22", IsTradingDay: true},
			{Date: "2026-10-24", IsTradingDay: false},
		},
	}
	if got := liveProbeTradingCalendarContractError(snapshot, "2026-10-22", "2026-10-24"); !strings.Contains(got, "coverage mismatch") {
		t.Fatalf("expected coverage mismatch, got %q", got)
	}
}

func liveProbeDragonTigerTradeDate(now time.Time, calendar *model.TradingCalendarSnapshot) string {
	today := now.Format("2006-01-02")
	best := ""
	if calendar != nil {
		for _, day := range calendar.Items {
			if day.IsTradingDay && day.Date < today && day.Date > best {
				best = day.Date
			}
		}
	}
	if best != "" {
		return best
	}
	// 当 BaoStock 日历探测不可用时，退回到上一个工作日；节假日的准确性仍以日历为准。
	candidate := now.AddDate(0, 0, -1)
	for candidate.Weekday() == time.Saturday || candidate.Weekday() == time.Sunday {
		candidate = candidate.AddDate(0, 0, -1)
	}
	return candidate.Format("2006-01-02")
}

func TestLiveProbeDragonTigerDateUsesLastTradingDay(t *testing.T) {
	now := time.Date(2026, 10, 26, 9, 50, 0, 0, time.FixedZone("CST", 8*60*60))
	calendar := &model.TradingCalendarSnapshot{Items: []model.TradingCalendarDay{
		{Date: "2026-10-23", IsTradingDay: true},
		{Date: "2026-10-24", IsTradingDay: false},
		{Date: "2026-10-25", IsTradingDay: false},
		{Date: "2026-10-26", IsTradingDay: true},
	}}
	if got := liveProbeDragonTigerTradeDate(now, calendar); got != "2026-10-23" {
		t.Fatalf("dragon-tiger trade date=%s, want 2026-10-23", got)
	}
	if got := liveProbeDragonTigerTradeDate(now, nil); got != "2026-10-23" {
		t.Fatalf("weekday fallback trade date=%s, want 2026-10-23", got)
	}
}

func TestLiveProviderProbe(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_PROVIDER_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_PROVIDER_TESTS=1 to call real public provider endpoints")
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	startDate := now.AddDate(0, 0, -35).Format("2006-01-02")
	endDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	quoteSymbol := strings.TrimSpace(os.Getenv("LIVE_PROBE_QUOTE_SYMBOL"))
	if quoteSymbol == "" {
		quoteSymbol = "600519.SH"
	}
	dailySymbol := "600036.SH"

	report := liveProbeReport{
		GeneratedAt: now.Format(time.RFC3339),
		Runner:      "GitHub Actions hosted runner",
		QuoteSymbol: quoteSymbol,
		DailySymbol: dailySymbol,
		StartDate:   startDate,
		EndDate:     endDate,
		Results:     make([]liveProbeResult, 0, 15),
	}
	var failures []string

	runQuote := func(providerName, expectedSource, symbol string, client *http.Client, call func(context.Context) (*model.Quote, error)) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		started := time.Now()
		quote, callErr := call(ctx)
		result := liveProbeResult{
			Provider: providerName, Capability: "quote", Symbol: symbol,
			ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot(),
		}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, providerName+" quote: "+result.ErrorKind+" ("+result.Error+")")
		case quote == nil:
			result.Status = "invalid"
			result.Error = "provider returned a nil quote without an error"
			failures = append(failures, providerName+" quote: nil result")
		case quote.Symbol != symbol || quote.Source != expectedSource:
			result.Status = "invalid"
			result.Quote = quote
			result.Error = fmt.Sprintf("identity mismatch: symbol=%q source=%q", quote.Symbol, quote.Source)
			failures = append(failures, providerName+" quote: identity mismatch")
		case quote.CurrentPrice == nil || quote.Volume == nil:
			result.Status = "invalid"
			result.Quote = quote
			result.Error = "quote response is missing current_price or volume"
			failures = append(failures, providerName+" quote: incomplete required fields")
		case math.IsNaN(*quote.CurrentPrice) || math.IsInf(*quote.CurrentPrice, 0) || *quote.CurrentPrice <= 0 || *quote.Volume < 0:
			result.Status = "invalid"
			result.Quote = quote
			result.Error = "quote response has an invalid price or volume"
			failures = append(failures, providerName+" quote: invalid numeric fields")
		case quote.Amount == nil && (quote.RawAmount != nil || quote.RawAmountField != ""):
			result.Status = "invalid"
			result.Quote = quote
			result.Error = "missing amount still has raw amount value/field metadata"
			failures = append(failures, providerName+" quote: inconsistent amount metadata")
		case quote.Amount == nil && quote.QualityStatus == model.QualityValid:
			result.Status = "invalid"
			result.Quote = quote
			result.Error = "amount is missing but quote quality is marked valid"
			failures = append(failures, providerName+" quote: missing amount not marked degraded")
		default:
			result.Status = "returned"
			result.Quote = quote
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runStockBasic := func(providerName, symbol string, client *http.Client, call func(context.Context) (*model.StockBasic, error)) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := call(ctx)
		result := liveProbeResult{Provider: providerName, Capability: "stock_basic", Symbol: symbol, ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot()}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, providerName+" stock_basic: "+result.ErrorKind+" ("+result.Error+")")
		case item == nil:
			result.Status, result.Error = "invalid", "provider returned an empty stock-basic response"
			failures = append(failures, providerName+" stock_basic: nil result")
		case item.Symbol != symbol || item.Source != providerName || strings.TrimSpace(item.Name) == "":
			result.Status, result.Error, result.StockBasic = "invalid", "stock-basic identity/name contract mismatch", item
			failures = append(failures, providerName+" stock_basic: contract mismatch")
		case item.QualityStatus != model.QualityValid:
			result.Status, result.Error, result.StockBasic = "invalid", "stock-basic quality must be valid for this identity endpoint", item
			failures = append(failures, providerName+" stock_basic: invalid quality")
		default:
			result.Status, result.StockBasic = "returned", item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runStockSearch := func(searchProvider StockSearchProvider, client *http.Client, query, expectedSymbol string, timeout time.Duration) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		started := time.Now()
		snapshot, callErr := searchProvider.SearchStocks(ctx, query, 10)
		providerName := searchProvider.Name()
		result := liveProbeResult{
			Provider: providerName, Capability: "stock_search", Symbol: expectedSymbol,
			ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot(),
		}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, providerName+" stock_search: "+result.ErrorKind+" ("+result.Error+")")
		case snapshot == nil || snapshot.Query != query || snapshot.Source != providerName || snapshot.FetchedAt.IsZero():
			result.Status, result.Error, result.StockSearch = "invalid", "stock-search snapshot identity/source/fetched_at mismatch", snapshot
			failures = append(failures, providerName+" stock_search: snapshot identity mismatch")
		case snapshot.Total < len(snapshot.Items) || len(snapshot.Items) == 0:
			result.Status, result.Error, result.StockSearch = "invalid", "stock-search total/items are inconsistent or empty", snapshot
			failures = append(failures, providerName+" stock_search: empty or inconsistent result")
		default:
			found := false
			for _, item := range snapshot.Items {
				if item.Symbol == expectedSymbol && item.Name != "" && item.Code != "" {
					found = true
					break
				}
			}
			if !found {
				result.Status, result.Error = "invalid", "known Chinese stock-name query did not return 600519.SH"
				failures = append(failures, providerName+" stock_search: expected 600519.SH was not returned")
			} else if snapshot.QualityStatus == model.QualityInvalid || snapshot.QualityStatus == model.QualityUnknown {
				result.Status, result.Error = "invalid", "stock-search result has unusable quality"
				failures = append(failures, providerName+" stock_search: unusable quality")
			} else {
				result.Status = "returned"
			}
			result.StockSearch = snapshot
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runStockAnnouncements := func(eastmoney *Eastmoney, symbol, startDate, endDate string) {
		capture := &liveProbeCapture{}
		eastmoney.client.Transport = capture.transport()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := eastmoney.FetchStockAnnouncements(ctx, symbol, startDate, endDate, 1, 20)
		result := liveProbeResult{Provider: eastmoney.Name(), Capability: "stock_announcements", Symbol: symbol, ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot()}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, "eastmoney stock_announcements: "+result.ErrorKind+" ("+result.Error+")")
		case item == nil || item.Symbol != symbol || item.StartDate != startDate || item.EndDate != endDate || item.Source != "eastmoney":
			result.Status, result.Error, result.Announcements = "invalid", "announcement snapshot identity mismatch", item
			failures = append(failures, "eastmoney stock_announcements: identity mismatch")
		case item.Total == 0 && !item.HasData && item.QualityStatus == model.QualityUnknown:
			result.Status, result.Announcements = "empty", item
		case item.Total < 0 || item.Pages < 1 || item.Returned != len(item.Items) || item.Returned == 0 || item.Total < item.Returned:
			result.Status, result.Error, result.Announcements = "invalid", "announcement page counters are inconsistent", item
			failures = append(failures, "eastmoney stock_announcements: page counters inconsistent")
		default:
			result.Status, result.Announcements = "returned", item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runStockBoards := func(providerName, symbol string, client *http.Client, call func(context.Context) (*model.StockBoardSnapshot, error)) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := call(ctx)
		result := liveProbeResult{Provider: providerName, Capability: "stock_boards", Symbol: symbol, ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot()}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, providerName+" stock_boards: "+result.ErrorKind+" ("+result.Error+")")
		case item == nil || item.Symbol != symbol || item.Source != providerName:
			result.Status, result.Error, result.StockBoards = "invalid", "stock-board symbol/source contract mismatch", item
			failures = append(failures, providerName+" stock_boards: contract mismatch")
		case !item.HasData || len(item.Items) == 0 || item.Total != len(item.Items):
			result.Status, result.Error, result.StockBoards = "invalid", "stock-board membership response is empty or incomplete", item
			failures = append(failures, providerName+" stock_boards: empty or incomplete result")
		default:
			result.Status, result.StockBoards = "returned", item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runSectorRankings := func(providerName, category string, client *http.Client, call func(context.Context) (*model.SectorRankingSnapshot, error)) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := call(ctx)
		result := liveProbeResult{
			Provider: providerName, Capability: "sector_rankings", SectorType: category,
			ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot(),
		}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, providerName+" sector_rankings/"+category+": "+result.ErrorKind+" ("+result.Error+")")
		case item == nil || item.Category != category || item.Source != providerName || len(item.Items) == 0:
			result.Status, result.Error, result.SectorRankings = "invalid", "sector-ranking category, source, or item contract mismatch", item
			failures = append(failures, providerName+" sector_rankings/"+category+": contract mismatch")
		default:
			result.Status, result.SectorRankings = "returned", item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runIndexQuotes := func(providerName string, client *http.Client, call func(context.Context) (*model.IndexQuoteSnapshot, error)) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := call(ctx)
		result := liveProbeResult{Provider: providerName, Capability: "index_quotes", ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot()}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, providerName+" index_quotes: "+result.ErrorKind+" ("+result.Error+")")
		case item == nil || len(item.Items) == 0 || item.Source != providerName:
			result.Status, result.Error, result.IndexQuotes = "invalid", "index-quote source/item contract mismatch", item
			failures = append(failures, providerName+" index_quotes: contract mismatch")
		default:
			result.Status, result.IndexQuotes = "returned", item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runMarketBreadth := func(eastmoney *Eastmoney) {
		capture := &liveProbeCapture{}
		eastmoney.client.Transport = capture.transport()
		ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := eastmoney.FetchMarketBreadth(ctx)
		result := liveProbeResult{Provider: eastmoney.Name(), Capability: "market_breadth", ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot()}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, "eastmoney market_breadth: "+result.ErrorKind+" ("+result.Error+")")
		case item == nil:
			result.Status, result.Error = "invalid", "market-breadth provider returned nil"
			failures = append(failures, "eastmoney market_breadth: nil snapshot")
		case item.Source != "eastmoney" || item.TotalSecurities <= 0 || item.CoveredSecurities != item.TotalSecurities || item.AdvancingCount+item.DecliningCount+item.UnchangedCount+item.UnclassifiedCount != item.CoveredSecurities:
			result.Status, result.Error, result.MarketBreadth = "invalid", "market-breadth coverage/count contract mismatch", item
			failures = append(failures, "eastmoney market_breadth: coverage/count contract mismatch")
		default:
			result.Status, result.MarketBreadth = "returned", item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}
	runLimitPool := func(eastmoney *Eastmoney, poolType, tradeDate string) {
		capture := &liveProbeCapture{}
		eastmoney.client.Transport = capture.transport()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := eastmoney.FetchLimitPool(ctx, poolType, tradeDate, 30)
		result := liveProbeResult{Provider: eastmoney.Name(), Capability: "limit_pool", PoolType: poolType, ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot()}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, "eastmoney limit_pool/"+poolType+": "+result.ErrorKind+" ("+result.Error+")")
		case item == nil || item.PoolType != poolType || item.TradeDate != tradeDate || item.Source != "eastmoney":
			result.Status, result.Error, result.LimitPool = "invalid", "limit-pool identity contract mismatch", item
			failures = append(failures, "eastmoney limit_pool/"+poolType+": identity mismatch")
		case item.HasData && len(item.Items) == 0:
			result.Status, result.Error, result.LimitPool = "invalid", "has_data=true but item list is empty", item
			failures = append(failures, "eastmoney limit_pool/"+poolType+": empty items despite has_data")
		case !item.HasData:
			// Empty may mean non-trading date or unavailable source, never a verified zero count.
			result.Status, result.LimitPool = "empty", item
		default:
			result.Status, result.LimitPool = "returned", item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runDragonTiger := func(eastmoney *Eastmoney, tradeDate string) {
		capture := &liveProbeCapture{}
		eastmoney.client.Transport = capture.transport()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := eastmoney.FetchDragonTiger(ctx, tradeDate, 1, 100)
		result := liveProbeResult{Provider: eastmoney.Name(), Capability: "dragon_tiger", ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot()}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, "eastmoney dragon_tiger: "+result.ErrorKind+" ("+result.Error+")")
		case item == nil || item.TradeDate != tradeDate || item.Source != "eastmoney":
			result.Status, result.Error, result.DragonTiger = "invalid", "dragon-tiger date/source contract mismatch", item
			failures = append(failures, "eastmoney dragon_tiger: identity mismatch")
		case item.Total < item.SourceReturned || item.SourceReturned != item.Returned+item.ExcludedNonAShareCount || item.Returned != len(item.Items):
			result.Status, result.Error, result.DragonTiger = "invalid", "dragon-tiger source/accepted/excluded count mismatch", item
			failures = append(failures, "eastmoney dragon_tiger: source/accepted/excluded count mismatch")
		case !item.HasData:
			// 当前源分页可能只含非 A 股记录；保持 unknown，不把它归为确定的零上榜。
			result.Status, result.DragonTiger = "empty", item
		case item.Total < item.Returned || item.Returned == 0 || item.Pages < 1:
			result.Status, result.Error, result.DragonTiger = "invalid", "dragon-tiger page contract mismatch", item
			failures = append(failures, "eastmoney dragon_tiger: incomplete page contract")
		default:
			result.Status, result.DragonTiger = "returned", item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runDaily := func(providerName, symbol string, basis model.AdjustmentBasis, client *http.Client, call func(context.Context) ([]model.DailyBar, error)) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		started := time.Now()
		bars, callErr := call(ctx)
		result := liveProbeResult{
			Provider: providerName, Capability: "daily_bars", Symbol: symbol,
			Adjustment: basis, ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot(),
			BarCount: len(bars),
		}
		contractError := ""
		if callErr == nil && len(bars) > 0 {
			contractError = liveProbeDailyContractError(bars, providerName, symbol, basis, startDate, endDate, 8)
		}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, providerName+" daily_bars "+string(basis)+": "+result.ErrorKind+" ("+result.Error+")")
		case len(bars) == 0:
			result.Status = "invalid"
			result.Error = "provider returned an empty bar series without an error"
			failures = append(failures, providerName+" daily_bars "+string(basis)+": empty result")
		case contractError != "":
			result.Status = "invalid"
			result.Error = contractError
			failures = append(failures, providerName+" daily_bars "+string(basis)+": "+contractError)
			result.SampleBars = append([]model.DailyBar(nil), bars...)
		default:
			result.Status = "returned"
			// Providers normally return rows in ascending date order. Keep the newest three samples in the report.
			first := len(bars) - 3
			if first < 0 {
				first = 0
			}
			result.SampleBars = append([]model.DailyBar(nil), bars[first:]...)
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runChip := func(mx *MiaoXiang, symbol string) {
		capture := &liveProbeCapture{}
		mx.client.Transport = capture.transport()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := mx.FetchChipDistribution(ctx, symbol)
		result := liveProbeResult{
			Provider: mx.Name(), Capability: "chip_distribution", Symbol: symbol,
			ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot(),
		}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, "miaoxiang chip_distribution: "+result.ErrorKind+" ("+result.Error+")")
		case item == nil:
			result.Status = "invalid"
			result.Error = "MX API returned a nil chip distribution without an error"
			failures = append(failures, "miaoxiang chip_distribution: nil result")
		case item.Symbol != symbol || item.Source != mx.Name() || !finiteNumber(item.AverageCost) || item.AverageCost <= 0:
			result.Status = "invalid"
			result.ChipDistribution = item
			result.Error = "MX chip distribution identity or average-cost contract is invalid"
			failures = append(failures, "miaoxiang chip_distribution: contract mismatch")
		case item.QualityStatus == model.QualityInvalid:
			result.Status = "invalid"
			result.ChipDistribution = item
			result.Error = "MX chip distribution was marked invalid quality"
			failures = append(failures, "miaoxiang chip_distribution: invalid quality")
		default:
			// 妙想缺少明确可用时间，degraded 是预期结果，不能要求它伪装成 valid。
			result.Status = "returned"
			result.ChipDistribution = item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runFlow := func(mx *MiaoXiang, symbol string) {
		capture := &liveProbeCapture{}
		mx.client.Transport = capture.transport()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		started := time.Now()
		item, callErr := mx.FetchCapitalFlow(ctx, symbol)
		result := liveProbeResult{
			Provider: mx.Name(), Capability: "capital_flow", Symbol: symbol,
			ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot(),
		}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, "miaoxiang capital_flow: "+result.ErrorKind+" ("+result.Error+")")
		case item == nil:
			result.Status = "invalid"
			result.Error = "MX API returned a nil capital-flow response without an error"
			failures = append(failures, "miaoxiang capital_flow: nil result")
		case item.Symbol != symbol || item.Source != mx.Name() || item.Observations < 0:
			result.Status = "invalid"
			result.CapitalFlow = item
			result.Error = "MX capital-flow identity or observation count is invalid"
			failures = append(failures, "miaoxiang capital_flow: contract mismatch")
		case item.QualityStatus == model.QualityInvalid:
			result.Status = "invalid"
			result.CapitalFlow = item
			result.Error = "MX capital flow was marked invalid quality"
			failures = append(failures, "miaoxiang capital_flow: invalid quality")
		case item.MainNetInflow != nil && !finiteNumber(*item.MainNetInflow),
			item.Inflow5D != nil && !finiteNumber(*item.Inflow5D),
			item.Inflow10D != nil && !finiteNumber(*item.Inflow10D):
			result.Status = "invalid"
			result.CapitalFlow = item
			result.Error = "MX capital flow contains a non-finite amount"
			failures = append(failures, "miaoxiang capital_flow: non-finite amount")
		default:
			result.Status = "returned"
			result.CapitalFlow = item
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runMX := strings.TrimSpace(os.Getenv("MARKET_DATA_LIVE_MX_TESTS")) == "1"
	mxAPIKey := strings.TrimSpace(os.Getenv("MARKET_DATA_MX_API_KEY"))
	if mxAPIKey == "" {
		mxAPIKey = strings.TrimSpace(os.Getenv("MX_APIKEY"))
	}
	if runMX {
		if mxAPIKey == "" {
			failures = append(failures, "miaoxiang: live test explicitly enabled but MX API key is missing")
			report.Results = append(report.Results,
				liveProbeResult{Provider: "miaoxiang", Capability: "chip_distribution", Symbol: dailySymbol, Status: "failed", Error: "MX API key is missing"},
				liveProbeResult{Provider: "miaoxiang", Capability: "capital_flow", Symbol: dailySymbol, Status: "failed", Error: "MX API key is missing"})
		} else if mx, mxErr := NewMiaoXiang(mxAPIKey, strings.TrimSpace(os.Getenv("MARKET_DATA_MX_API_URL")), 20*time.Second); mxErr != nil {
			failures = append(failures, "miaoxiang: configure live probe failed: "+mxErr.Error())
			report.Results = append(report.Results,
				liveProbeResult{Provider: "miaoxiang", Capability: "chip_distribution", Symbol: dailySymbol, Status: "failed", Error: "MX API configuration rejected"},
				liveProbeResult{Provider: "miaoxiang", Capability: "capital_flow", Symbol: dailySymbol, Status: "failed", Error: "MX API configuration rejected"})
		} else {
			runChip(mx, dailySymbol)
			runFlow(mx, dailySymbol)
		}
	} else {
		// MX API 有独立 Key/调用配额，普通 PR CI 明确记录为 skipped，不偷偷消耗线上请求额度。
		report.Results = append(report.Results,
			liveProbeResult{Provider: "miaoxiang", Capability: "chip_distribution", Symbol: dailySymbol, Status: "skipped", Error: "set MARKET_DATA_LIVE_MX_TESTS=1 and provide MX_APIKEY to run the opt-in live probe"},
			liveProbeResult{Provider: "miaoxiang", Capability: "capital_flow", Symbol: dailySymbol, Status: "skipped", Error: "set MARKET_DATA_LIVE_MX_TESTS=1 and provide MX_APIKEY to run the opt-in live probe"})
	}

	sina := NewSina(12*time.Second, "")
	runQuote("sina", "sina", quoteSymbol, sina.client, func(ctx context.Context) (*model.Quote, error) {
		return sina.FetchQuote(ctx, quoteSymbol)
	})
	runDaily("sina", dailySymbol, model.AdjustmentNone, sina.client, func(ctx context.Context) ([]model.DailyBar, error) {
		return sina.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentNone)
	})

	tencent := NewTencent(12 * time.Second)
	runQuote("tencent", "tencent", quoteSymbol, tencent.client, func(ctx context.Context) (*model.Quote, error) {
		return tencent.FetchQuote(ctx, quoteSymbol)
	})
	runDaily("tencent", dailySymbol, model.AdjustmentNone, tencent.client, func(ctx context.Context) ([]model.DailyBar, error) {
		return tencent.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentNone)
	})
	runDaily("tencent", dailySymbol, model.AdjustmentForward, tencent.client, func(ctx context.Context) ([]model.DailyBar, error) {
		return tencent.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentForward)
	})

	eastmoney := NewEastmoney(12*time.Second, "", "")
	runStockBasic("eastmoney", dailySymbol, eastmoney.client, func(ctx context.Context) (*model.StockBasic, error) {
		return eastmoney.FetchStockBasic(ctx, dailySymbol)
	})
	tencentSearch := NewTencent(15 * time.Second)
	runStockSearch(tencentSearch, tencentSearch.client, "贵州茅台", "600519.SH", 15*time.Second)
	runStockSearch(eastmoney, eastmoney.client, "贵州茅台", "600519.SH", 4*time.Minute)
	announcementStart := now.AddDate(0, 0, -7).Format("2006-01-02")
	announcementEnd := now.AddDate(0, 0, -1).Format("2006-01-02")
	runStockAnnouncements(eastmoney, dailySymbol, announcementStart, announcementEnd)
	runStockBoards("eastmoney", dailySymbol, eastmoney.client, func(ctx context.Context) (*model.StockBoardSnapshot, error) {
		return eastmoney.FetchStockBoards(ctx, dailySymbol)
	})
	runSectorRankings("eastmoney", "industry", eastmoney.client, func(ctx context.Context) (*model.SectorRankingSnapshot, error) {
		return eastmoney.FetchSectorRankings(ctx, "industry", 10)
	})
	runSectorRankings("eastmoney", "concept", eastmoney.client, func(ctx context.Context) (*model.SectorRankingSnapshot, error) {
		return eastmoney.FetchSectorRankings(ctx, "concept", 10)
	})
	runIndexQuotes("eastmoney", eastmoney.client, func(ctx context.Context) (*model.IndexQuoteSnapshot, error) {
		return eastmoney.FetchIndexQuotes(ctx)
	})
	runMarketBreadth(eastmoney)
	for _, poolType := range []string{"up", "down", "broken"} {
		runLimitPool(eastmoney, poolType, now.Format("2006-01-02"))
	}
	runQuote("eastmoney", "eastmoney", dailySymbol, eastmoney.client, func(ctx context.Context) (*model.Quote, error) {
		return eastmoney.FetchQuote(ctx, dailySymbol)
	})
	eastmoneyDelay := NewEastmoneyConfigured(12*time.Second, "https://push2delay.eastmoney.com/api/qt/stock/get", "", "")
	runQuote("eastmoney-delay", "eastmoney", dailySymbol, eastmoneyDelay.client, func(ctx context.Context) (*model.Quote, error) {
		return eastmoneyDelay.FetchQuote(ctx, dailySymbol)
	})
	runDaily("eastmoney", dailySymbol, model.AdjustmentForward, eastmoney.client, func(ctx context.Context) ([]model.DailyBar, error) {
		return eastmoney.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentForward)
	})

	baostockClient := baostock.NewClient(baostock.Config{ConnectTimeout: 8 * time.Second, RequestTimeout: 20 * time.Second})
	baostockProvider := NewBaostock(baostockClient)
	beforeBaoStockDaily := len(report.Results)
	runDaily("baostock", dailySymbol, model.AdjustmentForward, nil, func(ctx context.Context) ([]model.DailyBar, error) {
		return baostockProvider.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentForward)
	})
	dailyProbePassed := len(report.Results) > beforeBaoStockDaily &&
		report.Results[len(report.Results)-1].Provider == "baostock" &&
		report.Results[len(report.Results)-1].Capability == "daily_bars" &&
		report.Results[len(report.Results)-1].Status == "returned"
	calendarStart := now.AddDate(0, 0, -14).Format("2006-01-02")
	calendarEnd := now.Format("2006-01-02")
	calendarResult := liveProbeResult{Provider: "baostock", Capability: "trading_calendar"}
	if dailyProbePassed {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		started := time.Now()
		snapshot, callErr := baostockProvider.FetchTradingCalendar(ctx, calendarStart, calendarEnd)
		cancel()
		calendarResult.ElapsedMS = time.Since(started).Milliseconds()
		calendarResult.TradingCalendar = snapshot
		switch {
		case callErr != nil:
			calendarResult.Status = "failed"
			calendarResult.ErrorKind, calendarResult.Error = liveProbeErrorSummary(callErr)
			failures = append(failures, "baostock trading_calendar: "+calendarResult.ErrorKind+" ("+calendarResult.Error+")")
		case liveProbeTradingCalendarContractError(snapshot, calendarStart, calendarEnd) != "":
			calendarResult.Status = "invalid"
			calendarResult.Error = liveProbeTradingCalendarContractError(snapshot, calendarStart, calendarEnd)
			failures = append(failures, "baostock trading_calendar: "+calendarResult.Error)
		default:
			calendarResult.Status = "returned"
		}
	} else {
		calendarResult.Status = "skipped"
		calendarResult.Error = "skipped because BaoStock daily-bar probe could not establish a working TCP session; no duplicate timeout request was sent"
	}
	report.Results = append(report.Results, calendarResult)
	runDragonTiger(eastmoney, liveProbeDragonTigerTradeDate(now, calendarResult.TradingCalendar))
	calendarJSON, _ := json.Marshal(calendarResult)
	t.Logf("LIVE_PROVIDER_RESULT %s", calendarJSON)
	yahoo := NewYahooFinance(12*time.Second, "")
	runQuote("yahoo", "yahoo", quoteSymbol, yahoo.client, func(ctx context.Context) (*model.Quote, error) {
		return yahoo.FetchQuote(ctx, quoteSymbol)
	})
	runDaily("yahoo", dailySymbol, model.AdjustmentForward, yahoo.client, func(ctx context.Context) ([]model.DailyBar, error) {
		return yahoo.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentForward)
	})

	if os.Getenv("MARKET_DATA_LIVE_NETEASE_TESTS") == "1" {
		netease := NewNetease(12*time.Second, "")
		runQuote("netease", "netease", quoteSymbol, netease.client, func(ctx context.Context) (*model.Quote, error) {
			return netease.FetchQuote(ctx, quoteSymbol)
		})
		runDaily("netease", dailySymbol, model.AdjustmentNone, netease.client, func(ctx context.Context) ([]model.DailyBar, error) {
			return netease.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentNone)
		})
	} else {
		report.Results = append(report.Results,
			liveProbeResult{Provider: "netease", Capability: "quote", Symbol: quoteSymbol, Status: "skipped", Error: "NetEase quote endpoint DNS resolution failed in hosted runners; set MARKET_DATA_LIVE_NETEASE_TESTS=1 to retry"},
			liveProbeResult{Provider: "netease", Capability: "daily_bars", Symbol: dailySymbol, Adjustment: model.AdjustmentNone, Status: "skipped", Error: "NetEase historical CSV endpoint returned HTTP 502; set MARKET_DATA_LIVE_NETEASE_TESTS=1 to retry"},
		)
	}

	if closeErr := baostockClient.Close(); closeErr != nil {
		t.Logf("LIVE_PROVIDER_CLEANUP warning provider=baostock error=%q", closeErr.Error())
	}

	report.Failures = failures
	encoded, marshalErr := json.MarshalIndent(report, "", "  ")
	if marshalErr != nil {
		t.Errorf("marshal live provider report: %v", marshalErr)
	} else {
		t.Logf("LIVE_PROVIDER_REPORT_BEGIN\n%s\nLIVE_PROVIDER_REPORT_END", encoded)
		if outputPath := strings.TrimSpace(os.Getenv("LIVE_PROVIDER_REPORT_PATH")); outputPath != "" {
			if writeErr := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); writeErr != nil {
				t.Errorf("write live provider report: %v", writeErr)
			}
		}
	}
	if len(failures) > 0 {
		t.Errorf("%d live provider probe(s) failed; inspect LIVE_PROVIDER_RESULT records and the uploaded report artifact", len(failures))
	}
}
