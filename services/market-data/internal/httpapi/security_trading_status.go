package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

func (s *Server) securityTradingStatus(w http.ResponseWriter, r *http.Request) {
	values, supplied := r.URL.Query()["date"]
	if !supplied || len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date is required and must be a single YYYY-MM-DD value", requestID(r))
		return
	}
	date := strings.TrimSpace(values[0])
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil || parsed.Format("2006-01-02") != date {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date must use YYYY-MM-DD", requestID(r))
		return
	}
	if date > time.Now().In(time.FixedZone("CST", 8*60*60)).Format("2006-01-02") {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date must not be in the future in Asia/Shanghai", requestID(r))
		return
	}
	asOfValues, hasAsOf := r.URL.Query()["as_of"]
	var snapshot *model.SecurityTradingStatusSnapshot
	var snapshotErr error
	if hasAsOf {
		if len(asOfValues) != 1 || strings.TrimSpace(asOfValues[0]) == "" {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "as_of must be a single RFC3339 timestamp", requestID(r))
			return
		}
		asOf, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(asOfValues[0]))
		if parseErr != nil || asOf.After(time.Now().UTC()) {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "as_of must be a valid non-future RFC3339 timestamp", requestID(r))
			return
		}
		snapshot, snapshotErr = s.svc.SecurityTradingStatusAsOf(r.Context(), r.PathValue("symbol"), date, asOf)
		switch {
		case errors.Is(snapshotErr, service.ErrSecurityTradingStatusAsOfUnavailable):
			writeErr(w, http.StatusServiceUnavailable, "PIT_STORAGE_UNAVAILABLE", "security-trading-status point-in-time storage is not enabled", requestID(r))
			return
		case errors.Is(snapshotErr, service.ErrSecurityTradingStatusAsOfNotFound):
			writeErr(w, http.StatusNotFound, "PIT_REVISION_NOT_FOUND", "no usable security-trading-status revision was recorded by the requested as_of time", requestID(r))
			return
		}
	} else {
		snapshot, snapshotErr = s.svc.SecurityTradingStatus(r.Context(), r.PathValue("symbol"), date)
	}
	if snapshotErr != nil {
		writeDomainErr(w, snapshotErr, requestID(r))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, snapshot)
}
