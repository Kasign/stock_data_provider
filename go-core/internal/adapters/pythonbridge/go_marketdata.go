package pythonbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/adapters/marketdatahttp"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

// GoMarketDataAdapter 将原生 Go 行情服务作为实时行情与日 K 的唯一来源，
// 并优先使用原生指数、市场广度和板块接口聚合中国市场概况。
// 不支持的市场或原生市场概况无可用数据时保留兼容 bridge；个股行情和日 K 不回退。
type GoMarketDataAdapter struct {
	native *marketdatahttp.Client
	bridge *Client
}

func NewGoMarketDataAdapter(native *marketdatahttp.Client, bridge *Client) (*GoMarketDataAdapter, error) {
	if native == nil {
		return nil, errors.New("Go market-data client is required")
	}
	if bridge == nil {
		return nil, errors.New("capability bridge client is required for non-CN overview fallback and auxiliary capabilities")
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
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" { region = "cn" }
	if region == "cn" {
		native, nativeErr := a.native.FetchMarketOverview(ctx, region, asOf)
		if nativeErr == nil && usableNativeMarketOverview(native) { return native, nil }
		if nativeErr == nil { nativeErr = errors.New("native market overview returned no usable data") }
		if ctx != nil && ctx.Err() != nil { return domain.MarketOverview{}, ctx.Err() }
		var legacy domain.MarketOverview
		request := map[string]any{"region": region}
		if strings.TrimSpace(asOf) != "" { request["as_of"] = asOf }
		legacyErr := a.bridge.Post(ctx, "/internal/v1/market-data/market-overview", request, &legacy)
		if legacyErr == nil {
			legacy.Warnings = appendUniqueWarning(legacy.Warnings, "native_market_overview_unavailable_legacy_data_used")
			return legacy, nil
		}
		return domain.MarketOverview{}, errors.Join(fmt.Errorf("native Go market overview: %w", nativeErr), fmt.Errorf("legacy bridge market overview: %w", legacyErr))
	}
	var legacy domain.MarketOverview
	request := map[string]any{"region": region}
	if strings.TrimSpace(asOf) != "" { request["as_of"] = asOf }
	if err := a.bridge.Post(ctx, "/internal/v1/market-data/market-overview", request, &legacy); err != nil { return domain.MarketOverview{}, err }
	legacy.Warnings = appendUniqueWarning(legacy.Warnings, "market_overview_region_uses_legacy_bridge")
	return legacy, nil
}

func usableNativeMarketOverview(snapshot domain.MarketOverview) bool {
	if len(snapshot.Indices) > 0 || snapshot.UpCount > 0 || snapshot.DownCount > 0 || snapshot.FlatCount > 0 || snapshot.TotalAmount > 0 { return true }
	return len(snapshot.TopSectors) > 0 || len(snapshot.BottomSectors) > 0 || len(snapshot.TopConcepts) > 0 || len(snapshot.BottomConcepts) > 0
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
