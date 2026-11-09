package httpapi

import (
	"net/http"
	"strings"
	"time"
)

func (s *Server) indexCatalog(w http.ResponseWriter, r *http.Request) {
	tag := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("tag")))
	if tag == "" {
		tag = "cn_concept"
	}
	switch tag {
	case "cn_concept", "region", "tszs", "industry", "tushare":
	default:
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "tag must be cn_concept, region, tszs, industry or tushare", requestID(r))
		return
	}
	snapshot, cacheHit, err := s.svc.IndexCatalog(r.Context(), tag)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, cacheHit)
	writeJSON(w, http.StatusOK, snapshot)
}

func validateIndexProviderSource(rawSource string) (string, string) {
	source := strings.ToLower(strings.TrimSpace(rawSource))
	if source == "" {
		return "", ""
	}
	switch source {
	case "hithink", "tushare":
		return source, ""
	default:
		return "", "source must be hithink or tushare"
	}
}

func validateIndexConstituentsAsOfQuery(rawAsOfDate string) (string, string) {
	asOfDate := strings.TrimSpace(rawAsOfDate)
	if asOfDate == "" {
		return "", ""
	}
	parsed, err := time.Parse("2006-01-02", asOfDate)
	if err != nil || parsed.Format("2006-01-02") != asOfDate {
		return "", "as_of must use YYYY-MM-DD"
	}
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	if asOfDate > time.Now().In(location).Format("2006-01-02") {
		return "", "as_of must not be in the future"
	}
	return asOfDate, ""
}

func (s *Server) indexConstituents(w http.ResponseWriter, r *http.Request) {
	indexSymbol := strings.TrimSpace(r.PathValue("symbol"))
	source, validationErr := validateIndexProviderSource(r.URL.Query().Get("source"))
	if validationErr != "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", validationErr, requestID(r))
		return
	}
	asOfDate, asOfValidationErr := validateIndexConstituentsAsOfQuery(r.URL.Query().Get("as_of"))
	if asOfValidationErr != "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", asOfValidationErr, requestID(r))
		return
	}
	if asOfDate != "" {
		if source == "hithink" {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "as_of queries currently require source=tushare or automatic selection; hithink only supports current constituents", requestID(r))
			return
		}
		snapshot, cacheHit, err := s.svc.IndexConstituentsAsOfFromSource(r.Context(), indexSymbol, asOfDate, source)
		if err != nil {
			writeDomainErr(w, err, requestID(r))
			return
		}
		setCacheHeader(w, cacheHit)
		writeJSON(w, http.StatusOK, snapshot)
		return
	}
	snapshot, cacheHit, err := s.svc.IndexConstituentsFromSource(r.Context(), indexSymbol, source)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, cacheHit)
	writeJSON(w, http.StatusOK, snapshot)
}

func validateIndexDailyBarsQuery(startDate, endDate string) (string, string, string) {
	startDate, endDate = strings.TrimSpace(startDate), strings.TrimSpace(endDate)
	if startDate == "" || endDate == "" {
		return "", "", "both start and end query parameters are required"
	}
	start, startErr := time.Parse("2006-01-02", startDate)
	end, endErr := time.Parse("2006-01-02", endDate)
	if startErr != nil || start.Format("2006-01-02") != startDate {
		return "", "", "start must use YYYY-MM-DD"
	}
	if endErr != nil || end.Format("2006-01-02") != endDate {
		return "", "", "end must use YYYY-MM-DD"
	}
	if start.After(end) {
		return "", "", "start must not be after end"
	}
	if end.After(start.AddDate(10, 0, 0)) || int(end.Sub(start)/(24*time.Hour))+1 > 3660 {
		return "", "", "date range must not exceed 10 years / 3660 calendar days"
	}
	return startDate, endDate, ""
}

func (s *Server) indexDailyBars(w http.ResponseWriter, r *http.Request) {
	start, end, validationErr := validateIndexDailyBarsQuery(
		r.URL.Query().Get("start"),
		r.URL.Query().Get("end"),
	)
	if validationErr != "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", validationErr, requestID(r))
		return
	}
	source, sourceErr := validateIndexProviderSource(r.URL.Query().Get("source"))
	if sourceErr != "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", sourceErr, requestID(r))
		return
	}
	snapshot, cacheHit, err := s.svc.IndexDailyBarsFromSource(r.Context(), r.PathValue("symbol"), start, end, source)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, cacheHit)
	writeJSON(w, http.StatusOK, snapshot)
}
