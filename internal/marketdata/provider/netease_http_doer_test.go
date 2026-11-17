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

type recordingNeteaseHTTPDoer struct {
	status int
	calls  int
	method string
	url    string
	header http.Header
}

func (d *recordingNeteaseHTTPDoer) Do(request *http.Request) (*http.Response, error) {
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

func TestNeteaseUsesSeparateQuoteAndHistoryHTTPDoers(t *testing.T) {
	const (
		quoteURL   = "https://api.money.126.example/data/feed"
		historyURL = "https://quotes.money.163.example/service/chddata.html"
	)
	quoteDoer := &recordingNeteaseHTTPDoer{status: http.StatusBadGateway}
	historyDoer := &recordingNeteaseHTTPDoer{status: http.StatusBadGateway}
	provider := NewNeteaseConfiguredWithHTTPDoers(time.Second, quoteURL, historyURL, quoteDoer, historyDoer)

	if _, err := provider.FetchQuote(context.Background(), "600000.SH"); err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
		t.Fatalf("FetchQuote() error = %v, want HTTP status 502", err)
	}
	if quoteDoer.calls != 1 || historyDoer.calls != 0 || !strings.HasPrefix(quoteDoer.url, quoteURL+"/") {
		t.Fatalf("quote calls=%d history calls=%d quote URL=%q", quoteDoer.calls, historyDoer.calls, quoteDoer.url)
	}

	if _, err := provider.FetchDailyBars(context.Background(), "600000.SH", "2026-10-01", "2026-10-10", 10, model.AdjustmentNone); err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
		t.Fatalf("FetchDailyBars() error = %v, want HTTP status 502", err)
	}
	if quoteDoer.calls != 1 || historyDoer.calls != 1 || !strings.HasPrefix(historyDoer.url, historyURL+"?") {
		t.Fatalf("quote calls=%d history calls=%d history URL=%q", quoteDoer.calls, historyDoer.calls, historyDoer.url)
	}
	if quoteDoer.header.Get("Referer") != "https://money.163.com/" || historyDoer.header.Get("Referer") != "https://money.163.com/" {
		t.Fatalf("unexpected NetEase headers: quote=%v history=%v", quoteDoer.header, historyDoer.header)
	}
}

func TestNeteaseConfiguredKeepsDefaultHTTPClientCompatibility(t *testing.T) {
	provider := NewNeteaseConfigured(time.Second, "https://api.money.126.example/data/feed", "https://quotes.money.163.example/service/chddata.html")
	if provider.client == nil || provider.quoteHTTPDoer == nil || provider.historyHTTPDoer == nil {
		t.Fatal("default HTTP client and both endpoint executors must be configured")
	}
}
