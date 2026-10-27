package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

func (s *DataService) SetValuationProviders(providers []provider.ValuationProvider) {
	s.valuationProviders = append([]provider.ValuationProvider(nil), providers...)
}

func (s *DataService) ValuationProviders() []provider.ValuationProvider {
	return append([]provider.ValuationProvider(nil), s.valuationProviders...)
}

func validateValuationSnapshot(requested []string, snapshot *model.ValuationSnapshot) error {
	if snapshot == nil {
		return errors.New("valuation provider returned an empty snapshot")
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("valuation snapshot source or fetched_at is missing")
	}
	if snapshot.ProviderReportDate != "" {
		reportDate, dateErr := time.Parse("2006-01-02", snapshot.ProviderReportDate)
		if dateErr != nil || reportDate.Format("2006-01-02") != snapshot.ProviderReportDate {
			return errors.New("valuation snapshot provider_report_date is invalid")
		}
		today := time.Now().In(marketLocation()).Format("2006-01-02")
		if snapshot.ProviderReportDate > today {
			return fmt.Errorf("valuation snapshot provider_report_date %s is in the future (China market date %s)", snapshot.ProviderReportDate, today)
		}
	}
	if snapshot.ProviderTimestamp != nil {
		if snapshot.ProviderTimestamp.IsZero() {
			return errors.New("valuation snapshot provider_timestamp is zero")
		}
		if snapshot.ProviderTimestamp.After(time.Now().Add(60 * time.Second)) {
			return errors.New("valuation snapshot provider_timestamp is in the future")
		}
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded && snapshot.QualityStatus != model.QualityUnknown {
		return fmt.Errorf("valuation snapshot quality %q is unsupported", snapshot.QualityStatus)
	}
	if snapshot.Total != len(snapshot.Items) {
		return errors.New("valuation snapshot total does not match items")
	}
	if len(snapshot.RequestedSymbols) != len(requested) {
		return errors.New("valuation snapshot requested-symbol count mismatch")
	}
	expected := make(map[string]struct{}, len(requested))
	for index, symbol := range requested {
		expected[symbol] = struct{}{}
		if snapshot.RequestedSymbols[index] != symbol {
			return errors.New("valuation snapshot request order/identity mismatch")
		}
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	hasDegradation := len(snapshot.MissingSymbols) > 0 || snapshot.ProviderTimestamp == nil || snapshot.ProviderReportDate == ""
	for _, item := range snapshot.Items {
		symbol, err := normalize.NormalizeSymbol(item.Symbol)
		if err != nil || symbol != item.Symbol {
			return fmt.Errorf("valuation item has invalid/noncanonical symbol %q", item.Symbol)
		}
		if _, ok := expected[symbol]; !ok {
			return fmt.Errorf("valuation provider returned unrequested symbol %q", symbol)
		}
		if _, duplicate := seen[symbol]; duplicate {
			return fmt.Errorf("valuation provider returned duplicate symbol %q", symbol)
		}
		seen[symbol] = struct{}{}
		if item.Code != strings.Split(symbol, ".")[0] || strings.TrimSpace(item.Source) == "" || item.Source != snapshot.Source || item.FetchedAt.IsZero() {
			return fmt.Errorf("valuation item %s has inconsistent identity/source metadata", symbol)
		}
		if item.QualityStatus != model.QualityValid && item.QualityStatus != model.QualityDegraded {
			return fmt.Errorf("valuation item %s has unsupported quality %q", symbol, item.QualityStatus)
		}
		for _, value := range []*float64{item.PETTM, item.PEMRQ, item.PEStatic, item.PBMRQ, item.PSTTM, item.PCFTTM} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return fmt.Errorf("valuation item %s has non-finite metric", symbol)
			}
			if value == nil {
				hasDegradation = true
			}
		}
		if item.QualityStatus != model.QualityValid {
			hasDegradation = true
		}
	}
	missing := make(map[string]struct{}, len(snapshot.MissingSymbols))
	for _, symbol := range snapshot.MissingSymbols {
		if _, ok := expected[symbol]; !ok {
			return fmt.Errorf("valuation snapshot marks unrequested symbol %q as missing", symbol)
		}
		if _, alsoReturned := seen[symbol]; alsoReturned {
			return fmt.Errorf("valuation snapshot both returned and marked missing symbol %q", symbol)
		}
		if _, duplicate := missing[symbol]; duplicate {
			return fmt.Errorf("valuation snapshot has duplicate missing symbol %q", symbol)
		}
		missing[symbol] = struct{}{}
	}
	if len(seen)+len(missing) != len(requested) {
		return errors.New("valuation snapshot returned/missing symbol count does not match request")
	}
	if hasDegradation && snapshot.QualityStatus == model.QualityValid {
		return errors.New("valuation snapshot claims valid quality despite missing/stale fields")
	}
	return nil
}

