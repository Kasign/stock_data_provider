package bootstrap

import (
	"context"
	"log/slog"
	"time"
)

type quoteRetentionStore interface {
	PurgeQuotesBefore(context.Context, time.Time) (int64, error)
}

// runQuoteRetentionWorker 在调用 goroutine 中运行，直到 ctx 取消后退出。
func runQuoteRetentionWorker(ctx context.Context, store quoteRetentionStore, retention time.Duration, logger *slog.Logger) {
	if ctx == nil || store == nil || retention < 24*time.Hour {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	purge := func() {
		if err := ctx.Err(); err != nil {
			return
		}
		jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		cutoff := time.Now().UTC().Add(-retention)
		deleted, err := store.PurgeQuotesBefore(jobCtx, cutoff)
		if err != nil {
			logger.Error("quote snapshot retention purge failed", "cutoff", cutoff.Format(time.RFC3339Nano), "error", err)
			return
		}
		if deleted > 0 {
			logger.Info("quote snapshot retention purge completed", "deleted_rows", deleted, "retention", retention.String())
		}
	}
	purge()
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			purge()
		}
	}
}
