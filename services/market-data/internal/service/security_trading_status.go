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

// SecurityTradingStatus loads historical name/risk-warning and suspension evidence for a date.
// It does not infer an absent ST marker when the provider cannot prove which name was effective.
func (s *DataService) SecurityTradingStatus(ctx context.Context, rawSymbol, rawTradeDate string) (*model.SecurityTradingStatusSnapshot, error) {
	if s == nil || len(s.securityTradingStatusProviders) == 0 {
		return nil, errors.New("no enabled security-trading-status provider")
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	parsedDate, err := time.Parse("2006-01-02", strings.TrimSpace(rawTradeDate))
	if err != nil || parsedDate.Format("2006-01-02") != strings.TrimSpace(rawTradeDate) {
		return nil, fmt.Errorf("invalid trade date %q", rawTradeDate)
	}
	tradeDate := parsedDate.Format("2006-01-02")
	if tradeDate > time.Now().In(marketLocation()).Format("2006-01-02") {
		return nil, errors.New("trade date must not be in the future in Asia/Shanghai")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var failures []string
	var best *model.SecurityTradingStatusSnapshot
	bestScore := -1
	for _, p := range s.securityTradingStatusProviders {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		candidate, fetchErr := p.FetchSecurityTradingStatus(ctx, symbol, tradeDate)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateSecurityTradingStatusSnapshot(symbol, tradeDate, p.Name(), candidate); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		score := securityTradingStatusEvidenceScore(candidate)
		if score > bestScore {
			best, bestScore = candidate, score
		}
		if score >= 100 {
			return candidate, nil
		}
	}
	if best != nil {
		return best, nil
	}
	if len(failures) == 0 {
		return nil, errors.New("all security-trading-status providers returned no usable data")
	}
	return nil, fmt.Errorf("all security-trading-status providers failed: %s", strings.Join(failures, "; "))
}

func validateSecurityTradingStatusSnapshot(symbol, tradeDate, providerName string, snapshot *model.SecurityTradingStatusSnapshot) error {
	if snapshot == nil {
		return errors.New("security-trading-status provider returned an empty snapshot")
	}
	returnedSymbol, err := normalize.NormalizeSymbol(snapshot.Symbol)
	if err != nil || returnedSymbol != symbol || snapshot.TradeDate != tradeDate {
		return fmt.Errorf("security-trading-status identity mismatch: requested %s/%s, got %q/%s", symbol, tradeDate, snapshot.Symbol, snapshot.TradeDate)
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.Source != providerName ||
		!hasIdentifiableSourceVersion(snapshot.SourceVersion) ||
		!serviceTimestampIsSane(snapshot.FetchedAt, time.Now().UTC()) {
		return errors.New("security-trading-status source, version or fetched_at is missing/invalid")
	}
	switch snapshot.DelistingStatus {
	case model.DelistingStatusUnknown:
		if snapshot.QualityStatus == model.QualityValid {
			return errors.New("unknown delisting status cannot be marked valid")
		}
	case model.DelistingStatusNormal, model.DelistingStatusDelistingRisk, model.DelistingStatusDelistingPeriod:
	default:
		return fmt.Errorf("unsupported delisting status %q", snapshot.DelistingStatus)
	}
	switch snapshot.RiskWarningStatus {
	case model.RiskWarningUnknown:
		if snapshot.QualityStatus == model.QualityValid {
			return errors.New("unknown risk-warning status cannot be marked valid")
		}
	case model.RiskWarningNormal, model.RiskWarningApplied:
		if strings.TrimSpace(snapshot.SecurityName) == "" {
			return errors.New("known risk-warning status requires the effective historical security name")
		}
		start, parseErr := time.Parse("2006-01-02", snapshot.NameEffectiveFrom)
		if parseErr != nil || start.Format("2006-01-02") != snapshot.NameEffectiveFrom || snapshot.NameEffectiveFrom > tradeDate {
			return errors.New("effective historical name does not prove its start date")
		}
		if snapshot.NameEffectiveTo != "" {
			end, endErr := time.Parse("2006-01-02", snapshot.NameEffectiveTo)
			if endErr != nil || end.Format("2006-01-02") != snapshot.NameEffectiveTo || snapshot.NameEffectiveTo < tradeDate {
				return errors.New("effective historical name does not cover requested date")
			}
		}
	default:
		return fmt.Errorf("unsupported risk-warning status %q", snapshot.RiskWarningStatus)
	}
	if snapshot.TradingDayKnown {
		if snapshot.SuspensionStatus == model.SuspensionStatusNotTradingDay && snapshot.IsTradingDay {
			return errors.New("non-trading-day suspension state conflicts with trading calendar")
		}
		if !snapshot.IsTradingDay && snapshot.SuspensionStatus != model.SuspensionStatusNotTradingDay &&
			snapshot.SuspensionStatus != model.SuspensionStatusUnknown {
			return errors.New("suspension state conflicts with non-trading date")
		}
		switch snapshot.SuspensionStatus {
		case model.SuspensionStatusNotTradingDay, model.SuspensionStatusNoEvent,
			model.SuspensionStatusSuspended, model.SuspensionStatusIntraday,
			model.SuspensionStatusResumed, model.SuspensionStatusUnknown:
		default:
			return fmt.Errorf("unsupported suspension status %q", snapshot.SuspensionStatus)
		}
	} else if snapshot.SuspensionStatus != model.SuspensionStatusUnknown {
		return errors.New("suspension status cannot be known without a verified trading-calendar status")
	}
	if snapshot.ListingStatusKnown {
		listingDate, parseErr := time.Parse("2006-01-02", snapshot.ListingDate)
		if parseErr != nil || listingDate.Format("2006-01-02") != snapshot.ListingDate ||
			listingDate.After(parsedDateOrZero(tradeDate)) || snapshot.ListingTradingDayNumber < 1 {
			return errors.New("listing trading-day metadata is inconsistent")
		}
	}
	switch snapshot.QualityStatus {
	case model.QualityValid, model.QualityDegraded, model.QualityUnknown:
	default:
		return fmt.Errorf("unsupported security-trading-status quality %q", snapshot.QualityStatus)
	}
	if snapshot.RiskWarningStatus == model.RiskWarningUnknown ||
		snapshot.DelistingStatus == model.DelistingStatusUnknown ||
		!snapshot.TradingDayKnown || snapshot.SuspensionStatus == model.SuspensionStatusUnknown ||
		!snapshot.ListingStatusKnown {
		if snapshot.QualityStatus == model.QualityValid {
			return errors.New("incomplete historical security status cannot be marked valid")
		}
	}
	return nil
}

func parsedDateOrZero(value string) time.Time {
	parsed, _ := time.Parse("2006-01-02", value)
	return parsed
}

func securityTradingStatusEvidenceScore(snapshot *model.SecurityTradingStatusSnapshot) int {
	if snapshot == nil {
		return -1
	}
	score := 0
	switch snapshot.QualityStatus {
	case model.QualityValid:
		score += 100
	case model.QualityDegraded:
		score += 40
	case model.QualityUnknown:
		score += 10
	}
	if snapshot.RiskWarningStatus != model.RiskWarningUnknown {
		score += 10
	}
	if snapshot.TradingDayKnown {
		score += 10
	}
	if snapshot.SuspensionStatus != model.SuspensionStatusUnknown {
		score += 10
	}
	if snapshot.ListingStatusKnown {
		score += 5
	}
	return score
}
