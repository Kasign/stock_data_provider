package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

var (
	ErrAdjustmentFactorsAsOfNotFound = errors.New("no adjustment-factor revision with proven availability was recorded by as_of")
	ErrCorporateActionsAsOfNotFound = errors.New("no corporate-action revision with proven availability was recorded by as_of")
)

// RefreshAdjustmentFactors 从已注册 Provider 采集原始复权因子并追加到历史版本存储。
// Provider 没有提供 available_at 时保留 degraded；不推断因子首次公开时间。
func (s *DataService) RefreshAdjustmentFactors(ctx context.Context, rawSymbol, startDate, endDate string) ([]model.AdjustmentFactor, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil { return nil, err }
	if !validServiceDateRange(startDate, endDate) { return nil, errors.New("invalid adjustment-factor date range") }
	if s == nil || s.store == nil { return nil, errors.New("MariaDB adjustment-factor storage is not enabled") }
	var failures []string
	for _, named := range s.providers {
		p, ok := named.(provider.AdjustmentFactorProvider)
		if !ok { continue }
		values, fetchErr := p.FetchAdjustmentFactors(ctx, symbol, startDate, endDate)
		if fetchErr != nil { failures = append(failures, p.Name()+": "+fetchErr.Error()); continue }
		if len(values) == 0 { failures = append(failures, p.Name()+": empty adjustment-factor response"); continue }
		now := time.Now().UTC()
		valid := make([]model.AdjustmentFactor, 0, len(values))
		seen := make(map[string]struct{}, len(values))
		for _, value := range values {
			date, dateErr := time.Parse("2006-01-02", value.TradeDate)
			if dateErr != nil || date.Format("2006-01-02") != value.TradeDate ||
				(startDate != "" && value.TradeDate < startDate) || (endDate != "" && value.TradeDate > endDate) ||
				value.Factor <= 0 || math.IsNaN(value.Factor) || math.IsInf(value.Factor, 0) ||
				func() bool { got, normalizeErr := normalize.NormalizeSymbol(value.Symbol); return normalizeErr != nil || got != symbol }() ||
				strings.TrimSpace(value.Source) != p.Name() || !hasIdentifiableSourceVersion(value.SourceVersion) ||
				value.FetchedAt.IsZero() || value.FetchedAt.After(now.Add(time.Minute)) {
				fetchErr = fmt.Errorf("provider returned invalid adjustment-factor contract for %s", value.TradeDate)
				break
			}
			if _, exists := seen[value.TradeDate]; exists {
				fetchErr = fmt.Errorf("provider returned duplicate adjustment factor for %s", value.TradeDate)
				break
			}
			seen[value.TradeDate] = struct{}{}
			if value.AvailableAt != nil && (value.AvailableAt.IsZero() || value.AvailableAt.After(value.FetchedAt)) {
				fetchErr = fmt.Errorf("provider returned invalid available_at for adjustment factor %s", value.TradeDate)
				break
			}
			valid = append(valid, value)
		}
		if fetchErr != nil { failures = append(failures, p.Name()+": "+fetchErr.Error()); continue }
		if err := s.store.SaveAdjustmentFactors(ctx, valid); err != nil {
			return nil, fmt.Errorf("persist %s adjustment factors: %w", p.Name(), err)
		}
		return valid, nil
	}
	if len(failures) == 0 { return nil, errors.New("no enabled adjustment-factor provider") }
	return nil, fmt.Errorf("all adjustment-factor providers failed: %s", strings.Join(failures, "; "))
}

