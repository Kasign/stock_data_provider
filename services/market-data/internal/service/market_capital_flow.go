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
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

func (s *DataService) SetMarketCapitalFlowProviders(providers []provider.MarketCapitalFlowProvider) {
	s.marketCapitalFlowProviders = append([]provider.MarketCapitalFlowProvider(nil), providers...)
}

func (s *DataService) MarketCapitalFlowProviders() []provider.MarketCapitalFlowProvider {
	return append([]provider.MarketCapitalFlowProvider(nil), s.marketCapitalFlowProviders...)
}

// MarketCapitalFlow returns provider-reported market-wide capital-flow history.
// It never derives a market total by summing a partial set of individual stocks.
func (s *DataService) MarketCapitalFlow(ctx context.Context, startDate, endDate string) (*model.MarketCapitalFlowSnapshot, bool, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil || start.Format("2006-01-02") != startDate {
		return nil, false, errors.New("market-capital-flow start must use YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil || end.Format("2006-01-02") != endDate || end.Before(start) {
		return nil, false, errors.New("market-capital-flow end must use YYYY-MM-DD and not precede start")
	}
	key := fmt.Sprintf("md:v1:market-capital-flow:%s:%s", startDate, endDate)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.MarketCapitalFlowSnapshot
		if json.Unmarshal(data, &cached) == nil && validateMarketCapitalFlowSnapshot(startDate, endDate, &cached) == nil && time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < 15*time.Minute {
			return &cached, true, nil
		}
	}
	providers := s.MarketCapitalFlowProviders()
	if len(providers) == 0 {
		return nil, false, errors.New("no market-capital-flow provider enabled")
	}
	failures := make([]string, 0, len(providers))
	for _, p := range providers {
		if p == nil {
			continue
		}
		snapshot, fetchErr := p.FetchMarketCapitalFlow(ctx, startDate, endDate)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if validateErr := validateMarketCapitalFlowSnapshot(startDate, endDate, snapshot); validateErr != nil {
			failures = append(failures, p.Name()+": "+validateErr.Error())
			continue
		}
		if data, marshalErr := json.Marshal(snapshot); marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, data, 15*time.Minute); cacheErr != nil && s.logger != nil {
				s.logger.Warn("cache market capital flow failed", "provider", p.Name(), "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	if len(failures) == 0 {
		return nil, false, errors.New("no market-capital-flow provider is available")
	}
	return nil, false, fmt.Errorf("all market-capital-flow providers failed: %s", strings.Join(failures, "; "))
}

func validateMarketCapitalFlowSnapshot(startDate, endDate string, snapshot *model.MarketCapitalFlowSnapshot) error {
	if snapshot == nil {
		return errors.New("market-capital-flow provider returned an empty response")
	}
	if snapshot.StartDate != startDate || snapshot.EndDate != endDate {
		return errors.New("market-capital-flow date range mismatch")
	}
	if strings.TrimSpace(snapshot.Source) == "" || !hasIdentifiableSourceVersion(snapshot.SourceVersion) || snapshot.FetchedAt.IsZero() {
		return errors.New("market-capital-flow source, source_version or fetched_at is missing")
	}
	if snapshot.QualityStatus != model.QualityDegraded {
		return errors.New("market-capital-flow must remain degraded without a verifiable provider snapshot timestamp")
	}
	if snapshot.Total != len(snapshot.Items) || snapshot.Total <= 0 {
		return errors.New("market-capital-flow total does not match returned observations")
	}
	seen := make(map[string]bool, len(snapshot.Items))
	previousDate := ""
	for _, item := range snapshot.Items {
		parsed, err := time.Parse("2006-01-02", item.TradeDate)
		if err != nil || parsed.Format("2006-01-02") != item.TradeDate || item.TradeDate < startDate || item.TradeDate > endDate {
			return fmt.Errorf("market-capital-flow contains an invalid/out-of-range trade date %q", item.TradeDate)
		}
		if seen[item.TradeDate] || (previousDate != "" && item.TradeDate <= previousDate) {
			return fmt.Errorf("market-capital-flow trade dates are duplicated or not ascending at %s", item.TradeDate)
		}
		seen[item.TradeDate] = true
		previousDate = item.TradeDate
		for _, value := range []*float64{
			item.CloseSH, item.ChangePercentSH, item.CloseSZ, item.ChangePercentSZ,
			item.NetAmountCNY, item.NetAmountRatePercent, item.ExtraLargeNetInflowCNY,
			item.ExtraLargeNetInflowRatePct, item.LargeNetInflowCNY, item.LargeNetInflowRatePercent,
			item.MediumNetInflowCNY, item.MediumNetInflowRatePercent, item.SmallNetInflowCNY,
			item.SmallNetInflowRatePercent,
		} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return fmt.Errorf("market-capital-flow row %s contains a non-finite value", item.TradeDate)
			}
		}
	}
	return nil
}
