package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestHithinkFetchIndexQuotesCrossChecksReportedChanges(t *testing.T) {
	cases := []struct {
		name             string
		priceChange      float64
		changeRatio      float64
		wantDropped      bool
		wantQuality      model.QualityStatus
	}{
		{
			name:        "reported values within rounding tolerance use calculated values",
			priceChange: 1.04,
			changeRatio: 1.14,
			wantQuality: model.QualityValid,
		},
		{
			name:        "inconsistent reported change amount drops row",
			priceChange: 1.06,
			changeRatio: 1.0,
			wantDropped: true,
			wantQuality: model.QualityDegraded,
		},
		{
			name:        "inconsistent reported percentage drops row",
			priceChange: 1.0,
			changeRatio: 1.16,
			wantDropped: true,
			wantQuality: model.QualityDegraded,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/api/a-share-index/prices/snapshot" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.NotFound(w, r)
					return
				}
				if r.Header.Get("X-api-key") != "unit-test-key" {
					t.Errorf("missing API key header")
					http.Error(w, "missing API key", http.StatusUnauthorized)
					return
				}

				items := make([]map[string]any, 0, len(hithinkCoreIndexDefinitions))
				for _, definition := range hithinkCoreIndexDefinitions {
					row := map[string]any{
						"thscode": definition.Symbol,
						"name": definition.Name,
						"last_price": 101.0,
						"price_change": 1.0,
						"price_change_ratio_pct": 1.0,
						"open_price": 99.0,
						"high_price": 102.0,
						"low_price": 98.0,
						"prev_price": 100.0,
						"turnover": 123456.0,
					}
					if definition.Symbol == "000001.SH" {
						row["price_change"] = tc.priceChange
						row["price_change_ratio_pct"] = tc.changeRatio
					}
					items = append(items, row)
				}
				payload := map[string]any{
					"code": 0,
					"data": map[string]any{
						"timestamp": time.Now().UTC().Add(-time.Minute).UnixMilli(),
						"item": items,
					},
				}
				if err := json.NewEncoder(w).Encode(payload); err != nil {
					t.Errorf("encode fixture response: %v", err)
				}
			}))
			defer server.Close()

			provider := NewHithinkConfigured(2*time.Second, server.URL, "unit-test-key")
			got, err := provider.FetchIndexQuotes(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if got.QualityStatus != tc.wantQuality {
				t.Fatalf("snapshot quality = %q, want %q; reasons=%v", got.QualityStatus, tc.wantQuality, got.QualityReasons)
			}

			var shanghai *model.MarketIndexQuote
			for index := range got.Items {
				if got.Items[index].Symbol == "000001.SH" {
					shanghai = &got.Items[index]
					break
				}
			}
			if tc.wantDropped {
				if shanghai != nil {
					t.Fatalf("inconsistent source values must drop the index row: %+v", *shanghai)
				}
				if len(got.Items) != len(hithinkCoreIndexDefinitions)-1 {
					t.Fatalf("got %d items, want %d after dropping one inconsistent row", len(got.Items), len(hithinkCoreIndexDefinitions)-1)
				}
				if !containsHithinkReason(got.QualityReasons, "provider_index_change_fields_inconsistent") {
					t.Fatalf("snapshot missing change-consistency reason: %v", got.QualityReasons)
				}
				return
			}

			if shanghai == nil {
				t.Fatal("consistent source values should retain the index row")
			}
			if shanghai.Change == nil || *shanghai.Change != 1 {
				t.Fatalf("canonical change must be calculated from prices, got %v", shanghai.Change)
			}
			if shanghai.ChangePercent == nil || *shanghai.ChangePercent != 1 {
				t.Fatalf("canonical change percent must be calculated from prices, got %v", shanghai.ChangePercent)
			}
		})
	}
}

func TestHithinkFetchIndexQuotesMissingOptionalChangesUsesCalculatedValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items := make([]map[string]any, 0, len(hithinkCoreIndexDefinitions))
		for _, definition := range hithinkCoreIndexDefinitions {
			items = append(items, map[string]any{
				"thscode": definition.Symbol,
				"name": definition.Name,
				"last_price": 101.0,
				"open_price": 99.0,
				"high_price": 102.0,
				"low_price": 98.0,
				"prev_price": 100.0,
				"turnover": 123456.0,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"timestamp": time.Now().UTC().Add(-time.Minute).UnixMilli(),
				"item": items,
			},
		})
	}))
	defer server.Close()

	provider := NewHithinkConfigured(2*time.Second, server.URL, "unit-test-key")
	got, err := provider.FetchIndexQuotes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.QualityStatus != model.QualityValid || len(got.Items) != len(hithinkCoreIndexDefinitions) {
		t.Fatalf("optional absent change fields should be calculable without degrading otherwise valid quotes: %+v", got)
	}
	if got.Items[0].Change == nil || *got.Items[0].Change != 1 ||
		got.Items[0].ChangePercent == nil || *got.Items[0].ChangePercent != 1 {
		t.Fatalf("unexpected calculated changes: %+v", got.Items[0])
	}
}

