package marketdatahttp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFetchStockBundlePreservesUnitsSourceAndQuality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer market-secret" {
			t.Errorf("missing auth header")
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/bundles/") {
			http.Error(w, "auxiliary provider disabled in this fixture", http.StatusServiceUnavailable)
			return
		}
		if r.URL.Query().Get("adjustment") != "forward_adjusted" {
			t.Errorf("unexpected adjustment=%s", r.URL.Query().Get("adjustment"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "symbol":"600519.SH","name":"贵州茅台","fetched_at":"2026-10-21T03:28:02Z",
		  "quality_status":"degraded","sources":{"daily_bars":"eastmoney"},"warnings":["quote_and_daily_bars_adjustment_basis_mismatch"],
		  "quote":{"symbol":"600519.SH","name":"贵州茅台","current_price":1313.31,"change":-18.57,"change_percent":-1.394,"open":1331.88,"high":1331.98,"low":1312,"prev_close":1331.88,"volume":3413600,"amount":4525398959,"quote_time":"2026-10-21T03:28:02Z","fetched_at":"2026-10-21T03:28:02Z","normalized_at":"2026-10-21T03:28:02Z","source":"eastmoney","volume_unit":"shares","amount_unit":"CNY","adjustment_basis":"none","quality_status":"valid"},
		  "daily_bars":[{"symbol":"600519.SH","trade_date":"2026-10-20","period":"1d","open":1321.01,"high":1335,"low":1316.14,"close":1331.88,"volume":3413600,"amount":4525398959,"change_percent":0.81,"adjustment_basis":"forward_adjusted","source":"eastmoney","volume_unit":"shares","amount_unit":"CNY","quality_status":"valid"}]
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "market-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := client.FetchStockBundle(context.Background(), "600519", 60, "2026-10-21T03:28:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Code != "600519" {
		t.Fatalf("code=%s", bundle.Code)
	}
	if bundle.Quote == nil || bundle.Quote.Volume == nil || *bundle.Quote.Volume != 3413600 {
		t.Fatalf("quote volume not mapped to shares: %+v", bundle.Quote)
	}
	if bundle.Quote.VolumeUnit != "shares" || bundle.Quote.AmountUnit != "CNY" || bundle.Quote.Source != "eastmoney" {
		t.Fatalf("quote metadata lost: %+v", bundle.Quote)
	}
	if len(bundle.DailyBars) != 1 || bundle.DailyBars[0].AdjustmentBasis != "forward_adjusted" {
		t.Fatalf("bar metadata lost: %+v", bundle.DailyBars)
	}
	if bundle.DailyBars[0].Amount != 4525398959 || bundle.DailyBars[0].Volume != 3413600 {
		t.Fatalf("bar values not mapped: %+v", bundle.DailyBars[0])
	}
	if bundle.QualityStatus != "degraded" || len(bundle.Warnings) == 0 {
		t.Fatalf("quality metadata lost: %+v", bundle)
	}
}

func TestNewClientRejectsURLCredentialsAndMissingKey(t *testing.T) {
	if _, err := NewClient("http://user:pass@localhost:8091", "x", time.Second); err == nil {
		t.Fatal("expected credentials in URL to be rejected")
	}
	if _, err := NewClient("http://localhost:8091", "", time.Second); err == nil {
		t.Fatal("expected missing API key to be rejected")
	}
}

func TestClientRejectsTrailingJSONValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{} {}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "market-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := client.get(context.Background(), "/invalid", &result); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("expected multiple-JSON-values rejection, got %v", err)
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"padding":"%s"}`, strings.Repeat("x", int(maxMarketDataResponseBytes)))))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "market-secret", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := client.get(context.Background(), "/large", &result); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected response size-limit rejection, got %v", err)
	}
}

func TestFetchMarketOverviewAggregatesNativeDomesticEndpoints(t *testing.T) {
	fetchedAt := time.Now().UTC().Format(time.RFC3339Nano)
	responses := map[string]string{
		"/api/v1/market/indices": fmt.Sprintf(`{"items":[{"symbol":"000001.SH","code":"000001","name":"上证指数","latest_price":3250.5,"change":10.5,"change_percent":0.32,"open":3240,"high":3260,"low":3235,"prev_close":3240,"amount":120000000000,"amount_unit":"CNY"}],"source":"eastmoney","fetched_at":%q,"quality_status":"valid"}`, fetchedAt),
		"/api/v1/market/breadth": fmt.Sprintf(`{"total_securities":5200,"covered_securities":5100,"advancing_count":2800,"declining_count":2100,"unchanged_count":200,"unclassified_count":100,"limit_up_count":52,"limit_down_count":8,"total_amount_cny":1234567890000,"amount_coverage_count":5100,"provider_report_date":"2026-10-27","source":"eastmoney","fetched_at":%q,"quality_status":"degraded","quality_reasons":["timestamp_precision_limited"]}`, fetchedAt),
		"/api/v1/market/sectors": "",
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer market-secret" {
			t.Errorf("missing market-data auth header")
		}
		body, ok := responses[r.URL.Path]
		if !ok {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/api/v1/market/sectors" {
			category := r.URL.Query().Get("type")
			if (category != "industry" && category != "concept") || r.URL.Query().Get("limit") != "20" {
				t.Errorf("unexpected sector query: %s", r.URL.RawQuery)
			}
			name := map[string]string{"industry": "银行", "concept": "人工智能"}[category]
			body = fmt.Sprintf(`{"category":%q,"total":1,"items":[{"rank":1,"code":"BK0001","name":%q,"category":%q,"latest_price":1000,"change":25,"change_percent":2.5,"amount":1000000,"amount_unit":"CNY","advancing_count":30,"declining_count":5,"leading_stock":"示例股份","leader_change_percent":8.2,"source":"eastmoney","provider_report_date":"2026-10-27","quality_status":"valid"}],"source":"eastmoney","fetched_at":%q,"quality_status":"valid"}`, category, name, category, fetchedAt)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "market-secret", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.FetchMarketOverview(context.Background(), "CN", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Region != "cn" || got.Date != "2026-10-27" {
		t.Fatalf("unexpected market overview identity/date: %+v", got)
	}
	if len(got.Indices) != 1 || got.Indices[0].Current != 3250.5 || got.Indices[0].ChangePct != 0.32 {
		t.Fatalf("index quote not mapped: %+v", got.Indices)
	}
	if got.UpCount != 2800 || got.DownCount != 2100 || got.FlatCount != 200 || got.LimitUpCount != 52 || got.LimitDownCount != 8 {
		t.Fatalf("breadth counts not mapped: %+v", got)
	}
	if got.TotalAmount != 1234567890000 || len(got.TopSectors) != 1 || len(got.TopConcepts) != 1 {
		t.Fatalf("turnover/sectors not mapped: %+v", got)
	}
	if !strings.Contains(strings.Join(got.Warnings, " "), "bottom_sector_rankings_not_exposed") {
		t.Fatalf("missing bottom-sector limitation warning: %+v", got.Warnings)
	}
	if _, err := client.FetchMarketOverview(context.Background(), "us", ""); err == nil {
		t.Fatal("native provider must reject non-CN region")
	}
	staleAsOf := time.Now().Add(-2 * time.Minute).UTC().Format(time.RFC3339Nano)
	if _, err := client.FetchMarketOverview(context.Background(), "cn", staleAsOf); err == nil {
		t.Fatal("historical as_of must not be mislabeled as live")
	}
}

func TestFetchStockBundleFetchesNativeAuxiliaryData(t *testing.T) {
	fetchedAt := time.Now().UTC().Format(time.RFC3339Nano)
	year, quarter := latestPublicReportPeriod(time.Now())
	periodEnd := time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	publicationDate := time.Date(year, time.Month(quarter*3+1), 20, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	var mu sync.Mutex
	seen := make(map[string]int)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer market-secret" {
			t.Errorf("missing market-data auth header for %s", r.URL.Path)
		}
		mu.Lock()
		seen[r.URL.Path]++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/bundles/"):
			_, _ = w.Write([]byte(fmt.Sprintf(`{"symbol":"600519.SH","name":"贵州茅台","daily_bars":[],"sources":{"daily_bars":"eastmoney"},"fetched_at":%q,"quality_status":"valid"}`, fetchedAt)))
		case strings.HasSuffix(r.URL.Path, "/fundamentals"):
			if r.URL.Query().Get("year") != fmt.Sprintf("%d", year) || r.URL.Query().Get("quarter") != fmt.Sprintf("%d", quarter) {
				t.Errorf("unexpected requested fundamentals period: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(fmt.Sprintf(`{"symbol":"600519.SH","year":%d,"quarter":%d,"has_data":true,"sections":[{"category":"profitability","report_date":%q,"publication_date":%q,"has_data":true,"metrics":[{"key":"roe_avg","name":"平均净资产收益率","value":0.074,"unit":"ratio","source_field":"roeAvg"}],"quality_status":"valid","quality_reasons":[]}],"source":"baostock","source_version":"test-v1","fetched_at":%q,"quality_status":"valid","quality_reasons":[]}`, year, quarter, periodEnd, publicationDate, fetchedAt)))
		case strings.HasSuffix(r.URL.Path, "/boards"):
			_, _ = w.Write([]byte(fmt.Sprintf(`{"symbol":"600519.SH","total":1,"has_data":true,"items":[{"code":"BK0477","name":"白酒","change_percent":1.2,"source":"eastmoney","source_version":"test-v1","fetched_at":%q,"quality_status":"valid","quality_reasons":[]}],"source":"eastmoney","source_version":"test-v1","fetched_at":%q,"quality_status":"valid","quality_reasons":[]}`, fetchedAt, fetchedAt)))
		case strings.HasSuffix(r.URL.Path, "/chip-distribution"):
			_, _ = w.Write([]byte(fmt.Sprintf(`{"symbol":"600519.SH","date":"2026-10-27","average_cost":1500.5,"cost_90_low":1300,"cost_90_high":1700,"cost_70_low":1400,"cost_70_high":1600,"profit_ratio":0.82,"concentration_90":0.15,"concentration_70":0.11,"source":"eastmoney","source_version":"test-v1","provider_date":"2026-10-27","fetched_at":%q,"quality_status":"valid","quality_reasons":[]}`, fetchedAt)))
		case strings.HasSuffix(r.URL.Path, "/capital-flow"):
			_, _ = w.Write([]byte(fmt.Sprintf(`{"symbol":"600519.SH","main_net_inflow_yuan":1200000,"inflow_5d_yuan":4200000,"inflow_10d_yuan":7600000,"observations":10,"source":"miaoxiang","source_version":"test-v1","fetched_at":%q,"quality_status":"degraded","quality_reasons":["exact_publication_time_unavailable"]}`, fetchedAt)))
		case strings.HasSuffix(r.URL.Path, "/profile"):
			_, _ = w.Write([]byte(fmt.Sprintf(`{"symbol":"600519.SH","name":"贵州茅台","ipo_date":"2001-08-27","security_type":1,"security_type_label":"stock","listing_status":1,"listing_status_label":"listed","pe_dynamic":23.5,"pb":8.4,"industry":"白酒","market_cap_cny":2100000000000,"float_market_cap_cny":2090000000000,"board_code":"SH","roe_percent":25.2,"net_profit_margin_percent":52.1,"net_profit_cny":80000000000,"gross_margin_percent":91.4,"source":"eastmoney","source_version":"test-v1","fetched_at":%q,"quality_status":"valid","quality_reasons":[],"normalization_version":"market-data-v1"}`, fetchedAt)))
		default:
			t.Errorf("unexpected route %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "market-secret", 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := client.FetchStockBundle(context.Background(), "600519", 60, "")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Fundamentals["roe_avg"] != 0.074 {
		t.Fatalf("flattened fundamental metric missing: %+v", bundle.Fundamentals)
	}
	if len(bundle.Boards) != 1 || bundle.Boards[0]["name"] != "白酒" {
		t.Fatalf("stock boards not mapped: %+v", bundle.Boards)
	}
	if bundle.Chip == nil || bundle.Chip.CostAverage == nil || *bundle.Chip.CostAverage != 1500.5 ||
		bundle.Chip.ProfitRatio == nil || *bundle.Chip.ProfitRatio != 0.82 {
		t.Fatalf("chip distribution not mapped: %+v", bundle.Chip)
	}
	if bundle.CapitalFlow["main_net_inflow_yuan"] != float64(1200000) {
		t.Fatalf("capital-flow data not mapped: %+v", bundle.CapitalFlow)
	}
	if bundle.Profile == nil || bundle.Profile.Industry != "白酒" || bundle.Profile.DynamicPE == nil || *bundle.Profile.DynamicPE != 23.5 {
		t.Fatalf("stock profile not mapped: %+v", bundle.Profile)
	}
	for _, sourceKey := range []string{"fundamentals", "stock_boards", "chip_distribution", "capital_flow", "stock_profile"} {
		if bundle.Sources[sourceKey] == "" {
			t.Errorf("missing source provenance for %s: %+v", sourceKey, bundle.Sources)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range []string{
		"/api/v1/bundles/600519",
		"/api/v1/stocks/600519/fundamentals",
		"/api/v1/stocks/600519/boards",
		"/api/v1/stocks/600519/chip-distribution",
		"/api/v1/stocks/600519/capital-flow",
		"/api/v1/stocks/600519/profile",
	} {
		if seen[path] != 1 {
			t.Errorf("route %s called %d times, want once", path, seen[path])
		}
	}
}

func TestLatestPublicReportPeriodRespectsDisclosureDeadlines(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	cases := []struct {
		date    string
		year    int
		quarter int
	}{
		{"2026-01-15", 2025, 3},
		{"2026-04-30", 2025, 3},
		{"2026-05-01", 2026, 1},
		{"2026-08-31", 2026, 1},
		{"2026-09-01", 2026, 2},
		{"2026-10-31", 2026, 2},
		{"2026-11-01", 2026, 3},
	}
	for _, tc := range cases {
		now, err := time.ParseInLocation("2006-01-02", tc.date, location)
		if err != nil {
			t.Fatal(err)
		}
		year, quarter := latestPublicReportPeriod(now)
		if year != tc.year || quarter != tc.quarter {
			t.Errorf("%s => %d Q%d, want %d Q%d", tc.date, year, quarter, tc.year, tc.quarter)
		}
	}
}

func TestFetchStockAnnouncementsPreservesPublicationSemantics(t *testing.T) {
	location := time.FixedZone("CST", 8*60*60)
	today := time.Now().In(location).Format("2006-01-02")
	announcementDate := time.Now().In(location).AddDate(0, 0, -1).Format("2006-01-02")
	publishedAt := announcementDate + "T15:30:00+08:00"
	fetchedAt := time.Now().UTC().Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/stocks/600519.SH/announcements" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer market-secret" {
			t.Error("missing market-data authorization")
		}
		if r.URL.Query().Get("start_date") != announcementDate || r.URL.Query().Get("end_date") != today || r.URL.Query().Get("page") != "1" || r.URL.Query().Get("page_size") != "100" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"symbol":"600519.SH","start_date":%q,"end_date":%q,"page":1,"page_size":100,"total":1,"pages":1,"returned":1,"has_data":true,"has_more":false,"items":[{"article_id":"AN-1","symbol":"600519.SH","stock_name":"贵州茅台","title":"贵州茅台年度报告","category":"定期报告","announcement_date":%q,"published_at":%q,"source_url":"https://data.eastmoney.com/notices/detail/600519/AN-1.html","source":"eastmoney","source_version":"test-v1","fetched_at":%q,"quality_status":"valid"}],"source":"eastmoney","source_version":"test-v1","fetched_at":%q,"quality_status":"valid"}`, announcementDate, today, announcementDate, publishedAt, fetchedAt, fetchedAt)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "market-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := client.FetchStockAnnouncements(context.Background(), "600519.SH", announcementDate, today, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("unexpected announcements: %+v", snapshot)
	}
	item := snapshot.Items[0]
	if item.AnnouncementDate != announcementDate || item.PublishedAt == nil || item.PublishedAt.Format(time.RFC3339) != publishedAt {
		t.Fatalf("publication-time semantics were not preserved: %+v", item)
	}
	if item.SourceURL == "" || item.Source != "eastmoney" || item.FetchedAt.IsZero() {
		t.Fatalf("source provenance was not preserved: %+v", item)
	}
}

