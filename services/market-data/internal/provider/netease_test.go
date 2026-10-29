package provider

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestNeteaseFetchDailyBarsParsesCSVAndSortsByDate(t *testing.T) {
	payload := "日期,股票代码,名称,收盘价,最高价,最低价,开盘价,前收盘,涨跌额,涨跌幅,换手率,成交量,成交金额\n" +
		"2026-10-21,'600036,招商银行,42.28,42.60,42.00,42.60,42.52,-0.24,-0.56,0.36,74818026,3157692775.06\n" +
		"2026-10-20,'600036,招商银行,42.52,43.13,42.18,42.95,43.05,-0.53,-1.23,0.37,76094847,3235055172.92\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("code") != "0600036" || q.Get("start") != "20261020" || q.Get("end") != "20261021" {
			t.Errorf("unexpected Netease query: %v", q)
		}
		if !strings.Contains(q.Get("fields"), "VATURNOVER") || !strings.Contains(q.Get("fields"), "VOTURNOVER") {
			t.Errorf("query must request both volume and amount: %q", q.Get("fields"))
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	p := NewNetease(2*time.Second, server.URL)
	bars, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 || bars[0].TradeDate != "2026-10-20" || bars[1].TradeDate != "2026-10-21" {
		t.Fatalf("unexpected dates/order: %+v", bars)
	}
	if bars[0].QualityStatus != model.QualityValid || bars[0].Amount == nil || *bars[0].Amount < 3.2e9 {
		t.Fatalf("verified amount/volume contract should be valid: %+v", bars[0])
	}
	if *bars[0].Volume != 76094847 || bars[0].VolumeUnit != "shares" || bars[0].RawAmountUnit != "CNY" {
		t.Fatalf("unexpected units: %+v", bars[0])
	}
	if bars[0].RawAmountField != "VATURNOVER" || bars[0].AdjustmentBasis != model.AdjustmentNone {
		t.Fatalf("missing raw field/adjustment metadata: %+v", bars[0])
	}
}

func TestNeteaseRejectsAdjustedRequestsAndUnsupportedExchange(t *testing.T) {
	p := NewNetease(2*time.Second, "http://127.0.0.1:1/history")
	if _, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentForward); err == nil || !strings.Contains(err.Error(), "only supports verified adjustment basis") {
		t.Fatalf("expected adjustment error, got %v", err)
	}
	if _, err := p.FetchDailyBars(context.Background(), "830799.BJ", "2026-10-20", "2026-10-21", 8, model.AdjustmentNone); err == nil || !strings.Contains(err.Error(), "supports Shanghai and Shenzhen") {
		t.Fatalf("expected unsupported exchange error, got %v", err)
	}
	if _, err := neteaseCode("830799.BJ"); err == nil {
		t.Fatal("Beijing exchange symbols must be rejected")
	}
}

func TestNeteaseUnitCrossCheckInfersHandsOnlyWhenUnambiguous(t *testing.T) {
	shares, rawUnit, amount, amountUnit, quality, reasons := neteaseValidateTradingUnits(100000, 4200000, 41, 43)
	if shares == nil || *shares != 100000 || rawUnit != "shares" || amount == nil || amountUnit != "CNY" || quality != model.QualityValid {
		t.Fatalf("unexpected shares interpretation: shares=%v rawUnit=%s amount=%v amountUnit=%s quality=%s reasons=%v", shares, rawUnit, amount, amountUnit, quality, reasons)
	}
	shares, rawUnit, amount, amountUnit, quality, reasons = neteaseValidateTradingUnits(1000, 4200000, 41, 43)
	if shares == nil || *shares != 100000 || rawUnit != "hands_inferred" || amount == nil || amountUnit != "CNY" || quality != model.QualityDegraded || len(reasons) == 0 {
		t.Fatalf("unexpected inferred-hands interpretation: shares=%v rawUnit=%s amount=%v amountUnit=%s quality=%s reasons=%v", shares, rawUnit, amount, amountUnit, quality, reasons)
	}
	shares, rawUnit, amount, amountUnit, quality, reasons = neteaseValidateTradingUnits(100000, 4200000000, 41, 43)
	if shares != nil || amount != nil || amountUnit != "unknown" || rawUnit != "unknown" || quality != model.QualityDegraded || len(reasons) == 0 {
		t.Fatalf("inconsistent amount must not expose normalized shares or trusted amount: shares=%v rawUnit=%s amount=%v amountUnit=%s quality=%s reasons=%v", shares, rawUnit, amount, amountUnit, quality, reasons)
	}
}