func (s *DataService) Valuations(ctx context.Context, rawSymbols []string) (*model.ValuationSnapshot, bool, error) {
	if len(rawSymbols) == 0 || len(rawSymbols) > 100 {
		return nil, false, errors.New("valuation request must contain 1..100 raw symbol tokens")
	}
	symbols := make([]string, 0, len(rawSymbols))
	seen := make(map[string]struct{}, len(rawSymbols))
	for _, rawSymbol := range rawSymbols {
		symbol, err := normalize.NormalizeSymbol(strings.TrimSpace(rawSymbol))
		if err != nil {
			return nil, false, err
		}
		parts := strings.Split(symbol, ".")
		if len(parts) != 2 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "BJ") || len(parts[0]) != 6 {
			return nil, false, fmt.Errorf("valuation only supports six-digit A-share symbols with SH/SZ/BJ suffix: %q", rawSymbol)
		}
		for _, digit := range parts[0] {
			if digit < '0' || digit > '9' {
				return nil, false, fmt.Errorf("valuation symbol must use six numeric digits: %q", rawSymbol)
			}
		}
		if _, duplicate := seen[symbol]; duplicate {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	if len(symbols) == 0 {
		return nil, false, errors.New("valuation request has no valid symbols")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}

	cacheKey := "md:v1:valuations:" + strings.Join(symbols, ",")
	if data, ok, cacheErr := s.cache.Get(ctx, cacheKey); cacheErr == nil && ok {
		var cached model.ValuationSnapshot
		if json.Unmarshal(data, &cached) == nil && validateValuationSnapshot(symbols, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := 5 * time.Minute
			if cached.QualityStatus == model.QualityDegraded {
				ttl = time.Minute
			} else if cached.QualityStatus == model.QualityUnknown {
				ttl = 10 * time.Second
			}
			if time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("valuation cache read failed", "symbols", strings.Join(symbols, ","), "error", cacheErr)
	}
	if len(s.valuationProviders) == 0 {
		return nil, false, errors.New("no enabled valuation provider")
	}
	var failures []string
	for _, p := range s.valuationProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchValuations(ctx, symbols)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateValuationSnapshot(symbols, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		payload, marshalErr := json.Marshal(snapshot)
		if marshalErr != nil {
			s.logger.Warn("valuation cache encode failed", "symbols", strings.Join(symbols, ","), "error", marshalErr)
		} else {
			ttl := 5 * time.Minute
			if snapshot.QualityStatus == model.QualityDegraded {
				ttl = time.Minute
			} else if snapshot.QualityStatus == model.QualityUnknown {
				ttl = 10 * time.Second
			}
			if cacheErr := s.cache.Set(ctx, cacheKey, payload, ttl); cacheErr != nil {
				s.logger.Warn("valuation cache write failed", "symbols", strings.Join(symbols, ","), "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all valuation providers failed: %s", strings.Join(failures, "; "))
}
