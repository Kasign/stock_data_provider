// Package httpapi exposes the analysis use case over HTTP.
package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
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
	tasks           taskService
	analysisTimeout time.Duration
}

func New(runner Runner, store Store, analysisTimeout time.Duration) (*Server, error) {
	return NewWithTasks(runner, store, analysisTimeout, nil)
}

func NewWithTasks(runner Runner, store Store, analysisTimeout time.Duration, tasks taskService) (*Server, error) {
	if runner == nil || store == nil {
		return nil, errors.New("http API runner and store are required")
	}
	if analysisTimeout <= 0 {
		analysisTimeout = 5 * time.Minute
	}
	return &Server{runner: runner, store: store, tasks: tasks, analysisTimeout: analysisTimeout}, nil
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /readyz", s.handleReady)
	mux.HandleFunc("GET /", s.handleRoot)
	mux.HandleFunc("POST /api/v1/analysis/runs", s.handleCreateRun)
	mux.HandleFunc("GET /api/v1/analysis/runs/", s.handleGetRun)
	mux.HandleFunc("POST /api/v1/analysis/tasks", s.handleCreateTask)
	mux.HandleFunc("GET /api/v1/analysis/tasks/", s.handleTaskRoute)
	return requestIDMiddleware(requestLog(recoverer(mux)))
}
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"service": "stock-analysis-go", "api_version": "v1", "health": "/healthz", "readiness": "/readyz", "create_analysis": "POST /api/v1/analysis/runs", "query_analysis": "GET /api/v1/analysis/runs/{run_id}", "query_events": "GET /api/v1/analysis/runs/{run_id}/events", "create_task": "POST /api/v1/analysis/tasks", "query_task": "GET /api/v1/analysis/tasks/{task_id}", "query_task_events": "GET /api/v1/analysis/tasks/{task_id}/events"})
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

	slog.Info("分析任务开始",
		"service", "stock-analysis-api",
		"request_id", requestIDFromContext(r),
		"task_type", req.TaskType,
		"market_id", req.MarketID,
		"as_of", req.AsOf.UTC().Format(time.RFC3339Nano),
		"as_of_mode", req.AsOfMode,
		"symbols_count", len(req.Symbols),
		"strategies_count", len(req.StrategyIDs))
	started := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), s.analysisTimeout)
	defer cancel()
	result, err := s.runner.Run(ctx, req)
	if err != nil {
		slog.Error("分析任务失败",
			"service", "stock-analysis-api",
			"request_id", requestIDFromContext(r),
			"run_id", result.RunID,
			"task_type", req.TaskType,
			"duration_ms", time.Since(started).Milliseconds(),
			"error_type", fmt.Sprintf("%T", err),
			"error", boundedLogText(err.Error(), 300))
		writeJSON(w, http.StatusBadGateway, map[string]any{"error": map[string]string{"code": "analysis_failed", "message": err.Error()}, "partial_result": result})
		return
	}
	status := http.StatusOK
	if result.Status == domain.RunStatusRejected {
		status = http.StatusBadRequest
	}
	slog.Info("分析任务完成",
		"service", "stock-analysis-api",
		"request_id", requestIDFromContext(r),
		"run_id", result.RunID,
		"task_type", result.Request.TaskType,
		"run_status", result.Status,
		"duration_ms", time.Since(started).Milliseconds(),
		"stage_events_count", len(result.Events),
		"signals_count", len(result.Signals),
		"plans_count", len(result.Plans),
		"warnings_count", len(result.Warnings),
		"quality_core_pass", result.Quality.CorePass,
		"failure_code", failureCode(result.Failure))
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
func failureCode(failure *domain.Failure) string {
	if failure == nil {
		return ""
	}
	return failure.Code
}

func boundedLogText(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
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
		slog.Error("HTTP response serialization failed", "service", "stock-analysis-api", "error", err.Error())
	}
}
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("panic while handling HTTP request", "service", "stock-analysis-api", "request_id", requestIDFromContext(r), "method", r.Method, "path", boundedLogPath(r.URL.Path), "panic_type", fmt.Sprintf("%T", v))
				writeError(w, http.StatusInternalServerError, "internal_error", "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type requestIDKey struct{}

func requestIDFromContext(r *http.Request) string {
	value, _ := r.Context().Value(requestIDKey{}).(string)
	return value
}

func validRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return time.Now().UTC().Format("20060102T150405.000000000Z")
}

// requestLog 只记录请求元数据，不记录请求体、响应体、查询参数或鉴权头。
func requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		slog.Info("HTTP 请求完成",
			"service", "stock-analysis-api",
			"request_id", requestIDFromContext(r),
			"method", r.Method,
			"path", boundedLogPath(r.URL.Path),
			"status_code", status,
			"duration_ms", time.Since(started).Milliseconds(),
			"response_bytes", recorder.bytes)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusRecorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}

func boundedLogPath(path string) string {
	if len(path) > 160 {
		return path[:160]
	}
	return path
}
