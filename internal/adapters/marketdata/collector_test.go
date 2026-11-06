package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

func TestCollectorPreservesNativeProviderAndAdjustmentMetadata(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 20, 15, 5, 0, 0, loc)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != testAPIKey {
			t.Errorf("missing API key")
		}
		switch r.URL.Path {
		case "/api/v1/quotes/600519.SH":
			_, _ = fmt.Fprint(w, `{"symbol":"600519.SH","name":"贵州茅台","current_price":1326,"change":6,"change_percent":0.45,"open":1320,"high":1330,"low":1310,"prev_close":1320,"volume":10000,"amount":13260000,"quote_time":"2026-10-20T07:00:00Z","fetched_at":"2026-10-20T07:00:01Z","source":"eastmoney","source_version":"v1","quality_status":"valid","quality_reasons":[],"volume_unit":"shares","amount_unit":"CNY","adjustment_basis":"none"}`)
		case "/api/v1/bars/600519.SH/daily":
			_, _ = fmt.Fprint(w, `{"symbol":"600519.SH","period":"1d","adjustment_basis":"forward_adjusted","volume_unit":"shares","amount_unit":"CNY","quality_status":"valid","warnings":[],"bars":[{"symbol":"600519.SH","trade_date":"2026-10-19","period":"1d","open":1320,"high":1330,"low":1310,"close":1326,"volume":10000,"amount":13260000,"change_percent":0.45,"adjustment_basis":"forward_adjusted","source":"eastmoney","source_version":"v1","provider_timestamp":"2026-10-19T07:00:00Z","fetched_at":"2026-10-20T07:00:01Z","normalized_at":"2026-10-20T07:00:01Z","volume_unit":"shares","amount_unit":"CNY","quality_status":"valid","quality_reasons":[]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := testConfig(server.URL)
	cfg.IndexSymbols = nil
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	collector, err := NewCollector(client, cfg)
	if err != nil {
		t.Fatal(err)
	}
	request := domain.RunRequest{AsOf: asOf}
	universe := domain.UniverseSnapshot{Securities: []domain.Security{{Symbol: "600519", AssetType: "equity"}}}
	requirements := domain.RequirementSet{Core: []domain.DataRequirement{
		{Dataset: historyDataset, Required: true},
		{Dataset: quoteDataset, Required: false},
	}}
	snapshot, err := collector.Collect(context.Background(), request, universe, requirements)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(snapshot.Quotes) != 1 || len(snapshot.Bars) != 1 {
		t.Fatalf("expected one quote and one bar, got quotes=%d bars=%d notes=%+v", len(snapshot.Quotes), len(snapshot.Bars), snapshot.SourceNotes)
	}
	if snapshot.Quotes[0].Symbol != "600519" || snapshot.Quotes[0].Source != "eastmoney" {
		t.Fatalf("quote source/symbol not preserved: %+v", snapshot.Quotes[0])
	}
	bar := snapshot.Bars[0]
	if bar.Symbol != "600519" || bar.Source != "eastmoney" || bar.AdjustBasis != "forward_adjusted" {
		t.Fatalf("daily-bar source or adjustment basis not preserved: %+v", bar)
	}
	if bar.Volume == nil || *bar.Volume != 10000 {
		t.Fatalf("share volume not preserved: %+v", bar.Volume)
	}
	foundAvailabilityWarning := false
	for _, note := range snapshot.SourceNotes {
		if note.QualityStatus == "partial_metadata" && contains(note.MissingFields, "available_at") {
			foundAvailabilityWarning = true
		}
		if contains(note.MissingFields, "adjustment_basis") {
			t.Fatalf("known adjustment basis must not be marked missing: %+v", note)
		}
	}
	if !foundAvailabilityWarning {
		t.Fatalf("expected honest point-in-time availability warning; notes=%+v", snapshot.SourceNotes)
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
