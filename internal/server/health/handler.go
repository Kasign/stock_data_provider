// Package health exposes process liveness and dependency readiness HTTP endpoints.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync/atomic"
	"time"

	serverdiagnostics "example.com/stock-analysis-flow/internal/server/diagnostics"
)

type readinessChecker interface {
	Run(context.Context) (serverdiagnostics.Report, error)
}

type Handler struct {
	checker  readinessChecker
	draining atomic.Bool
}

func NewHandler(checker readinessChecker) (*Handler, error) {
	if checker == nil {
		return nil, errors.New("readiness checker is required")
	}
	return &Handler{checker: checker}, nil
}

// MarkDraining 在关闭 HTTP Listener 前立即撤销 Readiness。
func (h *Handler) MarkDraining() {
	h.draining.Store(true)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeJSON(w, r, http.StatusMethodNotAllowed, map[string]any{
			"error": map[string]string{"code": "method_not_allowed", "message": "method not allowed"},
		})
		return
	}

	switch r.URL.Path {
	case "/healthz":
		writeJSON(w, r, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
	case "/readyz":
		if h.draining.Load() {
			writeJSON(w, r, http.StatusServiceUnavailable, map[string]any{
				"error": map[string]string{"code": "server_draining", "message": "server is shutting down"},
			})
			return
		}
		report, err := h.checker.Run(r.Context())
		if err != nil || !report.Readiness.Ready {
			writeJSON(w, r, http.StatusServiceUnavailable, map[string]any{
				"error": map[string]string{
					"code": "dependencies_unready", "message": "one or more dependencies are unavailable",
				},
			})
			return
		}
		writeJSON(w, r, http.StatusOK, map[string]any{
			"status":       "ready",
			"dependencies": map[string]string{"mariadb": "ok", "redis": "ok"},
			"time":         time.Now().UTC(),
		})
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
