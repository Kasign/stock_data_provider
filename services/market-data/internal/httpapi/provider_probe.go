package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) dataSourceProbe(w http.ResponseWriter, r *http.Request) {
	run, err := s.svc.ProbeProviderCapabilities(r.Context())
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) dataSourceProbeHistory(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be between 1 and 100", requestID(r))
			return
		}
		limit = value
	}
	history, err := s.svc.ProviderProbeHistory(r.Context(), limit)
	if err != nil {
		if strings.Contains(err.Error(), "storage is not enabled") {
			writeErr(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "provider-probe history storage is not enabled", requestID(r))
			return
		}
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, history)
}
