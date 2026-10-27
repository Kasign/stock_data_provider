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

func TestTencentQuoteNormalizesVolumeAndAmount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "utf8/q=") {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		f := make([]string, 60)
		for i := range f {
			f[i] = "0"
		}
		f[1] = "贵州茅台"
		f[2] = "600519"
		f[3] = "1313.31"
		f[4] = "1331.88"
		f[5] = "1331.88"
		f[6] = "34136"
		f[30] = "20261021112802"
		f[31] = "-18.57"
		f[32] = "-1.394"
		f[33] = "1331.98"
		f[34] = "1312"
		f[35] = "1313.31/34136/4525398959" // high-precision tuple amount in CNY
		f[37] = "452540"                   // legacy rounded turnover, in 10,000 CNY
		f[38] = "5.0"                      // turnover rate (%) for cross-checking volume unit
		f[44] = "896.8"                    // circulating market value in 100 million CNY
		f[57] = "999999999"                // unrelated field; must never be treated as turnover
		fmt.Fprintf(w, "v_sh600519=\"%s\";", strings.Join(f, "~"))
	}))
	defer srv.Close()
	p := NewTencent(2 * time.Second)
	p.quoteURL = srv.URL + "/utf8/q"
	q, err := p.FetchQuote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if q.QuoteTime == nil || q.QuoteTime.In(time.FixedZone("CST", 8*3600)).Format("2006-01-02 15:04:05") != "2026-10-21 11:28:02" {
		t.Fatalf("unexpected Tencent provider timestamp: %#v", q.QuoteTime)
	}
	if q.QuoteTimeSemantics != "provider_snapshot_time" {
		t.Fatalf("quote time semantics=%q, want provider_snapshot_time", q.QuoteTimeSemantics)
	}
	if *q.Volume != 3413600 {
		t.Fatalf("volume=%d", *q.Volume)
	}
	if *q.Amount < 4525398958 || *q.Amount > 4525398960 {
		t.Fatalf("amount=%v; expected the precise amount from field 35 tuple", *q.Amount)
	}
	if q.RawAmountField != "f35.amount" || q.RawAmountUnit != "CNY" {
		t.Fatalf("raw amount metadata=%q/%q, want f35.amount/CNY", q.RawAmountField, q.RawAmountUnit)
	}
	if q.RawVolumeUnit != "hands_inferred" {
		t.Fatalf("raw volume unit=%q, want hands_inferred from turnover cross-check", q.RawVolumeUnit)
	}
	if q.Name != "贵州茅台" {
		t.Fatalf("name=%s", q.Name)
	}
	if q.TurnoverRate == nil || *q.TurnoverRate != 5.0 {
		t.Fatalf("Tencent turnover rate must be returned in percentage points, got %v", q.TurnoverRate)
	}
	if q.VolumeRatio != nil {
		t.Fatalf("Tencent volume ratio must remain unknown unless the provider contract supplies it, got %v", q.VolumeRatio)
	}
}
func TestTencentQuoteAmountFallsBackWhenTupleAmountIsImplausible(t *testing.T) {
	fields := make([]string, 60)
	for i := range fields {
		fields[i] = "0"
	}
	fields[35] = "1313.31/34136/999999999999999" // implausible CNY amount
	fields[37] = "452539.8959"                   // plausible legacy amount in 10,000 CNY
	fields[57] = "999999999999999"               // unrelated field and ignored
	raw, amount, field, unit := tencentQuoteAmount(fields, 1312, 1331.98, 3413600)
	if raw == nil || amount == nil || field != "f37" || unit != "ten_thousand_CNY" {
		t.Fatalf("expected valid legacy fallback, got raw=%v amount=%v field=%q unit=%q", raw, amount, field, unit)
	}
	if *amount < 4525398958 || *amount > 4525398960 {
		t.Fatalf("unexpected fallback amount %.2f", *amount)
	}
}

func TestTencentQuoteVolumeUsesTurnoverAndMarketValueToInferUnit(t *testing.T) {
	fields := make([]string, 60)
	for i := range fields {
		fields[i] = "0"
	}
	fields[38] = "5.0"
	fields[44] = "896.8"
	shares, unit, verified, err := tencentQuoteVolume(fields, 34136, 1313.31)
	if err != nil {
		t.Fatal(err)
	}
	if shares != 3413600 || unit != "hands_inferred" || !verified {
		t.Fatalf("unexpected inferred volume: shares=%d unit=%q verified=%v", shares, unit, verified)
	}

	fields[38] = "0.5"
	fields[44] = "89.68" // expected volume is close to the raw value in shares
	shares, unit, verified, err = tencentQuoteVolume(fields, 34136, 1313.31)
	if err != nil {
		t.Fatal(err)
	}
	if shares != 34136 || unit != "shares_inferred" || !verified {
		t.Fatalf("expected inferred shares units, got shares=%d unit=%q verified=%v", shares, unit, verified)
	}
}

