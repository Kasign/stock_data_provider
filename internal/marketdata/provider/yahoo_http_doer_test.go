package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

type recordingYahooHTTPDoer struct {
	status int
	calls  int
	method string
	url    string
	header http.Header
}

func (d *recordingYahooHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	d.method = request.Method
	d.url = request.URL.String()
	d.header = request.Header.Clone()
	return &http.Response{
		StatusCode: d.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
		Request:    request,
	}, nil
}

func TestYahooUsesInjectedHTTPDoerForQuoteAndDailyBars(t *testing.T) {
	const baseURL = "https://query1.finance.yahoo.example/v8/finance/chart"
	doer := &recordingYahooHTTPDoer{status: http.StatusBadGateway}
	provider := NewYahooFinanceWithHTTPDoer(time.Second, baseURL, doer)

	if _, err := provider.FetchQuote(context.Background(), "600000.SH"); err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
		t.Fatalf("FetchQuote() error = %v, want HTTP status 502", err)
	}
	if doer.calls != 1 || doer.method != http.MethodGet || !strings.HasPrefix(doer.url, baseURL+"/600000.SS?") {
		t.Fatalf("quote request = calls:%d method:%q url:%q", doer.calls, doer.method, doer.url)
	}
	if doer.header.Get("Accept") != "application/json" || doer.header.Get("Referer") != "https://finance.yahoo.com/" {
		t.Fatalf("quote headers = %v", doer.header)
	}

	doer.status = http.StatusBadGateway
	if _, err := provider.FetchDailyBars(context.Background(), "600000.SH", "2026-10-01", "2026-10-02", 10, model.AdjustmentNone); err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
		t.Fatalf("FetchDailyBars() error = %v, want HTTP status 502", err)
	}
	if doer.calls != 2 || doer.method != http.MethodGet || !strings.HasPrefix(doer.url, baseURL+"/600000.SS?") {
		t.Fatalf("daily request = calls:%d method:%q url:%q", doer.calls, doer.method, doer.url)
	}
}

func TestYahooConfiguredKeepsDefaultHTTPClientCompatibility(t *testing.T) {
	provider := NewYahooFinance(time.Second, "https://query1.finance.yahoo.example/v8/finance/chart")
	if provider.client == nil || provider.httpDoer == nil {
		t.Fatal("default HTTP client and executor must be configured")
	}
}
