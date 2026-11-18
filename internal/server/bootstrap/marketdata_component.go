package bootstrap

import (
	"context"
	"errors"

	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
)

// marketDataComponent 将 Data Module 生命周期纳入 Root Server 的统一启动/停止序列。
type marketDataComponent struct {
	module *marketdatabootstrap.Module
}

func (c marketDataComponent) Name() string { return "market-data-module" }

func (c marketDataComponent) Start(ctx context.Context) error {
	if c.module == nil {
		return errors.New("Market Data module is required")
	}
	return c.module.Start(ctx)
}

func (c marketDataComponent) Stop(ctx context.Context) error {
	if c.module == nil {
		return nil
	}
	return c.module.Stop(ctx)
}
