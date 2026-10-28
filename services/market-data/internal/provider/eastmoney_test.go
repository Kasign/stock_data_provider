package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
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
func TestEastmoneyQuoteAcceptsNumericUnixSecondsTimestamp(t *testing.T) {
	timestamp := time.Now().UTC().Add(-time.Minute).Unix()
	body := fmt.Sprintf(`{"rc":0,"data":{"f43":41.76,"f44":42.09,"f45":41.66,"f46":41.70,"f47":125595,"f48":525556667,"f57":"600036","f58":"招商银行","f60":41.82,"f86":%d,"f169":-0.06,"f170":-0.14,"f50":2.85,"f168":0.06}}`, timestamp)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, body)
	}))
	defer server.Close()

	p := NewEastmoney(2*time.Second, server.URL, server.URL)
	q, err := p.FetchQuote(context.Background(), "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Unix(timestamp, 0).UTC()
	if q.QuoteTime == nil || !q.QuoteTime.Equal(want) {
		t.Fatalf("numeric Unix seconds should be parsed exactly, got %#v want %s", q.QuoteTime, want)
	}
	if q.QualityStatus != model.QualityValid {
		t.Fatalf("a complete quote with a valid numeric timestamp should not be degraded: %+v", q)
	}
}

func TestParseEastmoneyTimeSupportsUnixSecondsAndMilliseconds(t *testing.T) {
	want := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	for _, raw := range []string{
		fmt.Sprintf("%d", want.Unix()),
		fmt.Sprintf("%d", want.UnixMilli()),
	} {
		got := parseEastmoneyTime(raw)
		if got == nil || !got.Equal(want) {
			t.Errorf("parseEastmoneyTime(%q)=%v, want %s", raw, got, want)
		}
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

func TestEastmoneyFetchStockBasicNormalizesF10IdentityAndDates(t *testing.T) {
	var gotQueries []url.Values
	var quoteFields string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("reportName") != "" && r.Header.Get("Referer") != "https://emweb.securities.eastmoney.com/" {
			t.Errorf("unexpected profile referer %q", r.Header.Get("Referer"))
		}
		switch query.Get("reportName") {
		case "RPT_PCF10_ORG_ISSUEINFO":
			gotQueries = append(gotQueries, query)
			fmt.Fprint(w, `{"success":true,"result":{"data":[{"SECUCODE":"600519.SH","SECURITY_CODE":"600519","LISTING_DATE":"2001-08-27 00:00:00","OUT_DATE":null}]}}`)
		case "RPT_F10_BASIC_ORGINFO":
			gotQueries = append(gotQueries, query)
			fmt.Fprint(w, `{"success":true,"result":{"data":[{"SECUCODE":"600519.SH","SECURITY_CODE":"600519","STR_NAMEA":"贵州茅台","ORG_NAME":"贵州茅台酒股份有限公司"}]}}`)
		default:
			quoteFields = query.Get("fields")
			fmt.Fprint(w, `{"rc":0,"data":{"f57":"600519","f58":"贵州茅台","f162":25.1,"f167":8.2,"f127":"白酒","f116":1000000000000,"f117":900000000000,"f84":1200000000,"f85":950000000,"f198":"BK0450","f173":25,"f187":50,"f105":80000000000,"f186":90}}`)
		}
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithStockBasic(2*time.Second, server.URL, "", server.URL, "")
	item, err := p.FetchStockBasic(context.Background(), "600519")
	if err != nil {
		t.Fatal(err)
	}
	if item.Symbol != "600519.SH" || item.Name != "贵州茅台" || item.IPODate != "2001-08-27" {
		t.Fatalf("unexpected stock-basic response: %+v", item)
	}
	if item.Source != "eastmoney" || item.QualityStatus != model.QualityDegraded || item.NormalizationVer == "" {
		t.Fatalf("missing source/quality/normalization metadata: %+v", item)
	}
	for _, reason := range item.QualityReasons {
		if reason == "market_profile_request_failed" {
			t.Fatalf("local market-profile fixture unexpectedly failed: %+v", item.QualityReasons)
		}
	}
	if item.DynamicPE == nil || *item.DynamicPE != 25.1 || item.Industry != "白酒" {
		t.Fatalf("market-profile fields were not normalized: %+v", item)
	}
	if item.TotalShares == nil || *item.TotalShares != 1200000000 || item.FloatShares == nil || *item.FloatShares != 950000000 {
		t.Fatalf("share-count fields were not mapped: total=%v float=%v", item.TotalShares, item.FloatShares)
	}
	fields := make(map[string]bool)
	for _, field := range strings.Split(quoteFields, ",") { fields[field] = true }
	if !fields["f84"] || !fields["f85"] {
		t.Fatalf("quote fields must request total and floating shares: %q", quoteFields)
	}
	if len(gotQueries) != 2 || gotQueries[0].Get("reportName") != "RPT_PCF10_ORG_ISSUEINFO" || gotQueries[1].Get("reportName") != "RPT_F10_BASIC_ORGINFO" {
		t.Fatalf("unexpected F10 requests: %+v", gotQueries)
	}
	for _, query := range gotQueries {
		if query.Get("filter") != `(SECUCODE="600519.SH")` || query.Get("pageSize") != "1" {
			t.Fatalf("unexpected F10 query: %v", query)
		}
	}
}

func TestEastmoneyFetchStockBasicRejectsInvalidShareCounts(t *testing.T) {
	for _, tc := range []struct {
		name, totalShares, floatShares, wantReason string
		wantTotal, wantFloat bool
	}{
		{name: "non-positive total shares", totalShares: "0", floatShares: "100", wantReason: "market_profile_total_shares_non_positive", wantFloat: true},
		{name: "negative floating shares", totalShares: "1000", floatShares: "-1", wantReason: "market_profile_float_shares_negative", wantTotal: true},
		{name: "floating shares exceed total", totalShares: "1000", floatShares: "1001", wantReason: "market_profile_float_shares_exceed_total_shares", wantTotal: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				query := r.URL.Query()
				if query.Get("reportName") != "" {
					fmt.Fprint(w, `{"success":true,"result":{"data":[{"SECUCODE":"600519.SH","SECURITY_CODE":"600519","SECURITY_NAME_ABBR":"贵州茅台","LISTING_DATE":"2001-08-27"}]}}`)
					return
				}
				fmt.Fprintf(w, `{"rc":0,"data":{"f57":"600519","f58":"贵州茅台","f84":%s,"f85":%s}}`, tc.totalShares, tc.floatShares)
			}))
			defer server.Close()
			p := NewEastmoneyConfiguredWithStockBasic(2*time.Second, server.URL, "", server.URL, "")
			got, err := p.FetchStockBasic(context.Background(), "600519.SH")
			if err != nil {
				t.Fatalf("FetchStockBasic returned error: %v", err)
			}
			if (got.TotalShares != nil) != tc.wantTotal || (got.FloatShares != nil) != tc.wantFloat {
				t.Fatalf("unexpected share fields: total=%v float=%v", got.TotalShares, got.FloatShares)
			}
			found := false
			for _, reason := range got.QualityReasons {
				if reason == tc.wantReason { found = true; break }
			}
			if !found {
				t.Fatalf("missing quality reason %q: %+v", tc.wantReason, got.QualityReasons)
			}
		})
	}
}