func TestTencentQuoteVolumeLeavesUnitUnknownWithoutCrosscheck(t *testing.T) {
	fields := make([]string, 60)
	for i := range fields {
		fields[i] = "0"
	}
	shares, unit, verified, err := tencentQuoteVolume(fields, 100, 10)
	if err != nil {
		t.Fatal(err)
	}
	if shares != 0 || unit != "unknown" || verified {
		t.Fatalf("unverified volume must not be normalized: shares=%d unit=%q verified=%v", shares, unit, verified)
	}
}

func TestTencentQuoteRejectsUnverifiedVolumeUnits(t *testing.T) {
	fields := make([]string, 60)
	for i := range fields {
		fields[i] = "0"
	}
	fields[1], fields[2] = "贵州茅台", "600519"
	fields[3], fields[4], fields[5], fields[6] = "100", "99", "99", "1234"
	fields[30], fields[31], fields[32], fields[33], fields[34] = "20261021112802", "1", "1.01", "101", "98"
	fields[35], fields[37] = "100/1234/123400", "-"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, "v_sh600519=%q;", strings.Join(fields, "~"))
	}))
	defer server.Close()
	p := NewTencent(2 * time.Second)
	p.quoteURL = server.URL + "/quote"
	if _, err := p.FetchQuote(context.Background(), "600519.SH"); err == nil || !strings.Contains(err.Error(), "volume unit cannot be confirmed") {
		t.Fatalf("unverified volume unit must reject this provider result, got %v", err)
	}
}

func TestTencentQuoteRejectsInvalidPriceContract(t *testing.T) {
	tests := []struct {
		name  string
		price string
		high  string
	}{
		{name: "non-finite current price", price: "NaN", high: "10.5"},
		{name: "current price above high", price: "10.8", high: "10.5"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fields := make([]string, 60)
			for i := range fields {
				fields[i] = "0"
			}
			fields[1], fields[2] = "Test", "600519"
			fields[3], fields[4], fields[5], fields[6] = tc.price, "9.8", "10.0", "1000"
			fields[30], fields[31], fields[32], fields[33], fields[34] = "20261021112802", "0.4", "4.08", tc.high, "9.5"
			fields[35] = "10.2/1000/1020000"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, "v_sh600519=\"%s\";", strings.Join(fields, "~"))
			}))
			defer server.Close()
			p := NewTencent(2 * time.Second)
			p.quoteURL = server.URL + "/quote"
			if _, err := p.FetchQuote(context.Background(), "600519.SH"); err == nil || !strings.Contains(err.Error(), "quote data contract") {
				t.Fatalf("expected invalid quote contract error, got %v", err)
			}
		})
	}
}

