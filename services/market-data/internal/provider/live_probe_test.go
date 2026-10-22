package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

const liveProbeBodySampleLimit = 2048

type liveProbeReport struct {
	GeneratedAt string              `json:"generated_at"`
	Runner      string              `json:"runner"`
	QuoteSymbol string              `json:"quote_symbol"`
	DailySymbol string              `json:"daily_symbol"`
	StartDate   string              `json:"start_date"`
	EndDate     string              `json:"end_date"`
	Results     []liveProbeResult   `json:"results"`
	Failures    []string            `json:"failures,omitempty"`
}

type liveProbeResult struct {
	Provider    string                   `json:"provider"`
	Capability  string                   `json:"capability"`
	Symbol      string                   `json:"symbol"`
	Adjustment  model.AdjustmentBasis    `json:"adjustment,omitempty"`
	Status      string                   `json:"status"`
	ErrorKind   string                   `json:"error_kind,omitempty"`
	Error       string                   `json:"error,omitempty"`
	ElapsedMS   int64                    `json:"elapsed_ms"`
	Quote       *model.Quote             `json:"quote,omitempty"`
	BarCount    int                      `json:"bar_count,omitempty"`
	SampleBars  []model.DailyBar         `json:"sample_bars,omitempty"`
	HTTP        []liveProbeHTTPResponse  `json:"http_responses,omitempty"`
}

type liveProbeHTTPResponse struct {
	Endpoint          string `json:"endpoint"`
	StatusCode        int    `json:"status_code,omitempty"`
	ContentType       string `json:"content_type,omitempty"`
	TransportError    string `json:"transport_error,omitempty"`
	BodyBytesObserved int64  `json:"body_bytes_observed,omitempty"`
	BodyTruncated     bool   `json:"body_truncated,omitempty"`
	BodySample        string `json:"body_sample,omitempty"`
	rawSample         []byte
}

type liveProbeCapture struct {
	mu        sync.Mutex
	responses []*liveProbeHTTPResponse
}

func (c *liveProbeCapture) add(response *liveProbeHTTPResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.responses = append(c.responses, response)
}

func (c *liveProbeCapture) snapshot() []liveProbeHTTPResponse {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]liveProbeHTTPResponse, 0, len(c.responses))
	for _, response := range c.responses {
		copyOfResponse := *response
		copyOfResponse.rawSample = nil
		out = append(out, copyOfResponse)
	}
	return out
}

func (c *liveProbeCapture) transport() http.RoundTripper {
	return liveProbeCaptureTransport{capture: c, base: http.DefaultTransport}
}

type liveProbeCaptureTransport struct {
	capture *liveProbeCapture
	base    http.RoundTripper
}

func (t liveProbeCaptureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	// Never record RawQuery: provider URLs may include public tokens or request parameters.
	record := &liveProbeHTTPResponse{Endpoint: request.URL.Host + request.URL.EscapedPath()}
	t.capture.add(record)

	response, err := t.base.RoundTrip(request)
	if err != nil {
		_, message := providerFailureSummary(err)
		record.TransportError = message
		return nil, err
	}
	record.StatusCode = response.StatusCode
	record.ContentType = response.Header.Get("Content-Type")
	response.Body = &liveProbeCaptureBody{ReadCloser: response.Body, record: record}
	return response, nil
}

type liveProbeCaptureBody struct {
	io.ReadCloser
	record *liveProbeHTTPResponse
}

func (b *liveProbeCaptureBody) Read(buffer []byte) (int, error) {
	n, err := b.ReadCloser.Read(buffer)
	if n > 0 {
		b.record.BodyBytesObserved += int64(n)
		remaining := liveProbeBodySampleLimit - len(b.record.rawSample)
		if remaining > 0 {
			take := n
			if take > remaining {
				take = remaining
			}
			b.record.rawSample = append(b.record.rawSample, buffer[:take]...)
		}
	}
	return n, err
}

func (b *liveProbeCaptureBody) Close() error {
	err := b.ReadCloser.Close()
	b.record.BodySample = strings.ToValidUTF8(string(b.record.rawSample), "�")
	b.record.BodyTruncated = b.record.BodyBytesObserved > int64(len(b.record.rawSample))
	return err
}

