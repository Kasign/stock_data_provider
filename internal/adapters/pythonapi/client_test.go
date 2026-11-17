package pythonapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

func TestClientQuoteHistoryAndBasePath(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/prefix/api/v1/stocks/600519/quote", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing bearer token")
		}
		if r.URL.Path != "/prefix/api/v1/stocks/600519/quote" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stock_code":"600519","stock_name":"贵州茅台","current_price":1326,"update_time":"2026-10-20 14:52:15"}`))
	})
	mux.HandleFunc("/prefix/api/v1/stocks/600519/history", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("days") != "50" || r.URL.Query().Get("period") != "daily" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"stock_code":"600519","stock_name":"贵州茅台","period":"daily","data":[{"date":"2026-10-19","open":1320,"high":1330,"low":1310,"close":1326,"volume":12,"amount":15912}]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	cfg := DefaultConfig()
	cfg.BaseURL = server.URL + "/prefix"
	cfg.BearerToken = "test-token"
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	quote, _, err := client.GetQuote(context.Background(), "600519")
	if err != nil {
		t.Fatal(err)
	}
	if quote.CurrentPrice != 1326 || quote.StockName != "贵州茅台" {
		t.Fatalf("unexpected quote: %#v", quote)
	}
	history, err := client.GetHistory(context.Background(), "600519", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.Bars) != 1 || history.Bars[0].Close == nil || *history.Bars[0].Close != 1326 {
		t.Fatalf("unexpected history: %#v", history)
	}
}

func TestClientRejectsInvalidURLAndBadQuote(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "ftp://example.com"}); err == nil {
		t.Fatal("expected invalid URL error")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"stock_code":"600519","current_price":0}`))
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.GetQuote(context.Background(), "600519"); err == nil {
		t.Fatal("expected zero quote to be rejected")
	}
}

func TestClientStatusErrorDoesNotExposeBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("sensitive error detail"))
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.GetQuote(context.Background(), "600519")
	if err == nil || strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseTimestampUsesMarketTimezoneForNaiveTime(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseTimestamp("2026-10-20 15:00:00", loc)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := parsed.UTC().Format(time.RFC3339), "2026-10-20T07:00:00Z"; got != want {
		t.Fatalf("timestamp = %s, want %s", got, want)
	}
}

func TestCollectorFiltersFutureBarsAndPreservesMissingMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/history"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"stock_code":"600519","stock_name":"贵州茅台","period":"daily","data":[
				{"date":"2026-10-19","open":1320,"high":1330,"low":1310,"close":1326},
				{"date":"2026-10-21","open":1320,"high":1330,"low":1310,"close":1326,"volume":10,"amount":10000}
			]}`))
		case strings.HasSuffix(r.URL.Path, "/quote"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"stock_code":"600519","current_price":1326,"update_time":"2026-10-20 14:00:00"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.IndexSymbols = nil
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := NewCollector(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	asOf, err := time.ParseInLocation("2006-01-02 15:00:00", "2026-10-20 15:00:00", time.FixedZone("CST", 8*3600))
	if err != nil {
		t.Fatal(err)
	}
	request := domain.RunRequest{AsOf: asOf}
	universe := domain.UniverseSnapshot{Securities: []domain.Security{{Symbol: "600519", AssetType: "stock"}}}
	requirements := domain.RequirementSet{Core: []domain.DataRequirement{{Dataset: historyDataset, Required: true}, {Dataset: quoteDataset}}}
	snapshot, err := collector.Collect(context.Background(), request, universe, requirements)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Bars) != 1 {
		t.Fatalf("expected future bar to be filtered; bars=%d", len(snapshot.Bars))
	}
	if snapshot.Bars[0].TradeDate.Format("2006-01-02") != "2026-10-19" {
		t.Fatalf("unexpected retained bar date: %s", snapshot.Bars[0].TradeDate)
	}
	if len(snapshot.Bars[0].MissingFields) != 2 {
		t.Fatalf("expected missing volume/amount markers, got %#v", snapshot.Bars[0].MissingFields)
	}
	if len(snapshot.Quotes) != 1 {
		t.Fatalf("expected quote to be present, got %d", len(snapshot.Quotes))
	}
	for _, note := range snapshot.SourceNotes {
		if note.Dataset == historyDataset && !strings.Contains(strings.Join(note.MissingFields, ","), "adjustment_basis") {
			t.Fatalf("expected missing adjustment metadata note: %#v", note)
		}
	}
}

func TestGetDataOverviewReturnsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/data/overview" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"providers": []any{}, "datasets": []any{}})
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := client.GetDataOverview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(payload) {
		t.Fatalf("invalid JSON: %s", payload)
	}
}

func TestClientRejectsUnsafeSymbol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("unsafe symbol must be rejected before a request")
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := client.GetQuote(context.Background(), "../../admin"); err == nil {
		t.Fatal("expected unsafe symbol error")
	}
}

func TestCollectorRejectsQuoteAfterAsOf(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/quote"):
			_, _ = w.Write([]byte(`{"stock_code":"600519","current_price":1326,"update_time":"2026-10-20 15:01:00"}`))
		case strings.HasSuffix(r.URL.Path, "/history"):
			_, _ = w.Write([]byte(`{"stock_code":"600519","data":[{"date":"2026-10-19","open":1320,"high":1330,"low":1310,"close":1326}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cfg := DefaultConfig()
	cfg.BaseURL = server.URL
	cfg.IndexSymbols = nil
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := NewCollector(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Shanghai")
	asOf := time.Date(2026, 10, 20, 15, 0, 0, 0, loc)
	snapshot, err := collector.Collect(context.Background(), domain.RunRequest{AsOf: asOf, AsOfMode: domain.AsOfModeExplicit}, domain.UniverseSnapshot{Securities: []domain.Security{{Symbol: "600519", AssetType: "stock"}}}, domain.RequirementSet{Core: []domain.DataRequirement{{Dataset: quoteDataset}, {Dataset: historyDataset}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Quotes) != 0 {
		t.Fatalf("future quote should be discarded, got %d", len(snapshot.Quotes))
	}
	found := false
	for _, note := range snapshot.SourceNotes {
		if note.Dataset == quoteDataset && note.QualityStatus == "unavailable" && strings.Contains(note.Diagnostic, "later than request as_of") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected explicit future-quote diagnostic: %#v", snapshot.SourceNotes)
	}
}
