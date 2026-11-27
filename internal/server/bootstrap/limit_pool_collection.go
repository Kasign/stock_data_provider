package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

// limitPoolCollectorPort 是采集组件依赖的最小端口：只落库，不签发授权。
type limitPoolCollectorPort interface {
	PersistLimitPoolFactsSnapshot(
		ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
	) (string, error)
}

// limitPoolCollectionConfig 是涨跌停事实预采集的调度配置。
//
// 默认关闭：证券池是部署事实，不由代码猜。Symbols 为空即视为未启用。
type limitPoolCollectionConfig struct {
	Enabled     bool
	Symbols     []string
	Purpose     string
	AfterLocal  time.Duration // 市场本地时间「收盘后可采」的门槛，默认 15:30
	Interval    time.Duration // 检查间隔
	MaxAttempts int           // 同一交易日最多尝试次数
}

const (
	limitPoolCollectionDefaultAfter      = 15*time.Hour + 30*time.Minute
	limitPoolCollectionDefaultInterval   = 5 * time.Minute
	limitPoolCollectionDefaultMaxAttempt = 3
	limitPoolCollectionDefaultPurpose    = "risk_review"
	limitPoolCollectorSubject            = "limit-pool-collect"
)

// loadLimitPoolCollectionConfig 从环境变量读取配置。符号列表为空则整体关闭，
// 且不报错——这是「未配置」而不是「配置错误」。
func loadLimitPoolCollectionConfig() (limitPoolCollectionConfig, error) {
	config := limitPoolCollectionConfig{
		Purpose:     limitPoolCollectionDefaultPurpose,
		AfterLocal:  limitPoolCollectionDefaultAfter,
		Interval:    limitPoolCollectionDefaultInterval,
		MaxAttempts: limitPoolCollectionDefaultMaxAttempt,
	}
	for _, raw := range strings.Split(os.Getenv("SERVER_LIMIT_POOL_COLLECT_SYMBOLS"), ",") {
		if symbol := strings.TrimSpace(raw); symbol != "" {
			config.Symbols = append(config.Symbols, symbol)
		}
	}
	if len(config.Symbols) == 0 {
		return config, nil
	}
	config.Enabled = true

	if raw := strings.TrimSpace(os.Getenv("SERVER_LIMIT_POOL_COLLECT_PURPOSE")); raw != "" {
		config.Purpose = raw
	}
	if raw := strings.TrimSpace(os.Getenv("SERVER_LIMIT_POOL_COLLECT_AFTER")); raw != "" {
		parsed, err := parseClockDuration(raw)
		if err != nil {
			return config, fmt.Errorf("SERVER_LIMIT_POOL_COLLECT_AFTER: %w", err)
		}
		config.AfterLocal = parsed
	}
	if raw := strings.TrimSpace(os.Getenv("SERVER_LIMIT_POOL_COLLECT_INTERVAL")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed < time.Minute {
			return config, fmt.Errorf("SERVER_LIMIT_POOL_COLLECT_INTERVAL must be a Go duration of at least 1m, got %q", raw)
		}
		config.Interval = parsed
	}
	if raw := strings.TrimSpace(os.Getenv("SERVER_LIMIT_POOL_COLLECT_MAX_ATTEMPTS")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 20 {
			return config, fmt.Errorf("SERVER_LIMIT_POOL_COLLECT_MAX_ATTEMPTS must be an integer in [1,20], got %q", raw)
		}
		config.MaxAttempts = parsed
	}
	return config, nil
}

// parseClockDuration 解析 HH:MM 形式的市场本地时刻。
func parseClockDuration(value string) (time.Duration, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("expected HH:MM, got %q", value)
	}
	hour, hourErr := strconv.Atoi(parts[0])
	minute, minuteErr := strconv.Atoi(parts[1])
	if hourErr != nil || minuteErr != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("expected HH:MM within 00:00-23:59, got %q", value)
	}
	return time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute, nil
}

