package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

var (
	ErrSecurityTradingStatusAsOfUnavailable = errors.New("security-trading-status point-in-time storage is not enabled")
	ErrSecurityTradingStatusAsOfNotFound    = errors.New("no usable security-trading-status revision was recorded by the requested as_of time")
)

// SecurityTradingStatusAsOf only reads the append-only local revision table.
// It never calls live Providers or substitutes a currently fetched historical record.
func (s *DataService) SecurityTradingStatusAsOf(ctx context.Context, rawSymbol, rawTradeDate string, asOf time.Time) (*model.SecurityTradingStatusSnapshot, error) {
	if s == nil || s.store == nil {
		return nil, ErrSecurityTradingStatusAsOfUnavailable
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	date := strings.TrimSpace(rawTradeDate)
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil || parsedDate.Format("2006-01-02") != date {
		return nil, fmt.Errorf("invalid trade date %q", rawTradeDate)
	}
	if asOf.IsZero() || asOf.After(time.Now().UTC()) {
		return nil, errors.New("security-trading-status as_of must be a valid non-future timestamp")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	revisions, err := s.store.ListSecurityTradingStatusAsOf(ctx, symbol, date, asOf)
	if err != nil {
		return nil, err
	}
	var best *model.SecurityTradingStatusSnapshot
	bestScore := -1
	for index := range revisions {
		candidate := &revisions[index]
		if candidate.SystemObservedAt == nil || candidate.SystemObservedAt.After(asOf) ||
			candidate.FetchedAt.After(asOf) || candidate.Source == "" {
			continue
		}
		if err := validateSecurityTradingStatusSnapshot(symbol, date, candidate.Source, candidate); err != nil {
			continue
		}
		score := securityTradingStatusEvidenceScore(candidate)
		if best == nil || score > bestScore ||
			(score == bestScore && best.SystemObservedAt != nil && candidate.SystemObservedAt.After(*best.SystemObservedAt)) {
			copy := *candidate
			copy.QualityReasons = append([]string(nil), candidate.QualityReasons...)
			best, bestScore = &copy, score
		}
	}
	if best == nil {
		return nil, ErrSecurityTradingStatusAsOfNotFound
	}
	return best, nil
}
