package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestEastmoneyQuoteNormalizesUnitsAndProviderTime(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":{"f43":1313.31,"f44":1331.98,"f45":1312.00,"f46":1331.88,"f47":34136,"f48":4525398959,"f57":"600519","f58":"贵州茅台","f60":1331.88,"f86":"202610211128","f169":-18.57,"f170":-1.394,"f50":2.13,"f168":0.2}}`)
	}))
	defer srv.Close()
	p := NewEastmoney(2*time.Second, srv.URL, srv.URL)
	q, err := p.FetchQuote(context.Background(), "600519")
	if err != nil {
		t.Fatal(err)
	}
	if *q.CurrentPrice != 1313.31 {
		t.Fatalf("price=%v", *q.CurrentPrice)
	}
	if *q.Volume != 3413600 {
		t.Fatalf("volume shares=%d", *q.Volume)
	}
	if *q.Amount != 4525398959 {
		t.Fatalf("amount=%v", *q.Amount)
	}
	if q.QuoteTime == nil || q.QuoteTime.In(time.FixedZone("CST", 8*3600)).Hour() != 11 {
		t.Fatalf("unexpected quote time %#v", q.QuoteTime)
	}
	if q.QuoteTimeSemantics != "provider_snapshot_time" {
		t.Fatalf("quote time semantics=%q, want provider_snapshot_time", q.QuoteTimeSemantics)
	}
	if q.VolumeUnit != "shares" || q.RawVolumeUnit != "hands" {
		t.Fatalf("unit metadata wrong: %+v", q)
	}
	if q.VolumeRatio == nil || *q.VolumeRatio != 2.13 || q.TurnoverRate == nil || *q.TurnoverRate != 0.2 {
		t.Fatalf("volume ratio/turnover rate must survive the provider contract: ratio=%v turnover=%v", q.VolumeRatio, q.TurnoverRate)
	}
}
func TestEastmoneyQuoteRejectsInvalidPriceContract(t *testing.T) {
	responses := []string{
		`{"rc":0,"data":{"f43":0,"f44":1331.98,"f45":1312.00,"f46":1331.88,"f47":34136,"f48":4525398959,"f57":"600519","f58":"贵州茅台","f60":1331.88,"f86":"202610211128","f169":-18.57,"f170":-1.394}}`,
		`{"rc":0,"data":{"f43":1335.00,"f44":1331.98,"f45":1312.00,"f46":1331.88,"f47":34136,"f48":4525398959,"f57":"600519","f58":"贵州茅台","f60":1331.88,"f86":"202610211128","f169":3.12,"f170":0.234}}`,
	}
	for i, body := range responses {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) }))
		p := NewEastmoney(2*time.Second, server.URL, server.URL)
		if _, err := p.FetchQuote(context.Background(), "600519.SH"); err == nil || !strings.Contains(err.Error(), "quote data contract") {
			server.Close()
			t.Fatalf("fixture %d: expected invalid quote contract error, got %v", i, err)
		}
		server.Close()
	}
}

func TestEastmoneyRedirectPolicyAllowsOnlyHTTPSFirstPartyHosts(t *testing.T) {
	source := &http.Request{URL: &url.URL{Scheme: "https", Host: "push2.eastmoney.com"}}
	allowed := &http.Request{URL: &url.URL{Scheme: "https", Host: "push2delay.eastmoney.com"}}
	if err := eastmoneyRedirectPolicy(allowed, []*http.Request{source}); err != nil {
		t.Fatalf("trusted Eastmoney HTTPS redirect should be followed: %v", err)
	}
	for _, target := range []*url.URL{
		{Scheme: "https", Host: "eastmoney.com.attacker.invalid"},
		{Scheme: "http", Host: "push2delay.eastmoney.com"},
		{Scheme: "https", Host: "attacker.invalid"},
	} {
		err := eastmoneyRedirectPolicy(&http.Request{URL: target}, []*http.Request{source})
		if !errors.Is(err, http.ErrUseLastResponse) {
			t.Fatalf("unsafe redirect target %s should not be followed, got %v", target, err)
		}
	}
	if err := eastmoneyRedirectPolicy(allowed, []*http.Request{source, source, source}); err == nil {
		t.Fatal("redirect loops must be capped")
	}
}

func TestEastmoneyDoesNotFollowExternalRedirect(t *testing.T) {
	var targetHits int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { targetHits++ }))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/steal", http.StatusFound)
	}))
	defer source.Close()
	p := NewEastmoney(2*time.Second, source.URL, source.URL)
	if _, err := p.FetchQuote(context.Background(), "600519.SH"); err == nil || !strings.Contains(err.Error(), "HTTP status 302") {
		t.Fatalf("external redirect should remain an HTTP status error, got %v", err)
	}
	if targetHits != 0 {
		t.Fatalf("external redirect target was contacted %d time(s)", targetHits)
	}
}