func TestEastmoneyFetchStockBasicRejectsMismatchedOrEmptyRows(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
	}{
		{"mismatch", `{"result":{"data":[{"SECUCODE":"000001.SZ","SECURITY_NAME_ABBR":"wrong"}]}}`, "symbol mismatch"},
		{"empty", `{"result":{"data":[]}}`, "no row"},
		{"missing name", `{"result":{"data":[{"SECUCODE":"600519.SH","LISTING_DATE":"2001-08-27"}]}}`, "name is empty"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			p := NewEastmoneyConfiguredWithStockBasic(2*time.Second, "", "", server.URL, "")
			if _, err := p.FetchStockBasic(context.Background(), "600519.SH"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestEastmoneyFetchSectorRankingsParsesBoardContract(t *testing.T) {
	for _, tc := range []struct{ category, filter string }{
		{"industry", "m:90+t:2+f:!50"},
		{"concept", "m:90+t:3+f:!50"},
	} {
		t.Run(tc.category, func(t *testing.T) {
			var got url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = r.URL.Query()
				fmt.Fprint(w, `{"rc":0,"data":{"total":120,"diff":[
					{"f2":1234.5,"f3":2.3,"f4":27.8,"f6":123456789,"f12":"BK0475","f14":"电池","f104":15,"f105":7,"f128":"宁德时代","f136":4.12},
					{"f2":120.5,"f3":1.5,"f4":1.8,"f6":223456789,"f12":"BK0476","f14":"通信","f104":10,"f105":8,"f128":"中际旭创","f136":3.12},
					{"f2":88.5,"f3":0.7,"f4":0.6,"f6":323456789,"f12":"BK0477","f14":"电子","f104":9,"f105":6,"f128":"立讯精密","f136":2.12},
					{"f2":66.5,"f3":0.2,"f4":0.1,"f6":423456789,"f12":"BK0478","f14":"汽车","f104":8,"f105":5,"f128":"比亚迪","f136":1.12},
					{"f2":55.5,"f3":-0.5,"f4":-0.3,"f6":523456789,"f12":"BK0479","f14":"计算机","f104":6,"f105":7,"f128":"中科曙光","f136":0.12}
				]}}`)
			}))
			defer server.Close()
			p := NewEastmoneyConfiguredWithMarketData(2*time.Second, "", "", "", server.URL, "")
			snapshot, err := p.FetchSectorRankings(context.Background(), tc.category, 5)
			if err != nil {
				t.Fatal(err)
			}
			if got.Get("fs") != tc.filter || got.Get("pz") != "5" || got.Get("fid") != "f3" {
				t.Fatalf("unexpected CLIST request: %v", got)
			}
			if len(snapshot.Items) != 5 || snapshot.Category != tc.category || snapshot.Total != 120 {
				t.Fatalf("unexpected snapshot metadata: %+v", snapshot)
			}
			item := snapshot.Items[0]
			if item.Code != "BK0475" || item.Name != "电池" || item.LeadingStock != "宁德时代" {
				t.Fatalf("unexpected board identity: %+v", item)
			}
			if item.ChangePercent == nil || *item.ChangePercent != 2.3 || item.Amount == nil || *item.Amount != 123456789 || item.AmountUnit != "CNY" {
				t.Fatalf("unexpected normalized price/amount fields: %+v", item)
			}
			if item.AdvancingCount == nil || *item.AdvancingCount != 15 || item.DecliningCount == nil || *item.DecliningCount != 7 {
				t.Fatalf("unexpected advancing/declining counts: %+v", item)
			}
			if snapshot.QualityStatus != model.QualityDegraded || !strings.Contains(strings.Join(snapshot.QualityReasons, ","), "provider_snapshot_timestamp_not_exposed") {
				t.Fatalf("provider timestamp is not exposed; snapshot must be degraded: %+v", snapshot)
			}
		})
	}
}

