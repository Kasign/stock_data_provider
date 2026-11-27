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

// LimitPoolFactsSnapshotCreator 是涨跌停事实授权快照的唯一生产入口。
// 它只在交易日收盘后工作：盘中池不完整，任何「未在池中」的结论都不可信。
type LimitPoolFactsSnapshotCreator struct {
	reader     LimitPoolReader
	repository snapshotport.LimitPoolFactsSnapshotRepository
	owner      *DataAuthorizationOwner
}

func NewLimitPoolFactsSnapshotCreator(
	reader LimitPoolReader,
	repository snapshotport.LimitPoolFactsSnapshotRepository,
	owner *DataAuthorizationOwner,
) (*LimitPoolFactsSnapshotCreator, error) {
	if reader == nil || repository == nil || owner == nil {
		return nil, ErrLimitPoolFactsSnapshotCreationUnavailable
	}
	return &LimitPoolFactsSnapshotCreator{reader: reader, repository: repository, owner: owner}, nil
}

// Create 读取当日两个全量池、构建并持久化不可变快照，最后签发绑定的数据类授权。
func (c *LimitPoolFactsSnapshotCreator) Create(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (LimitPoolFactsSnapshotReferences, error) {
	if c == nil || c.reader == nil || c.repository == nil || c.owner == nil || ctx == nil {
		return LimitPoolFactsSnapshotReferences{}, ErrLimitPoolFactsSnapshotCreationUnavailable
	}
	symbol = strings.TrimSpace(symbol)
	tradeDate = strings.TrimSpace(tradeDate)
	if symbol == "" || symbol != strings.TrimSpace(symbol) {
		return LimitPoolFactsSnapshotReferences{}, errors.New("limit-pool facts snapshot symbol is required and must be canonical")
	}
	if tradeDate == "" || tradeDate != strings.TrimSpace(tradeDate) {
		return LimitPoolFactsSnapshotReferences{}, errors.New("limit-pool facts snapshot trade_date is required")
	}
	if systemAsOf.IsZero() || systemAsOf.Location() != time.UTC {
		return LimitPoolFactsSnapshotReferences{}, errors.New("limit-pool facts snapshot requires a UTC system_as_of")
	}
	if !limitPoolTradeDateClosed(tradeDate, systemAsOf) {
		return LimitPoolFactsSnapshotReferences{}, errors.New("limit-pool facts require a system_as_of at or after the trade date close")
	}
	_, callerRef, _, err := trustedDataPrincipal(ctx)
	if err != nil {
		return LimitPoolFactsSnapshotReferences{}, err
	}
	if strings.TrimSpace(callerRef) == "" {
		return LimitPoolFactsSnapshotReferences{}, ErrDataAuthorizationDenied
	}

	existing, err := c.repository.GetLatestLimitPoolFactsSnapshotAtOrBefore(ctx, symbol, tradeDate, purpose, systemAsOf)
	switch {
	case err == nil:
		if existing.Manifest.Symbol != symbol || existing.Manifest.TradeDate != tradeDate ||
			existing.Manifest.PurposeCode != purpose {
			return LimitPoolFactsSnapshotReferences{}, errors.New("reused limit-pool facts snapshot binding does not match the request")
		}
		// as-of 复用只允许更早发布的快照：发布晚于评估时点的快照在 PIT 上还没发生。
		reusedAsOf, parseErr := time.Parse(time.RFC3339Nano, existing.Manifest.SystemAsOf)
		if parseErr != nil || reusedAsOf.After(systemAsOf) {
			return LimitPoolFactsSnapshotReferences{}, errors.New("reused limit-pool facts snapshot was published after the requested system_as_of")
		}
		authorizationRef, issueErr := c.owner.IssueFor(
			ctx, existing.Manifest.SnapshotRef, purpose, authorization.LimitPoolFactsPolicyRef,
		)
		if issueErr != nil {
			return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("reused Snapshot authorization was not issued: %w", issueErr)
		}
		return LimitPoolFactsSnapshotReferences{
			SnapshotRef: existing.Manifest.SnapshotRef, AuthorizationRef: authorizationRef,
		}, nil
	case errors.Is(err, snapshotport.ErrLimitPoolFactsSnapshotNotFound):
		// 继续走真实 Provider。
	default:
		return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("lookup reusable limit-pool facts Snapshot: %w", err)
	}

	up, err := c.reader.FetchFullLimitPool(ctx, "up", tradeDate)
	if err != nil {
		return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("read full limit-up pool: %w", err)
	}
	down, err := c.reader.FetchFullLimitPool(ctx, "down", tradeDate)
	if err != nil {
		return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("read full limit-down pool: %w", err)
	}
	if up == nil || down == nil {
		return LimitPoolFactsSnapshotReferences{}, errors.New("limit-pool reader returned an empty snapshot")
	}
	if up.TradeDate != tradeDate || down.TradeDate != tradeDate {
		return LimitPoolFactsSnapshotReferences{}, errors.New("limit-pool source trade date does not match the request")
	}
	canonicalSymbol, err := normalize.NormalizeSymbol(symbol)
	if err != nil || canonicalSymbol != symbol || strings.ContainsAny(symbol, " \t\n") {
		return LimitPoolFactsSnapshotReferences{}, errors.New("limit-pool request symbol is not canonical")
	}

	payload, err := app.BuildLimitPoolFactsPayload(up, down, symbol)
	if err != nil {
		return LimitPoolFactsSnapshotReferences{}, err
	}
	if payload.ObservedAt.After(systemAsOf) {
		return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("%w: observed_at=%s system_as_of=%s",
			ErrLimitPoolFactsSnapshotStaleAsOf, payload.ObservedAt.Format(time.RFC3339Nano), systemAsOf.UTC().Format(time.RFC3339Nano))
	}
	sources := []app.DailyBarsCloseSeriesSource{{SourceRef: up.Source}}
	snapshot, err := app.BuildLimitPoolFactsSnapshot(payload, sources, callerRef, purpose, systemAsOf)
	if err != nil {
		return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("build limit-pool facts Snapshot: %w", err)
	}
	if err := c.repository.SaveLimitPoolFactsSnapshot(ctx, snapshot); err != nil {
		return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("persist limit-pool facts Snapshot: %w", err)
	}
	authorizationRef, err := c.owner.IssueFor(
		ctx, snapshot.Manifest.SnapshotRef, purpose, authorization.LimitPoolFactsPolicyRef,
	)
	if err != nil {
		return LimitPoolFactsSnapshotReferences{}, fmt.Errorf("Snapshot persisted but authorization was not issued: %w", err)
	}
	return LimitPoolFactsSnapshotReferences{
		SnapshotRef: snapshot.Manifest.SnapshotRef, AuthorizationRef: authorizationRef,
	}, nil
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
