package provider

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestLiveProviderProbeQuarterlyFundamentals(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_PROVIDER_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_PROVIDER_TESTS=1 to query live BaoStock quarterly financial endpoints")
	}
	client := baostock.NewClient(baostock.Config{ConnectTimeout: 8 * time.Second, RequestTimeout: 20 * time.Second})
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("BaoStock fundamentals cleanup warning: %v", err)
		}
	}()
	p := NewBaostock(client)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	started := time.Now()
	snapshot, err := p.FetchFundamentals(ctx, "600519.SH", 2026, 2)
	result := map[string]any{
		"provider": "baostock", "capability": "fundamentals", "symbol": "600519.SH",
		"year": 2026, "quarter": 2, "elapsed_ms": time.Since(started).Milliseconds(),
		"status": "failed",
	}
	if snapshot != nil {
		result["fundamentals"] = snapshot
	}
	if err != nil {
		kind, message := liveProbeErrorSummary(err)
		result["error_kind"], result["error"] = kind, message
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
		t.Fatalf("BaoStock quarterly fundamentals failed: %s (%s)", kind, message)
	}
	if snapshot == nil || snapshot.Symbol != "600519.SH" || snapshot.Year != 2026 || snapshot.Quarter != 2 ||
		snapshot.Source != "baostock" || snapshot.FetchedAt.IsZero() || len(snapshot.Sections) != 6 || !snapshot.HasData {
		result["status"], result["error"] = "invalid", "snapshot identity/period/coverage mismatch"
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
		t.Fatal("BaoStock quarterly fundamentals identity/period/coverage mismatch")
	}
	wantDate := "2026-06-30"
	seen := map[string]bool{}
	for _, section := range snapshot.Sections {
		if seen[section.Category] {
			t.Fatalf("duplicate fundamentals category %q", section.Category)
		}
		seen[section.Category] = true
		if section.HasData && section.ReportDate != wantDate {
			t.Fatalf("category %s report_date=%s want=%s", section.Category, section.ReportDate, wantDate)
		}
		if section.HasData && section.PublicationDate != "" {
			if _, parseErr := time.Parse("2006-01-02", section.PublicationDate); parseErr != nil {
				t.Fatalf("category %s invalid publication date %q", section.Category, section.PublicationDate)
			}
		}
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("unexpected live fundamentals quality %q", snapshot.QualityStatus)
	}
	result["status"] = "returned"
	encoded, _ := json.Marshal(result)
	t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
}
