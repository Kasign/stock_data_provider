package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHithinkDragonTigerRejectsFutureResponseDateWhenRequestDateIsOmitted(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	futureDate := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/a-share/special-data/dragon-tiger-list" {
			t.Errorf("unexpected endpoint path %q", r.URL.Path)
		}
		if r.URL.Query().Get("date") != "" {
			t.Errorf("omitted request date should not be synthesized into query: %q", r.URL.Query().Get("date"))
		}
		_, _ = fmt.Fprintf(w, `{"code":0,"message":"ok","data":{"trade_date":%q,"stock_count":0,"stock_items":[]}}`, futureDate)
	}))
	defer server.Close()

	p := NewHithinkConfigured(2*time.Second, server.URL, "test-api-key")
	_, err = p.FetchDragonTiger(context.Background(), "", 1, 100)
	if err == nil || !strings.Contains(err.Error(), "晚于中国市场当前日期") || !called {
		t.Fatalf("future source report date must be rejected after response validation: err=%v called=%v", err, called)
	}
}
