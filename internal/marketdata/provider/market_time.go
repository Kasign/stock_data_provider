package provider

import (
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/markettime"
)

// providerMarketLocation 返回 MarketData Provider 共用的 A 股业务时区。
func providerMarketLocation() *time.Location {
	return markettime.Location()
}