func TestLiveProviderProbe(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_PROVIDER_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_PROVIDER_TESTS=1 to call real public provider endpoints")
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	startDate := now.AddDate(0, 0, -35).Format("2006-01-02")
	endDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	quoteSymbol := strings.TrimSpace(os.Getenv("LIVE_PROBE_QUOTE_SYMBOL"))
	if quoteSymbol == "" {
		quoteSymbol = "600519.SH"
	}
	dailySymbol := "600036.SH"

	report := liveProbeReport{
		GeneratedAt: now.Format(time.RFC3339),
		Runner:      "GitHub Actions hosted runner",
		QuoteSymbol: quoteSymbol,
		DailySymbol: dailySymbol,
		StartDate:   startDate,
		EndDate:     endDate,
		Results:     make([]liveProbeResult, 0, 7),
	}
	var failures []string

	runQuote := func(providerName, symbol string, client *http.Client, call func(context.Context) (*model.Quote, error)) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		started := time.Now()
		quote, callErr := call(ctx)
		result := liveProbeResult{
			Provider: providerName, Capability: "quote", Symbol: symbol,
			ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot(),
		}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = providerFailureSummary(callErr)
			failures = append(failures, providerName+" quote: "+result.ErrorKind+" ("+result.Error+")")
		case quote == nil:
			result.Status = "invalid"
			result.Error = "provider returned a nil quote without an error"
			failures = append(failures, providerName+" quote: nil result")
		case quote.CurrentPrice == nil || quote.Volume == nil:
			result.Status = "invalid"
			result.Quote = quote
			result.Error = "quote response is missing current_price or volume"
			failures = append(failures, providerName+" quote: incomplete required fields")
		default:
			result.Status = "returned"
			result.Quote = quote
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	runDaily := func(providerName, symbol string, basis model.AdjustmentBasis, client *http.Client, call func(context.Context) ([]model.DailyBar, error)) {
		capture := &liveProbeCapture{}
		if client != nil {
			client.Transport = capture.transport()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		started := time.Now()
		bars, callErr := call(ctx)
		result := liveProbeResult{
			Provider: providerName, Capability: "daily_bars", Symbol: symbol,
			Adjustment: basis, ElapsedMS: time.Since(started).Milliseconds(), HTTP: capture.snapshot(),
			BarCount: len(bars),
		}
		switch {
		case callErr != nil:
			result.Status = "failed"
			result.ErrorKind, result.Error = providerFailureSummary(callErr)
			failures = append(failures, providerName+" daily_bars "+string(basis)+": "+result.ErrorKind+" ("+result.Error+")")
		case len(bars) == 0:
			result.Status = "invalid"
			result.Error = "provider returned an empty bar series without an error"
			failures = append(failures, providerName+" daily_bars "+string(basis)+": empty result")
		default:
			result.Status = "returned"
			// Providers normally return rows in ascending date order. Keep the newest three samples in the report.
			first := len(bars) - 3
			if first < 0 {
				first = 0
			}
			result.SampleBars = append([]model.DailyBar(nil), bars[first:]...)
		}
		report.Results = append(report.Results, result)
		encoded, _ := json.Marshal(result)
		t.Logf("LIVE_PROVIDER_RESULT %s", encoded)
	}

	sina := NewSina(12*time.Second, "")
	runQuote("sina", quoteSymbol, sina.client, func(ctx context.Context) (*model.Quote, error) {
		return sina.FetchQuote(ctx, quoteSymbol)
	})

	tencent := NewTencent(12 * time.Second)
	runQuote("tencent", quoteSymbol, tencent.client, func(ctx context.Context) (*model.Quote, error) {
		return tencent.FetchQuote(ctx, quoteSymbol)
	})
	runDaily("tencent", dailySymbol, model.AdjustmentNone, tencent.client, func(ctx context.Context) ([]model.DailyBar, error) {
		return tencent.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentNone)
	})
	runDaily("tencent", dailySymbol, model.AdjustmentForward, tencent.client, func(ctx context.Context) ([]model.DailyBar, error) {
		return tencent.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentForward)
	})

	eastmoney := NewEastmoney(12*time.Second, "", "")
	runQuote("eastmoney", dailySymbol, eastmoney.client, func(ctx context.Context) (*model.Quote, error) {
		return eastmoney.FetchQuote(ctx, dailySymbol)
	})
	runDaily("eastmoney", dailySymbol, model.AdjustmentForward, eastmoney.client, func(ctx context.Context) ([]model.DailyBar, error) {
		return eastmoney.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentForward)
	})

	baostockClient := baostock.NewClient(baostock.Config{ConnectTimeout: 8 * time.Second, RequestTimeout: 20 * time.Second})
	baostockProvider := NewBaostock(baostockClient)
	runDaily("baostock", dailySymbol, model.AdjustmentForward, nil, func(ctx context.Context) ([]model.DailyBar, error) {
		return baostockProvider.FetchDailyBars(ctx, dailySymbol, startDate, endDate, 8, model.AdjustmentForward)
	})
	if closeErr := baostockClient.Close(); closeErr != nil {
		t.Logf("LIVE_PROVIDER_CLEANUP warning provider=baostock error=%q", closeErr.Error())
	}

	report.Failures = failures
	encoded, marshalErr := json.MarshalIndent(report, "", "  ")
	if marshalErr != nil {
		t.Errorf("marshal live provider report: %v", marshalErr)
	} else {
		t.Logf("LIVE_PROVIDER_REPORT_BEGIN
%s
LIVE_PROVIDER_REPORT_END", encoded)
		if outputPath := strings.TrimSpace(os.Getenv("LIVE_PROVIDER_REPORT_PATH")); outputPath != "" {
			if writeErr := os.WriteFile(outputPath, append(encoded, '
'), 0o644); writeErr != nil {
				t.Errorf("write live provider report: %v", writeErr)
			}
		}
	}
	if len(failures) > 0 {
		t.Errorf("%d live provider probe(s) failed; inspect LIVE_PROVIDER_RESULT records and the uploaded report artifact", len(failures))
	}
}
