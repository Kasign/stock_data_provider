package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
)

var (
	ErrOrderBookFactsSnapshotCreationUnavailable = errors.New("order-book facts snapshot creator unavailable")

	// ErrOrderBookFactsSnapshotStaleAsOf 表示观测时刻晚于请求的 system_as_of：
	// 该盘口在评估时点还不存在，不能用来回答那个时点的问题（PIT 方向）。
	ErrOrderBookFactsSnapshotStaleAsOf = errors.New("order-book observation is later than the requested system_as_of")
)

// OrderBookReader 是 Data 模块内的真实盘口来源窄端口。
// 它只返回带来源时间的事实，不接受调用方传入的盘口数值。
type OrderBookReader interface {
	FetchOrderBook(ctx context.Context, symbol string) (*model.OrderBook, error)
}

type OrderBookFactsSnapshotReferences struct {
	SnapshotRef      string
	AuthorizationRef string
}

// OrderBookFactsSnapshotCollector 只负责产出并持久化日内盘口事实快照，**不签发授权**。
//
// 它供 Run 之前的采集工具预置事实使用：授权仍由读取方在需要时签发，
// 因此采集器不需要 Owner 级凭据，权限面最小。
type OrderBookFactsSnapshotCollector struct {
	reader     OrderBookReader
	repository snapshotport.OrderBookFactsSnapshotRepository
}

func NewOrderBookFactsSnapshotCollector(
	reader OrderBookReader,
	repository snapshotport.OrderBookFactsSnapshotRepository,
) (*OrderBookFactsSnapshotCollector, error) {
	if reader == nil || repository == nil {
		return nil, ErrOrderBookFactsSnapshotCreationUnavailable
	}
	return &OrderBookFactsSnapshotCollector{reader: reader, repository: repository}, nil
}

