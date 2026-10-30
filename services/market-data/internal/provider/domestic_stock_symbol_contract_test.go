package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestDomesticStockProviderSymbolHelpersRejectNonCommonEquities(t *testing.T) {
	validators := []struct {
		name     string
		validate func(string) (string, error)
	}{
		{name: "Sina", validate: func(s string) (string, error) { return sinaCode(s) }},
		{name: "Netease", validate: func(s string) (string, error) { return neteaseCode(s) }},
		{name: "Pytdx", validate: func(s string) (string, error) { return pytdxDomesticSymbol(s) }},
		{name: "BaoStock", validate: func(s string) (string, error) { return baostockSecurityCode(s) }},
		{name: "Hithink", validate: func(s string) (string, error) { return hithinkDomesticSymbol(s) }},
		{name: "TickFlow", validate: func(s string) (string, error) { return tickflowDomesticSymbol(s) }},
	}
	accepted := []string{"600519.SH", "000001.SZ"}
	rejected := []string{"510300.SH", "399001.SZ", "900901.SH", "113001.SH"}

	for _, validator := range validators {
		t.Run(validator.name, func(t *testing.T) {
			for _, symbol := range accepted {
				if got, err := validator.validate(symbol); err != nil {
					t.Errorf("ordinary A-share %s should be accepted: %v", symbol, err)
				} else if got == "" {
					t.Errorf("ordinary A-share %s returned an empty provider symbol", symbol)
				}
			}
			for _, symbol := range rejected {
				if got, err := validator.validate(symbol); err == nil {
					t.Errorf("non-common-equity %s must be rejected, got provider symbol %q", symbol, got)
				}
			}
		})
	}
}

