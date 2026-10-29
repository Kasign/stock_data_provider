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
	snapshot, err := s.svc.SecurityTradingStatus(r.Context(), r.PathValue("symbol"), date)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, snapshot)
}
