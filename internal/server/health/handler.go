// Package health exposes process liveness and dependency readiness HTTP endpoints.
package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	serverdiagnostics "example.com/stock-analysis-flow/internal/server/diagnostics"
)

type readinessChecker interface {
	Run(context.Context) (serverdiagnostics.Report, error)
}

type Handler struct {
	checker readinessChecker
}

func NewHandler(checker readinessChecker) (*Handler, error) {
	if checker == nil {
		return nil, errors.New("readiness checker is required")
	}
	return &Handler{checker: checker}, nil
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	switch r.URL.Path {
	case "/healthz":
		writeJSON(w, r, http.StatusOK, map[string]string{"status": "ok"})
	case "/readyz":
		report, err := h.checker.Run(r.Context())
		if err != nil {
			writeJSON(w, r, http.StatusServiceUnavailable, serverdiagnostics.Readiness{
				Ready: false, ReasonCode: "server.readiness.diagnostics_failed",
				SafeMessage: "Readiness diagnostics could not be completed",
			})
			return
		}
		status := http.StatusOK
		if !report.Readiness.Ready {
			status = http.StatusServiceUnavailable
		}
		writeJSON(w, r, status, report.Readiness)
	default:
		http.NotFound(w, r)
	}
}

func writeJSON(w http.ResponseWriter, r *http.Request, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if r.Method == http.MethodHead {
		return
	}
	_ = json.NewEncoder(w).Encode(value)
}
