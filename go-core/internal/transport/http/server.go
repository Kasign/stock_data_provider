package httptransport

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/application"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

type Config struct {
	Token          string
	RequestTimeout time.Duration
	ReadyCheck     func(context.Context) error
}

type Server struct {
	stock  *application.StockFlow
	market *application.MarketReviewFlow
	cfg    Config
	logger *slog.Logger
	mux    *http.ServeMux
}

func NewServer(cfg Config, stock *application.StockFlow, market *application.MarketReviewFlow, logger *slog.Logger) *Server {
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 20 * time.Minute
	}
	if logger == nil {
		logger = slog.Default()
	}
	s := &Server{stock: stock, market: market, cfg: cfg, logger: logger, mux: http.NewServeMux()}
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /ready", s.ready)
	s.mux.HandleFunc("GET /internal/v1/capabilities", s.capabilities)
	s.mux.HandleFunc("POST /internal/v1/analysis/stock", s.runStock) // compatibility endpoint
	s.mux.HandleFunc("POST /internal/v1/analysis/stocks", s.runStockBatch)
	s.mux.HandleFunc("POST /internal/v1/analysis/market-review", s.runMarket)
	return s
}

func (s *Server) Handler() http.Handler { return s.requestID(s.recoverer(s.mux)) }

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "service": "dsa-go-core", "time": time.Now().UTC()})
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ReadyCheck == nil {
		writeError(w, http.StatusServiceUnavailable, "readiness_not_configured", "Python capability readiness probe is not configured", requestID(r))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.cfg.ReadyCheck(ctx); err != nil {
		s.logger.Warn("readiness check failed", "request_id", requestID(r), "error", err)
		writeError(w, http.StatusServiceUnavailable, "dependency_not_ready", "Python capability service is not ready", requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready", "service": "dsa-go-core"})
}

func (s *Server) capabilities(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"service":          "dsa-go-core",
		"api_version":      "1.0.0",
		"contract_version": "1.0.0",
		"workflows":        []string{"stock-analysis", "stock-batch-analysis", "market-review"},
		"regions":          []string{"cn", "hk", "us", "jp", "kr"},
		"max_batch_size":   100,
	})
}

func (s *Server) runStock(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	var req domain.AnalysisRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), requestID(r))
		return
	}
	ctx, cancel := contextWithTimeout(r, s.cfg.RequestTimeout)
	defer cancel()
	response, err := s.stock.Run(ctx, req)
	if err != nil {
		s.logger.Error("stock workflow failed", "request_id", requestID(r), "stock_code", req.StockCode, "error", err)
		status, code, message := workflowError(err, "stock")
		writeError(w, status, code, message, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) runStockBatch(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	var req domain.StockBatchRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), requestID(r))
		return
	}
	codes, err := normalizeBatchCodes(req.StockCodes)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, "invalid_request", err.Error(), requestID(r))
		return
	}
	// Reject a stale caller-supplied reference once at batch acceptance. The
	// Python adapters only provide live snapshots, so each sequential stock
	// item gets its own execution-time reference instead of reusing a timestamp
	// that becomes stale while a large batch is running.
	if !req.AsOf.IsZero() {
		delta := time.Since(req.AsOf)
		if delta < 0 {
			delta = -delta
		}
		if delta > 90*time.Second {
			writeError(w, http.StatusUnprocessableEntity, "invalid_request", "batch as_of must be within 90 seconds of request acceptance because Python capabilities only provide live snapshots", requestID(r))
			return
		}
	}
	ctx, cancel := contextWithTimeout(r, s.cfg.RequestTimeout)
	defer cancel()
	queryID := strings.TrimSpace(req.QueryID)
	if queryID == "" {
		queryID = "go-batch-stocks-" + time.Now().UTC().Format("20060102T150405.000000000")
	}
	response := domain.StockBatchWorkflowResponse{
		QueryID:  queryID,
		Total:    len(codes),
		Results:  make([]domain.StockWorkflowResponse, 0, len(codes)),
		Failures: make([]domain.StockBatchFailure, 0),
	}
	for _, code := range codes {
		if ctx.Err() != nil {
			response.Failures = append(response.Failures, domain.StockBatchFailure{StockCode: code, Message: "batch stopped because the request context ended"})
			continue
		}
		item := domain.AnalysisRequest{
			StockCode: code, QueryID: queryID, ReportType: req.ReportType,
			// Stock AsOf is intentionally zero: StockFlow freezes a fresh reference
			// per item so later items in a slow sequential batch do not fail the
			// live-snapshot freshness guard or silently omit their quote.
			LookbackDays: req.LookbackDays, Notify: req.Notify,
			SearchNews: req.SearchNews, ReportLanguage: req.ReportLanguage,
		}
		result, runErr := s.stock.Run(ctx, item)
		if runErr != nil {
			s.logger.Warn("stock in batch failed", "request_id", requestID(r), "stock_code", code, "error", runErr)
			response.Failures = append(response.Failures, domain.StockBatchFailure{StockCode: code, Message: publicBatchError(runErr)})
			continue
		}
		response.Results = append(response.Results, result)
	}
	response.Succeeded = len(response.Results)
	response.Failed = len(response.Failures)
	writeJSON(w, http.StatusOK, response)
}

