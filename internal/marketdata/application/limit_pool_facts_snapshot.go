package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/marketdata/markettime"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
)

var ErrLimitPoolFactsSnapshotCreationUnavailable = errors.New("limit-pool facts snapshot creator unavailable")

// ErrLimitPoolFactsSnapshotStaleAsOf 表示采集时刻晚于请求的 system_as_of：
// 该观测在评估时点还不存在，不能用来回答那个时点的问题（PIT 方向）。
var ErrLimitPoolFactsSnapshotStaleAsOf = errors.New("limit-pool observation is later than the requested system_as_of")

// LimitPoolReader 是 Data 模块内的全量涨跌停池来源窄端口。
// 必须返回完整池（Total == len(Items)）；部分池不得被当作「未涨跌停」的证据。
type LimitPoolReader interface {
	FetchFullLimitPool(ctx context.Context, poolType, tradeDate string) (*model.LimitPoolSnapshot, error)
}

type LimitPoolFactsSnapshotReferences struct {
	SnapshotRef      string
	AuthorizationRef string
}

// LimitPoolFactsSnapshotCollector 只负责产出并持久化涨跌停事实快照，**不签发授权**。
//
// 它供采集工具/调度在 Run 之前预置事实使用：授权仍由读取方在需要时签发，
// 因此采集器不需要 Owner 级凭据，权限面最小。
type LimitPoolFactsSnapshotCollector struct {
	reader     LimitPoolReader
	repository snapshotport.LimitPoolFactsSnapshotRepository
}

func NewLimitPoolFactsSnapshotCollector(
	reader LimitPoolReader,
	repository snapshotport.LimitPoolFactsSnapshotRepository,
) (*LimitPoolFactsSnapshotCollector, error) {
	if reader == nil || repository == nil {
		return nil, ErrLimitPoolFactsSnapshotCreationUnavailable
	}
	return &LimitPoolFactsSnapshotCollector{reader: reader, repository: repository}, nil
}

