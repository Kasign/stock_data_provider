package bootstrap

import (
	"context"
	"errors"
	"log/slog"
	"time"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	platformtime "example.com/stock-analysis-flow/internal/platform/time"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
)

type newsRetentionComponent struct {
	observationCleaner *newsapp.ObservationRetentionCleaner
	snapshotCleaner    *newsapp.EvidenceSnapshotRetentionCleaner
	config             serverconfig.NewsRetention
	clock              platformtime.Clock
	logger             *slog.Logger
	cancel             context.CancelFunc
	done               chan struct{}
}

func newNewsRetentionComponent(
	repository interface {
		newsapp.ObservationRetentionRepository
		newsapp.EvidenceSnapshotRetentionRepository
	},
	cfg serverconfig.NewsRetention,
	logger *slog.Logger,
) (*newsRetentionComponent, error) {
	if repository == nil {
		return nil, errors.New("News retention repository is required")
	}
	observationCleaner, err := newsapp.NewObservationRetentionCleaner(repository)
	if err != nil {
		return nil, err
	}
	snapshotCleaner, err := newsapp.NewEvidenceSnapshotRetentionCleaner(repository)
	if err != nil {
		return nil, err
	}
	if cfg.Interval < time.Second || cfg.BatchSize < 1 || cfg.BatchSize > 1000 {
		return nil, errors.New("News retention configuration is invalid")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &newsRetentionComponent{
		observationCleaner: observationCleaner,
		snapshotCleaner:    snapshotCleaner,
		config:             cfg,
		clock:              platformtime.SystemClock{},
		logger:             logger,
		done:               make(chan struct{}),
	}, nil
}

func (c *newsRetentionComponent) Name() string { return "news-retention-cleaner" }

func (c *newsRetentionComponent) Start(ctx context.Context) error {
	if c == nil || ctx == nil || c.observationCleaner == nil || c.snapshotCleaner == nil {
		return errors.New("News retention component is not configured")
	}
	if c.cancel != nil {
		return errors.New("News retention component has already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.run(runCtx)
	return nil
}

func (c *newsRetentionComponent) Stop(ctx context.Context) error {
	if c == nil || ctx == nil {
		return errors.New("News retention stop context is required")
	}
	if c.cancel == nil {
		return nil
	}
	c.cancel()
	select {
	case <-c.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *newsRetentionComponent) run(ctx context.Context) {
	defer close(c.done)
	c.runOnce(ctx)
	ticker := time.NewTicker(c.config.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.runOnce(ctx)
		}
	}
}

func (c *newsRetentionComponent) runOnce(ctx context.Context) {
	now := c.clock.Now().UTC()
	observation, observationErr := c.observationCleaner.Execute(ctx, now, c.config.BatchSize)
	if observationErr != nil && !errors.Is(observationErr, context.Canceled) {
		c.logger.Warn("News Observation 保留期清理失败", "error", observationErr.Error())
	}
	snapshots, snapshotErr := c.snapshotCleaner.Execute(ctx, now, c.config.BatchSize)
	if snapshotErr != nil && !errors.Is(snapshotErr, context.Canceled) {
		c.logger.Warn("News Evidence Snapshot 清理失败", "error", snapshotErr.Error())
	}
	if observationErr == nil && snapshotErr == nil &&
		(observation.Purged > 0 || observation.EvidencePurged > 0 || snapshots > 0) {
		c.logger.Info("News 保留期清理完成",
			"observation_payloads", observation.Purged,
			"availability_evidence_payloads", observation.EvidencePurged,
			"evidence_snapshots", snapshots,
		)
	}
}
