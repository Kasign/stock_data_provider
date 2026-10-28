package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"golang.org/x/text/encoding/htmlindex"
)

func TestSinaFetchQuoteNormalizesSharesAndAmount(t *testing.T) {
	fields := make([]string, 32)
	fields[0] = "SinaTest"
	fields[1] = "10.00"
	fields[2] = "9.80"
	fields[3] = "10.20"
	fields[4] = "10.50"
	fields[5] = "9.50"
	fields[8] = "10000" // 新浪成交量字段以“股”为单位，不是“手”。
	fields[9] = "102000"
	fields[30] = "2026-10-21"
	fields[31] = "15:00:00"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/list=sh600519" {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		if r.Header.Get("Referer") == "" {
			t.Error("expected Sina-compatible Referer header")
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = fmt.Fprintf(w, "var hq_str_sh600519=\"%s\";\n", strings.Join(fields, ","))
	}))
	defer server.Close()

	p := NewSina(0, server.URL+"/list=")
	quote, err := p.FetchQuote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchQuote returned error: %v", err)
	}
	if quote.Symbol != "600519.SH" || quote.Source != "sina" {
		t.Fatalf("unexpected identity: symbol=%q source=%q", quote.Symbol, quote.Source)
	}
	if quote.Volume == nil || *quote.Volume != 10000 || quote.VolumeUnit != "shares" {
		t.Fatalf("Sina share volume must not be multiplied by 100: %+v", quote)
	}
	if quote.Amount == nil || *quote.Amount != 102000 || quote.AmountUnit != "CNY" {
		t.Fatalf("unexpected normalized amount: %+v", quote)
	}
	if quote.CurrentPrice == nil || *quote.CurrentPrice != 10.2 {
		t.Fatalf("unexpected current price: %+v", quote.CurrentPrice)
	}
	if quote.AdjustmentBasis != model.AdjustmentNone {
		t.Fatalf("quote adjustment basis=%q", quote.AdjustmentBasis)
	}
	if quote.QuoteTime == nil || quote.QuoteTime.Format("2006-01-02 15:04:05") != "2026-10-21 07:00:00" {
		t.Fatalf("provider time should be interpreted as China Standard Time then stored in UTC: %v", quote.QuoteTime)
	}
	if quote.QuoteTimeSemantics != "provider_snapshot_time" {
		t.Fatalf("Sina quote must label source time semantics, got %q", quote.QuoteTimeSemantics)
	}
}

func TestSinaFetchQuoteRejectsResponseVariableSymbolMismatch(t *testing.T) {
	fields := make([]string, 32)
	fields[0] = "WrongSymbol"
	fields[1], fields[2], fields[3] = "10.00", "9.80", "10.20"
	fields[4], fields[5], fields[8], fields[9] = "10.50", "9.50", "10000", "102000"
	fields[30], fields[31] = "2026-10-21", "15:00:00"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 请求的是 sh600519，但上游变量名报告的是 sh600000。
		_, _ = fmt.Fprintf(w, "var hq_str_sh600000=\"%s\";\n", strings.Join(fields, ","))
	}))
	defer server.Close()

	p := NewSina(0, server.URL+"/list=")
	if _, err := p.FetchQuote(context.Background(), "600519.SH"); err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("expected response variable symbol mismatch to be rejected, got %v", err)
	}
}

func TestSinaFetchQuoteDecodesGB18030Name(t *testing.T) {
	fields := make([]string, 32)
	fields[0] = "贵州茅台"
	fields[1] = "1303.21"
	fields[2] = "1309.69"
	fields[3] = "1318.49"
	fields[4] = "1322.00"
	fields[5] = "1296.00"
	fields[8] = "4249133"
	fields[9] = "5576216948"
	fields[30] = "2026-10-22"
	fields[31] = "15:34:59"

	payload := fmt.Sprintf("var hq_str_sh600519=\"%s\";\n", strings.Join(fields, ","))
	encoding, err := htmlindex.Get("gb18030")
	if err != nil {
		t.Fatalf("get GB18030 encoder: %v", err)
	}
	body, err := encoding.NewEncoder().Bytes([]byte(payload))
	if err != nil {
		t.Fatalf("encode GB18030 fixture: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/javascript; charset=GB18030")
		if _, writeErr := w.Write(body); writeErr != nil {
			t.Errorf("write fixture response: %v", writeErr)
		}
	}))
	defer server.Close()

	p := NewSina(0, server.URL+"/list=")
	quote, err := p.FetchQuote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchQuote returned error: %v", err)
	}
	if quote.Name != "贵州茅台" {
		t.Fatalf("decoded name=%q, want 贵州茅台", quote.Name)
	}
	if quote.QualityStatus != model.QualityValid {
		t.Fatalf("valid GB18030 name should not degrade quote quality, got status=%q reasons=%v", quote.QualityStatus, quote.QualityReasons)
	}
}

func TestSinaRejectsUnsupportedExchange(t *testing.T) {
	p := NewSina(0, "http://127.0.0.1:1/list=")
	if _, err := p.FetchQuote(context.Background(), "600519.BJ"); err == nil {
		t.Fatal("Sina quote provider must reject unsupported exchanges")
	}
}

