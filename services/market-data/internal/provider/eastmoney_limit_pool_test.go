package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEastmoneyFetchLimitPoolRejectsFutureTradeDateWithoutCallingProvider(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	futureDate := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "future trade date must be rejected before request", http.StatusBadRequest)
	}))
	defer server.Close()

	provider := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "", "", server.URL, "",
	)
	_, err = provider.FetchLimitPool(context.Background(), "up", futureDate, 30)
	if err == nil || !strings.Contains(err.Error(), "must not be in the future") || called {
		t.Fatalf("future limit-pool date must be rejected before request: err=%v called=%v", err, called)
	}
}