func normalizeBatchCodes(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, errors.New("stock_codes must contain between 1 and 100 codes")
	}
	if len(raw) > 100 {
		return nil, errors.New("stock_codes cannot contain more than 100 codes")
	}
	codes := make([]string, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))
	for _, rawCode := range raw {
		code := strings.ToUpper(strings.TrimSpace(rawCode))
		if code == "" || len(code) > 32 {
			return nil, errors.New("each stock code must contain between 1 and 32 characters")
		}
		if _, exists := seen[code]; exists {
			return nil, fmt.Errorf("duplicate stock code %q is not allowed in a batch", code)
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	return codes, nil
}

func publicBatchError(err error) string {
	var inputErr *application.InputError
	if errors.As(err, &inputErr) {
		return inputErr.Message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "workflow timed out"
	}
	if errors.Is(err, context.Canceled) {
		return "workflow cancelled"
	}
	return "workflow failed; consult service logs"
}

func (s *Server) runMarket(w http.ResponseWriter, r *http.Request) {
	if !s.authorized(w, r) {
		return
	}
	var req domain.MarketReviewRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error(), requestID(r))
		return
	}
	ctx, cancel := contextWithTimeout(r, s.cfg.RequestTimeout)
	defer cancel()
	response, err := s.market.Run(ctx, req)
	if err != nil {
		s.logger.Error("market review workflow failed", "request_id", requestID(r), "region", req.Region, "error", err)
		status, code, message := workflowError(err, "market")
		writeError(w, status, code, message, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(s.cfg.Token) == "" {
		writeError(w, http.StatusServiceUnavailable, "auth_not_configured", "GO_CORE_TOKEN must be configured", requestID(r))
		return false
	}
	provided := r.Header.Get("X-Go-Core-Token")
	if len(provided) != len(s.cfg.Token) || subtle.ConstantTimeCompare([]byte(provided), []byte(s.cfg.Token)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized", "valid X-Go-Core-Token header required", requestID(r))
		return false
	}
	return true
}

func workflowError(err error, workflow string) (int, string, string) {
	var inputErr *application.InputError
	if errors.As(err, &inputErr) {
		return http.StatusUnprocessableEntity, "invalid_request", inputErr.Message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return http.StatusGatewayTimeout, "workflow_timeout", workflow + " workflow timed out"
	}
	return http.StatusBadGateway, workflow + "_workflow_failed", workflow + " workflow failed; consult service logs"
}

func decodeJSON(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request must contain exactly one JSON value")
		}
		return err
	}
	return nil
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	body, err := json.Marshal(value)
	if err != nil {
		status = http.StatusInternalServerError
		body = []byte(`{"code":"internal_error","message":"response serialization failed"}`)
	} else {
		body = append(body, '\n')
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	writeJSON(w, status, map[string]any{"code": code, "message": message, "request_id": requestID})
}

type requestIDKey struct{}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := sanitizeRequestID(strings.TrimSpace(r.Header.Get("X-Request-ID")))
		if id == "" {
			id = "req-" + time.Now().UTC().Format("20060102T150405.000000000")
		}
		r = contextWithRequestID(r, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r)
	})
}
func sanitizeRequestID(value string) string {
	if len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return ""
		}
	}
	return value
}

func requestID(r *http.Request) string { v, _ := r.Context().Value(requestIDKey{}).(string); return v }
func contextWithRequestID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
}
func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}
func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.logger.Error("http handler panic", "request_id", requestID(r), "panic", recovered)
				writeError(w, http.StatusInternalServerError, "internal_error", "unexpected internal error", requestID(r))
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// compile-time use of the request/response contract types keeps route payloads explicit.
var _ = domain.AnalysisRequest{}
