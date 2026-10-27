package provider

import (
	"testing"

	"github.com/injoyai/tdx/protocol"
)

func TestPytdxWireSymbolPreservesExplicitExchange(t *testing.T) {
	cases := []struct {
		symbol string
		want   string
	}{
		{symbol: "600519.SH", want: "sh600519"},
		{symbol: "000001.SZ", want: "sz000001"},
		{symbol: "300750.SZ", want: "sz300750"},
		{symbol: "920000.BJ", want: "bj920000"},
	}
	for _, tc := range cases {
		if got := pytdxWireSymbol(tc.symbol); got != tc.want {
			t.Errorf("pytdxWireSymbol(%q) = %q, want %q", tc.symbol, got, tc.want)
		}
	}
}

func TestPytdxDomesticSymbolRejectsExchangeMismatchAndIndex(t *testing.T) {
	for _, symbol := range []string{"600519.SZ", "000001.SH", "399001.SZ", "000300.SH"} {
		if _, err := pytdxDomesticSymbol(symbol); err == nil {
			t.Errorf("Pytdx equity adapter should reject %q", symbol)
		}
	}
	for _, symbol := range []string{"600519.SH", "000001.SZ", "300750.SZ", "920000.BJ"} {
		if got, err := pytdxDomesticSymbol(symbol); err != nil || got != symbol {
			t.Errorf("Pytdx should accept %q, got symbol=%q err=%v", symbol, got, err)
		}
	}
}

func TestPytdxQuoteMatchesSymbolRequiresMatchingExchange(t *testing.T) {
	valid := &protocol.Quote{Code: "000001", Exchange: protocol.ExchangeSZ, Kline: &protocol.Kline{}}
	if !pytdxQuoteMatchesSymbol(valid, "000001.SZ") {
		t.Fatal("matching code and exchange should be accepted")
	}
	wrongExchange := &protocol.Quote{Code: "000001", Exchange: protocol.ExchangeSH, Kline: &protocol.Kline{}}
	if pytdxQuoteMatchesSymbol(wrongExchange, "000001.SZ") {
		t.Fatal("same code from a different exchange must be rejected")
	}
	wrongCode := &protocol.Quote{Code: "600519", Exchange: protocol.ExchangeSH, Kline: &protocol.Kline{}}
	if pytdxQuoteMatchesSymbol(wrongCode, "000001.SZ") {
		t.Fatal("different quote code must be rejected")
	}
	if pytdxQuoteMatchesSymbol(nil, "000001.SZ") {
		t.Fatal("nil quote must be rejected")
	}
	if pytdxQuoteMatchesSymbol(&protocol.Quote{Code: "000001", Exchange: protocol.ExchangeSZ}, "000001.SZ") {
		t.Fatal("quote without K-line data must be rejected")
	}
}
