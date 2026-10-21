package provider

import (
	"context"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// NamedProvider is the smallest capability required for provider discovery and
// diagnostics. New providers do not need to implement unrelated data methods.
type NamedProvider interface {
	Name() string
}

// QuoteProvider and DailyBarProvider are independent capability interfaces. The
// aggregate Provider remains for backward compatibility with the first two
// implementations, while the service can register quote-only/bar-only adapters.
type QuoteProvider interface {
	NamedProvider
	FetchQuote(ctx context.Context, symbol string) (*model.Quote, error)
}

type DailyBarProvider interface {
	NamedProvider
	FetchDailyBars(ctx context.Context, symbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error)
}

type Provider interface {
	QuoteProvider
	DailyBarProvider
}
