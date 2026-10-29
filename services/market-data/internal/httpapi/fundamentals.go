package httpapi

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

func (s *Server) fundamentals(w http.ResponseWriter, r *http.Request) {
	asOfValues, hasAsOf := r.URL.Query()["as_of"]
	var asOf time.Time
	if hasAsOf {
		if len(asOfValues) != 1 || strings.TrimSpace(asOfValues[0]) == "" {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "as_of must be a single RFC3339 timestamp", requestID(r))
			return
		}
		parsed, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(asOfValues[0]))
		if parseErr != nil || parsed.After(time.Now().UTC()) {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "as_of must be a valid non-future RFC3339 timestamp", requestID(r))
			return
		}
		asOf = parsed
	}
	yearText, quarterText := strings.TrimSpace(r.URL.Query().Get("year")), strings.TrimSpace(r.URL.Query().Get("quarter"))
	if yearText == "" || quarterText == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "year and quarter are required; quarter must be 1..4", requestID(r))
		return
	}
	year, yearErr := strconv.Atoi(yearText)
	quarter, quarterErr := strconv.Atoi(quarterText)
	if yearErr != nil || quarterErr != nil || year < 2007 || year > time.Now().Year() || quarter < 1 || quarter > 4 {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "year must be 2007..current year and quarter must be 1..4", requestID(r))
		return
	}
	if hasAsOf {
		item, err := s.svc.FundamentalsAsOf(r.Context(), r.PathValue("symbol"), year, quarter, asOf)
		switch {
		case errors.Is(err, service.ErrFundamentalsAsOfUnavailable):
			writeErr(w, http.StatusServiceUnavailable, "PIT_STORAGE_UNAVAILABLE", "fundamentals point-in-time storage is not enabled", requestID(r))
			return
		case errors.Is(err, service.ErrFundamentalsAsOfNotFound):
			writeErr(w, http.StatusNotFound, "PIT_REVISION_NOT_FOUND", "no usable fundamentals revision was recorded by the requested as_of time", requestID(r))
			return
		case err != nil:
			writeDomainErr(w, err, requestID(r))
			return
		}
		w.Header().Set("X-Cache", "BYPASS")
		writeJSON(w, http.StatusOK, item)
		return
	}
	item, hit, err := s.svc.Fundamentals(r.Context(), r.PathValue("symbol"), year, quarter)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if hit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, item)
}
