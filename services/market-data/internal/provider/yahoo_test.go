package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func yahooFixture(t *testing.T, includeAdjusted bool, reportedSymbol string) (*httptest.Server, []int64) {
	t.Helper()
	loc := yahooChinaLocation()
	timestamps := []int64{
		time.Date(2026, 10, 20, 0, 0, 0, 0, loc).Unix(),
		time.Date(2026, 10, 21, 0, 0, 0, 0, loc).Unix(),
	}
	indicators := map[string]any{
		"quote": []any{map[string]any{
			"open":   []float64{10, 11},
			"high":   []float64{11, 12},
			"low":    []float64{9, 10},
			"close":  []float64{10, 11},
			"volume": []int64{1000, 2000},
		}},
	}
	if includeAdjusted {
		indicators["adjclose"] = []any{map[string]any{
			"adjclose": []float64{9, 10.45},
		}}
	}
	payload := map[string]any{
		"chart": map[string]any{
			"result": []any{map[string]any{
				"meta":       map[string]any{"symbol": reportedSymbol, "exchangeTimezoneName": "Asia/Shanghai"},
				"timestamp":  timestamps,
				"indicators": indicators,
			}},
			"error": nil,
		},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/600036.SS") {
			t.Errorf("unexpected Yahoo symbol path %q", r.URL.Path)
		}
		query := r.URL.Query()
		if query.Get("interval") != "1d" || query.Get("events") != "div,splits" {
			t.Errorf("unexpected Yahoo query parameters %v", query)
		}
		if _, err := strconv.ParseInt(query.Get("period1"), 10, 64); err != nil {
			t.Errorf("period1 must be Unix seconds: %v", err)
		}
		if _, err := strconv.ParseInt(query.Get("period2"), 10, 64); err != nil {
			t.Errorf("period2 must be Unix seconds: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Errorf("encode fixture: %v", err)
		}
	}))
	return server, timestamps
}


func yahooQuoteFixture(t *testing.T, reportedSymbol string, includeOpen bool) ([]byte, []int64) {
	t.Helper()
	loc := yahooChinaLocation()
	timestamps := []int64{
		time.Date(2026, 10, 23, 9, 30, 0, 0, loc).Unix(),
		time.Date(2026, 10, 23, 9, 31, 0, 0, loc).Unix(),
	}
	var openValues []*float64
	if includeOpen {
		first, second := 41.70, 42.10
		openValues = []*float64{&first, &second}
	} else {
		openValues = []*float64{nil, nil}
	}
	payload := map[string]any{
		"chart": map[string]any{
			"result": []any{map[string]any{
				"meta": map[string]any{
					"symbol": reportedSymbol,
					"longName": "China Merchants Bank Co., Ltd.",
					"shortName": "CHINA MERCHANTS BANK",
					"regularMarketTime": timestamps[1],
					"regularMarketPrice": 42.35,
					"regularMarketDayHigh": 42.38,
					"regularMarketDayLow": 41.57,
					"regularMarketVolume": int64(56829700),
					"chartPreviousClose": 41.82,
				},
				"timestamp": timestamps,
				"indicators": map[string]any{
					"quote": []any{map[string]any{
						"open": openValues,
					}},
				},
			}},
			"error": nil,
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return body, timestamps
}

func TestYahooFinanceFetchQuoteParsesIntradayChart(t *testing.T) {
	payload, timestamps := yahooQuoteFixture(t, "600036.SS", true)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/600036.SS") {
			t.Errorf("unexpected Yahoo quote path %q", r.URL.Path)
		}
		if r.URL.Query().Get("range") != "1d" || r.URL.Query().Get("interval") != "1m" {
			t.Errorf("Yahoo quote must request intraday one-day chart, got %v", r.URL.Query())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	p := NewYahooFinance(2*time.Second, server.URL)
	q, err := p.FetchQuote(context.Background(), "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if q.Symbol != "600036.SH" || q.Name != "China Merchants Bank Co., Ltd." || q.Source != "yahoo" {
		t.Fatalf("unexpected quote identity: %+v", q)
	}
	if q.CurrentPrice == nil || math.Abs(*q.CurrentPrice-42.35) > 1e-8 ||
		q.Open == nil || math.Abs(*q.Open-41.70) > 1e-8 ||
		q.High == nil || math.Abs(*q.High-42.38) > 1e-8 ||
		q.Low == nil || math.Abs(*q.Low-41.57) > 1e-8 ||
		q.PrevClose == nil || math.Abs(*q.PrevClose-41.82) > 1e-8 {
		t.Fatalf("unexpected Yahoo quote OHLC: %+v", q)
	}
	if q.Change == nil || math.Abs(*q.Change-0.53) > 1e-8 ||
		q.ChangePercent == nil || math.Abs(*q.ChangePercent-(0.53/41.82*100)) > 1e-8 {
		t.Fatalf("unexpected change fields: %+v", q)
	}
	if q.Volume == nil || *q.Volume != 56829700 || q.VolumeUnit != "shares" {
		t.Fatalf("Yahoo realtime volume should be shares: %+v", q)
	}
	if q.Amount != nil || q.AmountUnit != "unknown" || q.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing turnover amount must be degraded, not inferred: %+v", q)
	}
	if q.QuoteTime == nil || q.QuoteTime.Unix() != timestamps[1] || q.QuoteTimeSemantics != "provider_snapshot_time" {
		t.Fatalf("missing or incorrect provider timestamp: %+v", q)
	}
}

func TestYahooFinanceFetchQuoteRejectsWrongTickerAndMissingIntradayOpen(t *testing.T) {
	for name, fixture := range map[string]struct {
		symbol string
		open   bool
	}{
		"ticker mismatch": {"600000.SS", true},
		"missing open":   {"600036.SS", false},
	} {
		t.Run(name, func(t *testing.T) {
			payload, _ := yahooQuoteFixture(t, fixture.symbol, fixture.open)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write(payload)
			}))
			defer server.Close()
			p := NewYahooFinance(2*time.Second, server.URL)
			if _, err := p.FetchQuote(context.Background(), "600036.SH"); err == nil {
				t.Fatal("expected invalid Yahoo quote response to be rejected")
			}
		})
	}
}

