package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
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
