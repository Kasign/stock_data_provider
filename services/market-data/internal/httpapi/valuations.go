package httpapi

import (
	"net/http"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

func (s *Server) valuations(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("symbols")
	if strings.TrimSpace(raw) == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "symbols is required; pass comma-separated six-digit A-share codes with exchange suffixes", requestID(r))
		return
	}
	tokens := strings.Split(raw, ",")
	// 限制按原始 token 数计算，不能先去重再绕过供应商上限。
	if len(tokens) > 100 {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "symbols must contain no more than 100 raw tokens", requestID(r))
		return
	}
	symbols := make([]string, 0, len(tokens))
	seen := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "symbols must not contain empty tokens", requestID(r))
			return
		}
		symbol, err := normalize.NormalizeSymbol(token)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error(), requestID(r))
			return
		}
		parts := strings.Split(symbol, ".")
		if len(parts) != 2 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "BJ") || len(parts[0]) != 6 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "symbols only supports six-digit A-share symbols with SH/SZ/BJ suffixes", requestID(r))
			return
		}
		for _, digit := range parts[0] {
			if digit < '0' || digit > '9' {
				writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "each symbol must start with six numeric digits", requestID(r))
				return
			}
		}
		if _, duplicate := seen[symbol]; duplicate {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	snapshot, cacheHit, err := s.svc.Valuations(r.Context(), symbols)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, cacheHit)
	writeJSON(w, http.StatusOK, snapshot)
}
