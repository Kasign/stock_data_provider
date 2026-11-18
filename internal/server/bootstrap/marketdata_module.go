package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	marketdatacache "example.com/stock-analysis-flow/internal/marketdata/cache"
	marketdataconfig "example.com/stock-analysis-flow/internal/marketdata/config"
	marketdatarepository "example.com/stock-analysis-flow/internal/marketdata/repository"
	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	"example.com/stock-analysis-flow/internal/server/resources"
)

// newMarketDataModule 创建 Data Module，并将 MariaDB、Redis 和出站能力全部绑定到 Root 资源。
func newMarketDataModule(shared *resources.Resources, cfg marketdataconfig.Config) (*marketdatabootstrap.Module, error) {
	if shared == nil || shared.DB == nil || shared.Redis == nil {
		return nil, errors.New("Root shared MariaDB and Redis resources are required by Market Data")
	}
	store, err := marketdatarepository.NewWithBorrowedDB(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("创建 Data borrowed Store 失败: %w", err)
	}
	cacheAdapter, err := marketdatacache.NewGoRedis(shared.Redis)
	if err != nil {
		return nil, fmt.Errorf("创建 Data Redis Adapter 失败: %w", err)
	}
	dependencies, err := newMarketDataDependencies(shared, cfg)
	if err != nil {
		return nil, fmt.Errorf("创建 Data 受控出站依赖失败: %w", err)
	}
	dependencies.Store = store
	dependencies.Cache = cacheAdapter
	dependencies.Logger = slog.Default()
	dependencies.RequireEgressDependencies = true

	module, err := marketdatabootstrap.New(cfg, dependencies)
	if err != nil {
		return nil, fmt.Errorf("创建 Data Module 失败: %w", err)
	}
	return module, nil
}

// marketDataReadinessChecks 将 Data 自有依赖诊断纳入 Root Readiness；不复制 Provider 诊断规则。
func marketDataReadinessChecks(module *marketdatabootstrap.Module) []platformdiagnostics.Check {
	return []platformdiagnostics.Check{{
		ID: "market-data.module.readiness", Name: "Market Data module readiness",
		Required: true, Timeout: 6 * time.Second,
		Run: func(ctx context.Context) (platformdiagnostics.Observation, error) {
			if module == nil || module.Diagnostics() == nil {
				return platformdiagnostics.Observation{}, errors.New("Market Data module diagnostics are unavailable")
			}
			if err := module.Diagnostics().Ready(ctx); err != nil {
				return platformdiagnostics.Observation{}, err
			}
			summary := module.ProviderSummary()
			return platformdiagnostics.Observation{
				Status:      platformdiagnostics.StatusHealthy,
				SafeMessage: "Market Data module dependencies are ready",
				CheckedAt:   time.Now().UTC(),
				Attributes: map[string]string{
					"providers":           fmt.Sprintf("%d", len(summary.All)),
					"quote_providers":     fmt.Sprintf("%d", len(summary.Quotes)),
					"daily_bar_providers": fmt.Sprintf("%d", len(summary.DailyBars)),
				},
			}, nil
		},
	}}
}

// parseMarketDataIndexSymbols 保留旧 Analysis 配置的默认指数和去重语义。
func parseMarketDataIndexSymbols(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{"000001.SH"}
	}
	seen := make(map[string]struct{})
	symbols := make([]string, 0)
	for _, value := range strings.Split(raw, ",") {
		symbol := strings.TrimSpace(value)
		if symbol == "" {
			continue
		}
		key := strings.ToUpper(symbol)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		symbols = append(symbols, symbol)
	}
	if len(symbols) == 0 {
		return []string{"000001.SH"}
	}
	return symbols
}
