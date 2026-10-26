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

func TestEastmoneyFetchStockAnnouncementsParsesDatesPaginationAndURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		wants := map[string]string{
			"sr": "-1", "page_size": "20", "page_index": "1", "ann_type": "A",
			"client_source": "web", "f_node": "0", "s_node": "0",
			"begin_time": "2026-10-20", "end_time": "2026-10-26", "stock_list": "600036",
		}
		for key, want := range wants {
			if got := q.Get(key); got != want {
				t.Errorf("query %s=%q want %q", key, got, want)
			}
		}
		fmt.Fprint(w, `{"success":1,"data":{"page_index":1,"page_size":20,"total_hits":1,"list":[
			{"art_code":"AN202610231234567890","codes":[{"short_name":"招商银行","stock_code":"600036","market_code":"1"}],"columns":[{"column_code":"001001001001","column_name":"定期报告"}],"display_time":"2026-10-23 18:30:00","eiTime":"2026-10-23 18:30:00","language":"0","notice_date":"2026-10-23","sort_date":"2026-10-23 18:31:00","source_type":"3","title":"招商银行2026年第三季度报告","title_ch":"招商银行2026年第三季度报告","title_en":""}
		]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithAnnouncements(2*time.Second, "", "", "", "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchStockAnnouncements(context.Background(), "600036.SH", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Symbol != "600036.SH" || got.StartDate != "2026-10-20" || got.EndDate != "2026-10-26" ||
		got.Total != 1 || got.Pages != 1 || got.Page != 1 || got.PageSize != 20 || got.Returned != 1 || !got.HasData {
		t.Fatalf("unexpected announcement page: %+v", got)
	}
	item := got.Items[0]
	if item.ArticleID != "AN202610231234567890" || item.AnnouncementDate != "2026-10-23" || item.PublishedAt == nil {
		t.Fatalf("announcement ID/date/published time not retained: %+v", item)
	}
	if item.SourceURL != "https://data.eastmoney.com/notices/detail/600036/AN202610231234567890.html" {
		t.Fatalf("unexpected article URL %q", item.SourceURL)
	}
	if item.DisplayTime != "2026-10-23 18:30:00" || item.Category != "定期报告" || item.StockName != "招商银行" {
		t.Fatalf("announcement metadata was lost: %+v", item)
	}
}

func TestEastmoneyFetchStockAnnouncementsKeepsEmptyListUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":1,"data":{"page_index":1,"page_size":20,"total_hits":0,"list":[]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithAnnouncements(2*time.Second, "", "", "", "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchStockAnnouncements(context.Background(), "600036", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasData || got.Total != 0 || got.Returned != 0 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("empty announcement list must remain unknown: %+v", got)
	}
}

func TestEastmoneyFetchStockAnnouncementsRejectsRowsOutsideDateRange(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":1,"data":{"page_index":1,"page_size":20,"total_hits":1,"list":[{"art_code":"AN202610191234567890","codes":[{"stock_code":"600036"}],"notice_date":"2026-10-19","title":"测试公告"}]}}`)
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithAnnouncements(2*time.Second, "", "", "", "", "", "", "", "", "", server.URL, "")
	if _, err := p.FetchStockAnnouncements(context.Background(), "600036.SH", "2026-10-20", "2026-10-26", 1, 20); err == nil {
		t.Fatal("provider must reject rows outside requested announcement range")
	}
}

func TestEastmoneyAnnouncementDisplayTimeParserDoesNotUseNoticeDateAsTimestamp(t *testing.T) {
	if got := eastmoneyAnnouncementTime("2026-10-23"); got != nil {
		t.Fatalf("date-only value must not become a fake publish timestamp: %s", got.UTC().Format(time.RFC3339))
	}
	if got := eastmoneyAnnouncementTime("2026-10-23 18:30:00"); got == nil || got.In(time.FixedZone("CST", 8*3600)).Hour() != 18 {
		t.Fatalf("provider display time was not parsed: %v", got)
	}
	if strings.TrimSpace(eastmoneyAnnouncementURL("600036", "AN123")) == "" {
		t.Fatal("article URL pattern should be available")
	}
}
