package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

// PriceLimitEventEvidence 获取并验证指定证券、指定交易日的特殊涨跌幅事件证据。
// 没有独立事件源、来源覆盖不完整或证据质量不足时必须失败关闭。
func (s *DataService) PriceLimitEventEvidence(ctx context.Context, rawSymbol, rawTradeDate string) (*model.PriceLimitEventEvidence, error) {
	if s == nil || len(s.priceLimitEventProviders) == 0 {
		return nil, errors.New("no enabled price-limit-event provider")
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	tradeDate := strings.TrimSpace(rawTradeDate)
	parsedDate, err := time.Parse("2006-01-02", tradeDate)
	if err != nil || parsedDate.Format("2006-01-02") != tradeDate {
		return nil, fmt.Errorf("invalid trade date %q", rawTradeDate)
	}
	if tradeDate > time.Now().In(marketLocation()).Format("2006-01-02") {
		return nil, errors.New("trade date must not be in the future in Asia/Shanghai")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var failures []string
	for _, p := range s.priceLimitEventProviders {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		evidence, fetchErr := p.FetchPriceLimitEventEvidence(ctx, symbol, tradeDate)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validatePriceLimitEventEvidence(symbol, tradeDate, p.Name(), evidence, time.Now().UTC()); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		return evidence, nil
	}
	if len(failures) == 0 {
		return nil, errors.New("all price-limit-event providers returned no usable evidence")
	}
	return nil, fmt.Errorf("all price-limit-event providers failed: %s", strings.Join(failures, "; "))
}

func validatePriceLimitEventEvidence(symbol, tradeDate, providerName string, evidence *model.PriceLimitEventEvidence, now time.Time) error {
	if evidence == nil {
		return errors.New("provider returned empty price-limit-event evidence")
	}
	returnedSymbol, symbolErr := normalize.NormalizeSymbol(evidence.Symbol)
	if symbolErr != nil || returnedSymbol != symbol {
		return fmt.Errorf("price-limit-event symbol mismatch: requested %s, got %q", symbol, evidence.Symbol)
	}
	if strings.TrimSpace(evidence.Source) == "" || evidence.Source != providerName ||
		!hasIdentifiableSourceVersion(evidence.SourceVersion) {
		return errors.New("price-limit-event source or version is missing/invalid")
	}
	sourceURL, urlErr := url.Parse(strings.TrimSpace(evidence.SourceURL))
	if urlErr != nil || sourceURL.Host == "" || (sourceURL.Scheme != "http" && sourceURL.Scheme != "https") {
		return errors.New("price-limit-event source URL must be HTTP(S)")
	}
	if evidence.ObservedAt.IsZero() || evidence.ObservedAt.After(now.Add(5*time.Minute)) {
		return errors.New("price-limit-event observation time is missing or in the future")
	}
	if evidence.QualityStatus != model.QualityValid {
		return fmt.Errorf("price-limit-event evidence quality is %q, not valid", evidence.QualityStatus)
	}
	switch evidence.Status {
	case model.PriceLimitExceptionUnknown:
		return errors.New("special-event status is unknown")
	case model.PriceLimitExceptionNone:
		if !evidence.CoverageConfirmed {
			return errors.New("absence of special event is not backed by complete coverage")
		}
	case model.PriceLimitExceptionRelistingFirst, model.PriceLimitExceptionDelistingFirst, model.PriceLimitExceptionExchangeSpecial:
		if strings.TrimSpace(evidence.AnnouncementID) == "" {
			return errors.New("special event requires an announcement identifier")
		}
	default:
		return fmt.Errorf("unsupported special-event status %q", evidence.Status)
	}
	from, err := time.Parse("2006-01-02", evidence.EffectiveFrom)
	if err != nil || from.Format("2006-01-02") != evidence.EffectiveFrom {
		return errors.New("effective_from must use YYYY-MM-DD")
	}
	to := from
	if evidence.EffectiveTo != "" {
		to, err = time.Parse("2006-01-02", evidence.EffectiveTo)
		if err != nil || to.Format("2006-01-02") != evidence.EffectiveTo {
			return errors.New("effective_to must use YYYY-MM-DD")
		}
	}
	if to.Before(from) || tradeDate < evidence.EffectiveFrom || tradeDate > to.Format("2006-01-02") {
		return errors.New("price-limit-event evidence does not cover requested trade date")
	}
	return nil
}
