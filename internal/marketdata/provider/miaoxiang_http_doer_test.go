package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingMiaoXiangHTTPDoer struct {
	status int
	calls  int
	method string
	url    string
	header http.Header
}

func (d *recordingMiaoXiangHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	d.method = request.Method
	d.url = request.URL.String()
	d.header = request.Header.Clone()
	return &http.Response{
		StatusCode: d.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(`{"status":0,"data":{"data":{"searchDataResultDTO":{"dataTableDTOList":[]}}}}`)),
		Request:    request,
	}, nil
}

func TestMiaoXiangUsesInjectedHTTPDoer(t *testing.T) {
	doer := &recordingMiaoXiangHTTPDoer{status: http.StatusBadGateway}
	p, err := NewMiaoXiangWithHTTPDoer("mx-test-key-12345678", "https://mx.example.test/api/claw/query", time.Second, doer)
	if err != nil {
		t.Fatal(err)
	}
	resetMiaoXiangRateLimitForTest()
	_, err = p.FetchChipDistribution(context.Background(), "600519")
	if err == nil || !strings.Contains(err.Error(), "MX API returned HTTP status 502") {
		t.Fatalf("FetchChipDistribution() error = %v, want HTTP status 502", err)
	}
	if doer.calls != 1 || doer.method != http.MethodPost || doer.url != "https://mx.example.test/api/claw/query" {
		t.Fatalf("unexpected request: calls=%d method=%q URL=%q", doer.calls, doer.method, doer.url)
	}
	if doer.header.Get("apikey") != "mx-test-key-12345678" || doer.header.Get("Content-Type") != "application/json" {
		t.Fatalf("unexpected request headers: %v", doer.header)
	}
}

func TestMiaoXiangLegacyConstructorKeepsDefaultClient(t *testing.T) {
	p, err := NewMiaoXiang("mx-test-key-12345678", "https://mx.example.test/api/claw/query", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if p.client == nil || p.httpDoer == nil {
		t.Fatal("legacy constructor must configure default HTTP client and doer")
	}
}