// limitPoolCollectionTarget 返回「最近一个已收盘交易日」。
//
// 收盘后是今天，收盘前是上一个工作日；周末回退到周五。它不判断法定节假日——
// 节假日采集会失败并按 MaxAttempts 收敛，不会无限重试。
//
// 之所以总是返回「已收盘」的日期，而不是用「今天是否过了 15:30」做开关：进程可能在
// 任意时刻启动，收敛到最近已收盘交易日才能补上错过的采集，且重复采集是幂等的。
func limitPoolCollectionTarget(now time.Time, afterLocal time.Duration) string {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	local := now.In(location)
	midnight := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location)
	closedToday := local.Weekday() != time.Saturday && local.Weekday() != time.Sunday &&
		local.Sub(midnight) >= afterLocal

	candidate := local
	if !closedToday {
		candidate = candidate.AddDate(0, 0, -1)
	}
	for offset := 0; offset < 14; offset++ {
		day := candidate.AddDate(0, 0, -offset)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		return day.Format("2006-01-02")
	}
	return ""
}

// limitPoolCollectionComponent 在收盘后为配置的证券预置当日涨跌停事实。
//
// 它刻意不做持久化去重：失败或重启后重复采集是安全的，因为 Persist 幂等（as-of 复用）。
// 进程内只记「本交易日已成功」，避免无意义重复调用。
type limitPoolCollectionComponent struct {
	collector limitPoolCollectorPort
	config    limitPoolCollectionConfig
	logger    *slog.Logger
	now       func() time.Time
	principal platformsecurity.Principal

	mu            sync.Mutex
	lastSucceeded string
	attempts      int

	cancel context.CancelFunc
	done   chan struct{}
}

func newLimitPoolCollectionComponent(
	collector limitPoolCollectorPort, config limitPoolCollectionConfig, logger *slog.Logger,
) (*limitPoolCollectionComponent, error) {
	if collector == nil {
		return nil, errors.New("limit-pool collection requires a facts collector")
	}
	if !config.Enabled || len(config.Symbols) == 0 {
		return nil, errors.New("limit-pool collection is not enabled")
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &limitPoolCollectionComponent{
		collector: collector, config: config, logger: logger,
		now: func() time.Time { return time.Now().UTC() },
		principal: platformsecurity.Principal{
			Kind:       platformsecurity.IdentityService,
			AuthMethod: platformsecurity.AuthMethodServiceBearer,
			Subject:    limitPoolCollectorSubject,
			Scopes:     []string{platformsecurity.ScopeDataRead},
		},
		done: make(chan struct{}),
	}, nil
}

func (c *limitPoolCollectionComponent) Name() string { return "limit-pool-collection-worker" }

func (c *limitPoolCollectionComponent) Start(ctx context.Context) error {
	if c == nil || ctx == nil {
		return errors.New("limit-pool collection start context is required")
	}
	if c.cancel != nil {
		return errors.New("limit-pool collection has already started")
	}
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.run(runCtx)
	return nil
}

func (c *limitPoolCollectionComponent) Stop(ctx context.Context) error {
	if c == nil || ctx == nil {
		return errors.New("limit-pool collection stop context is required")
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

func (c *limitPoolCollectionComponent) run(ctx context.Context) {
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

func (c *limitPoolCollectionComponent) runOnce(ctx context.Context) {
	tradeDate := limitPoolCollectionTarget(c.now(), c.config.AfterLocal)
	if tradeDate == "" {
		return
	}

	c.mu.Lock()
	if c.lastSucceeded == tradeDate {
		c.mu.Unlock()
		return
	}
	if c.attempts >= c.config.MaxAttempts {
		c.mu.Unlock()
		return
	}
	c.attempts++
	attempt := c.attempts
	c.mu.Unlock()

	principalCtx, err := platformsecurity.WithPrincipal(ctx, c.principal)
	if err != nil {
		c.logger.Warn("涨跌停事实采集：构造采集身份失败", "error", err.Error())
		return
	}
	failed := 0
	for _, symbol := range c.config.Symbols {
		if _, persistErr := c.collector.PersistLimitPoolFactsSnapshot(
			principalCtx, symbol, tradeDate, time.Time{}, c.config.Purpose,
		); persistErr != nil {
			failed++
			c.logger.Warn("涨跌停事实采集失败",
				"symbol", symbol, "trade_date", tradeDate, "attempt", attempt, "error", persistErr.Error())
		}
	}
	if failed > 0 {
		return
	}
	c.mu.Lock()
	c.lastSucceeded = tradeDate
	c.attempts = 0
	c.mu.Unlock()
	c.logger.Info("涨跌停事实采集完成",
		"trade_date", tradeDate, "symbols", len(c.config.Symbols), "attempt", attempt)
}
