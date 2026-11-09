package httpapi

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) marketSession(w http.ResponseWriter, r *http.Request) {
	values, supplied := r.URL.Query()["at"]
	at := time.Now()
	if supplied {
		if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "at must be a single RFC3339 timestamp", requestID(r))
			return
		}
		parsed, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(values[0]))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "at must be a valid RFC3339 timestamp", requestID(r))
			return
		}
		at = parsed
	}

	snapshot, err := s.svc.MarketSession(r.Context(), at)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
