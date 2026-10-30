package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

func (s *Server) marketStocks(w http.ResponseWriter, r *http.Request) {
	page, pageSize := 1, 50
	if raw := strings.TrimSpace(r.URL.Query().Get("page")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 10000 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "page must be between 1 and 10000", requestID(r))
			return
		}
		page = value
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("page_size")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "page_size must be between 1 and 100", requestID(r))
			return
		}
		pageSize = value
	}
	includeMA := true
	if raw := strings.TrimSpace(r.URL.Query().Get("include_ma")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "include_ma must be true or false", requestID(r))
			return
		}
		includeMA = value
	}
	adjustment := strings.TrimSpace(r.URL.Query().Get("adjustment"))
	if adjustment == "" {
		adjustment = s.defaultAdjustment
	}
	basis, err := normalize.Adjustment(adjustment)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), requestID(r))
		return
	}
	snapshot, err := s.svc.MarketStocks(r.Context(), page, pageSize, includeMA, basis)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}