// AdjustmentFactorsAsOf 只返回系统当时已观察且 Provider 明确提供可用时间的因子。
func (s *DataService) AdjustmentFactorsAsOf(ctx context.Context, rawSymbol, startDate, endDate string, asOf time.Time) ([]model.AdjustmentFactor, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil { return nil, err }
	if !validServiceDateRange(startDate, endDate) || asOf.IsZero() || asOf.After(time.Now().UTC()) {
		return nil, errors.New("invalid adjustment-factor as_of query")
	}
	if s == nil || s.store == nil { return nil, errors.New("MariaDB adjustment-factor storage is not enabled") }
	values, err := s.store.ListAdjustmentFactorsAsOf(ctx, symbol, startDate, endDate, asOf)
	if err != nil { return nil, err }
	out := make([]model.AdjustmentFactor, 0, len(values))
	for _, value := range values {
		if value.SystemObservedAt == nil || value.SystemObservedAt.After(asOf) || value.AvailableAt == nil || value.AvailableAt.After(asOf) {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 { return nil, ErrAdjustmentFactorsAsOfNotFound }
	return out, nil
}

// RefreshCorporateActions 采集并追加保存公司行动修订；没有公告可用时间的事件仍保留来源质量状态。
func (s *DataService) RefreshCorporateActions(ctx context.Context, rawSymbol, startDate, endDate string) ([]model.CorporateAction, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil { return nil, err }
	if !validServiceDateRange(startDate, endDate) { return nil, errors.New("invalid corporate-action date range") }
	if s == nil || s.store == nil { return nil, errors.New("MariaDB corporate-action storage is not enabled") }
	var failures []string
	for _, named := range s.providers {
		p, ok := named.(provider.CorporateActionProvider)
		if !ok { continue }
		values, fetchErr := p.FetchCorporateActions(ctx, symbol, startDate, endDate)
		if fetchErr != nil { failures = append(failures, p.Name()+": "+fetchErr.Error()); continue }
		if len(values) == 0 { failures = append(failures, p.Name()+": empty corporate-action response"); continue }
		for i := range values {
			eventSymbol, symbolErr := normalize.NormalizeSymbol(values[i].Symbol)
			if symbolErr != nil || eventSymbol != symbol ||
				(values[i].ExDate != "" && ((startDate != "" && values[i].ExDate < startDate) || (endDate != "" && values[i].ExDate > endDate))) {
				fetchErr = fmt.Errorf("provider returned corporate action outside requested symbol/date scope: %s", values[i].ProviderEventID)
				break
			}
			if values[i].Source != p.Name() || !hasIdentifiableSourceVersion(values[i].SourceVersion) {
				fetchErr = fmt.Errorf("provider returned invalid source metadata for corporate action %s", values[i].ProviderEventID)
				break
			}
			if values[i].FetchedAt.IsZero() { fetchErr = fmt.Errorf("provider returned corporate action without fetched_at"); break }
			if values[i].AvailableAt != nil && (values[i].AvailableAt.IsZero() || values[i].AvailableAt.After(values[i].FetchedAt)) {
				fetchErr = fmt.Errorf("provider returned invalid available_at for corporate action %s", values[i].ProviderEventID)
				break
			}
		}
		if fetchErr != nil { failures = append(failures, p.Name()+": "+fetchErr.Error()); continue }
		if err := s.store.SaveCorporateActions(ctx, values); err != nil {
			return nil, fmt.Errorf("persist %s corporate actions: %w", p.Name(), err)
		}
		return values, nil
	}
	if len(failures) == 0 { return nil, errors.New("no enabled corporate-action provider") }
	return nil, fmt.Errorf("all corporate-action providers failed: %s", strings.Join(failures, "; "))
}

// CorporateActionsAsOf 只返回 as_of 前已被系统观察且可证明已公开的公司行动版本。
func (s *DataService) CorporateActionsAsOf(ctx context.Context, rawSymbol, startDate, endDate string, asOf time.Time) ([]model.CorporateAction, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil { return nil, err }
	if !validServiceDateRange(startDate, endDate) || asOf.IsZero() || asOf.After(time.Now().UTC()) {
		return nil, errors.New("invalid corporate-action as_of query")
	}
	if s == nil || s.store == nil { return nil, errors.New("MariaDB corporate-action storage is not enabled") }
	values, err := s.store.ListCorporateActionsAsOf(ctx, symbol, startDate, endDate, asOf)
	if err != nil { return nil, err }
	out := make([]model.CorporateAction, 0, len(values))
	for _, value := range values {
		if value.SystemObservedAt == nil || value.SystemObservedAt.After(asOf) || value.AvailableAt == nil || value.AvailableAt.After(asOf) {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 { return nil, ErrCorporateActionsAsOfNotFound }
	return out, nil
}

func validServiceDateRange(startDate, endDate string) bool {
	if startDate != "" {
		value, err := time.Parse("2006-01-02", startDate)
		if err != nil || value.Format("2006-01-02") != startDate { return false }
	}
	if endDate != "" {
		value, err := time.Parse("2006-01-02", endDate)
		if err != nil || value.Format("2006-01-02") != endDate { return false }
	}
	return startDate == "" || endDate == "" || startDate <= endDate
}
