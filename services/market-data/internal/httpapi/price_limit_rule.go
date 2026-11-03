package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

func (s *Server) priceLimitRule(w http.ResponseWriter, r *http.Request) {
	rawSymbol := r.PathValue("symbol")
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), requestID(r))
		return
	}
	values, supplied := r.URL.Query()["date"]
	if !supplied || len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date is required and must be a single YYYY-MM-DD value", requestID(r))
		return
	}
	tradeDate := strings.TrimSpace(values[0])
	parsedDate, err := time.Parse("2006-01-02", tradeDate)
	if err != nil || parsedDate.Format("2006-01-02") != tradeDate {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date must use YYYY-MM-DD", requestID(r))
		return
	}
	if tradeDate > time.Now().In(time.FixedZone("Asia/Shanghai", 8*60*60)).Format("2006-01-02") {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date must not be in the future in Asia/Shanghai", requestID(r))
		return
	}

	status, statusErr := s.svc.SecurityTradingStatus(r.Context(), symbol, tradeDate)
	var evidence *model.PriceLimitEventEvidence
	var evidenceErr error
	if statusErr == nil {
		evidence, evidenceErr = s.svc.PriceLimitEventEvidence(r.Context(), symbol, tradeDate)
	}
	result, resolveErr := service.ResolvePriceLimitRuleFromTradingStatus(symbol, tradeDate, status, evidence)
	if resolveErr != nil {
		writeDomainErr(w, resolveErr, requestID(r))
		return
	}
	if statusErr != nil {
		result.QualityStatus = model.QualityUnknown
		result.Status = model.PriceLimitStatusUnknown
		result.LimitPercent = nil
		result.QualityReasons = appendUniqueRuleReason(result.QualityReasons, "security_trading_status_provider_unavailable")
	}
	if evidenceErr != nil {
		result.QualityStatus = model.QualityUnknown
		result.Status = model.PriceLimitStatusUnknown
		result.LimitPercent = nil
		result.NoLimitReason = ""
		result.QualityReasons = appendUniqueRuleReason(result.QualityReasons, "price_limit_event_provider_unavailable_or_evidence_invalid")
	}
	writeJSON(w, http.StatusOK, result)
}

func appendUniqueRuleReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
