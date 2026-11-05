package provider

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/baostock"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/normalize"
)

type baostockAdjustmentFactorClient interface {
	QueryAdjustmentFactors(ctx context.Context, code, startDate, endDate string) ([]baostock.AdjustmentFactor, error)
}

// FetchAdjustmentFactors 将 BaoStock 的前/后复权累计因子拆成独立口径记录。
// OperateDate 按真实样本 raw/qfq/hfq 对账结果映射为因子生效日期；
// BaoStock 未提供可验证的首次公开时间，因此 AvailableAt 始终为空。
func (p *Baostock) FetchAdjustmentFactors(ctx context.Context, rawSymbol, startDate, endDate string) ([]model.AdjustmentFactor, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("BaoStock client is not configured")
	}
	client, ok := p.client.(baostockAdjustmentFactorClient)
	if !ok {
		return nil, fmt.Errorf("BaoStock client does not support adjustment-factor queries")
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	code, err := baostockSecurityCode(symbol)
	if err != nil {
		return nil, err
	}
	start, err := parseBaoStockFactorDate(startDate)
	if err != nil {
		return nil, fmt.Errorf("BaoStock adjustment-factor start date: %w", err)
	}
	end, err := parseBaoStockFactorDate(endDate)
	if err != nil {
		return nil, fmt.Errorf("BaoStock adjustment-factor end date: %w", err)
	}
	if start > end {
		return nil, fmt.Errorf("BaoStock adjustment-factor start date must not be after end date")
	}
	rows, err := client.QueryAdjustmentFactors(ctx, code, start, end)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("BaoStock returned no adjustment-factor rows for %s", symbol)
	}

	fetchedAt := time.Now().UTC()
	out := make([]model.AdjustmentFactor, 0, len(rows)*2)
	seen := make(map[string]struct{}, len(rows))
	previousDate := ""
	for index, row := range rows {
		if strings.ToLower(strings.TrimSpace(row.Code)) != code {
			return nil, fmt.Errorf("BaoStock adjustment-factor row %d symbol mismatch: requested %s, got %q", index, code, row.Code)
		}
		date, dateErr := parseBaoStockFactorDate(row.OperateDate)
		if dateErr != nil || date < start || date > end {
			return nil, fmt.Errorf("BaoStock adjustment-factor row %d has invalid/out-of-range operate date %q", index, row.OperateDate)
		}
		if date <= previousDate {
			return nil, fmt.Errorf("BaoStock adjustment-factor rows are duplicate or unsorted at %s", date)
		}
		previousDate = date
		if _, exists := seen[date]; exists {
			return nil, fmt.Errorf("BaoStock returned duplicate adjustment-factor date %s", date)
		}
		seen[date] = struct{}{}
		for _, item := range []struct {
			basis  model.AdjustmentFactorBasis
			factor float64
		}{
			{basis: model.AdjustmentFactorForward, factor: row.ForwardFactor},
			{basis: model.AdjustmentFactorBackward, factor: row.BackwardFactor},
		} {
			if item.factor <= 0 || math.IsNaN(item.factor) || math.IsInf(item.factor, 0) {
				return nil, fmt.Errorf("BaoStock %s factor is invalid for %s: %v", item.basis, date, item.factor)
			}
			out = append(out, model.AdjustmentFactor{
				Symbol: symbol, TradeDate: date, Factor: item.factor, Basis: item.basis,
				Source: p.Name(), SourceVersion: baostockVersion, FetchedAt: fetchedAt,
				QualityStatus:  model.QualityDegraded,
				QualityReasons: []string{"provider_available_at_not_exposed_by_query_adjust_factor"},
			})
		}
	}
	return out, nil
}

func parseBaoStockFactorDate(value string) (string, error) {
	parsed, err := time.Parse("2006-01-02", strings.TrimSpace(value))
	if err != nil || parsed.Format("2006-01-02") != strings.TrimSpace(value) {
		return "", fmt.Errorf("invalid YYYY-MM-DD date %q", value)
	}
	return parsed.Format("2006-01-02"), nil
}