// Persist 幂等地产出快照并返回其引用：已有「不晚于 system_as_of」的快照时直接复用。
// 它只做事实固化，不产生任何授权决策。
//
// systemAsOf 为零值时表示「以实际系统观测时刻作为快照时点」。这是采集工具在 Run
// 之前预置事实的正确语义；调用方若先取「现在」再采集，观测必然晚于该时点，PIT 会
// （正确地）拒绝。
func (c *OrderBookFactsSnapshotCollector) Persist(
	ctx context.Context, symbol string, systemAsOf time.Time, purpose string,
) (string, error) {
	if c == nil || c.reader == nil || c.repository == nil || ctx == nil {
		return "", ErrOrderBookFactsSnapshotCreationUnavailable
	}
	symbol = strings.TrimSpace(symbol)
	if symbol == "" || symbol != strings.TrimSpace(symbol) {
		return "", errors.New("order-book facts snapshot symbol is required and must be canonical")
	}
	asOfFromObservation := systemAsOf.IsZero()
	if !asOfFromObservation && systemAsOf.Location() != time.UTC {
		return "", errors.New("order-book facts snapshot requires a UTC system_as_of")
	}
	if strings.TrimSpace(purpose) == "" {
		return "", errors.New("order-book facts snapshot purpose is required")
	}
	reuseAsOf := systemAsOf
	if asOfFromObservation {
		// 还没有确定的时点：用「现在」作为复用上界，找到的旧快照一定早于即将产生的观测。
		reuseAsOf = time.Now().UTC()
	}
	_, callerRef, _, err := trustedDataPrincipal(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(callerRef) == "" {
		return "", ErrDataAuthorizationDenied
	}

	// 优先复用「不晚于评估时点」的最新盘口事实；旧事实能否使用由 Quant 绑定层
	// 按新鲜度裁决，这里只保证 PIT 上界。没有可复用快照才读取实时 Provider。
	existing, err := c.repository.GetLatestOrderBookFactsSnapshotAtOrBefore(ctx, symbol, purpose, reuseAsOf)
	switch {
	case err == nil:
		if existing.Manifest.Symbol != symbol || existing.Manifest.PurposeCode != purpose {
			return "", errors.New("reused order-book facts snapshot binding does not match the request")
		}
		reusedAsOf, parseErr := time.Parse(time.RFC3339Nano, existing.Manifest.SystemAsOf)
		if parseErr != nil || reusedAsOf.After(reuseAsOf) {
			return "", errors.New("reused order-book facts snapshot was published after the requested system_as_of")
		}
		return existing.Manifest.SnapshotRef, nil
	case errors.Is(err, snapshotport.ErrOrderBookFactsSnapshotNotFound):
		// 继续走真实 Provider。
	default:
		return "", fmt.Errorf("lookup reusable order-book facts Snapshot: %w", err)
	}

	book, err := c.reader.FetchOrderBook(ctx, symbol)
	if err != nil {
		return "", fmt.Errorf("read live order book for Snapshot: %w", err)
	}
	if book == nil {
		return "", errors.New("order-book reader returned an empty snapshot")
	}
	canonicalSymbol, err := normalize.NormalizeSymbol(book.Symbol)
	if err != nil || canonicalSymbol != symbol {
		return "", errors.New("order-book source identity does not match the request")
	}
	snapshotAsOf := systemAsOf
	if asOfFromObservation {
		if book.SystemObservedAt.IsZero() {
			return "", errors.New("order-book source system observation time is missing")
		}
		snapshotAsOf = book.SystemObservedAt.UTC()
	}
	if book.SystemObservedAt.After(snapshotAsOf) || book.ObservedAt.After(snapshotAsOf) {
		return "", fmt.Errorf("%w: observed_at=%s system_observed_at=%s system_as_of=%s",
			ErrOrderBookFactsSnapshotStaleAsOf,
			book.ObservedAt.Format(time.RFC3339Nano),
			book.SystemObservedAt.Format(time.RFC3339Nano),
			snapshotAsOf.Format(time.RFC3339Nano))
	}
	payload, err := app.BuildOrderBookFactsPayload(*book)
	if err != nil {
		return "", err
	}
	sources := []app.DailyBarsCloseSeriesSource{{SourceRef: book.Source}}
	snapshot, err := app.BuildOrderBookFactsSnapshot(payload, sources, callerRef, purpose, snapshotAsOf)
	if err != nil {
		return "", fmt.Errorf("build order-book facts Snapshot: %w", err)
	}
	if err := c.repository.SaveOrderBookFactsSnapshot(ctx, snapshot); err != nil {
		return "", fmt.Errorf("persist order-book facts Snapshot: %w", err)
	}
	return snapshot.Manifest.SnapshotRef, nil
}

// OrderBookFactsSnapshotCreator 是日内盘口授权快照的唯一生产入口。
// 它在 Collector 之上追加授权签发；不返回 Payload，下游必须通过授权读取入口获得事实。
type OrderBookFactsSnapshotCreator struct {
	collector *OrderBookFactsSnapshotCollector
	owner     *DataAuthorizationOwner
}

func NewOrderBookFactsSnapshotCreator(
	reader OrderBookReader,
	repository snapshotport.OrderBookFactsSnapshotRepository,
	owner *DataAuthorizationOwner,
) (*OrderBookFactsSnapshotCreator, error) {
	if owner == nil {
		return nil, ErrOrderBookFactsSnapshotCreationUnavailable
	}
	collector, err := NewOrderBookFactsSnapshotCollector(reader, repository)
	if err != nil {
		return nil, err
	}
	return &OrderBookFactsSnapshotCreator{collector: collector, owner: owner}, nil
}

// Create 读取一次真实盘口、构建并持久化不可变快照，最后签发绑定的盘口数据类授权。
// 授权签发失败时允许留下不可变快照，但绝不返回可消费引用。
func (c *OrderBookFactsSnapshotCreator) Create(
	ctx context.Context, symbol string, systemAsOf time.Time, purpose string,
) (OrderBookFactsSnapshotReferences, error) {
	if c == nil || c.collector == nil || c.owner == nil || ctx == nil {
		return OrderBookFactsSnapshotReferences{}, ErrOrderBookFactsSnapshotCreationUnavailable
	}
	if systemAsOf.IsZero() || systemAsOf.Location() != time.UTC {
		return OrderBookFactsSnapshotReferences{}, errors.New("order-book facts snapshot requires a UTC system_as_of")
	}
	snapshotRef, err := c.collector.Persist(ctx, symbol, systemAsOf, purpose)
	if err != nil {
		return OrderBookFactsSnapshotReferences{}, err
	}
	authorizationRef, err := c.owner.IssueFor(
		ctx, snapshotRef, purpose, authorization.OrderBookFactsPolicyRef,
	)
	if err != nil {
		return OrderBookFactsSnapshotReferences{}, fmt.Errorf("Snapshot exists but authorization was not issued: %w", err)
	}
	return OrderBookFactsSnapshotReferences{
		SnapshotRef: snapshotRef, AuthorizationRef: authorizationRef,
	}, nil
}
