package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

type fixedDoer struct {
	response *http.Response
}

func (d fixedDoer) Do(*http.Request) (*http.Response, error) {
	return d.response, nil
}

func TestCaptureDoerPreservesResponseBody(t *testing.T) {
	body := "provider-raw-payload"
	response := &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	doer := &captureDoer{next: fixedDoer{response: response}, limit: 1024}
	request, _ := http.NewRequest(http.MethodGet, "https://example.com/data", nil)
	got, err := doer.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Body.Close()
	adapterBody, err := io.ReadAll(got.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(adapterBody) != body {
		t.Fatalf("adapter body changed: %q", adapterBody)
	}
	items := doer.snapshot()
	if len(items) != 1 || !items[0].BodyComplete || items[0].RawPayloadBase64 != base64.StdEncoding.EncodeToString([]byte(body)) {
		t.Fatalf("unexpected capture: %#v", items)
	}
}

func TestSanitizeURLRedactsSensitiveQuery(t *testing.T) {
	parsed, err := url.Parse("https://example.com/kline?ut=private-value&beg=20261109&token=abc")
	if err != nil {
		t.Fatal(err)
	}
	got := sanitizeURL(parsed)
	if strings.Contains(got, "private-value") || strings.Contains(got, "token=abc") {
		t.Fatalf("sensitive query leaked: %s", got)
	}
	if !strings.Contains(got, "beg=20261109") {
		t.Fatalf("non-sensitive query was lost: %s", got)
	}
}