func TestFetchStockBundleBuildsDragonTigerSummaryWithQualityBoundary(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	recentDate := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	olderDate := time.Now().In(loc).AddDate(0, 0, -2).Format("2006-01-02")
	fetchedAt := time.Now().UTC().Format(time.RFC3339Nano)
	var mu sync.Mutex
	seenDates := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/bundles/"):
			fmt.Fprintf(w, `{"symbol":"600519.SH","name":"贵州茅台","daily_bars":[{"trade_date":%q,"open":1500,"high":1510,"low":1490,"close":1505,"volume":100,"amount":150500,"pct_chg":0.5,"source":"eastmoney","adjustment_basis":"forward_adjusted"},{"trade_date":%q,"open":1501,"high":1512,"low":1495,"close":1508,"volume":120,"amount":181000,"pct_chg":0.2,"source":"eastmoney","adjustment_basis":"forward_adjusted"}],"sources":{"daily_bars":"eastmoney"},"fetched_at":%q,"quality_status":"valid"}`, recentDate, olderDate, fetchedAt)
		case r.URL.Path == "/api/v1/market/dragon-tiger":
			date := r.URL.Query().Get("date")
			mu.Lock()
			seenDates[date]++
			mu.Unlock()
			fmt.Fprintf(w, `{"trade_date":%q,"page":1,"page_size":500,"total":1,"pages":1,"returned":1,"source_returned":1,"has_data":true,"has_more":false,"items":[{"symbol":"600519.SH","code":"600519","name":"贵州茅台","trade_date":%q}],"source":"eastmoney","source_version":"test-v1","fetched_at":%q,"quality_status":"degraded","quality_reasons":["provider_snapshot_timestamp_not_exposed"]}`, date, date, fetchedAt)
		default:
			http.Error(w, "optional auxiliary fixture unavailable", http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "market-secret", 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := client.FetchStockBundle(context.Background(), "600519.SH", 30, "")
	if err != nil {
		t.Fatal(err)
	}
	summary := bundle.DragonTiger
	if summary == nil {
		t.Fatalf("dragon-tiger context summary missing; warnings=%v", bundle.Warnings)
	}
	if summary["is_on_list"] != true || summary["recent_count"] != 2 || summary["latest_date"] != recentDate {
		t.Fatalf("unexpected dragon-tiger summary: %#v", summary)
	}
	if summary["quality_status"] != "degraded" || summary["recent_count_is_lower_bound"] != false {
		t.Fatalf("quality and coverage boundary not preserved: %#v", summary)
	}
	if bundle.Sources["dragon_tiger"] != "eastmoney" {
		t.Fatalf("dragon-tiger source provenance missing: %#v", bundle.Sources)
	}
	mu.Lock()
	defer mu.Unlock()
	if seenDates[recentDate] != 1 || seenDates[olderDate] != 1 {
		t.Fatalf("recent trade dates should be queried once each: %#v", seenDates)
	}
}


func TestFetchDragonTigerDateChecksRawSourceCounts(t *testing.T) {
	date := "2026-10-23"
	fetchedAt := time.Now().UTC().Format(time.RFC3339Nano)
	cases := []struct {
		name          string
		total         int
		sourceReturned int
		excluded      int
		wantComplete  bool
	}{
		{name: "incomplete source rows", total: 2, sourceReturned: 1, excluded: 0, wantComplete: false},
		{name: "excluded non-A-share rows counted", total: 2, sourceReturned: 2, excluded: 1, wantComplete: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"trade_date":%q,"page":1,"page_size":500,"total":%d,"pages":1,"returned":1,"source_returned":%d,"excluded_non_a_share_count":%d,"has_data":true,"has_more":false,"items":[{"symbol":"600519.SH","code":"600519","trade_date":%q}],"source":"eastmoney","source_version":"test-v1","fetched_at":%q,"quality_status":"degraded","quality_reasons":[]}`,
					date, tc.total, tc.sourceReturned, tc.excluded, date, fetchedAt)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, "market-secret", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			result := client.fetchDragonTigerDate(context.Background(), date)
			if result.err != nil {
				t.Fatalf("unexpected request/contract error: %v", result.err)
			}
			if result.complete != tc.wantComplete {
				t.Fatalf("complete=%v want %v; reasons=%v", result.complete, tc.wantComplete, result.qualityReasons)
			}
			hasCountMismatchReason := false
			for _, reason := range result.qualityReasons {
				if reason == "dragon_tiger_source_row_count_does_not_match_total" {
					hasCountMismatchReason = true
					break
				}
			}
			if hasCountMismatchReason == tc.wantComplete {
				t.Fatalf("unexpected source count mismatch reason: %v", result.qualityReasons)
			}
		})
	}
}


func TestFetchStockProfileMapsValuationProfitabilityAndProvenance(t *testing.T) {
	fetchedAt := time.Now().UTC().Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/stocks/600519.SH/profile" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer market-secret" {
			t.Error("missing market-data authorization")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"symbol":"600519.SH","name":"贵州茅台","ipo_date":"2001-08-27","security_type":1,"security_type_label":"stock","listing_status":1,"listing_status_label":"listed","pe_dynamic":23.5,"pb":8.4,"industry":"白酒","market_cap_cny":2100000000000,"float_market_cap_cny":2090000000000,"board_code":"SH","roe_percent":25.2,"net_profit_margin_percent":52.1,"net_profit_cny":80000000000,"gross_margin_percent":91.4,"source":"eastmoney","source_version":"test-v1","fetched_at":%q,"quality_status":"valid","quality_reasons":[],"normalization_version":"market-data-v1"}`, fetchedAt)
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "market-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := client.FetchStockProfile(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if profile.Symbol != "600519.SH" || profile.Name != "贵州茅台" || profile.Industry != "白酒" {
		t.Fatalf("stock identity fields not mapped: %+v", profile)
	}
	if profile.DynamicPE == nil || *profile.DynamicPE != 23.5 || profile.PB == nil || *profile.PB != 8.4 ||
		profile.MarketCapCNY == nil || *profile.MarketCapCNY != 2100000000000 || profile.ROEPercent == nil || *profile.ROEPercent != 25.2 {
		t.Fatalf("valuation/profitability fields not mapped: %+v", profile)
	}
	if profile.Source != "eastmoney" || profile.QualityStatus != "valid" || profile.FetchedAt.IsZero() {
		t.Fatalf("profile quality/provenance not preserved: %+v", profile)
	}
}