func TestTushareStockFinancialAndValuationRejectNonCommonEquitiesBeforeRequest(t *testing.T) {
	var requestCount atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		http.Error(w, "unexpected upstream request for non-common equity", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	ctx := context.Background()
	for _, symbol := range []string{"510300.SH", "399001.SZ", "900901.SH", "113001.SH"} {
		if _, err := client.FetchFundamentals(ctx, symbol, 2025, 4); err == nil {
			t.Errorf("FetchFundamentals(%s) must reject non-common equities", symbol)
		}
		if _, err := client.FetchValuations(ctx, []string{symbol}); err == nil {
			t.Errorf("FetchValuations(%s) must reject non-common equities", symbol)
		}
	}
	if got := requestCount.Load(); got != 0 {
		t.Fatalf("non-common-equity requests must be rejected before the upstream call; got %d requests", got)
	}
}

func TestDomesticStockProviderBeijingSymbolSupportMatrix(t *testing.T) {
	unsupported := []struct {
		name string
		validate func(string) (string, error)
	}{
		{name: "Sina", validate: sinaCode},
		{name: "Tencent", validate: tencentCode},
		{name: "NetEase", validate: neteaseCode},
		{name: "BaoStock", validate: baostockSecurityCode},
	}
	for _, item := range unsupported {
		t.Run(item.name+"_unsupported", func(t *testing.T) {
			if got, err := item.validate("920000.BJ"); err == nil {
				t.Fatalf("provider must explicitly reject unsupported Beijing symbol, got %q", got)
			}
		})
	}

	supported := []struct {
		name string
		validate func(string) (string, error)
	}{
		{name: "Pytdx", validate: pytdxDomesticSymbol},
		{name: "Hithink", validate: hithinkDomesticSymbol},
		{name: "TickFlow", validate: tickflowDomesticSymbol},
	}
	for _, item := range supported {
		t.Run(item.name+"_supported", func(t *testing.T) {
			if got, err := item.validate("920000.BJ"); err != nil || got != "920000.BJ" {
				t.Fatalf("provider should preserve canonical Beijing symbol, got %q, err=%v", got, err)
			}
		})
	}

	if got, err := tushareExchangeCode("920000.BJ"); err != nil || got != "920000.BJ" {
		t.Fatalf("Tushare should accept canonical Beijing symbol, got %q, err=%v", got, err)
	}
	if got, err := eastmoneySecID("920000.BJ"); err != nil || got != "0.920000" {
		t.Fatalf("Eastmoney should map Beijing symbol to its market security ID, got %q, err=%v", got, err)
	}
	if got := pytdxWireSymbol("920000.BJ"); got != "bj920000" {
		t.Fatalf("Pytdx should preserve the Beijing exchange prefix, got %q", got)
	}
}

func TestDomesticStockProviderMarketSegmentSymbolMappings(t *testing.T) {
	cases := []struct {
		symbol string
	}{
		{symbol: "600519.SH"}, // 沪市主板
		{symbol: "000001.SZ"}, // 深市主板
		{symbol: "300750.SZ"}, // 创业板
		{symbol: "688981.SH"}, // 科创板
		{symbol: "920000.BJ"}, // 北交所
	}
	providers := []struct {
		name string
		mapSymbol func(string) (string, error)
		want map[string]string
	}{
		{name: "Sina", mapSymbol: sinaCode, want: map[string]string{"600519.SH": "sh600519", "000001.SZ": "sz000001", "300750.SZ": "sz300750", "688981.SH": "sh688981"}},
		{name: "Tencent", mapSymbol: tencentCode, want: map[string]string{"600519.SH": "sh600519", "000001.SZ": "sz000001", "300750.SZ": "sz300750", "688981.SH": "sh688981"}},
		{name: "NetEase", mapSymbol: neteaseCode, want: map[string]string{"600519.SH": "0600519", "000001.SZ": "1000001", "300750.SZ": "1300750", "688981.SH": "0688981"}},
		{name: "BaoStock", mapSymbol: baostockSecurityCode, want: map[string]string{"600519.SH": "sh.600519", "000001.SZ": "sz.000001", "300750.SZ": "sz.300750", "688981.SH": "sh.688981"}},
		{name: "Pytdx", mapSymbol: pytdxDomesticSymbol, want: map[string]string{"600519.SH": "600519.SH", "000001.SZ": "000001.SZ", "300750.SZ": "300750.SZ", "688981.SH": "688981.SH", "920000.BJ": "920000.BJ"}},
		{name: "Hithink", mapSymbol: hithinkDomesticSymbol, want: map[string]string{"600519.SH": "600519.SH", "000001.SZ": "000001.SZ", "300750.SZ": "300750.SZ", "688981.SH": "688981.SH", "920000.BJ": "920000.BJ"}},
		{name: "TickFlow", mapSymbol: tickflowDomesticSymbol, want: map[string]string{"600519.SH": "600519.SH", "000001.SZ": "000001.SZ", "300750.SZ": "300750.SZ", "688981.SH": "688981.SH", "920000.BJ": "920000.BJ"}},
		{name: "Tushare", mapSymbol: tushareExchangeCode, want: map[string]string{"600519.SH": "600519.SH", "000001.SZ": "000001.SZ", "300750.SZ": "300750.SZ", "688981.SH": "688981.SH", "920000.BJ": "920000.BJ"}},
		{name: "Eastmoney", mapSymbol: eastmoneySecID, want: map[string]string{"600519.SH": "1.600519", "000001.SZ": "0.000001", "300750.SZ": "0.300750", "688981.SH": "1.688981", "920000.BJ": "0.920000"}},
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			for _, tc := range cases {
				want, supported := provider.want[tc.symbol]
				got, err := provider.mapSymbol(tc.symbol)
				if !supported {
					if err == nil {
						t.Errorf("unsupported market symbol %s unexpectedly mapped to %q", tc.symbol, got)
					}
					continue
				}
				if err != nil || got != want {
					t.Errorf("mapSymbol(%s) = %q, err=%v; want %q", tc.symbol, got, err, want)
				}
			}
		})
	}
}
