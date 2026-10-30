package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestEastmoneySectorRankingByFiveDayChangeUsesFullBoardUniverse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "" {
			code := r.URL.Query().Get("secid")
			base, latest := 10.0, 11.0
			if code == "90.BK0002" {
				base, latest = 10.0, 12.0
			}
			fmt.Fprintf(w, `{"rc":0,"data":{"code":"%s","klines":["2026-10-22,10,%g","2026-10-23,10,%g","2026-10-26,10,%g","2026-10-27,10,%g","2026-10-28,10,%g","2026-10-29,10,%g"]}}`,
				code[3:], base, base, base, base, base, latest)
			return
		}
		fmt.Fprint(w, `{"rc":0,"data":{"total":2,"diff":[
			{"f12":"BK0001","f14":"行业一","f3":1.2,"f6":100000000},
			{"f12":"BK0002","f14":"行业二","f3":0.8,"f6":90000000}
		]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithMarketData(3*time.Second, server.URL, server.URL, "", server.URL, "")
	got, err := p.FetchSectorRankingsByMetric(context.Background(), "industry", "change_5d", "up", 2)
	if err != nil {
		t.Fatal(err)
	}
	if got.SortBy != "change_5d" || got.ProviderReportDate != "2026-10-29" || got.Total != 2 || got.TotalIsLowerBound {
		t.Fatalf("unexpected snapshot metadata: %+v", got)
	}
	if len(got.Items) != 2 || got.Items[0].Code != "BK0002" || got.Items[0].Rank != 1 {
		t.Fatalf("unexpected five-day order: %+v", got.Items)
	}
	if got.Items[0].ChangePercent5D == nil || *got.Items[0].ChangePercent5D != 20 {
		t.Fatalf("five-day return = %v, want 20", got.Items[0].ChangePercent5D)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("quality must remain degraded without provider snapshot time: %s", got.QualityStatus)
	}
}

func TestEastmoneySectorRankingByFiveDayChangeRejectsIncompleteHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("secid") != "" {
			fmt.Fprint(w, `{"rc":0,"data":{"code":"BK0001","klines":["2026-10-29,10,11"]}}`)
			return
		}
		fmt.Fprint(w, `{"rc":0,"data":{"total":1,"diff":[{"f12":"BK0001","f14":"行业一","f3":1.2,"f6":100000000}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithMarketData(3*time.Second, server.URL, server.URL, "", server.URL, "")
	if _, err := p.FetchSectorRankingsByMetric(context.Background(), "industry", "change_5d", "up", 1); err == nil {
		t.Fatal("fewer than six closes must not produce a five-day return")
	}
}