func TestEastmoneyFetchSectorRankingsSupportsBottomDirection(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		fmt.Fprint(w, `{"rc":0,"data":{"total":3,"diff":[{"f2":12.5,"f3":2.3,"f4":0.15,"f6":100000,"f12":"BK0475","f14":"电池"},{"f2":8.5,"f3":-1.2,"f4":-0.1,"f6":80000,"f12":"BK0476","f14":"传媒"},{"f2":10.0,"f3":0.5,"f4":0.05,"f6":90000,"f12":"BK0477","f14":"食品"}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithMarketData(2*time.Second, "", "", "", server.URL, "")
	snapshot, err := p.FetchSectorRankingsDirection(context.Background(), "industry", "down", 3)
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("po") != "0" || got.Get("fid") != "f3" || got.Get("pz") != "3" {
		t.Fatalf("unexpected bottom-ranking query: %v", got)
	}
	if snapshot.Direction != "down" || len(snapshot.Items) != 3 {
		t.Fatalf("unexpected bottom-ranking metadata: %+v", snapshot)
	}
	wantCodes := []string{"BK0476", "BK0477", "BK0475"}
	for i, want := range wantCodes {
		if snapshot.Items[i].Code != want || snapshot.Items[i].Rank != i+1 {
			t.Fatalf("bottom ranking order/rank mismatch at %d: got=%+v want_code=%s", i, snapshot.Items[i], want)
		}
	}
}

func TestEastmoneyFetchSectorRankingsRetriesTemporary502(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusBadGateway)
			fmt.Fprint(w, `{"error":"temporary gateway error"}`)
			return
		}
		fmt.Fprint(w, `{"rc":0,"data":{"total":1,"diff":[{"f2":12.5,"f3":1.25,"f4":0.15,"f6":100000,"f12":"BK0475","f14":"测试板块","f104":2,"f105":1}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithMarketData(2*time.Second, "", "", "", server.URL, "")
	snapshot, err := p.FetchSectorRankings(context.Background(), "industry", 5)
	if err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("request attempts=%d want 2", attempts)
	}
	if len(snapshot.Items) != 1 || snapshot.Items[0].Code != "BK0475" {
		t.Fatalf("unexpected sector response after retry: %+v", snapshot)
	}
}

func TestEastmoneyFetchSectorRankingsRejectsInvalidCategoryAndProviderCode(t *testing.T) {
	p := NewEastmoney(2*time.Second, "", "")
	if _, err := p.FetchSectorRankings(context.Background(), "all", 5); err == nil {
		t.Fatal("unsupported category must be rejected")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":-1,"data":null}`)
	}))
	defer server.Close()
	p = NewEastmoneyConfiguredWithMarketData(2*time.Second, "", "", "", server.URL, "")
	if _, err := p.FetchSectorRankings(context.Background(), "industry", 5); err == nil || !strings.Contains(err.Error(), "rc=-1") {
		t.Fatalf("expected provider rc error, got %v", err)
	}
}

func TestEastmoneyFetchIndexQuotesParsesAllowListedCoreIndexes(t *testing.T) {
	var got url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		rows := make([]map[string]any, 0, len(eastmoneyCoreIndexDefinitions))
		for i, idx := range eastmoneyCoreIndexDefinitions {
			rows = append(rows, map[string]any{"f2": 3200.5 + float64(i), "f3": 0.8, "f4": 24.5, "f6": 123456789.0, "f12": idx.Code, "f13": idx.Market, "f14": idx.Name, "f15": 3210.0, "f16": 3180.0, "f17": 3190.0, "f18": 3176.0, "f86": "202610231500"})
		}
		body, err := json.Marshal(map[string]any{"rc": 0, "data": map[string]any{"total": len(rows), "diff": rows}})
		if err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithIndices(2*time.Second, "", "", "", "", server.URL, "")
	snapshot, err := p.FetchIndexQuotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Get("secids") != "1.000001,0.399001,0.399006,1.000300,1.000016,1.000905,1.000688" {
		t.Fatalf("requested secids=%q", got.Get("secids"))
	}
	if got.Get("fields") == "" || got.Get("fltt") != "2" {
		t.Fatalf("missing quote controls: %v", got)
	}
	if len(snapshot.Items) != len(eastmoneyCoreIndexDefinitions) {
		t.Fatalf("items=%d", len(snapshot.Items))
	}
	if snapshot.Items[0].Symbol != "000001.SH" || snapshot.Items[0].Name != "上证指数" || snapshot.Items[0].LatestPrice == nil || *snapshot.Items[0].LatestPrice != 3200.5 {
		t.Fatalf("unexpected index quote: %+v", snapshot.Items[0])
	}
	if snapshot.Items[0].QualityStatus != model.QualityValid || snapshot.Items[0].QuoteTime == nil {
		t.Fatalf("complete quote should be valid: %+v", snapshot.Items[0])
	}
}

func TestEastmoneyFetchIndexQuotesPreservesMissingAmountAsDegraded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":{"total":1,"diff":[{"f2":3200.5,"f3":0.8,"f4":24.5,"f12":"000001","f13":1,"f14":"上证指数"}]}}`)
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithIndices(2*time.Second, "", "", "", "", server.URL, "")
	snapshot, err := p.FetchIndexQuotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("items=%d", len(snapshot.Items))
	}
	item := snapshot.Items[0]
	if item.Amount != nil || item.AmountUnit != "unknown" || item.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing amount contract violated: %+v", item)
	}
	if snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("partial snapshot must degrade: %+v", snapshot)
	}
}

