package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingHithinkHTTPDoer struct {
	calls  int
	method string
	url    string
	header http.Header
}

func (d *recordingHithinkHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	d.method = request.Method
	d.url = request.URL.String()
	d.header = request.Header.Clone()
	return &http.Response{
		StatusCode: http.StatusBadGateway,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
		Request:    request,
	}, nil
}

func TestHithinkUsesInjectedHTTPDoer(t *testing.T) {
	const (
		baseURL = "https://fuyao.aicubes.cn"
		apiKey  = "test-hithink-key"
	)
	doer := &recordingHithinkHTTPDoer{}
	p := NewHithinkConfiguredWithHTTPDoer(time.Second, baseURL, apiKey, doer)
	_, err := p.get(context.Background(), "/api/meta/tickers/search", nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 状态码 502") {
		t.Fatalf("get() error = %v, want HTTP 502", err)
	}
	if doer.calls != 1 || doer.method != http.MethodGet || doer.url != baseURL+"/api/meta/tickers/search" {
		t.Fatalf("request = calls:%d method:%q url:%q", doer.calls, doer.method, doer.url)
	}
	if got := doer.header.Get("X-api-key"); got != apiKey {
		t.Fatalf("X-api-key = %q, want configured key", got)
	}
	if got := doer.header.Get("Accept"); got != "application/json" {
		t.Fatalf("Accept = %q, want application/json", got)
	}
}

func TestHithinkConfiguredKeepsDefaultHTTPClientCompatibility(t *testing.T) {
	p := NewHithinkConfigured(time.Second, "https://fuyao.aicubes.cn", "key")
	if p.client == nil {
		t.Fatal("default HTTP client must be configured")
	}
}
