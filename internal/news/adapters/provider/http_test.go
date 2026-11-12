package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/news/app"
)

func TestProviderResponseBodySizeLimit(t *testing.T) {
	tests := []struct {
		name          string
		bodySize      int
		contentLength int64
		wantErr       bool
	}{
		{name: "缺少 Content-Length 时拒绝超限响应", bodySize: int(maxProviderResponseBytes + 1), contentLength: -1, wantErr: true},
		{name: "实际响应超过声明长度时拒绝", bodySize: int(maxProviderResponseBytes + 1), contentLength: 1, wantErr: true},
		{name: "Content-Length 超限时提前拒绝", bodySize: 1, contentLength: maxProviderResponseBytes + 1, wantErr: true},
		{name: "恰好达到上限时允许读取", bodySize: int(maxProviderResponseBytes), contentLength: maxProviderResponseBytes},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode:    http.StatusOK,
					Body:          io.NopCloser(strings.NewReader(strings.Repeat("x", test.bodySize))),
					ContentLength: test.contentLength,
					Header:        make(http.Header),
				}, nil
			})
			cfg, err := newConfig("https://provider.example.test/search", "", client)
			if err != nil {
				t.Fatal(err)
			}
			got, err := cfg.do(context.Background(), http.MethodGet, "https://provider.example.test/search", nil, nil)
			if test.wantErr {
				providerErr, ok := err.(*app.ProviderError)
				if !ok || providerErr.Code != app.ProviderErrorResponseInvalid {
					t.Fatalf("error=%#v, want %q", err, app.ProviderErrorResponseInvalid)
				}
				if got != nil {
					t.Fatalf("response body must be nil on rejection, got %d bytes", len(got))
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if int64(len(got)) != maxProviderResponseBytes {
				t.Fatalf("response size=%d, want %d", len(got), maxProviderResponseBytes)
			}
		})
	}
}

func TestProviderOversizedErrorResponseKeepsHTTPStatusClassification(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusTooManyRequests,
			Body:          io.NopCloser(strings.NewReader(strings.Repeat("x", int(maxProviderResponseBytes+1)))),
			ContentLength: -1,
			Header:        make(http.Header),
		}, nil
	})
	cfg, err := newConfig("https://provider.example.test/search", "", client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = cfg.do(context.Background(), http.MethodGet, "https://provider.example.test/search", nil, nil)
	providerErr, ok := err.(*app.ProviderError)
	if !ok || providerErr.Code != app.ProviderErrorRateLimited || !providerErr.Retryable {
		t.Fatalf("error=%#v, want retryable rate-limited error", err)
	}
}
