package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingXueqiuHTTPDoer struct {
	status int
	calls  int
	method string
	url    string
	header http.Header
}

func (d *recordingXueqiuHTTPDoer) Do(request *http.Request) (*http.Response, error) {
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

func TestXueqiuUsesInjectedHTTPDoerAndPreservesCookieHeaders(t *testing.T) {
	const (
		baseURL = "https://stock.xueqiu.example"
		cookie  = "xq_a_token=test-cookie"
	)
	doer := &recordingXueqiuHTTPDoer{status: http.StatusBadGateway}
	provider := NewXueqiuConfiguredWithHTTPDoer(time.Second, baseURL, cookie, doer)
	_, err := provider.FetchHotStocks(context.Background(), "popular", 10)
	if err == nil || !strings.Contains(err.Error(), "HTTP 状态码 502") {
		t.Fatalf("FetchHotStocks() error = %v, want HTTP 502", err)
	}
	if doer.calls != 1 || doer.method != http.MethodGet || !strings.HasPrefix(doer.url, baseURL+"/v5/stock/hot_stock/list.json?") {
		t.Fatalf("request = calls:%d method:%q url:%q", doer.calls, doer.method, doer.url)
	}
	if got := doer.header.Get("Cookie"); got != cookie {
		t.Fatalf("Cookie = %q, want configured cookie", got)
	}
	if got := doer.header.Get("Referer"); got != "https://xueqiu.com/" {
		t.Fatalf("Referer = %q, want existing Xueqiu referer", got)
	}
	if got := doer.header.Get("Origin"); got != "https://xueqiu.com" {
		t.Fatalf("Origin = %q, want existing Xueqiu origin", got)
	}
	if got := doer.header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}
}

func TestXueqiuConfiguredKeepsDefaultHTTPClientCompatibility(t *testing.T) {
	provider := NewXueqiuConfigured(time.Second, "https://stock.xueqiu.example", "cookie")
	if provider.client == nil || provider.httpDoer == nil {
		t.Fatal("default HTTP client and executor must be configured")
	}
}
