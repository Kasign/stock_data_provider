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

var ErrOrderBookFactsSnapshotCreationUnavailable = errors.New("order-book facts snapshot creator unavailable")

// OrderBookReader 是 Data 模块内的真实盘口来源窄端口。
// 它只返回带来源时间的事实，不接受调用方传入的盘口数值。
type OrderBookReader interface {
	FetchOrderBook(ctx context.Context, symbol string) (*model.OrderBook, error)
}

type OrderBookFactsSnapshotReferences struct {
	SnapshotRef      string
	AuthorizationRef string
}

// OrderBookFactsSnapshotCreator 是日内盘口授权快照的唯一生产入口。
// 它不返回 Payload；下游必须通过授权读取入口获得事实。
type OrderBookFactsSnapshotCreator struct {
	reader     OrderBookReader
	repository snapshotport.OrderBookFactsSnapshotRepository
	owner      *DataAuthorizationOwner
}

func NewOrderBookFactsSnapshotCreator(
	reader OrderBookReader,
	repository snapshotport.OrderBookFactsSnapshotRepository,
	owner *DataAuthorizationOwner,
) (*OrderBookFactsSnapshotCreator, error) {
	if reader == nil || repository == nil || owner == nil {
		return nil, ErrOrderBookFactsSnapshotCreationUnavailable
	}
	return &OrderBookFactsSnapshotCreator{reader: reader, repository: repository, owner: owner}, nil
}

// Create 读取一次真实盘口、构建并持久化不可变快照，最后签发绑定的盘口数据类授权。
// 授权签发失败时允许留下不可变快照，但绝不返回可消费引用。
func (c *OrderBookFactsSnapshotCreator) Create(
	ctx context.Context, symbol string, systemAsOf time.Time, purpose string,
) (OrderBookFactsSnapshotReferences, error) {
	if c == nil || c.reader == nil || c.repository == nil || c.owner == nil || ctx == nil {
		return OrderBookFactsSnapshotReferences{}, ErrOrderBookFactsSnapshotCreationUnavailable
	}
	if strings.TrimSpace(symbol) == "" || symbol != strings.TrimSpace(symbol) {
		return OrderBookFactsSnapshotReferences{}, errors.New("order-book facts snapshot symbol is required and must be canonical")
	}
	if systemAsOf.IsZero() || systemAsOf.Location() != time.UTC {
		return OrderBookFactsSnapshotReferences{}, errors.New("order-book facts snapshot requires a UTC system_as_of")
	}
	_, callerRef, _, err := trustedDataPrincipal(ctx)
	if err != nil {
		return OrderBookFactsSnapshotReferences{}, err
	}
	if strings.TrimSpace(callerRef) == "" {
		return OrderBookFactsSnapshotReferences{}, ErrDataAuthorizationDenied
	}
	book, err := c.reader.FetchOrderBook(ctx, symbol)
	if err != nil {
		return OrderBookFactsSnapshotReferences{}, fmt.Errorf("read live order book for Snapshot: %w", err)
	}
	if book == nil {
		return OrderBookFactsSnapshotReferences{}, errors.New("order-book reader returned an empty snapshot")
	}
	canonicalSymbol, err := normalize.NormalizeSymbol(book.Symbol)
	if err != nil || canonicalSymbol != symbol {
		return OrderBookFactsSnapshotReferences{}, errors.New("order-book source identity does not match the request")
	}
	if book.SystemObservedAt.After(systemAsOf) || book.ObservedAt.After(systemAsOf) {
		return OrderBookFactsSnapshotReferences{}, errors.New("order-book observation is later than the requested system_as_of")
	}
	payload, err := app.BuildOrderBookFactsPayload(*book)
	if err != nil {
		return OrderBookFactsSnapshotReferences{}, err
	}
	sources := []app.DailyBarsCloseSeriesSource{{SourceRef: book.Source}}
	snapshot, err := app.BuildOrderBookFactsSnapshot(payload, sources, callerRef, purpose, systemAsOf)
	if err != nil {
		return OrderBookFactsSnapshotReferences{}, fmt.Errorf("build order-book facts Snapshot: %w", err)
	}
	if err := c.repository.SaveOrderBookFactsSnapshot(ctx, snapshot); err != nil {
		return OrderBookFactsSnapshotReferences{}, fmt.Errorf("persist order-book facts Snapshot: %w", err)
	}
	authorizationRef, err := c.owner.IssueFor(
		ctx, snapshot.Manifest.SnapshotRef, purpose, authorization.OrderBookFactsPolicyRef,
	)
	if err != nil {
		return OrderBookFactsSnapshotReferences{}, fmt.Errorf("Snapshot persisted but authorization was not issued: %w", err)
	}
	return OrderBookFactsSnapshotReferences{
		SnapshotRef: snapshot.Manifest.SnapshotRef, AuthorizationRef: authorizationRef,
	}, nil
}
