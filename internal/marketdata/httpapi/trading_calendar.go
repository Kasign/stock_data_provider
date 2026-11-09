package httpapi

import (
	"net/http"
	"strings"
	"time"
)

func validateTradingCalendarQuery(startDate, endDate string) (string, string, string) {
	startDate, endDate = strings.TrimSpace(startDate), strings.TrimSpace(endDate)
	if startDate == "" || endDate == "" {
		return "", "", "both start and end query parameters are required"
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return "", "", "start must use YYYY-MM-DD"
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return "", "", "end must use YYYY-MM-DD"
	}
	if start.After(end) {
		return "", "", "start must not be after end"
	}
	if int(end.Sub(start)/(24*time.Hour))+1 > 3660 {
		return "", "", "date range must not exceed 3660 calendar days"
	}
	return start.Format("2006-01-02"), end.Format("2006-01-02"), ""
}

func (s *Server) tradingCalendar(w http.ResponseWriter, r *http.Request) {
	startDate, endDate, validationErr := validateTradingCalendarQuery(
		r.URL.Query().Get("start"), r.URL.Query().Get("end"),
	)
	if validationErr != "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", validationErr, requestID(r))
		return
	}
	snapshot, cacheHit, err := s.svc.TradingCalendar(r.Context(), startDate, endDate)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if cacheHit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, snapshot)
}
