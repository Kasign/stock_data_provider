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

type recordingTencentHTTPDoer struct {
	status int
	calls  int
	method string
	url    string
	header http.Header
}

func (d *recordingTencentHTTPDoer) Do(request *http.Request) (*http.Response, error) {
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

func TestTencentUsesDistinctHTTPDoersForConfiguredEndpoints(t *testing.T) {
	const (
		quoteURL = "https://web.sqt.gtimg.example/utf8/q"
		qfqURL   = "https://web.ifzq.gtimg.example/appstock/app/fqkline/get"
		rawURL   = "https://raw-kline.gtimg.example/appstock/app/kline/kline"
		searchURL = "https://smartbox.gtimg.example/s3/"
	)
	quoteDoer := &recordingTencentHTTPDoer{status: http.StatusBadGateway}
	qfqDoer := &recordingTencentHTTPDoer{status: http.StatusBadGateway}
	rawDoer := &recordingTencentHTTPDoer{status: http.StatusBadGateway}
	searchDoer := &recordingTencentHTTPDoer{status: http.StatusBadGateway}
	p := NewTencentConfiguredWithHTTPDoers(time.Second, quoteURL, qfqURL, rawURL, searchURL, quoteDoer, qfqDoer, rawDoer, searchDoer)

	if _, err := p.FetchQuote(context.Background(), "600000.SH"); err == nil || !strings.Contains(err.Error(), "tencent HTTP status 502") {
		t.Fatalf("FetchQuote() error = %v, want Tencent HTTP status 502", err)
	}
	if quoteDoer.calls != 1 || qfqDoer.calls != 0 || rawDoer.calls != 0 || searchDoer.calls != 0 {
		t.Fatalf("quote path crossed executors: quote=%d qfq=%d raw=%d search=%d", quoteDoer.calls, qfqDoer.calls, rawDoer.calls, searchDoer.calls)
	}
	if quoteDoer.method != http.MethodGet || !strings.HasPrefix(quoteDoer.url, quoteURL+"=") {
		t.Fatalf("quote request method=%q URL=%q", quoteDoer.method, quoteDoer.url)
	}

	if _, err := p.FetchDailyBars(context.Background(), "600000.SH", "2026-10-01", "2026-10-10", 10, model.AdjustmentForward); err == nil || !strings.Contains(err.Error(), "tencent HTTP status 502") {
		t.Fatalf("qfq FetchDailyBars() error = %v, want HTTP status 502", err)
	}
	if qfqDoer.calls != 1 || rawDoer.calls != 0 || !strings.HasPrefix(qfqDoer.url, qfqURL+"?") {
		t.Fatalf("qfq path crossed executors: qfq=%d raw=%d URL=%q", qfqDoer.calls, rawDoer.calls, qfqDoer.url)
	}

	if _, err := p.FetchDailyBars(context.Background(), "600000.SH", "2026-10-01", "2026-10-10", 10, model.AdjustmentNone); err == nil || !strings.Contains(err.Error(), "tencent HTTP status 502") {
		t.Fatalf("raw FetchDailyBars() error = %v, want HTTP status 502", err)
	}
	if rawDoer.calls != 1 || qfqDoer.calls != 1 || !strings.HasPrefix(rawDoer.url, rawURL+"?") {
		t.Fatalf("raw path crossed executors: qfq=%d raw=%d URL=%q", qfqDoer.calls, rawDoer.calls, rawDoer.url)
	}

	if _, err := p.SearchStocks(context.Background(), "600000", 10); err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
		t.Fatalf("SearchStocks() error = %v, want HTTP status 502", err)
	}
	if searchDoer.calls != 1 || quoteDoer.calls != 1 || qfqDoer.calls != 1 || rawDoer.calls != 1 || !strings.HasPrefix(searchDoer.url, searchURL+"?") {
		t.Fatalf("search path crossed executors: quote=%d qfq=%d raw=%d search=%d URL=%q", quoteDoer.calls, qfqDoer.calls, rawDoer.calls, searchDoer.calls, searchDoer.url)
	}
	if searchDoer.header.Get("Referer") != "https://gu.qq.com/" || searchDoer.header.Get("User-Agent") == "" {
		t.Fatalf("unexpected Smartbox headers: %v", searchDoer.header)
	}
}

func TestTencentConfiguredKeepsDefaultHTTPClientCompatibility(t *testing.T) {
	p := NewTencentConfiguredWithSearchURL(time.Second, "", "", "", "")
	if p.client == nil || p.quoteHTTPDoer == nil || p.klineHTTPDoer == nil || p.noneKlineHTTPDoer == nil || p.searchHTTPDoer == nil {
		t.Fatal("default HTTP client and all endpoint executors must be configured")
	}
}