// Persist 幂等地产出快照并返回其引用：已有「不晚于 system_as_of」的快照时直接复用。
// 它只做事实固化，不产生任何授权决策。
//
// systemAsOf 为零值时表示「以观测时刻作为快照时点」。这是采集工具在 Run 之前预置事实
// 的正确语义：事实可用的最早时点就是它被观测到的时刻；调用方若先取「现在」再采集，
// 观测必然晚于该时点，PIT 会（正确地）拒绝。
func (c *LimitPoolFactsSnapshotCollector) Persist(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (string, error) {
	if c == nil || c.reader == nil || c.repository == nil || ctx == nil {
		return "", ErrLimitPoolFactsSnapshotCreationUnavailable
	}
	symbol = strings.TrimSpace(symbol)
	tradeDate = strings.TrimSpace(tradeDate)
	if symbol == "" || symbol != strings.TrimSpace(symbol) {
		return "", errors.New("limit-pool facts snapshot symbol is required and must be canonical")
	}
	if tradeDate == "" || tradeDate != strings.TrimSpace(tradeDate) {
		return "", errors.New("limit-pool facts snapshot trade_date is required")
	}
	asOfFromObservation := systemAsOf.IsZero()
	if !asOfFromObservation && systemAsOf.Location() != time.UTC {
		return "", errors.New("limit-pool facts snapshot requires a UTC system_as_of")
	}
	if !asOfFromObservation && !limitPoolTradeDateClosed(tradeDate, systemAsOf) {
		return "", errors.New("limit-pool facts require a system_as_of at or after the trade date close")
	}
	reuseAsOf := systemAsOf
	if asOfFromObservation {
		// 还没有确定的时点：用「现在」作为复用上界，找到的旧快照一定早于即将产生的观测。
		reuseAsOf = time.Now().UTC()
	}
	if strings.TrimSpace(purpose) == "" {
		return "", errors.New("limit-pool facts snapshot purpose is required")
	}
	_, callerRef, _, err := trustedDataPrincipal(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(callerRef) == "" {
		return "", ErrDataAuthorizationDenied
	}

	existing, err := c.repository.GetLatestLimitPoolFactsSnapshotAtOrBefore(ctx, symbol, tradeDate, purpose, reuseAsOf)
	switch {
	case err == nil:
		if existing.Manifest.Symbol != symbol || existing.Manifest.TradeDate != tradeDate ||
			existing.Manifest.PurposeCode != purpose {
			return "", errors.New("reused limit-pool facts snapshot binding does not match the request")
		}
		// as-of 复用只允许更早发布的快照：发布晚于评估时点的快照在 PIT 上还没发生。
		reusedAsOf, parseErr := time.Parse(time.RFC3339Nano, existing.Manifest.SystemAsOf)
		if parseErr != nil || reusedAsOf.After(reuseAsOf) {
			return "", errors.New("reused limit-pool facts snapshot was published after the requested system_as_of")
		}
		return existing.Manifest.SnapshotRef, nil
	case errors.Is(err, snapshotport.ErrLimitPoolFactsSnapshotNotFound):
		// 继续走真实 Provider。
	default:
		return "", fmt.Errorf("lookup reusable limit-pool facts Snapshot: %w", err)
	}

	up, err := c.reader.FetchFullLimitPool(ctx, "up", tradeDate)
	if err != nil {
		return "", fmt.Errorf("read full limit-up pool: %w", err)
	}
	down, err := c.reader.FetchFullLimitPool(ctx, "down", tradeDate)
	if err != nil {
		return "", fmt.Errorf("read full limit-down pool: %w", err)
	}
	if up == nil || down == nil {
		return "", errors.New("limit-pool reader returned an empty snapshot")
	}
	if up.TradeDate != tradeDate || down.TradeDate != tradeDate {
		return "", errors.New("limit-pool source trade date does not match the request")
	}
	canonicalSymbol, err := normalize.NormalizeSymbol(symbol)
	if err != nil || canonicalSymbol != symbol || strings.ContainsAny(symbol, " \t\n") {
		return "", errors.New("limit-pool request symbol is not canonical")
	}

	payload, err := app.BuildLimitPoolFactsPayload(up, down, symbol)
	if err != nil {
		return "", err
	}
	snapshotAsOf := systemAsOf
	if asOfFromObservation {
		snapshotAsOf = payload.ObservedAt.UTC()
	}
	if !limitPoolTradeDateClosed(tradeDate, snapshotAsOf) {
		return "", errors.New("limit-pool facts require a snapshot instant at or after the trade date close")
	}
	if payload.ObservedAt.After(snapshotAsOf) {
		return "", fmt.Errorf("%w: observed_at=%s system_as_of=%s",
			ErrLimitPoolFactsSnapshotStaleAsOf, payload.ObservedAt.Format(time.RFC3339Nano), snapshotAsOf.Format(time.RFC3339Nano))
	}
	sources := []app.DailyBarsCloseSeriesSource{{SourceRef: up.Source}}
	snapshot, err := app.BuildLimitPoolFactsSnapshot(payload, sources, callerRef, purpose, snapshotAsOf)
	if err != nil {
		return "", fmt.Errorf("build limit-pool facts Snapshot: %w", err)
	}
	if err := c.repository.SaveLimitPoolFactsSnapshot(ctx, snapshot); err != nil {
		return "", fmt.Errorf("persist limit-pool facts Snapshot: %w", err)
	}
	return snapshot.Manifest.SnapshotRef, nil
}

// LimitPoolFactsSnapshotCreator 是涨跌停事实授权快照的唯一生产入口。
// 它在 Collector 之上追加授权签发，供 Risk Stage 在读取时使用。
type LimitPoolFactsSnapshotCreator struct {
	collector *LimitPoolFactsSnapshotCollector
	owner     *DataAuthorizationOwner
}

func NewLimitPoolFactsSnapshotCreator(
	reader LimitPoolReader,
	repository snapshotport.LimitPoolFactsSnapshotRepository,
	owner *DataAuthorizationOwner,
) (*LimitPoolFactsSnapshotCreator, error) {
	if reader == nil || repository == nil || owner == nil {
		return nil, ErrLimitPoolFactsSnapshotCreationUnavailable
	}
	collector, err := NewLimitPoolFactsSnapshotCollector(reader, repository)
	if err != nil {
		return nil, err
	}
	return &LimitPoolFactsSnapshotCreator{collector: collector, owner: owner}, nil
}

// Create 读取当日两个全量池、构建并持久化不可变快照，最后签发绑定的数据类授权。
func (c *LimitPoolFactsSnapshotCreator) Create(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (LimitPoolFactsSnapshotReferences, error) {
	if c == nil || c.collector == nil || c.owner == nil || ctx == nil {
		return LimitPoolFactsSnapshotReferences{}, ErrLimitPoolFactsSnapshotCreationUnavailable
	}
	snapshotRef, err := c.collector.Persist(ctx, symbol, tradeDate, systemAsOf, purpose)
	if err != nil {
		return LimitPoolFactsSnapshotReferences{}, err
	}
	authorizationRef, err := c.owner.IssueFor(
		ctx, snapshotRef, purpose, authorization.LimitPoolFactsPolicyRef,
	)
	if err != nil {
		return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("limit-pool facts Snapshot exists but authorization was not issued: %w", err)
	}
	return LimitPoolFactsSnapshotReferences{SnapshotRef: snapshotRef, AuthorizationRef: authorizationRef}, nil
}

// limitPoolTradeDateClosed 要求 system_as_of 不早于交易日 15:00 市场本地时间。
// 收盘前的池子不完整，不能用来断言「未涨跌停」。
func limitPoolTradeDateClosed(tradeDate string, systemAsOf time.Time) bool {
	location := markettime.Location()
	day, err := time.ParseInLocation("2006-01-02", tradeDate, location)
	if err != nil {
		return false
	}
	closeAt := time.Date(day.Year(), day.Month(), day.Day(), 15, 0, 0, 0, location)
	return !systemAsOf.Before(closeAt)
}
