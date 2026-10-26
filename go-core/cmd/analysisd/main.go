package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/adapters/marketdatahttp"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/adapters/pythonbridge"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/adapters/newssearch"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/application"
	httptransport "github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/transport/http"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: parseLogLevel(os.Getenv("LOG_LEVEL"))}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("analysisd stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	if err := validateEnvironment(); err != nil {
		return err
	}
	addr := env("HTTP_ADDR", "127.0.0.1:8088")
	coreToken := strings.TrimSpace(os.Getenv("GO_CORE_TOKEN"))
	bridgeToken := strings.TrimSpace(os.Getenv("PYTHON_BRIDGE_TOKEN"))
	bridgeURL := env("PYTHON_BRIDGE_BASE_URL", "http://python-bridge:8090")
	if !validSecret(coreToken) {
		return errors.New("GO_CORE_TOKEN must be a non-placeholder secret with at least 32 characters")
	}
	if !validSecret(bridgeToken) {
		return errors.New("PYTHON_BRIDGE_TOKEN must be a non-placeholder secret with at least 32 characters")
	}
	client, err := pythonbridge.NewClient(pythonbridge.Config{BaseURL: bridgeURL, Token: bridgeToken, Timeout: durationEnv("PYTHON_BRIDGE_TIMEOUT", 20*time.Minute)})
	if err != nil {
		return err
	}
	nativeMarketDataURL := strings.TrimSpace(os.Getenv("MARKET_DATA_BASE_URL"))
	nativeMarketDataKey := strings.TrimSpace(os.Getenv("MARKET_DATA_API_KEY"))
	if nativeMarketDataURL == "" {
		return errors.New("MARKET_DATA_BASE_URL is required: stock quotes and daily K-lines must be served by Go; Python data fallback is disabled")
	}
	var nativeMarketData *marketdatahttp.Client
	if nativeMarketDataURL != "" {
		if !validSecret(nativeMarketDataKey) {
			return errors.New("MARKET_DATA_API_KEY must be a non-placeholder secret with at least 32 characters")
		}
		nativeMarketData, err = marketdatahttp.NewClient(nativeMarketDataURL, nativeMarketDataKey, durationEnv("MARKET_DATA_REQUEST_TIMEOUT", 135*time.Second))
		if err != nil {
			return err
		}
	}
	marketData, err := pythonbridge.NewGoMarketDataAdapter(nativeMarketData, client)
	if err != nil {
		return err
	}
	legacySearch := pythonbridge.NewSearchAdapter(client)
	search := newssearch.NewFromEnv(legacySearch, logger)
	analyzer := pythonbridge.NewAnalyzerAdapter(client)
	persistence := pythonbridge.NewPersistenceAdapter(client)
	notifier := pythonbridge.NewNotificationAdapter(client)
	stockFlow := application.NewStockFlow(marketData, search, analyzer, persistence, notifier)
	marketFlow := application.NewMarketReviewFlow(marketData, search, analyzer, persistence, notifier)
	server := httptransport.NewServer(httptransport.Config{
		Token:          coreToken,
		RequestTimeout: durationEnv("GO_CORE_REQUEST_TIMEOUT", 20*time.Minute),
		ReadyCheck: func(checkCtx context.Context) error {
			if err := client.Health(checkCtx); err != nil {
				return err
			}
			if err := nativeMarketData.Health(checkCtx); err != nil {
				return err
			}
			return nil
		},
	}, stockFlow, marketFlow, logger)
	httpServer := &http.Server{Addr: addr, Handler: server.Handler(), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	logger.Info("Go analysis core starting", "addr", addr, "python_bridge", bridgeURL, "market_data", nativeMarketDataURL, "market_data_required", true, "native_news_providers", strings.Join(search.ProviderNames(), ","))
	serveErr := make(chan error, 1)
	go func() { serveErr <- httpServer.ListenAndServe() }()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			_ = httpServer.Close()
			return fmt.Errorf("graceful HTTP shutdown: %w", err)
		}
		err := <-serveErr
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
func durationEnv(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}
func boolEnv(key string, fallback bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return parsed
}
func parseLogLevel(value string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func validSecret(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 32 {
		return false
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"replace-with", "change-me", "changeme", "your-secret", "example-secret"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

func validateEnvironment() error {
	for _, key := range []string{"PYTHON_BRIDGE_TIMEOUT", "GO_CORE_REQUEST_TIMEOUT", "MARKET_DATA_REQUEST_TIMEOUT"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			d, err := time.ParseDuration(value)
			if err != nil || d <= 0 {
				return fmt.Errorf("%s must be a positive Go duration", key)
			}
		}
	}
	if value := strings.TrimSpace(os.Getenv("MARKET_DATA_REQUIRED")); value != "" {
		if _, err := strconv.ParseBool(value); err != nil {
			return errors.New("MARKET_DATA_REQUIRED must be a boolean")
		}
	}
	return nil
}
