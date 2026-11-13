package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/marketdata/service"
)

type fundamentalsCountingHTTPProvider struct {
	fundamentalsHTTPProvider
	calls int
}

func (p *fundamentalsCountingHTTPProvider) FetchFundamentals(ctx context.Context, symbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	p.calls++
	return p.fundamentalsHTTPProvider.FetchFundamentals(ctx, symbol, year, quarter)
}

// TestFundamentalsHTTPToMariaDBPointInTime verifies the HTTP route persists the
// Provider result through DataService and reads that revision back through as_of.
func TestFundamentalsHTTPToMariaDBPointInTime(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate MariaDB: %v", err)
	}

	symbol := fmt.Sprintf("%06d.SH", 900000+time.Now().UnixNano()%99999)
	source := &fundamentalsCountingHTTPProvider{}
	svc := service.New(
		[]provider.Provider{source},
		cache.NewDisabled(),
		store,
		time.Second,
		time.Minute,
		120,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	svc.SetFundamentalsProviders([]provider.FundamentalsProvider{source})
	handler := New("integration-secret", "forward_adjusted", svc, []provider.Provider{source}, nil).Handler()

	liveRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/stocks/"+symbol+"/fundamentals?year=2026&quarter=2", nil)
	liveRequest.Header.Set("X-API-Key", "integration-secret")
	liveResponse := httptest.NewRecorder()
	handler.ServeHTTP(liveResponse, liveRequest)
	if liveResponse.Code != http.StatusOK {
		t.Fatalf("live fundamentals request status=%d body=%s", liveResponse.Code, liveResponse.Body.String())
	}
	var liveSnapshot model.FundamentalsSnapshot
	if err := json.Unmarshal(liveResponse.Body.Bytes(), &liveSnapshot); err != nil {
		t.Fatalf("decode live fundamentals response: %v", err)
	}
	if liveSnapshot.Symbol != symbol || liveSnapshot.Year != 2026 || liveSnapshot.Quarter != 2 {
		t.Fatalf("live response identity mismatch: %+v", liveSnapshot)
	}
	if source.calls != 1 {
		t.Fatalf("live request should call Provider once, got %d", source.calls)
	}

	asOf := time.Now().UTC()
	pitRequest := httptest.NewRequest(http.MethodGet,
		"/api/v1/stocks/"+symbol+"/fundamentals?year=2026&quarter=2&as_of="+url.QueryEscape(asOf.Format(time.RFC3339Nano)), nil)
	pitRequest.Header.Set("X-API-Key", "integration-secret")
	pitResponse := httptest.NewRecorder()
	handler.ServeHTTP(pitResponse, pitRequest)
	if pitResponse.Code != http.StatusOK {
		t.Fatalf("PIT fundamentals request status=%d body=%s", pitResponse.Code, pitResponse.Body.String())
	}

	var historical model.FundamentalsSnapshot
	if err := json.Unmarshal(pitResponse.Body.Bytes(), &historical); err != nil {
		t.Fatalf("decode PIT fundamentals response: %v", err)
	}
	if historical.Symbol != symbol || historical.Year != 2026 || historical.Quarter != 2 {
		t.Fatalf("PIT response identity mismatch: %+v", historical)
	}
	if historical.SystemObservedAt == nil || historical.SystemObservedAt.After(asOf) {
		t.Fatalf("PIT response must restore persisted system_observed_at <= as_of: %+v", historical.SystemObservedAt)
	}
	if len(historical.Sections) == 0 || historical.Sections[0].ReportDate != "2026-06-30" {
		t.Fatalf("PIT response did not restore persisted report data: %+v", historical.Sections)
	}
	if source.calls != 1 {
		t.Fatalf("PIT request must not call live Provider; calls=%d", source.calls)
	}
}