func TestSinaMissingOrInvalidAmountDoesNotClaimRawFieldMetadata(t *testing.T) {
	tests := []struct {
		name   string
		amount string
	}{
		{name: "missing", amount: ""},
		{name: "invalid", amount: "not-a-number"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := make([]string, 32)
			fields[0] = "SinaTest"
			fields[1] = "10.00"
			fields[2] = "9.80"
			fields[3] = "10.20"
			fields[4] = "10.50"
			fields[5] = "9.50"
			fields[8] = "10000"
			fields[9] = tc.amount
			fields[30] = "2026-10-21"
			fields[31] = "15:00:00"

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/list=sh600519" {
					t.Errorf("unexpected request path %q", r.URL.Path)
				}
				fmt.Fprintf(w, "var hq_str_sh600519=\"%s\";\n", strings.Join(fields, ","))
			}))
			defer server.Close()

			p := NewSina(0, server.URL+"/list=")
			quote, err := p.FetchQuote(context.Background(), "600519.SH")
			if err != nil {
				t.Fatalf("FetchQuote returned error: %v", err)
			}
			if quote.Amount != nil || quote.RawAmount != nil {
				t.Fatalf("missing or invalid amount must remain nil, got Amount=%v RawAmount=%v", quote.Amount, quote.RawAmount)
			}
			if quote.AmountUnit != "unknown" || quote.RawAmountUnit != "unknown" || quote.RawAmountField != "" {
				t.Fatalf("missing or invalid amount must not claim valid raw field metadata: amount_unit=%q raw_amount_unit=%q raw_amount_field=%q",
					quote.AmountUnit, quote.RawAmountUnit, quote.RawAmountField)
			}
			if quote.QualityStatus != model.QualityDegraded {
				t.Fatalf("missing or invalid amount must degrade quote quality, got %q", quote.QualityStatus)
			}
			foundReason := false
			for _, reason := range quote.QualityReasons {
				if reason == "amount_missing_or_invalid" {
					foundReason = true
					break
				}
			}
			if !foundReason {
				t.Fatalf("missing amount quality reason, got %v", quote.QualityReasons)
			}
		})
	}
}

func TestSinaFetchDailyBarsNormalizesUnadjustedRows(t *testing.T) {
	payload := []byte(`[{"day":"2026-10-21","open":"10.0","high":"11.0","low":"9.0","close":"10.5","volume":"10000"},{"day":"2026-10-20","open":10,"high":11,"low":9,"close":10.2,"volume":5000}]`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("symbol") != "sh600036" || query.Get("scale") != "240" || query.Get("ma") != "no" {
			t.Errorf("unexpected Sina K-line query: %v", query)
		}
		if n, err := strconv.Atoi(query.Get("datalen")); err != nil || n < 32 || n > maxSinaKlineBars {
			t.Errorf("unexpected Sina datalen %q, err=%v", query.Get("datalen"), err)
		}
		if r.Header.Get("Referer") == "" {
			t.Error("expected a Sina-compatible Referer")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	p := NewSinaConfigured(0, "http://127.0.0.1:1/unused", server.URL)
	bars, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentNone)
	if err != nil {
		t.Fatalf("FetchDailyBars returned error: %v", err)
	}
	if len(bars) != 2 || bars[0].TradeDate != "2026-10-20" || bars[1].TradeDate != "2026-10-21" {
		t.Fatalf("bars were not sorted/filtered correctly: %+v", bars)
	}
	if *bars[0].Volume != 5000 || *bars[1].Volume != 10000 || bars[0].VolumeUnit != "shares" {
		t.Fatalf("Sina K-line volume must remain in shares: %+v", bars)
	}
	if bars[0].Amount != nil || bars[0].AmountUnit != "unknown" || bars[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("missing turnover amount must remain unknown/degraded: %+v", bars[0])
	}
	if bars[0].AdjustmentBasis != model.AdjustmentNone || bars[0].AdjustmentMethod != "sina_raw_ohlc_no_adjustment" {
		t.Fatalf("Sina must explicitly label the unadjusted basis: %+v", bars[0])
	}
}

func TestSinaFetchDailyBarsRejectsUnverifiedAdjustmentAndDateRange(t *testing.T) {
	p := NewSinaConfigured(0, "http://127.0.0.1:1/unused", "http://127.0.0.1:1/kline")
	if _, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentForward); err == nil || !strings.Contains(err.Error(), "only supports verified adjustment basis") {
		t.Fatalf("expected explicit unsupported-adjustment error, got %v", err)
	}
	if _, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-22", "2026-10-21", 8, model.AdjustmentNone); err == nil || !strings.Contains(err.Error(), "must not be after") {
		t.Fatalf("expected reversed date-range error, got %v", err)
	}
	if _, _, err := sinaDailyDateRange("bad-date", "2026-10-21", 8, time.Now()); err == nil {
		t.Fatal("invalid start date must be rejected")
	}
	if _, _, err := sinaDailyDateRange("2026-10-20", "bad-date", 8, time.Now()); err == nil {
		t.Fatal("invalid end date must be rejected")
	}
}
