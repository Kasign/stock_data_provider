package marketdata

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL          string
	APIKey           string
	Timeout          time.Duration
	HistoryDays      int
	MaxConcurrency   int
	MaxResponseBytes int64
	IndexSymbols     []string
	MarketTimezone   string
	Adjustment       string
}

func DefaultConfig() Config {
	return Config{
		BaseURL:          "http://127.0.0.1:8091",
		Timeout:          135 * time.Second,
		HistoryDays:      200,
		MaxConcurrency:   4,
		MaxResponseBytes: 16 << 20,
		IndexSymbols:     []string{"000001.SH"},
		MarketTimezone:   "Asia/Shanghai",
		Adjustment:       "forward_adjusted",
	}
}

// ConfigFromEnv 读取原生 Go 行情 API 配置。如果 Go 接口地址或密钥缺失，配置校验会直接失败，不会回退到旧 Python API。
func ConfigFromEnv() (Config, error) {
	cfg := DefaultConfig()
	if value := strings.TrimSpace(os.Getenv("MARKET_DATA_BASE_URL")); value != "" {
		cfg.BaseURL = value
	}
	cfg.APIKey = strings.TrimSpace(os.Getenv("MARKET_DATA_API_KEY"))
	if value := strings.TrimSpace(os.Getenv("MARKET_DATA_REQUEST_TIMEOUT")); value != "" {
		d, err := time.ParseDuration(value)
		if err != nil || d <= 0 || d > 10*time.Minute {
			return Config{}, fmt.Errorf("MARKET_DATA_REQUEST_TIMEOUT must be between 1ns and 10m")
		}
		cfg.Timeout = d
	}
	if value := strings.TrimSpace(os.Getenv("MARKET_DATA_HISTORY_DAYS")); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 365 {
			return Config{}, fmt.Errorf("MARKET_DATA_HISTORY_DAYS must be between 1 and 365")
		}
		cfg.HistoryDays = n
	}
	if value := strings.TrimSpace(os.Getenv("MARKET_DATA_MAX_CONCURRENCY")); value != "" {
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 32 {
			return Config{}, fmt.Errorf("MARKET_DATA_MAX_CONCURRENCY must be between 1 and 32")
		}
		cfg.MaxConcurrency = n
	}
	if value := strings.TrimSpace(os.Getenv("MARKET_DATA_MAX_RESPONSE_BYTES")); value != "" {
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n < 1024 || n > 128<<20 {
			return Config{}, fmt.Errorf("MARKET_DATA_MAX_RESPONSE_BYTES must be between 1024 and 134217728")
		}
		cfg.MaxResponseBytes = n
	}
	if value, exists := os.LookupEnv("MARKET_DATA_INDEX_SYMBOLS"); exists {
		cfg.IndexSymbols = splitCSV(value)
	}
	if value := strings.TrimSpace(os.Getenv("MARKET_DATA_MARKET_TIMEZONE")); value != "" {
		cfg.MarketTimezone = value
	}
	if value := strings.TrimSpace(os.Getenv("MARKET_DATA_ADJUSTMENT")); value != "" {
		cfg.Adjustment = value
	}
	if _, err := time.LoadLocation(cfg.MarketTimezone); err != nil {
		return Config{}, fmt.Errorf("MARKET_DATA_MARKET_TIMEZONE must be a valid IANA timezone: %w", err)
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return Config{}, fmt.Errorf("MARKET_DATA_BASE_URL must be an absolute http/https URL without credentials, query, or fragment")
	}
	if len(cfg.APIKey) < 32 || isPlaceholderSecret(cfg.APIKey) {
		return Config{}, fmt.Errorf("MARKET_DATA_API_KEY must be a non-placeholder secret with at least 32 characters")
	}
	switch cfg.Adjustment {
	case "none", "forward_adjusted", "backward_adjusted":
	default:
		return Config{}, fmt.Errorf("MARKET_DATA_ADJUSTMENT must be one of none, forward_adjusted, backward_adjusted")
	}
	return cfg, nil
}

func isPlaceholderSecret(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(lower, "replace-me") ||
		strings.Contains(lower, "replace_with") ||
		strings.Contains(lower, "change_me") ||
		strings.Contains(lower, "your-secret") ||
		strings.Contains(lower, "example-secret")
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
