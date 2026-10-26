package httpapi

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) hotStocks(w http.ResponseWriter, r *http.Request) {
	rankingType := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	if rankingType == "" {
		rankingType = "popular"
	}
	if rankingType != "popular" && rankingType != "surge" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "type must be popular or surge", requestID(r))
		return
	}
	limit := 10
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be an integer between 1 and 100", requestID(r))
			return
		}
		limit = value
	}
	snapshot, cacheHit, err := s.svc.HotStocks(r.Context(), rankingType, limit)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, cacheHit)
	writeJSON(w, http.StatusOK, snapshot)
}
