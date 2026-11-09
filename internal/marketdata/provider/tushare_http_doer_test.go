package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

type recordingTushareHTTPDoer struct {
	status int
	calls  int
	method string
	url    string
	header http.Header
	body   []byte
}

func (d *recordingTushareHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	d.method = request.Method
	d.url = request.URL.String()
	d.header = request.Header.Clone()
	if request.Body != nil {
		body, err := io.ReadAll(request.Body)
		_ = request.Body.Close()
		if err != nil {
			return nil, err
		}
		d.body = body
	}
	return &http.Response{
		StatusCode: d.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
		Request:    request,
	}, nil
}

func TestTushareUsesInjectedHTTPDoerForDailyAndCapabilities(t *testing.T) {
	ctx := context.Background()
	endpoint := "https://api.tushare.pro"
	token := "test-token"

	t.Run("daily", func(t *testing.T) {
		doer := &recordingTushareHTTPDoer{status: http.StatusBadGateway}
		p := NewTushareConfiguredWithHTTPDoer(time.Second, endpoint, token, doer)
		_, err := p.FetchDailyBars(ctx, "600000.SH", "2026-10-01", "2026-10-02", 10, model.AdjustmentNone)
		if err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
			t.Fatalf("FetchDailyBars() error = %v, want HTTP 502", err)
		}
		assertTushareRequest(t, doer, endpoint, token, "daily")
	})

	t.Run("capability", func(t *testing.T) {
		doer := &recordingTushareHTTPDoer{status: http.StatusBadGateway}
		p := NewTushareConfiguredWithHTTPDoer(time.Second, endpoint, token, doer)
		_, err := p.fetchCapabilityTable(ctx, "trade_cal", map[string]string{"exchange": "SSE"}, "exchange,cal_date,is_open")
		if err == nil || !strings.Contains(err.Error(), "HTTP 状态码 502") {
			t.Fatalf("fetchCapabilityTable() error = %v, want HTTP 502", err)
		}
		assertTushareRequest(t, doer, endpoint, token, "trade_cal")
	})
}

func assertTushareRequest(t *testing.T, doer *recordingTushareHTTPDoer, endpoint, token, apiName string) {
	t.Helper()
	if doer.calls != 1 || doer.method != http.MethodPost || doer.url != endpoint {
		t.Fatalf("HTTP request = calls:%d method:%q url:%q", doer.calls, doer.method, doer.url)
	}
	if doer.header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", doer.header.Get("Content-Type"))
	}
	var payload struct {
		APIName string `json:"api_name"`
		Token   string `json:"token"`
	}
	if err := json.NewDecoder(bytes.NewReader(doer.body)).Decode(&payload); err != nil {
		t.Fatalf("decode captured request body: %v", err)
	}
	if payload.APIName != apiName || payload.Token != token {
		t.Fatalf("request payload = api_name:%q token:%q", payload.APIName, payload.Token)
	}
}