// TestEastmoneySearchStocksContinuesAfterProviderPageCap ensures a query match on a later
// page is not hidden when Eastmoney caps each CLIST page at 100 rows.
func TestEastmoneySearchStocksContinuesAfterProviderPageCap(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page, err := strconv.Atoi(r.URL.Query().Get("pn"))
		if err != nil {
			t.Errorf("invalid page number: %v", err)
		}
		if got := r.URL.Query().Get("pz"); got != "100" {
			t.Errorf("page size=%q want 100", got)
		}
		if r.URL.Query().Get("fields") != "f12,f13,f14" {
			t.Errorf("unexpected search fields: %s", r.URL.RawQuery)
		}
		rows := make([]map[string]any, 0)
		start, end := (page-1)*eastmoneyStockSearchPageSize, page*eastmoneyStockSearchPageSize
		if end > 205 {
			end = 205
		}
		for index := start; index < end; index++ {
			code := fmt.Sprintf("%06d", 300000+index)
			name := fmt.Sprintf("测试股票%d", index)
			market := 0
			if index%2 == 0 {
				market = 1
			}
			if index == 100 {
				code, name, market = "600519", "贵州茅台", 1
			}
			rows = append(rows, map[string]any{"f12": code, "f13": market, "f14": name})
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"rc":   0,
			"data": map[string]any{"total": 205, "diff": rows},
		}); err != nil {
			t.Errorf("encode page: %v", err)
		}
	}))
	defer server.Close()

	p := &Eastmoney{client: server.Client(), stockListURL: server.URL, ut: "test-token"}
	got, err := p.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("SearchStocks returned error: %v", err)
	}
	if requests != 3 {
		t.Fatalf("directory should fetch all 205 rows in three pages, got %d requests", requests)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.Items[0].Symbol != "600519.SH" || got.Items[0].Name != "贵州茅台" {
		t.Fatalf("later-page Chinese name match was lost: %+v", got)
	}
	if got.QualityStatus != model.QualityValid || len(got.QualityReasons) != 0 {
		t.Fatalf("complete multi-page directory should have valid quality: %+v", got)
	}
}

