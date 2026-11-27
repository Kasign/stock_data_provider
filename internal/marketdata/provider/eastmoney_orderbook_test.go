package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

const validEastmoneyOrderBookResponse = `{"rc":0,"data":{"f57":"600519","f86":"20261127101500",` +
	`"f19":"1313.10","f20":"12","f17":"1313.00","f18":"35","f15":"1312.90","f16":"48",` +
	`"f13":"1312.80","f14":"61","f11":"1312.70","f12":"74",` +
	`"f39":"1313.20","f40":"9","f37":"1313.30","f38":"17","f35":"1313.40","f36":"25",` +
	`"f33":"1313.50","f34":"33","f31":"1313.60","f32":"41"}}`

func TestEastmoneyFetchOrderBookProjectsFiveLevelsAndConvertsHands(t *testing.T) {
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = r.URL.Query()
		fmt.Fprint(w, validEastmoneyOrderBookResponse)
	}))
	defer server.Close()

	p := NewEastmoney(2*time.Second, server.URL, server.URL)
	book, err := p.FetchOrderBook(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchOrderBook() error = %v", err)
	}
	if err := book.Validate(); err != nil {
		t.Fatalf("provider returned an invalid order book: %v", err)
	}
	if book.Symbol != "600519.SH" || book.Source != "eastmoney" {
		t.Fatalf("unexpected identity: %+v", book)
	}
	if len(book.Bids) != 5 || len(book.Asks) != 5 {
		t.Fatalf("want five levels on each side, got bids=%d asks=%d", len(book.Bids), len(book.Asks))
	}
	if book.Bids[0].Price != 1313.10 || book.Bids[0].Volume != 1200 ||
		book.Bids[4].Price != 1312.70 || book.Bids[4].Volume != 7400 {
		t.Fatalf("unexpected bid levels: %+v", book.Bids)
	}
	if book.Asks[0].Price != 1313.20 || book.Asks[0].Volume != 900 ||
		book.Asks[4].Price != 1313.60 || book.Asks[4].Volume != 4100 {
		t.Fatalf("unexpected ask levels: %+v", book.Asks)
	}
	volume, err := book.TradableVolumeShares()
	if err != nil || volume != 23000 {
		t.Fatalf("tradable volume = %d, err = %v; want 23000 shares", volume, err)
	}
	if book.ObservedAt.IsZero() || !book.ObservedAt.Before(book.SystemObservedAt) {
		t.Fatalf("invalid PIT timestamps: observed=%s system=%s", book.ObservedAt, book.SystemObservedAt)
	}
	if book.QualityStatus != model.QualityValid {
		t.Fatalf("quality = %q, want valid", book.QualityStatus)
	}
	fields := strings.Split(captured.Get("fields"), ",")
	for _, required := range []string{"f11", "f12", "f13", "f14", "f15", "f16", "f17", "f18", "f19", "f20", "f31", "f32", "f33", "f34", "f35", "f36", "f37", "f38", "f39", "f40", "f57", "f86"} {
		found := false
		for _, field := range fields {
			if field == required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("request fields %q missing %s", captured.Get("fields"), required)
		}
	}
	if captured.Get("fltt") != "2" || captured.Get("invt") != "2" || captured.Get("secid") != "1.600519" {
		t.Fatalf("unexpected request parameters: %v", captured)
	}
}

func TestEastmoneyFetchOrderBookRejectsSymbolMismatch(t *testing.T) {
	response := strings.Replace(validEastmoneyOrderBookResponse, `"f57":"600519"`, `"f57":"000001"`, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	_, err := NewEastmoney(2*time.Second, server.URL, server.URL).FetchOrderBook(context.Background(), "600519.SH")
	if err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("symbol mismatch must be rejected, got %v", err)
	}
}

func TestEastmoneyFetchOrderBookRejectsIncompleteLevels(t *testing.T) {
	response := strings.Replace(validEastmoneyOrderBookResponse, `"f12":"74",`, "", 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	_, err := NewEastmoney(2*time.Second, server.URL, server.URL).FetchOrderBook(context.Background(), "600519.SH")
	if err == nil {
		t.Fatal("incomplete five-level depth must be rejected")
	}
}

func TestEastmoneyFetchOrderBookRejectsMissingProviderTimestamp(t *testing.T) {
	response := strings.Replace(validEastmoneyOrderBookResponse, `"f86":"20261127101500",`, "", 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, response)
	}))
	defer server.Close()

	_, err := NewEastmoney(2*time.Second, server.URL, server.URL).FetchOrderBook(context.Background(), "600519.SH")
	if err == nil {
		t.Fatal("provider timestamp is required for an order-book PIT snapshot")
	}
}

func TestEastmoneyHTTPOriginsForOrderBookCapability(t *testing.T) {
	p := NewEastmoney(2*time.Second, "https://push2.eastmoney.com/api/qt/stock/get", "https://push2his.eastmoney.com/api/qt/stock/kline/get")
	origins, err := p.HTTPOriginsForCapabilities(EastmoneyCapabilities{OrderBook: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(origins) != 1 || origins[0] != "https://push2.eastmoney.com" {
		t.Fatalf("order-book capability origins = %v", origins)
	}
}
