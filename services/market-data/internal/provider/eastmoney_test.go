package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
