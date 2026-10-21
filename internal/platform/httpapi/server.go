// Package httpapi exposes the analysis use case over HTTP.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/storage"
	"example.com/stock-analysis-flow/internal/analysis/domain"
)

type Runner interface {
	Run(context.Context, domain.RunRequest) (domain.RunResult, error)
}
type Store interface {
	Health(context.Context) error
	GetResult(context.Context, string) (domain.RunResult, error)
	GetEvents(context.Context, string) ([]domain.StageEvent, error)
}

type Server struct {
	runner          Runner
	store           Store
	analysisTimeout time.Duration
}

func New(runner Runner, store Store, analysisTimeout time.Duration) (*Server, error) {
	if runner == nil || store == nil {
		return nil, errors.New("http API runner and store are required")
	}
	if analysisTimeout <= 0 {
		analysisTimeout = 3 * time.Minute
	}
	return &Server{runner: runner, store: store, analysisTimeout: analysisTimeout}, nil
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("POST /api/v1/analysis/runs", s.handleCreateRun)
	mux.HandleFunc("GET /api/v1/analysis/runs/", s.handleGetRun)
	return requestLog(recoverer(mux))
}
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"service": "stock-analysis-go", "api_version": "v1", "health": "/healthz", "readiness": "/readyz", "create_analysis": "POST /api/v1/analysis/runs", "query_analysis": "GET /api/v1/analysis/runs/{run_id}", "query_events": "GET /api/v1/analysis/runs/{run_id}/events"})
}
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
}
func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	if err := s.store.Health(ctx); err != nil {
		writeError(w, http.StatusServiceUnavailable, "dependencies_unready", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "dependencies": map[string]string{"mariadb": "ok", "redis": "ok"}, "time": time.Now().UTC()})
}
func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		writeError(w, http.StatusUnsupportedMediaType, "content_type_required", "Content-Type must be application/json")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var req domain.RunRequest
	if err := decoder.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		writeError(w, http.StatusBadRequest, "invalid_json", "request body must contain one JSON object")
		return
	}
	asOfExplicit := !req.AsOf.IsZero()

	fillDefaults(&req)

	if req.TaskType == domain.TaskHistoricalRun && !asOfExplicit {
		writeError(
			w,
			http.StatusBadRequest,
			"as_of_required",
			"historical replay requires an explicit as_of timestamp",
		)
		return
	}

	if asOfExplicit {
		req.AsOfMode = domain.AsOfModeExplicit
	} else {
		req.AsOfMode = domain.AsOfModeRealtimeSnapshot
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.analysisTimeout)
	defer cancel()
	result, err := s.runner.Run(ctx, req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]string{"code": "analysis_failed", "message": err.Error()}, "partial_result": result})
		return
	}
	status := http.StatusOK
	if result.Status == domain.RunStatusRejected {
		status = http.StatusBadRequest
	}
	writeJSON(w, status, result)
}
func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/analysis/runs/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 || parts[0] == "" || len(parts) > 2 {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	runID := parts[0]
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if len(parts) == 2 {
		if parts[1] != "events" {
			writeError(w, http.StatusNotFound, "not_found", "route not found")
			return
		}
		events, err := s.store.GetEvents(ctx, runID)
		if err != nil {
			if errors.Is(err, storage.ErrRunNotFound) {
				writeError(w, http.StatusNotFound, "run_not_found", err.Error())
			} else {
				writeError(w, http.StatusInternalServerError, "read_events_failed", err.Error())
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "events": events})
		return
	}
	result, err := s.store.GetResult(ctx, runID)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrRunNotFound):
			writeError(w, http.StatusNotFound, "run_not_found", err.Error())
		case errors.Is(err, storage.ErrRunNotFinished):
			writeError(w, http.StatusConflict, "run_not_finished", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "read_result_failed", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
func fillDefaults(req *domain.RunRequest) {
	if req.TaskType == "" {
		req.TaskType = domain.TaskPostMarketScan
	}
	if req.AsOf.IsZero() {
		req.AsOf = time.Now().In(mustLocation("Asia/Shanghai"))
	}
	if strings.TrimSpace(req.UniverseID) == "" {
		req.UniverseID = "request-watchlist"
	}
	if strings.TrimSpace(req.PortfolioID) == "" {
		req.PortfolioID = "paper-unconfigured"
	}
	if strings.TrimSpace(req.MarketID) == "" {
		req.MarketID = "CN-A"
	}
	if len(req.StrategyIDs) == 0 {
		req.StrategyIDs = []string{"trend-following"}
	}
	if req.StrategyConfigVersion == "" {
		req.StrategyConfigVersion = "trend-following-mvp-v1"
	}
	if req.RiskConfigVersion == "" {
		req.RiskConfigVersion = "unverified-tradability-block-v1"
	}
	if req.TradingRulesVersion == "" {
		req.TradingRulesVersion = "not-configured-v1"
	}
	if req.DataPolicyVersion == "" {
		req.DataPolicyVersion = "baseline-quality-v1"
	}
}
func mustLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone(name, 8*3600)
	}
	return loc
}
func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		log.Printf("WARN: write HTTP response failed: %v", err)
	}
}
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				log.Printf("ERROR: panic handling %s %s: %v", r.Method, r.URL.Path, v)
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("http method=%s path=%s duration_ms=%d remote=%s", r.Method, r.URL.Path, time.Since(started).Milliseconds(), r.RemoteAddr)
	})
}
