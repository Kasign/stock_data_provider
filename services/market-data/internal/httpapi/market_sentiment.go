package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) marketSentiment(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date != "" {
		parsed, err := time.Parse("2006-01-02", date)
		if err != nil || parsed.Format("2006-01-02") != date {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date must use YYYY-MM-DD", requestID(r))
			return
		}
		loc, locErr := time.LoadLocation("Asia/Shanghai")
		if locErr != nil {
			loc = time.FixedZone("Asia/Shanghai", 8*60*60)
		}
		if date != time.Now().In(loc).Format("2006-01-02") {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "live collection only supports the current China market date; use the history endpoint for persisted dates", requestID(r))
			return
		}
	}
	snapshot, err := s.svc.MarketSentiment(r.Context(), date)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) marketSentimentHistory(w http.ResponseWriter, r *http.Request) {
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	if start == "" || end == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "start and end are required", requestID(r))
		return
	}
	limit := 366
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 500 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be between 1 and 500", requestID(r))
			return
		}
		limit = parsed
	}
	startTime, startErr := time.Parse("2006-01-02", start)
	endTime, endErr := time.Parse("2006-01-02", end)
	if startErr != nil || endErr != nil || startTime.Format("2006-01-02") != start || endTime.Format("2006-01-02") != end || endTime.Before(startTime) || endTime.Sub(startTime) > 366*24*time.Hour {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "start/end must be valid dates with an inclusive range no longer than 366 days", requestID(r))
		return
	}
	result, err := s.svc.MarketSentimentHistory(r.Context(), start, end, limit)
	if err != nil {
		if strings.Contains(err.Error(), "storage is not enabled") {
			writeErr(w, http.StatusServiceUnavailable, "STORAGE_UNAVAILABLE", "market-sentiment history storage is not enabled", requestID(r))
			return
		}
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}
