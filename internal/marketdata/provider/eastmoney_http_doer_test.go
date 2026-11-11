package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingEastmoneyHTTPDoer struct {
	status int
	calls  int
	url    string
	method string
}

func (d *recordingEastmoneyHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	d.url = request.URL.String()
	d.method = request.Method
	return &http.Response{
		StatusCode: d.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"rc":0,"data":{}}`)),
		Request:    request,
	}, nil
}

func TestEastmoneyHTTPDoersRouteByOrigin(t *testing.T) {
	quote := &recordingEastmoneyHTTPDoer{status: http.StatusOK}
	kline := &recordingEastmoneyHTTPDoer{status: http.StatusOK}
	stockBasic := &recordingEastmoneyHTTPDoer{status: http.StatusBadGateway}
	p := NewEastmoneyConfiguredWithStockBasic(time.Second,
		"https://push2.example.test/api/quote",
		"https://push2his.example.test/api/kline",
		"https://datacenter.example.test/api/basic", "",
	).WithHTTPDoers(map[string]HTTPDoer{
		"https://push2.example.test":      quote,
		"https://push2his.example.test":   kline,
		"https://datacenter.example.test": stockBasic,
	})

	if err := p.getJSON(context.Background(), p.quoteURL, nil, &map[string]any{}); err != nil {
		t.Fatalf("quote request: %v", err)
	}
	if err := p.getJSON(context.Background(), p.klineURL, nil, &map[string]any{}); err != nil {
		t.Fatalf("kline request: %v", err)
	}
	if quote.calls != 1 || kline.calls != 1 || stockBasic.calls != 0 {
		t.Fatalf("unexpected route calls: quote=%d kline=%d stockBasic=%d", quote.calls, kline.calls, stockBasic.calls)
	}
	if quote.method != http.MethodGet || kline.method != http.MethodGet {
		t.Fatalf("unexpected methods: quote=%q kline=%q", quote.method, kline.method)
	}
}

func TestEastmoneyHTTPDoersFailClosedForUnknownOrigin(t *testing.T) {
	p := NewEastmoney(time.Second, "https://unlisted.example.test/quote", "")
	p.WithHTTPDoers(map[string]HTTPDoer{
		"https://allowed.example.test": &recordingEastmoneyHTTPDoer{status: http.StatusOK},
	})
	err := p.getJSON(context.Background(), p.quoteURL, nil, &map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "controlled HTTP executor is not configured") {
		t.Fatalf("getJSON() error = %v, want missing controlled executor", err)
	}
}

func TestEastmoneyHTTPOriginRejectsInvalidURL(t *testing.T) {
	for _, raw := range []string{"", "/relative/path", "https://user:pass@example.test/path"} {
		if _, err := EastmoneyHTTPOrigin(raw); err == nil {
			t.Errorf("EastmoneyHTTPOrigin(%q) expected error", raw)
		}
	}
	origin, err := EastmoneyHTTPOrigin("HTTPS://Push2.Eastmoney.Test:443/api/qt")
	if err != nil {
		t.Fatal(err)
	}
	if origin != "https://push2.eastmoney.test:443" {
		t.Fatalf("origin = %q", origin)
	}
}

func TestEastmoneyWithHTTPDoersCopiesMap(t *testing.T) {
	doer := &recordingEastmoneyHTTPDoer{status: http.StatusOK}
	input := map[string]HTTPDoer{"https://push2.example.test": doer}
	p := NewEastmoney(time.Second, "", "").WithHTTPDoers(input)
	delete(input, "https://push2.example.test")
	if _, err := p.doerForURL("https://push2.example.test/quote"); err != nil {
		t.Fatalf("injected executor map should be copied: %v", err)
	}
}
