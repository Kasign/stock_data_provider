package pythonbridge

import (
	"context"
	"errors"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/adapters/marketdatahttp"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

// GoMarketDataAdapter 将原生 Go 行情服务作为实时行情与日 K 的唯一来源。
// 这两类数据不会回退到 Python bridge，避免重新引入单位、复权口径和来源追溯的不确定性。
// 市场概况和其他非行情能力在独立迁移前仍暂由 bridge 承担。
type GoMarketDataAdapter struct {
	native *marketdatahttp.Client
	bridge *Client
}

func NewGoMarketDataAdapter(native *marketdatahttp.Client, bridge *Client) (*GoMarketDataAdapter, error) {
	if native == nil {
		return nil, errors.New("Go market-data client is required")
	}
	if bridge == nil {
		return nil, errors.New("capability bridge client is required for the not-yet-migrated market overview")
	}
	return &GoMarketDataAdapter{native: native, bridge: bridge}, nil
}

func (a *GoMarketDataAdapter) FetchStockBundle(ctx context.Context, code string, lookback int, asOf string) (domain.StockBundle, error) {
	out, err := a.native.FetchStockBundle(ctx, code, lookback, asOf)
	if err != nil {
		return domain.StockBundle{}, err
	}
	// 不要把缺失的辅助数据伪装成成功返回的空结果。
	// 这些数据域需要独立的 Go Provider 契约，将在后续步骤迁移；当前接口会明确声明这些能力尚不可用。
	if out.Chip == nil {
		out.Warnings = appendUniqueWarning(out.Warnings, "chip_distribution_not_yet_migrated_to_go")
	}
	if out.Fundamentals == nil {
		out.Warnings = appendUniqueWarning(out.Warnings, "fundamentals_not_yet_migrated_to_go")
	}
	if len(out.Boards) == 0 {
		out.Warnings = appendUniqueWarning(out.Warnings, "stock_boards_not_yet_migrated_to_go")
	}
	if out.Sources == nil {
		out.Sources = map[string]string{}
	}
	out.Sources["daily_bars"] = firstNonBlank(out.Sources["daily_bars"], "go-market-data")
	if out.Quote != nil {
		out.Sources["realtime_quote"] = firstNonBlank(out.Sources["realtime_quote"], out.Quote.Source, "go-market-data")
	}
	return out, nil
}

func (a *GoMarketDataAdapter) FetchMarketOverview(ctx context.Context, region, asOf string) (domain.MarketOverview, error) {
	// 市场广度、板块、涨停/跌停等 Provider 尚未迁移到 Go。
	// 必须明确标记能力缺失，不能从单只股票行情伪造全市场统计。
	// 当前保留的 bridge 调用独立存在，不会获取个股行情或日 K。
	var out domain.MarketOverview
	request := map[string]any{"region": strings.ToLower(strings.TrimSpace(region))}
	if strings.TrimSpace(asOf) != "" {
		request["as_of"] = asOf
	}
	err := a.bridge.Post(ctx, "/internal/v1/market-data/market-overview", request, &out)
	return out, err
}

func appendUniqueWarning(in []string, value string) []string {
	for _, existing := range in {
		if existing == value {
			return in
		}
	}
	return append(in, value)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return "unknown"
}
