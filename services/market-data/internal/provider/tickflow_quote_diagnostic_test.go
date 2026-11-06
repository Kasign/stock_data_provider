package provider

import (
	"context"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/validation"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTickFlowQuoteRawFieldInvalidNumberIssue(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":[{"symbol":"600519.SH","last_price":"NaN","prev_close":100,"open":99,"high":102,"low":98,"volume":1000,"amount":100000}]}`))
	}))
	defer s.Close()
	rep := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), rep)
	p := NewTickFlowConfigured(2*time.Second, s.URL, "", "test-key")
	if _, err := p.FetchQuote(ctx, "600519.SH"); err == nil {
		t.Fatal("expected invalid required field")
	}
	issues := rep.Snapshot().Issues
	if len(issues) != 1 || issues[0].Stage != validation.StageParse || issues[0].Field != "current_price" || issues[0].ProviderField != "last_price" || issues[0].Code != "invalid_number" {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}

func TestTickFlowQuoteVolumeIsNormalizedFromHands(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != "test-key" {
			t.Errorf("missing TickFlow API key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":[{"symbol":"600519.SH","last_price":1446.44,"prev_close":1480,"open":1470,"high":1480.6,"low":1439,"volume":65434,"amount":9536448000,"timestamp":1793775600000}]}`))
	}))
	defer server.Close()

	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	quote, err := provider.FetchQuote(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchQuote: %v", err)
	}
	if quote.RawVolume == nil || *quote.RawVolume != 65434 || quote.RawVolumeUnit != "hands" {
		t.Fatalf("unexpected raw volume: %+v", quote)
	}
	if quote.Volume == nil || *quote.Volume != 6543400 || quote.VolumeUnit != "shares" {
		t.Fatalf("unexpected normalized volume: %+v", quote)
	}
	if quote.QualityStatus != model.QualityValid {
		t.Fatalf("expected valid quote after hands-to-shares normalization, got %s: %v", quote.QualityStatus, quote.QualityReasons)
	}
}