func TestEastmoneySearchStocksUsesAndCachesRealDirectoryShape(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Query().Get("fs") == "" || r.URL.Query().Get("fields") != "f12,f13,f14" {
			t.Errorf("unexpected list request parameters: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"rc":0,"data":{"total":3,"diff":[{"f12":"600519","f13":1,"f14":"贵州茅台"},{"f12":"300750","f13":0,"f14":"宁德时代"},{"f12":"000858","f13":0,"f14":"五粮液"}]}}`)
	}))
	defer server.Close()
	p := &Eastmoney{client: server.Client(), stockListURL: server.URL, ut: "test-token"}

	got, err := p.SearchStocks(context.Background(), "贵州茅台", 10)
	if err != nil {
		t.Fatalf("SearchStocks returned error: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 {
		t.Fatalf("unexpected exact-name search result: %+v", got)
	}
	if got.Items[0].Symbol != "600519.SH" || got.Items[0].Code != "600519" || got.Items[0].Name != "贵州茅台" || got.Items[0].Exchange != "SH" {
		t.Fatalf("unexpected canonical result: %+v", got.Items[0])
	}
	byCode, err := p.SearchStocks(context.Background(), "300750", 10)
	if err != nil {
		t.Fatalf("SearchStocks by code returned error: %v", err)
	}
	if byCode.Total != 1 || byCode.Items[0].Symbol != "300750.SZ" || byCode.Items[0].Name != "宁德时代" {
		t.Fatalf("unexpected code search result: %+v", byCode)
	}
	if requests != 1 {
		t.Fatalf("directory should be fetched once and reused from cache, got %d requests", requests)
	}
}