func TestNeteaseAmbiguousVolumeUnitDoesNotProduceNormalizedShares(t *testing.T) {
	payload := "日期,股票代码,名称,收盘价,最高价,最低价,开盘价,前收盘,涨跌额,涨跌幅,换手率,成交量,成交金额\n" +
		"2026-10-21,'600036,招商银行,42.28,42.60,42.00,42.60,42.52,-0.24,-0.56,0.36,100000,4200000000\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	p := NewNetease(2*time.Second, server.URL)
	if _, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-21", "2026-10-21", 1, model.AdjustmentNone); err == nil || !strings.Contains(err.Error(), "no valid daily bars") {
		t.Fatalf("ambiguous volume units must not pass as normalized shares; err=%v", err)
	}
}

func TestNeteaseDateRangeRejectsFutureEndDate(t *testing.T) {
	now := time.Now()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	future := now.In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	if _, _, err := neteaseDateRange("2026-10-20", future, 10, now); err == nil || !strings.Contains(err.Error(), "end date must not be after today") {
		t.Fatalf("future Netease end date must be rejected, got %v", err)
	}
}

func TestNeteaseDateRangeRejectsInvalidAndReversedDates(t *testing.T) {
	loc, _ := time.LoadLocation("Asia/Shanghai")
	now := time.Date(2026, 10, 23, 15, 0, 0, 0, loc)
	start, end, err := neteaseDateRange("2026-10-20", "2026-10-21", 8, now)
	if err != nil || start.Format("20060102") != "20261020" || end.Format("20060102") != "20261021" {
		t.Fatalf("unexpected date range %s..%s err=%v", start, end, err)
	}
	if _, _, err := neteaseDateRange("2026-10-22", "2026-10-21", 8, now); err == nil {
		t.Fatal("reversed date range must be rejected")
	}
	if _, _, err := neteaseDateRange("bad-date", "2026-10-21", 8, now); err == nil {
		t.Fatal("invalid start date must be rejected")
	}
	if _, _, err := neteaseDateRange("2026-10-20", "bad-date", 8, now); err == nil {
		t.Fatal("invalid end date must be rejected")
	}
}

func TestNeteaseCSVRejectsMissingRequiredColumns(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintln(w, "日期,股票代码,开盘价")
		fmt.Fprintln(w, "2026-10-21,600036,42")
	}))
	defer server.Close()
	p := NewNetease(2*time.Second, server.URL)
	_, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentNone)
	if err == nil || !strings.Contains(err.Error(), "missing required column") {
		t.Fatalf("expected missing-column error, got %v", err)
	}
}

func TestNeteaseFetchQuoteParsesJSONPAndNormalizesPercent(t *testing.T) {
	payload := "_ntes_quote_callback({\"0600036\":{\"code\":\"0600036\",\"symbol\":\"600036\",\"name\":\"招商银行\",\"price\":42.35,\"percent\":0.0127,\"updown\":0.53,\"open\":41.70,\"high\":42.38,\"low\":41.57,\"yestclose\":41.82,\"volume\":56829700,\"turnover\":2386858667,\"time\":\"2026/10/23 11:07:58\",\"update\":\"2026/10/23 11:07:59\"}});"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/0600036,money.api") {
			t.Errorf("unexpected Netease quote path %q", r.URL.Path)
		}
		if r.URL.Query().Get("callback") != "_ntes_quote_callback" {
			t.Errorf("missing expected callback query: %v", r.URL.Query())
		}
		if r.Header.Get("Referer") == "" {
			t.Error("expected Netease Referer header")
		}
		w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	p := NewNeteaseConfigured(2*time.Second, server.URL, "http://127.0.0.1:1/history")
	q, err := p.FetchQuote(context.Background(), "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if q.Symbol != "600036.SH" || q.Name != "招商银行" || q.Source != "netease" {
		t.Fatalf("unexpected quote identity: %+v", q)
	}
	wantChange := 42.35 - 41.82
	wantChangePercent := wantChange / 41.82 * 100
	if q.CurrentPrice == nil || math.Abs(*q.CurrentPrice-42.35) > 1e-9 ||
		q.Change == nil || math.Abs(*q.Change-wantChange) > 1e-9 ||
		q.ChangePercent == nil || math.Abs(*q.ChangePercent-wantChangePercent) > 1e-9 {
		t.Fatalf("canonical change fields were not derived from latest/previous close (want change=%.12f percent=%.12f): %+v", wantChange, wantChangePercent, q)
	}
	if q.Volume == nil || *q.Volume != 56829700 || q.Amount == nil || *q.Amount != 2386858667 {
		t.Fatalf("unexpected volume/amount: %+v", q)
	}
	if q.VolumeUnit != "shares" || q.AmountUnit != "CNY" || q.RawAmountField != "turnover" {
		t.Fatalf("missing quote units/field metadata: %+v", q)
	}
	if q.QuoteTime == nil || q.QualityStatus != model.QualityValid {
		t.Fatalf("timestamp or quality status incorrect: %+v", q)
	}
}

