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

type recordingSinaHTTPDoer struct {
	status int
	calls  int
	method string
	url    string
	header http.Header
}

func (d *recordingSinaHTTPDoer) Do(request *http.Request) (*http.Response, error) {
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

func TestSinaUsesSeparateQuoteAndKlineHTTPDoers(t *testing.T) {
	const (
		quoteURL = "https://hq.sinajs.example/list="
		klineURL = "https://money.finance.sina.example/kline"
	)
	quoteDoer := &recordingSinaHTTPDoer{status: http.StatusBadGateway}
	klineDoer := &recordingSinaHTTPDoer{status: http.StatusBadGateway}
	p := NewSinaConfiguredWithHTTPDoers(time.Second, quoteURL, klineURL, quoteDoer, klineDoer)

	if _, err := p.FetchQuote(context.Background(), "600519.SH"); err == nil || !strings.Contains(err.Error(), "Sina quote HTTP status 502") {
		t.Fatalf("FetchQuote() error = %v, want Sina quote HTTP status 502", err)
	}
	if quoteDoer.calls != 1 || klineDoer.calls != 0 || quoteDoer.method != http.MethodGet || quoteDoer.url != quoteURL+"sh600519" {
		t.Fatalf("quote calls=%d kline calls=%d method=%q URL=%q", quoteDoer.calls, klineDoer.calls, quoteDoer.method, quoteDoer.url)
	}
	if quoteDoer.header.Get("Referer") != "https://finance.sina.com.cn/" || quoteDoer.header.Get("Accept") != "text/plain,*/*" {
		t.Fatalf("unexpected quote headers: %v", quoteDoer.header)
	}

	if _, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-01", "2026-10-10", 10, model.AdjustmentNone); err == nil || !strings.Contains(err.Error(), "Sina K-line HTTP status 502") {
		t.Fatalf("FetchDailyBars() error = %v, want Sina K-line HTTP status 502", err)
	}
	if quoteDoer.calls != 1 || klineDoer.calls != 1 || klineDoer.method != http.MethodGet || !strings.HasPrefix(klineDoer.url, klineURL+"?") {
		t.Fatalf("quote calls=%d kline calls=%d method=%q URL=%q", quoteDoer.calls, klineDoer.calls, klineDoer.method, klineDoer.url)
	}
	if klineDoer.header.Get("Referer") != "https://finance.sina.com.cn/" || klineDoer.header.Get("Accept") != "application/json,*/*" {
		t.Fatalf("unexpected kline headers: %v", klineDoer.header)
	}
}

func TestSinaConfiguredKeepsDefaultHTTPClientCompatibility(t *testing.T) {
	p := NewSinaConfigured(time.Second, "", "")
	if p.client == nil || p.quoteHTTPDoer == nil || p.klineHTTPDoer == nil {
		t.Fatal("default HTTP client and both endpoint executors must be configured")
	}
}
