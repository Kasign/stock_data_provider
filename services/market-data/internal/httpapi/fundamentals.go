package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *Server) fundamentals(w http.ResponseWriter, r *http.Request) {
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