func TestTencentBarsExplicitlyMarkMissingAmount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"sh600519":{"qfqday":[["2026-10-20","1321.01","1331.88","1335.00","1316.14","34136"]]}}}`)
	}))
	defer srv.Close()
	p := NewTencent(2 * time.Second)
	p.klineURL = srv.URL
	p.noneKlineURL = srv.URL
	bars, err := p.FetchDailyBars(context.Background(), "600519", "2026-10-01", "2026-10-21", 100, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if *bars[0].Volume != 3413600 {
		t.Fatalf("volume=%d", *bars[0].Volume)
	}
	if bars[0].Amount != nil {
		t.Fatal("Tencent K-line amount must be unknown, not zero")
	}
	if bars[0].AmountUnit != "unknown" {
		t.Fatalf("Tencent K-line without amount must not claim an amount unit, got %q", bars[0].AmountUnit)
	}
}

func TestTencentBarsNeverTreatsTrailingFieldsAsAmount(t *testing.T) {
	responses := []string{
		`{"data":{"sh600519":{"qfqday":[["2026-10-20","1321.01","1331.88","1335.00","1316.14","34136",{"corporate_action":"metadata"}]]}}}`,
		`{"data":{"sh600519":{"qfqday":[["2026-10-20","1321.01","1331.88","1335.00","1316.14","34136","4525398959.00"]]}}}`,
	}
	for i, response := range responses {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, response) }))
		p := NewTencent(2 * time.Second)
		p.klineURL, p.noneKlineURL = srv.URL, srv.URL
		bars, err := p.FetchDailyBars(context.Background(), "600519", "2026-10-01", "2026-10-21", 100, model.AdjustmentForward)
		srv.Close()
		if err != nil {
			t.Fatalf("fixture %d: %v", i, err)
		}
		if len(bars) != 1 || *bars[0].Volume != 3413600 {
			t.Fatalf("fixture %d should preserve row, got %+v", i, bars)
		}
		if bars[0].Amount != nil || bars[0].RawAmount != nil {
			t.Fatalf("undocumented trailing fields must never be treated as turnover, got %+v", bars[0])
		}
		if bars[0].AmountUnit != "unknown" || bars[0].RawAmountUnit != "" || bars[0].RawAmountField != "" {
			t.Fatalf("unknown trailing field must not get fabricated amount metadata: %+v", bars[0])
		}
		if bars[0].QualityStatus != model.QualityDegraded {
			t.Fatalf("Tencent K-line without a verified amount must be degraded, got %+v", bars[0])
		}
		foundReason := false
		for _, reason := range bars[0].QualityReasons {
			if reason == "tencent_daily_kline_has_no_verified_amount_field" {
				foundReason = true
			}
		}
		if !foundReason {
			t.Fatalf("missing explicit Tencent amount limitation: %+v", bars[0].QualityReasons)
		}
	}
}

func TestTencentStarMarketDailyVolumeAlreadyInShares(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"sh688111":{"qfqday":[["2026-10-20","200.00","201.00","202.00","199.00","4421640","887000000"]]}}}`)
	}))
	defer srv.Close()
	p := NewTencent(2 * time.Second)
	p.klineURL, p.noneKlineURL = srv.URL, srv.URL
	bars, err := p.FetchDailyBars(context.Background(), "688111.SH", "2026-10-01", "2026-10-21", 100, model.AdjustmentForward)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("bars=%+v", bars)
	}
	if got := *bars[0].Volume; got != 4421640 {
		t.Fatalf("STAR market volume must remain shares, got %d", got)
	}
	if bars[0].RawVolumeUnit != "shares" || bars[0].VolumeUnit != "shares" {
		t.Fatalf("STAR market raw and normalized volume unit metadata must both be shares, got raw=%q normalized=%q", bars[0].RawVolumeUnit, bars[0].VolumeUnit)
	}
	if bars[0].Amount != nil || bars[0].AmountUnit != "unknown" {
		t.Fatalf("undocumented trailing numeric field must not be interpreted as amount, got %+v", bars[0])
	}
}

