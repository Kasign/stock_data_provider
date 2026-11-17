package pythonapi

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config controls access to the existing daily_stock_analysis FastAPI service.
// BaseURL is the service root, for example http://127.0.0.1:8000.
type Config struct {
	BaseURL          string
	Timeout          time.Duration
	HistoryDays      int
	MaxConcurrency   int
	MaxResponseBytes int64
	BearerToken      string
	Cookie           string
	IndexSymbols     []string
	MarketTimezone   string
}

func DefaultConfig() Config {
	return Config{
		BaseURL:          "http://127.0.0.1:8000",
		Timeout:          20 * time.Second,
		HistoryDays:      200,
		MaxConcurrency:   4,
		MaxResponseBytes: 8 << 20,
		IndexSymbols:     []string{"sh000001"},
		MarketTimezone:   "Asia/Shanghai",
	}
}

// ConfigFromEnv reads configuration after .env has been loaded by the process.
// Existing process environment variables should take precedence over .env.
func ConfigFromEnv() (Config, error) {
	cfg := DefaultConfig()
	if v, ok := os.LookupEnv("PYTHON_API_BASE_URL"); ok && strings.TrimSpace(v) != "" {
		cfg.BaseURL = strings.TrimSpace(v)
	}
	if v, ok := os.LookupEnv("PYTHON_API_TIMEOUT"); ok && strings.TrimSpace(v) != "" {
		d, err := time.ParseDuration(strings.TrimSpace(v))
		if err != nil || d <= 0 {
			return Config{}, fmt.Errorf("PYTHON_API_TIMEOUT must be a positive duration such as 20s")
		}
		cfg.Timeout = d
	}
	if v, ok := os.LookupEnv("PYTHON_API_HISTORY_DAYS"); ok && strings.TrimSpace(v) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 || n > 365 {
			return Config{}, fmt.Errorf("PYTHON_API_HISTORY_DAYS must be between 1 and 365")
		}
		cfg.HistoryDays = n
	}
	if v, ok := os.LookupEnv("PYTHON_API_MAX_CONCURRENCY"); ok && strings.TrimSpace(v) != "" {
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil || n < 1 || n > 32 {
			return Config{}, fmt.Errorf("PYTHON_API_MAX_CONCURRENCY must be between 1 and 32")
		}
		cfg.MaxConcurrency = n
	}
	if v, ok := os.LookupEnv("PYTHON_API_MAX_RESPONSE_BYTES"); ok && strings.TrimSpace(v) != "" {
		n, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err != nil || n < 1024 || n > 128<<20 {
			return Config{}, fmt.Errorf("PYTHON_API_MAX_RESPONSE_BYTES must be between 1024 and 134217728")
		}
		cfg.MaxResponseBytes = n
	}
	cfg.BearerToken = strings.TrimSpace(os.Getenv("PYTHON_API_BEARER_TOKEN"))
	cfg.Cookie = strings.TrimSpace(os.Getenv("PYTHON_API_COOKIE"))
	if raw, ok := os.LookupEnv("PYTHON_API_INDEX_SYMBOLS"); ok {
		cfg.IndexSymbols = splitCSV(raw)
	}
	if v, ok := os.LookupEnv("PYTHON_API_MARKET_TIMEZONE"); ok && strings.TrimSpace(v) != "" {
		cfg.MarketTimezone = strings.TrimSpace(v)
	}
	if _, err := time.LoadLocation(cfg.MarketTimezone); err != nil {
		return Config{}, fmt.Errorf("PYTHON_API_MARKET_TIMEZONE must be a valid IANA timezone: %w", err)
	}
	return cfg, nil
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		item := strings.TrimSpace(part)
		if item == "" {
			continue
		}
		key := strings.ToUpper(item)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}
