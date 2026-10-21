package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/storage"
	"example.com/stock-analysis-flow/internal/analysis/domain"
)

type fakeRunner struct {
	got    domain.RunRequest
	result domain.RunResult
	err    error
}

func (f *fakeRunner) Run(_ context.Context, r domain.RunRequest) (domain.RunResult, error) {
	f.got = r
	if f.result.RunID == "" {
		f.result = domain.RunResult{RunID: "run-test", Status: domain.RunStatusPartial, Request: r}
	}
	return f.result, f.err
}

type fakeStore struct {
	healthErr error
	result    domain.RunResult
	resultErr error
	events    []domain.StageEvent
	eventsErr error
}

func (f *fakeStore) Health(context.Context) error { return f.healthErr }
func (f *fakeStore) GetResult(context.Context, string) (domain.RunResult, error) {
	return f.result, f.resultErr
}
func (f *fakeStore) GetEvents(context.Context, string) ([]domain.StageEvent, error) {
	return f.events, f.eventsErr
}

func TestCreateRunFillsDefaultsAndRuns(t *testing.T) {
	runner := &fakeRunner{}
	store := &fakeStore{}
	api, err := New(runner, store, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	body := `{"symbols":["600519"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/runs", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	if runner.got.TaskType != domain.TaskPostMarketScan || runner.got.UniverseID == "" || runner.got.PortfolioID == "" || runner.got.MarketID == "" || len(runner.got.StrategyIDs) != 1 || runner.got.StrategyIDs[0] != "trend-following" {
		t.Fatalf("defaults not filled: %+v", runner.got)
	}
	if len(runner.got.Symbols) != 1 || runner.got.Symbols[0] != "600519" {
		t.Fatalf("symbols lost: %+v", runner.got.Symbols)
	}
}
func TestCreateRunRejectsUnknownFields(t *testing.T) {
	api, _ := New(&fakeRunner{}, &fakeStore{}, time.Second)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/analysis/runs", strings.NewReader(`{"symbols":["600519"],"typo":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
func TestReadyzFailsWhenDependencyUnavailable(t *testing.T) {
	api, _ := New(&fakeRunner{}, &fakeStore{healthErr: errors.New("redis down")}, time.Second)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}
func TestReadResultMapsNotFound(t *testing.T) {
	api, _ := New(&fakeRunner{}, &fakeStore{resultErr: storage.ErrRunNotFound}, time.Second)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/analysis/runs/missing", nil)
	w := httptest.NewRecorder()
	api.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
}
