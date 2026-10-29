package httpapi

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

type batchQuoteItem struct {
	Symbol       string        `json:"symbol"`
	Quote        *model.Quote  `json:"quote,omitempty"`
	ErrorCode    string        `json:"error_code,omitempty"`
	QualityStatus model.QualityStatus `json:"quality_status"`
}

func (s *Server) batchQuotes(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimSpace(r.URL.Query().Get("symbols"))
	if raw == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "symbols is required", requestID(r))
		return
	}
	parts := strings.Split(raw, ",")
	if len(parts) > 50 {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "symbols must contain no more than 50 securities", requestID(r))
		return
	}
	symbols := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		symbol, err := normalize.NormalizeSymbol(strings.TrimSpace(part))
		if err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "symbols contains an invalid A-share symbol", requestID(r))
			return
		}
		if _, exists := seen[symbol]; exists {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "symbols must not contain duplicates", requestID(r))
			return
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}

	items := make([]batchQuoteItem, len(symbols))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i, symbol := range symbols {
		wg.Add(1)
		go func(index int, symbol string) {
			defer wg.Done()
			items[index] = batchQuoteItem{Symbol: symbol, QualityStatus: model.QualityUnknown}
			select {
			case sem <- struct{}{}:
			case <-r.Context().Done():
				items[index].ErrorCode = "QUOTE_UNAVAILABLE"
				return
			}
			defer func() { <-sem }()
			quote, _, err := s.svc.Quote(r.Context(), symbol)
			if err != nil || quote == nil {
				items[index].ErrorCode = "QUOTE_UNAVAILABLE"
				return
			}
			items[index].Quote = quote
			items[index].QualityStatus = quote.QualityStatus
		}(i, symbol)
	}
	wg.Wait()

	succeeded, failed := 0, 0
	quality := model.QualityValid
	for _, item := range items {
		if item.Quote == nil {
			failed++
			quality = model.QualityDegraded
			continue
		}
		succeeded++
		if item.QualityStatus != model.QualityValid {
			quality = model.QualityDegraded
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items": items, "total": len(items), "succeeded": succeeded, "failed": failed,
		"fetched_at": time.Now().UTC(), "quality_status": quality,
	})
}
