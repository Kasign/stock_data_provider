package config

import (
	"fmt"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

// NewsRetention controls bounded, Server-owned cleanup of News retention records.
type NewsRetention struct {
	Interval  time.Duration
	BatchSize int
}

func LoadNewsRetention(env platformconfig.Env) (NewsRetention, error) {
	cfg := NewsRetention{Interval: 5 * time.Minute, BatchSize: 100}
	var err error
	if cfg.Interval, err = env.Duration("NEWS_RETENTION_INTERVAL", cfg.Interval); err != nil {
		return NewsRetention{}, err
	}
	if cfg.BatchSize, err = env.Int("NEWS_RETENTION_BATCH_SIZE", cfg.BatchSize); err != nil {
		return NewsRetention{}, err
	}
	if cfg.Interval < time.Second || cfg.Interval > 24*time.Hour {
		return NewsRetention{}, fmt.Errorf("NEWS_RETENTION_INTERVAL 必须在 1s 到 24h 之间")
	}
	if cfg.BatchSize < 1 || cfg.BatchSize > 1000 {
		return NewsRetention{}, fmt.Errorf("NEWS_RETENTION_BATCH_SIZE 必须在 1 到 1000 之间")
	}
	return cfg, nil
}