func TestNeteaseFetchQuoteDegradesFutureProviderTimestamp(t *testing.T) {
	payload := `_ntes_quote_callback({"0600036":{"code":"0600036","symbol":"600036","name":"招商银行","price":42.35,"percent":0.0127,"updown":0.53,"open":41.70,"high":42.38,"low":41.57,"yestclose":41.82,"volume":56829700,"turnover":2386858667,"time":"2099/12/31 11:07:58"}});`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(payload)) }))
	defer server.Close()
	p := NewNeteaseConfigured(2*time.Second, server.URL, "http://127.0.0.1:1/history")
	q, err := p.FetchQuote(context.Background(), "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if q.QuoteTime != nil || q.QualityStatus != model.QualityDegraded {
		t.Fatalf("future provider timestamp must be omitted and quality degraded: %+v", q)
	}
}

func TestNeteaseFetchQuoteRejectsInconsistentChangeFields(t *testing.T) {
	for name, payload := range map[string]string{
		"change mismatch":  "_ntes_quote_callback({\"0600036\":{\"code\":\"0600036\",\"symbol\":\"600036\",\"name\":\"招商银行\",\"price\":42.35,\"percent\":0.0127,\"updown\":0.80,\"open\":41.70,\"high\":42.38,\"low\":41.57,\"yestclose\":41.82,\"volume\":56829700,\"turnover\":2386858667,\"time\":\"2026/10/23 11:07:58\"}});",
		"percent mismatch": "_ntes_quote_callback({\"0600036\":{\"code\":\"0600036\",\"symbol\":\"600036\",\"name\":\"招商银行\",\"price\":42.35,\"percent\":0.02,\"updown\":0.53,\"open\":41.70,\"high\":42.38,\"low\":41.57,\"yestclose\":41.82,\"volume\":56829700,\"turnover\":2386858667,\"time\":\"2026/10/23 11:07:58\"}});",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			p := NewNeteaseConfigured(2*time.Second, server.URL, "http://127.0.0.1:1/history")
			if _, err := p.FetchQuote(context.Background(), "600036.SH"); err == nil {
				t.Fatal("inconsistent source change fields must be rejected")
			}
		})
	}
}

func TestNeteaseFetchQuoteRejectsWrongSymbolAndMalformedJSONP(t *testing.T) {
	for name, payload := range map[string]string{
		"wrong symbol":    "_ntes_quote_callback({\"0600036\":{\"code\":\"0600036\",\"symbol\":\"600000\",\"name\":\"wrong\",\"price\":42,\"percent\":0.01,\"updown\":1,\"open\":41,\"high\":43,\"low\":40,\"yestclose\":41,\"volume\":100,\"turnover\":4200,\"time\":\"2026/10/23 11:00:00\"}});",
		"malformed JSONP": "{\"0600036\":{}}",
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(payload))
			}))
			defer server.Close()
			p := NewNeteaseConfigured(2*time.Second, server.URL, "http://127.0.0.1:1/history")
			if _, err := p.FetchQuote(context.Background(), "600036.SH"); err == nil {
				t.Fatal("expected invalid quote response to be rejected")
			}
		})
	}
}

func TestNeteaseQuoteTimestampParser(t *testing.T) {
	if parseNeteaseQuoteTime("2026/10/23 11:07:58") == nil {
		t.Fatal("expected NetEase local quote timestamp to parse")
	}
	if parseNeteaseQuoteTime("not-a-time") != nil {
		t.Fatal("invalid timestamp must return nil")
	}
}
