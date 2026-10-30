package httpapi

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) marketCapitalFlow(w http.ResponseWriter, r *http.Request) {
	start := strings.TrimSpace(r.URL.Query().Get("start"))
	end := strings.TrimSpace(r.URL.Query().Get("end"))
	if start == "" || end == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "start and end are required", requestID(r))
		return
	}
	startDate, startErr := time.Parse("2006-01-02", start)
	endDate, endErr := time.Parse("2006-01-02", end)
	if startErr != nil || endErr != nil || startDate.Format("2006-01-02") != start || endDate.Format("2006-01-02") != end || endDate.Before(startDate) || endDate.Sub(startDate) > 2000*24*time.Hour {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "start/end must be valid YYYY-MM-DD dates with an inclusive range no longer than 2000 calendar days", requestID(r))
		return
	}
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if end > time.Now().In(loc).Format("2006-01-02") {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "end must not be in the future in Asia/Shanghai", requestID(r))
		return
	}
	result, _, err := s.svc.MarketCapitalFlow(r.Context(), start, end)
	if err != nil {
		if strings.Contains(err.Error(), "no market-capital-flow provider enabled") {
			writeErr(w, http.StatusServiceUnavailable, "PROVIDER_UNAVAILABLE", "no market-capital-flow provider is enabled", requestID(r))
			return
		}
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, result)
}