func TestYahooFinanceForwardAdjustmentIsRebasedToRequestedEnd(t *testing.T) {
	server, _ := yahooFixture(t, true, "600036.SS")
	defer server.Close()
	p := NewYahooFinance(2*time.Second, server.URL+"/v8/finance/chart")
	bars, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 {
		t.Fatalf("bars=%d, want 2", len(bars))
	}
	if bars[0].TradeDate != "2026-10-20" || bars[1].TradeDate != "2026-10-21" {
		t.Fatalf("unexpected trade dates: %s, %s", bars[0].TradeDate, bars[1].TradeDate)
	}
	wantFirstClose := 10.0 * (0.9 / 0.95)
	if math.Abs(bars[0].Close-wantFirstClose) > 1e-8 || math.Abs(bars[1].Close-11) > 1e-8 {
		t.Fatalf("forward-adjusted close values = [%v,%v], want [%v,11]", bars[0].Close, bars[1].Close, wantFirstClose)
	}
	if bars[0].AdjustmentMethod != "yahoo_adjclose_ratio_rebased_to_end" || bars[0].AdjustmentRefDate != "2026-10-21" {
		t.Fatalf("missing adjustment method/reference metadata: %+v", bars[0])
	}
	if bars[0].Amount != nil || bars[0].AmountUnit != "unknown" || bars[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("missing turnover amount must be explicitly degraded: %+v", bars[0])
	}
	if *bars[0].Volume != 1000 || bars[0].VolumeUnit != "shares" {
		t.Fatalf("unexpected normalized volume metadata: %+v", bars[0])
	}
}

func TestYahooFinanceBackwardAdjustmentIsRebasedToRequestedStart(t *testing.T) {
	server, _ := yahooFixture(t, true, "600036.SS")
	defer server.Close()
	p := NewYahooFinance(2*time.Second, server.URL)
	bars, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentBackward)
	if err != nil {
		t.Fatal(err)
	}
	wantSecondClose := 11.0 * (0.95 / 0.9)
	if len(bars) != 2 || math.Abs(bars[0].Close-10) > 1e-8 || math.Abs(bars[1].Close-wantSecondClose) > 1e-8 {
		t.Fatalf("backward-adjusted bars do not rebase to the start date: %+v", bars)
	}
	if bars[0].AdjustmentMethod != "yahoo_adjclose_ratio_rebased_to_start" || bars[0].AdjustmentRefDate != "2026-10-20" {
		t.Fatalf("missing backward adjustment metadata: %+v", bars[0])
	}
}

func TestYahooFinanceUnadjustedDoesNotRequireAdjustedClose(t *testing.T) {
	server, _ := yahooFixture(t, false, "600036.SS")
	defer server.Close()
	p := NewYahooFinance(2*time.Second, server.URL)
	bars, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 || bars[0].Close != 10 || bars[1].Close != 11 {
		t.Fatalf("unadjusted OHLC should remain raw: %+v", bars)
	}
	if bars[0].AdjustmentMethod != "yahoo_raw_ohlc" || bars[0].AdjustmentRefDate != "" {
		t.Fatalf("unexpected unadjusted metadata: %+v", bars[0])
	}
}

func TestYahooFinanceRejectsAdjustedRequestWithoutAdjustedClose(t *testing.T) {
	server, _ := yahooFixture(t, false, "600036.SS")
	defer server.Close()
	p := NewYahooFinance(2*time.Second, server.URL)
	if _, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentForward); err == nil || !strings.Contains(err.Error(), "adjusted-close series is required") {
		t.Fatalf("expected adjusted-close contract error, got %v", err)
	}
}

func TestYahooFinanceRejectsTickerMismatchAndUnsupportedExchange(t *testing.T) {
	server, _ := yahooFixture(t, true, "600000.SS")
	defer server.Close()
	p := NewYahooFinance(2*time.Second, server.URL)
	if _, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentForward); err == nil || !strings.Contains(err.Error(), "ticker mismatch") {
		t.Fatalf("expected ticker mismatch, got %v", err)
	}
	if _, err := yahooFinanceTicker("830799.BJ"); err == nil || !strings.Contains(err.Error(), "does not support A-share exchange") {
		t.Fatalf("expected unsupported exchange error, got %v", err)
	}
}

func TestYahooDateRangeAndEndpointShape(t *testing.T) {
	loc := yahooChinaLocation()
	now := time.Date(2026, 10, 23, 15, 0, 0, 0, loc)
	start, end, err := yahooDateRange("", "2026-10-21", 8, now)
	if err != nil {
		t.Fatal(err)
	}
	if start.Format("2006-01-02") != "2026-08-20" || end.Format("2006-01-02") != "2026-10-21" {
		t.Fatalf("unexpected bounded date range: %s..%s", start, end)
	}
	raw := "https://query1.finance.yahoo.com/v8/finance/chart"
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.RawQuery != "" || u.User != nil {
		t.Fatalf("unexpected default endpoint URL shape: %s", raw)
	}
	if _, err := strconv.ParseInt(strconv.FormatInt(start.Unix(), 10), 10, 64); err != nil {
		t.Fatalf("start Unix timestamp round trip failed: %v", err)
	}
	_ = fmt.Sprintf("end timestamp=%d", end.Unix())
}
