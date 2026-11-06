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

func TestNeteaseDailyBarsReportsInvalidOptionalChangePercent(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(location).AddDate(0, 0, -1).Format("2006-01-02")
	payload := "日期,股票代码,名称,收盘价,最高价,最低价,开盘价,前收盘,涨跌幅,成交量,成交金额\n" +
		date + ",600036,招商银行,42.28,42.60,42.00,42.60,42.52,bad-percent,74818026,3157692775.06\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprint(w, payload)
	}))
	defer server.Close()

	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewNetease(2*time.Second, server.URL)
	bars, err := provider.FetchDailyBars(ctx, "600036.SH", date, date, 1, model.AdjustmentNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 || bars[0].ChangePercent != nil {
		t.Fatalf("invalid optional percent must be omitted without dropping bar: %+v", bars)
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Stage != validation.StageParse || issues[0].Field != "change_percent" || issues[0].ProviderField != "涨跌幅" || issues[0].Code != "invalid_optional_number" || issues[0].RowIndex == nil || *issues[0].RowIndex != 0 {
		t.Fatalf("unexpected Netease parse issue: %+v", issues)
	}
}

func TestNeteaseDailyBarsReportsMissingRequiredCSVColumn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "日期,股票代码,开盘价")
		_, _ = fmt.Fprintln(w, "2026-10-21,600036,42")
	}))
	defer server.Close()
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewNetease(2*time.Second, server.URL)
	_, err := provider.FetchDailyBars(ctx, "600036.SH", "2026-10-20", "2026-10-21", 8, model.AdjustmentNone)
	if err == nil || !strings.Contains(err.Error(), "missing required column") {
		t.Fatalf("expected missing-column error, got %v", err)
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Field != "high" || issues[0].ProviderField != "high" || issues[0].Code != "missing_column" || issues[0].RowIndex != nil {
		t.Fatalf("unexpected missing-column issue: %+v", issues)
	}
}