func TestEastmoneyHotRankingEndpointsAreConfigurable(t *testing.T) {
	p := NewEastmoneyConfiguredWithAnnouncements(2*time.Second, "", "", "", "", "", "", "", "", "", "", "")
	p.WithHotRankingURLs("  https://rank.example.test/current  ", "  https://rank.example.test/surge  ")
	if p.hotRankingURL != "https://rank.example.test/current" {
		t.Fatalf("popular ranking URL = %q", p.hotRankingURL)
	}
	if p.hotSurgeURL != "https://rank.example.test/surge" {
		t.Fatalf("surge ranking URL = %q", p.hotSurgeURL)
	}

	legacy := NewEastmoneyConfiguredWithAnnouncements(2*time.Second, "", "", "", "", "", "", "", "", "", "", "")
	if legacy.hotRankingURL != eastmoneyHotRankURL || legacy.hotSurgeURL != eastmoneyHotSurgeURL {
		t.Fatal("existing constructors must preserve the default Eastmoney ranking endpoints")
	}
}


// eastmoneyStockSearchRowsForContractTest 生成用于分页契约测试的有效证券目录行。
func eastmoneyStockSearchRowsForContractTest(start, count int) []map[string]any {
	rows := make([]map[string]any, 0, count)
	for index := start; index < start+count; index++ {
		rows = append(rows, map[string]any{
			"f12": fmt.Sprintf("%06d", 300000+index),
			"f13": 0,
			"f14": fmt.Sprintf("测试股票%d", index),
		})
	}
	return rows
}

func TestEastmoneyStockSearchRejectsChangingTotalBetweenPages(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page, err := strconv.Atoi(r.URL.Query().Get("pn"))
		if err != nil {
			t.Errorf("invalid page number: %v", err)
		}
		total := 101
		count := 100
		if page > 1 {
			total = 102
			count = 2
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"rc": 0,
			"data": map[string]any{
				"total": total,
				"diff":  eastmoneyStockSearchRowsForContractTest((page-1)*100, count),
			},
		}); err != nil {
			t.Errorf("encode page: %v", err)
		}
	}))
	defer server.Close()

	p := &Eastmoney{client: server.Client(), stockListURL: server.URL, ut: "test-token"}
	_, err := p.SearchStocks(context.Background(), "测试股票", 10)
	if err == nil || !strings.Contains(err.Error(), "total changed across pages") {
		t.Fatalf("directory with changing page totals must be rejected, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("pagination should stop when page total changes, got %d requests", requests)
	}
}

func TestEastmoneyStockSearchRejectsPageRowsBeyondDeclaredTotal(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		page, err := strconv.Atoi(r.URL.Query().Get("pn"))
		if err != nil {
			t.Errorf("invalid page number: %v", err)
		}
		count := 100
		if page > 1 {
			count = 2 // total=101 时，末页最多只能返回 1 条。
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{
			"rc": 0,
			"data": map[string]any{
				"total": 101,
				"diff":  eastmoneyStockSearchRowsForContractTest((page-1)*100, count),
			},
		}); err != nil {
			t.Errorf("encode page: %v", err)
		}
	}))
	defer server.Close()

	p := &Eastmoney{client: server.Client(), stockListURL: server.URL, ut: "test-token"}
	_, err := p.SearchStocks(context.Background(), "测试股票", 10)
	if err == nil || !strings.Contains(err.Error(), "exceeding 1 rows implied by total 101") {
		t.Fatalf("page rows beyond declared total must be rejected, got %v", err)
	}
	if requests != 2 {
		t.Fatalf("pagination should stop on an overfull page, got %d requests", requests)
	}
}


func TestEastmoneyFetchSectorRankingsRejectsUnderfilledPage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":{"total":120,"diff":[{"f2":1234.5,"f3":2.3,"f4":27.8,"f6":123456789,"f12":"BK0475","f14":"电池","f104":15,"f105":7,"f128":"宁德时代","f136":4.12}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithMarketData(2*time.Second, "", "", "", server.URL, "")
	_, err := p.FetchSectorRankings(context.Background(), "industry", 5)
	if err == nil || !strings.Contains(err.Error(), "expected 5 rows from total=120 and limit=5") {
		t.Fatalf("underfilled sector-ranking response must be rejected, got %v", err)
	}
}
