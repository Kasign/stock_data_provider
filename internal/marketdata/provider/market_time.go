package provider

import (
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/markettime"
	platformtime "example.com/stock-analysis-flow/internal/platform/time"
)

// providerMarketLocation 返回 MarketData Provider 共用的 A 股业务时区。
func providerMarketLocation() *time.Location {
	return markettime.Location()
}

// providerNowUTC 返回 Platform 提供的 UTC 系统时刻。
func providerNowUTC() time.Time {
	return platformtime.SystemClock{}.Now()
}