func TestEastmoneyDailyBarsNormalizesHandToShare(t *testing.T) {
	utCh := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		utCh <- r.URL.Query().Get("ut")
		fmt.Fprint(w, `{"rc":0,"data":{"code":"600519","name":"贵州茅台","klines":["2026-10-20,1321.01,1331.88,1335.00,1316.14,34136,4525398959,1.42,0.81,10.87,0.23"]}}`)
	}))
	defer srv.Close()
	p := NewEastmoney(2*time.Second, srv.URL, srv.URL)
	bars, err := p.FetchDailyBars(context.Background(), "600519", "2026-10-01", "2026-10-21", 100, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("bars=%d", len(bars))
	}
	if *bars[0].Volume != 3413600 {
		t.Fatalf("volume=%d", *bars[0].Volume)
	}
	if *bars[0].Amount != 4525398959 {
		t.Fatalf("amount=%v", *bars[0].Amount)
	}
	if bars[0].AdjustmentBasis != model.AdjustmentForward {
		t.Fatalf("basis=%s", bars[0].AdjustmentBasis)
	}
	if got := <-utCh; got != defaultEastmoneyUT {
		t.Fatalf("Eastmoney K-line request ut=%q, want default public token", got)
	}
}

func TestEastmoneyDailyBarsSortsAndDropsDuplicateOutOfRangeAndInvalidRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":{"code":"600519","name":"贵州茅台","klines":[
			"2026-10-21,10.0,10.2,10.3,9.9,100,1020000,1.42,0.81,10.87,0.23",
			"2026-10-19,9.8,10.0,10.1,9.7,100,990000,1.42,0.81,10.87,0.23",
			"2026-10-21,10.0,10.1,10.3,9.9,100,1010000,1.42,0.81,10.87,0.23",
			"2026-10-18,9.5,9.6,9.7,9.4,100,960000,1.42,0.81,10.87,0.23",
			"2026-10-20,10.0,10.5,10.4,9.9,100,1050000,1.42,0.81,10.87,0.23"
		]}}`)
	}))
	defer server.Close()
	p := NewEastmoney(2*time.Second, server.URL, server.URL)
	bars, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-19", "2026-10-21", 8, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 2 || bars[0].TradeDate != "2026-10-19" || bars[1].TradeDate != "2026-10-21" {
		t.Fatalf("bars must be unique, in-range, and sorted ascending: %+v", bars)
	}
	for _, bar := range bars {
		if bar.QualityStatus != model.QualityDegraded || !strings.Contains(strings.Join(bar.QualityReasons, ","), "provider_response_contract_warning") {
			t.Fatalf("filtered upstream rows must degrade accepted rows: %+v", bar)
		}
	}
}

func TestEastmoneyDailyBarsBoundsImplicitStartDate(t *testing.T) {
	type requestRange struct {
		beg string
		end string
		lmt string
		ut  string
	}
	requests := make(chan requestRange, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		requests <- requestRange{beg: q.Get("beg"), end: q.Get("end"), lmt: q.Get("lmt"), ut: q.Get("ut")}
		fmt.Fprint(w, `{"rc":0,"data":{"code":"600519","name":"贵州茅台","klines":["2026-10-20,1321.01,1331.88,1335.00,1316.14,34136,4525398959,1.42,0.81,10.87,0.23"]}}`)
	}))
	defer srv.Close()
	p := NewEastmoney(2*time.Second, srv.URL, srv.URL)
	if _, err := p.FetchDailyBars(context.Background(), "600519.SH", "", "2026-10-21", 30, model.AdjustmentForward); err != nil {
		t.Fatal(err)
	}
	got := <-requests
	if got.beg != "20260524" {
		t.Fatalf("implicit beg=%q, want bounded lookback 20260524", got.beg)
	}
	if got.end != "20261021" || got.lmt != "30" {
		t.Fatalf("unexpected request range end/limit: %+v", got)
	}
	if got.ut != defaultEastmoneyUT {
		t.Fatalf("request ut=%q, want default public token", got.ut)
	}
}

func TestEastmoneyConfiguredUTOverridesDefault(t *testing.T) {
	p := NewEastmoneyConfigured(time.Second, "", "", "  rotated-public-token  ")
	if p.ut != "rotated-public-token" {
		t.Fatalf("configured Eastmoney ut=%q, want trimmed override", p.ut)
	}
}

func TestEastmoneyRejectsNonZeroProviderResultCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":-1,"data":{"f43":1313.31,"f44":1331.98,"f45":1312.00,"f46":1331.88,"f47":34136,"f48":4525398959,"f57":"600519","f58":"贵州茅台","f60":1331.88,"f86":"202610211128","f169":-18.57,"f170":-1.394,"f168":0.2}}`)
	}))
	defer srv.Close()
	p := NewEastmoney(2*time.Second, srv.URL, srv.URL)
	if _, err := p.FetchQuote(context.Background(), "600519"); err == nil || !strings.Contains(err.Error(), "rc=-1") {
		t.Fatalf("expected provider rc error, got %v", err)
	}
}

func TestEastmoneyRejectsKlineSymbolMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":{"code":"000001","name":"wrong","klines":["2026-10-20,1321.01,1331.88,1335.00,1316.14,34136,4525398959,1.42,0.81,10.87,0.23"]}}`)
	}))
	defer srv.Close()
	p := NewEastmoney(2*time.Second, srv.URL, srv.URL)
	if _, err := p.FetchDailyBars(context.Background(), "600519", "2026-10-01", "2026-10-21", 100, model.AdjustmentForward); err == nil || !strings.Contains(err.Error(), "symbol mismatch") {
		t.Fatalf("expected symbol mismatch error, got %v", err)
	}
}