func TestTencentDailyBarsUsesRequestedAdjustmentEndpointAndPayloadKey(t *testing.T) {
	type requestRecord struct {
		path  string
		param string
	}
	var requests []requestRecord

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		param := r.URL.Query().Get("param")
		requests = append(requests, requestRecord{path: r.URL.Path, param: param})
		switch r.URL.Path {
		case "/none":
			if param != "sh600036,day,2026-10-19,2026-10-21,8" {
				http.Error(w, "unexpected unadjusted parameters", http.StatusBadRequest)
				return
			}
		case "/adjusted":
			if param != "sh600036,day,2026-10-19,2026-10-21,8,qfq" &&
				param != "sh600036,day,2026-10-19,2026-10-21,8,hfq" {
				http.Error(w, "unexpected adjusted parameters", http.StatusBadRequest)
				return
			}
		default:
			http.NotFound(w, r)
			return
		}

		fmt.Fprint(w, `{"data":{"sh600036":{
			"day":[["2026-10-20","42.95","42.52","43.13","42.18","760948"]],
			"qfqday":[["2026-10-20","43.40","43.05","43.60","43.00","760948"]],
			"hfqday":[["2026-10-20","91.40","90.50","92.00","90.00","760948"]]
		}}}`)
	}))
	defer srv.Close()

	p := NewTencentConfigured(2*time.Second, srv.URL+"/quote", srv.URL+"/adjusted", srv.URL+"/none")
	tests := []struct {
		name      string
		basis     model.AdjustmentBasis
		wantPath  string
		wantParam string
		wantClose float64
	}{
		{
			name:      "none uses plain endpoint and day key",
			basis:     model.AdjustmentNone,
			wantPath:  "/none",
			wantParam: "sh600036,day,2026-10-19,2026-10-21,8",
			wantClose: 42.52,
		},
		{
			name:      "forward adjusted uses qfq parameter and qfqday key",
			basis:     model.AdjustmentForward,
			wantPath:  "/adjusted",
			wantParam: "sh600036,day,2026-10-19,2026-10-21,8,qfq",
			wantClose: 43.05,
		},
		{
			name:      "backward adjusted uses hfq parameter and hfqday key",
			basis:     model.AdjustmentBackward,
			wantPath:  "/adjusted",
			wantParam: "sh600036,day,2026-10-19,2026-10-21,8,hfq",
			wantClose: 90.50,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bars, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-19", "2026-10-21", 8, tc.basis)
			if err != nil {
				t.Fatalf("FetchDailyBars returned error: %v", err)
			}
			if len(bars) != 1 {
				t.Fatalf("bar count=%d, want 1", len(bars))
			}
			if bars[0].AdjustmentBasis != tc.basis {
				t.Fatalf("adjustment basis=%q, want %q", bars[0].AdjustmentBasis, tc.basis)
			}
			if bars[0].Close != tc.wantClose {
				t.Fatalf("close=%v, want %v", bars[0].Close, tc.wantClose)
			}
			if bars[0].Amount != nil || bars[0].AmountUnit != "unknown" || bars[0].QualityStatus != model.QualityDegraded {
				t.Fatalf("unverified Tencent K-line amount must remain unknown/degraded, got %+v", bars[0])
			}
			if *bars[0].Volume != 76094800 || bars[0].RawVolumeUnit != "hands" {
				t.Fatalf("unexpected volume normalization: volume=%v raw_unit=%q", bars[0].Volume, bars[0].RawVolumeUnit)
			}
		})
		if got := requests[len(requests)-1]; got.path != tc.wantPath || got.param != tc.wantParam {
			t.Fatalf("request path/param=%q/%q, want %q/%q", got.path, got.param, tc.wantPath, tc.wantParam)
		}
	}
}

func TestTencentDailyBarsSortsAndDropsDuplicateOutOfRangeAndInvalidRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":{"sh600519":{"qfqday":[
			["2026-10-21","10.0","10.2","10.3","9.9","100"],
			["2026-10-19","9.8","10.0","10.1","9.7","100"],
			["2026-10-21","10.0","10.1","10.3","9.9","100"],
			["2026-10-18","9.5","9.6","9.7","9.4","100"],
			["2026-10-20","10.0","10.5","10.4","9.9","100"]
		]}}}`)
	}))
	defer server.Close()
	p := NewTencent(2 * time.Second)
	p.klineURL, p.noneKlineURL = server.URL, server.URL
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

func TestTencentDailyBarsRejectsMissingRequestedAdjustedSeries(t *testing.T) {
	for _, basis := range []model.AdjustmentBasis{model.AdjustmentForward, model.AdjustmentBackward} {
		t.Run(string(basis), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprint(w, `{"data":{"sh600036":{"day":[["2026-10-20","42.95","42.52","43.13","42.18","760948"]]}}}`)
			}))
			defer srv.Close()

			p := NewTencentConfigured(2*time.Second, srv.URL+"/quote", srv.URL+"/adjusted", srv.URL+"/none")
			if _, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-19", "2026-10-21", 8, basis); err == nil {
				t.Fatal("missing requested adjusted series must return an error instead of using unadjusted day data")
			}
		})
	}
}

func TestTencentChinaLocationFallsBackWhenZoneinfoIsUnavailable(t *testing.T) {
	loc := tencentChinaLocation(func(string) (*time.Location, error) {
		return nil, fmt.Errorf("zoneinfo unavailable in minimal container")
	})
	_, offset := time.Date(2026, 10, 27, 9, 30, 0, 0, loc).Zone()
	if offset != 8*60*60 {
		t.Fatalf("fallback location offset = %d seconds, want UTC+8", offset)
	}
}

func TestParseTencentTimeTreatsProviderTimestampAsChinaLocalTime(t *testing.T) {
	got := parseTencentTime("20261027093000")
	if got == nil {
		t.Fatal("valid Tencent quote timestamp should be parsed")
	}
	want := time.Date(2026, 10, 27, 1, 30, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Fatalf("parsed timestamp = %s, want %s", got.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}
