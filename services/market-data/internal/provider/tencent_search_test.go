package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestTencentSearchStocksDecodesGBKAndFiltersChineseName(t *testing.T) {
	payload := "v_hint=\"sh~600519~贵州茅台^sz~300750~宁德时代^sh~600519~贵州茅台\";\n"
	encoded, err := simplifiedchinese.GBK.NewEncoder().Bytes([]byte(payload))
	if err != nil {
		t.Fatalf("encode GBK fixture: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%q, want GET", r.Method)
		}
		if r.URL.Query().Get("v") != "2" || r.URL.Query().Get("t") != "all" || r.URL.Query().Get("c") != "1" {
			t.Errorf("unexpected Smartbox parameters: %s", r.URL.RawQuery)
		}
		if got := r.URL.Query().Get("q"); got != "贵州茅台" {
			t.Errorf("q=%q, want Chinese query", got)
		}
		w.Header().Set("Content-Type", "text/plain; charset=gbk")
		_, _ = w.Write(encoded)
	}))
	defer server.Close()

	p := NewTencent(2 * time.Second)
	p.searchURL = server.URL
	got, err := p.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("SearchStocks returned error: %v", err)
	}
	if got.Query != "贵州茅台" || got.Source != "tencent" || got.SourceVersion != tencentStockSearchVersion || got.FetchedAt.IsZero() {
		t.Fatalf("unexpected snapshot metadata: %+v", got)
	}
	if got.Total != 1 || len(got.Items) != 1 {
		t.Fatalf("expected one unique matching result, got %+v", got)
	}
	item := got.Items[0]
	if item.Symbol != "600519.SH" || item.Code != "600519" || item.Name != "贵州茅台" || item.Exchange != "SH" {
		t.Fatalf("unexpected normalized item: %+v", item)
	}
	if got.QualityStatus != model.QualityDegraded || len(got.QualityReasons) == 0 {
		t.Fatalf("missing conservative source-quality annotation: %+v", got)
	}
}

func TestTencentSearchStocksNormalizesExchangeSuffixForCodeQuery(t *testing.T) {
	payload := "v_hint=\"sz~300750~宁德时代^\";"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("q"); got != "300750" {
			t.Errorf("q=%q, want 300750", got)
		}
		fmt.Fprint(w, payload)
	}))
	defer server.Close()

	p := NewTencent(2 * time.Second)
	p.searchURL = server.URL
	got, err := p.SearchStocks(context.Background(), "300750.SZ", 5)
	if err != nil {
		t.Fatalf("SearchStocks returned error: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.Items[0].Symbol != "300750.SZ" {
		t.Fatalf("unexpected code search result: %+v", got)
	}
}

func TestTencentSearchStocksRejectsEmptyOrUnparseableResponse(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "empty payload", body: "v_hint=\"\";"},
		{name: "missing marker", body: "not a Smartbox response"},
		{name: "unsupported market", body: "v_hint=\"hk~00700~Tencent^\";"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			p := NewTencent(2 * time.Second)
			p.searchURL = server.URL
			_, err := p.SearchStocks(context.Background(), "Tencent", 10)
			if err == nil {
				t.Fatal("expected an error so the service can try its fallback provider")
			}
			if !strings.Contains(strings.ToLower(err.Error()), "tencent stock") && !strings.Contains(strings.ToLower(err.Error()), "smartbox") && !strings.Contains(strings.ToLower(err.Error()), "v_hint") {
				t.Fatalf("error did not describe the invalid response: %v", err)
			}
		})
	}
}

func TestTencentStockSearchTermStripsExchangeSuffix(t *testing.T) {
	for input, want := range map[string]string{
		"600519.SH": "600519",
		"300750.sz": "300750",
		"贵州茅台":      "贵州茅台",
	} {
		if got := tencentStockSearchTerm(input); got != want {
			t.Errorf("tencentStockSearchTerm(%q)=%q, want %q", input, got, want)
		}
	}
}

func TestTencentSmartboxUnescapesUnicodeName(t *testing.T) {
	raw := `v_hint="sh~600519~\u8d35\u5dde\u8305\u53f0~gzmt~GP-A"`
	items, err := parseTencentSmartboxStocks([]byte(raw))
	if err != nil {
		t.Fatalf("parse Smartbox response: %v", err)
	}
	if len(items) != 1 || items[0].Name != "贵州茅台" || items[0].Symbol != "600519.SH" {
		t.Fatalf("unexpected decoded Smartbox item: %+v", items)
	}
}
