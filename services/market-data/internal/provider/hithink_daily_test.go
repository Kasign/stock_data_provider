package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/validation"
)

func TestHithinkDailyBarParseIssuesIncludeRowAndProviderField(t *testing.T) {
	payload := `{"code":0,"data":{"item":[{"date_ms":1792454400000,"open_price":"bad","high_price":11,"low_price":9,"close_price":10,"volume":1000,"turnover":10000}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-api-key") != "test-key" {
			t.Errorf("missing API key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	provider := NewHithinkConfigured(time.Second, server.URL, "test-key")
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	if _, err := provider.FetchDailyBars(ctx, "600036.SH", "2026-10-20", "2026-10-21", 10, model.AdjustmentNone); err == nil {
		t.Fatal("expected invalid open price to reject the response")
	}
	found := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Field == "open" && issue.ProviderField == "open_price" && issue.Code == "type_mismatch" && issue.RowIndex != nil && *issue.RowIndex == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected structured open_price issue at row 0, got %+v", reporter.Snapshot().Issues)
	}
}

func TestHithinkDailyPaginationDuplicateReportsPageOffsetAndRow(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	dateMS := func(day int) int64 {
		return time.Date(2026, time.October, day, 0, 0, 0, 0, loc).UnixMilli()
	}
	row := func(day int) string {
		return fmt.Sprintf("{\"date_ms\":%d,\"open_price\":10,\"high_price\":11,\"low_price\":9,\"close_price\":10,\"volume\":1000,\"turnover\":10000}", dateMS(day))
	}
	payload := func(rows ...string) string {
		return "{\"code\":0,\"data\":{\"item\":[" + strings.Join(rows, ",") + "]}}"
	}
	var offsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		offset := r.URL.Query().Get("offset")
		offsets = append(offsets, offset)
		w.Header().Set("Content-Type", "application/json")
		switch offset {
		case "0":
			_, _ = w.Write([]byte(payload(row(7), row(8))))
		case "2":
			// 模拟服务端第二页与第一页边界重叠；客户端必须报错，不能静默去重。
			_, _ = w.Write([]byte(payload(row(8), row(9))))
		default:
			_, _ = w.Write([]byte(payload()))
		}
	}))
	defer server.Close()

	provider := NewHithinkConfigured(time.Second, server.URL, "test-key")
	_, err := provider.FetchDailyBars(context.Background(), "600036.SH", "2026-10-07", "2026-10-09", 10, model.AdjustmentNone)
	if err == nil || !strings.Contains(err.Error(), "first_page=1 first_offset=0 first_row=1 first_page_item_count=2 first_page_first_date=2026-10-07 first_page_last_date=2026-10-08 duplicate_page=2 duplicate_offset=2 duplicate_row=0 duplicate_page_item_count=2 duplicate_page_first_date=2026-10-08 duplicate_page_last_date=2026-10-09") || !strings.Contains(err.Error(), "2026-10-08") {
		t.Fatalf("expected page/offset/row duplicate diagnostic, got %v", err)
	}
	if len(offsets) != 2 || offsets[0] != "0" || offsets[1] != "2" {
		t.Fatalf("unexpected pagination offsets: %v", offsets)
	}
}
