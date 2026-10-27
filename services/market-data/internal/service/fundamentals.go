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

func (s *DataService) SetFundamentalsProviders(providers []provider.FundamentalsProvider) {
	s.fundamentalsProviders = append([]provider.FundamentalsProvider(nil), providers...)
}
func (s *DataService) FundamentalsProviders() []provider.FundamentalsProvider {
	return append([]provider.FundamentalsProvider(nil), s.fundamentalsProviders...)
}
func fundamentalPeriodEnd(year, quarter int) string {
	return time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func validateFundamentalsSnapshot(symbol string, year, quarter int, snapshot *model.FundamentalsSnapshot) error {
	if snapshot == nil {
		return errors.New("fundamentals provider returned an empty snapshot")
	}
	got, err := normalize.NormalizeSymbol(snapshot.Symbol)
	if err != nil || got != symbol || snapshot.Year != year || snapshot.Quarter != quarter {
		return errors.New("fundamentals snapshot identity/period mismatch")
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("fundamentals source or fetched_at is missing")
	}
	if len(snapshot.Sections) != 6 {
		return fmt.Errorf("fundamentals snapshot has %d sections, expected 6", len(snapshot.Sections))
	}
	allowed := map[string]bool{"profitability": true, "operating": true, "growth": true, "solvency": true, "cash_flow": true, "dupont": true}
	seen := make(map[string]bool, len(snapshot.Sections))
	hasData, allValid := false, true
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	for _, section := range snapshot.Sections {
		if !allowed[section.Category] || seen[section.Category] {
			return fmt.Errorf("unsupported or duplicate fundamentals section %q", section.Category)
		}
		seen[section.Category] = true
		if section.HasData != (len(section.Metrics) > 0) {
			return fmt.Errorf("fundamentals section %s has inconsistent has_data/metrics", section.Category)
		}
		if section.QualityStatus != model.QualityValid && section.QualityStatus != model.QualityDegraded && section.QualityStatus != model.QualityUnknown {
			return fmt.Errorf("fundamentals section %s has unsupported quality %q", section.Category, section.QualityStatus)
		}
		if section.HasData {
			hasData = true
			if section.ReportDate != fundamentalPeriodEnd(year, quarter) {
				return fmt.Errorf("fundamentals section %s report date mismatches requested period", section.Category)
			}
			if section.ReportDate > today {
				return fmt.Errorf("fundamentals section %s report period is in the future", section.Category)
			}
			if section.PublicationDate == "" {
				if section.QualityStatus == model.QualityValid {
					return fmt.Errorf("fundamentals section %s cannot be valid without a publication date", section.Category)
				}
			} else {
				publicationDate, dateErr := time.Parse("2006-01-02", section.PublicationDate)
				if dateErr != nil || publicationDate.Format("2006-01-02") != section.PublicationDate {
					return fmt.Errorf("fundamentals section %s has invalid publication date", section.Category)
				}
				if section.PublicationDate > today {
					return fmt.Errorf("fundamentals section %s publication date is in the future", section.Category)
				}
			}
			if section.QualityStatus != model.QualityValid {
				allValid = false
			}
		} else {
			allValid = false
		}
		for _, metric := range section.Metrics {
			if metric.Key == "" || metric.Name == "" || metric.Unit == "" || metric.SourceField == "" || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
				return fmt.Errorf("fundamentals section %s has invalid metric metadata/value", section.Category)
			}
		}
	}
	if snapshot.HasData != hasData {
		return errors.New("fundamentals snapshot has inconsistent has_data")
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded && snapshot.QualityStatus != model.QualityUnknown {
		return fmt.Errorf("fundamentals snapshot has unsupported quality %q", snapshot.QualityStatus)
	}
	if snapshot.QualityStatus == model.QualityValid && (!hasData || !allValid) {
		return errors.New("fundamentals snapshot cannot be valid with missing/incomplete sections")
	}
	return nil
}

func fundamentalsCacheTTL(snapshot *model.FundamentalsSnapshot) time.Duration {
	if snapshot == nil || !snapshot.HasData {
		return 5 * time.Minute
	}
	if snapshot.QualityStatus != model.QualityValid {
		return time.Hour
	}
	return 12 * time.Hour
}

func (s *DataService) Fundamentals(ctx context.Context, rawSymbol string, year, quarter int) (*model.FundamentalsSnapshot, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	if year < 2007 || year > time.Now().Year() {
		return nil, false, fmt.Errorf("fundamental year must be between 2007 and %d", time.Now().Year())
	}
	if quarter < 1 || quarter > 4 {
		return nil, false, errors.New("fundamental quarter must be between 1 and 4")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key := fmt.Sprintf("md:v1:fundamentals:%s:%d:%d", symbol, year, quarter)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.FundamentalsSnapshot
		if json.Unmarshal(data, &cached) == nil && validateFundamentalsSnapshot(symbol, year, quarter, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := fundamentalsCacheTTL(&cached)
			if time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("fundamentals cache read failed", "symbol", symbol, "year", year, "quarter", quarter, "error", cacheErr)
	}
	if len(s.fundamentalsProviders) == 0 {
		return nil, false, errors.New("no enabled fundamentals provider")
	}
	var failures []string
	for _, p := range s.fundamentalsProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchFundamentals(ctx, symbol, year, quarter)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateFundamentalsSnapshot(symbol, year, quarter, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		payload, err := json.Marshal(snapshot)
		if err != nil {
			s.logger.Warn("fundamentals cache encode failed", "symbol", symbol, "year", year, "quarter", quarter, "error", err)
		} else {
			ttl := fundamentalsCacheTTL(snapshot)
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("fundamentals cache write failed", "symbol", symbol, "year", year, "quarter", quarter, "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all fundamentals providers failed: %s", strings.Join(failures, "; "))
}
