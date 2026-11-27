package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	marketdataapp "example.com/stock-analysis-flow/internal/marketdata/app"
	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
)

// quantOrderBookFactsPort 是 Data 侧盘口事实快照的窄边界。
// 生产实现是 marketdata/bootstrap.Module；此处只依赖不可变快照的创建与授权读取。
type quantOrderBookFactsPort interface {
	CreateOrderBookFactsSnapshot(
		context.Context, string, time.Time, string,
	) (marketdataapplication.OrderBookFactsSnapshotReferences, error)
	ReadAuthorizedOrderBookFactsSnapshot(
		context.Context, string, string, string,
	) (marketdataapp.OrderBookFactsSnapshot, error)
}

// quantOrderBookFactsAdapter 把 Data 的授权盘口快照转换为 Quant 的消费 DTO。
// 两侧类型刻意不同名：任一侧变更都必须显式落到这里，不能靠类型别名静默传播。
type quantOrderBookFactsAdapter struct {
	data quantOrderBookFactsPort
}

func newQuantOrderBookFactsAdapter(data quantOrderBookFactsPort) (*quantOrderBookFactsAdapter, error) {
	if data == nil {
		return nil, errors.New("Quant order-book facts adapter requires the Data Module")
	}
	return &quantOrderBookFactsAdapter{data: data}, nil
}

func (adapter *quantOrderBookFactsAdapter) CreateOrderBookFactsSnapshot(
	ctx context.Context, symbol string, systemAsOf time.Time, purpose string,
) (string, string, error) {
	if adapter == nil || adapter.data == nil || ctx == nil {
		return "", "", errors.New("Quant order-book facts creator is unavailable")
	}
	refs, err := adapter.data.CreateOrderBookFactsSnapshot(ctx, symbol, systemAsOf.UTC(), purpose)
	if err != nil {
		return "", "", fmt.Errorf("Data CreateOrderBookFactsSnapshot: %w", err)
	}
	if refs.SnapshotRef == "" || refs.AuthorizationRef == "" {
		return "", "", errors.New("Data returned empty order-book facts references")
	}
	return refs.SnapshotRef, refs.AuthorizationRef, nil
}

func (adapter *quantOrderBookFactsAdapter) ReadOrderBookFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (quantapplication.OrderBookFactsSnapshot, error) {
	if adapter == nil || adapter.data == nil || ctx == nil {
		return quantapplication.OrderBookFactsSnapshot{}, errors.New("Quant order-book facts reader is unavailable")
	}
	snapshot, err := adapter.data.ReadAuthorizedOrderBookFactsSnapshot(ctx, snapshotRef, authorizationRef, purpose)
	if err != nil {
		return quantapplication.OrderBookFactsSnapshot{}, fmt.Errorf("Data ReadAuthorizedOrderBookFactsSnapshot: %w", err)
	}
	// Data 已复核过双指纹，这里仍独立复核一次，避免传输层替换 Payload。
	if err := snapshot.Verify(); err != nil {
		return quantapplication.OrderBookFactsSnapshot{}, fmt.Errorf("Data returned an invalid order-book facts Snapshot: %w", err)
	}
	manifest := snapshot.Manifest
	payload := snapshot.Payload
	if manifest.SnapshotRef != snapshotRef || manifest.PurposeCode != purpose {
		return quantapplication.OrderBookFactsSnapshot{},
			errors.New("Data authorized order-book Snapshot identity does not match the requested reference and purpose")
	}
	if payload.PayloadKind != quantapplication.OrderBookFactsPayloadKind {
		return quantapplication.OrderBookFactsSnapshot{},
			errors.New("Data authorized Snapshot payload kind is not order-book facts")
	}
	return quantapplication.OrderBookFactsSnapshot{
		SnapshotRef:           manifest.SnapshotRef,
		AuthorizationRef:      authorizationRef,
		ManifestSchemaVersion: manifest.SchemaVersion,
		PayloadSchemaVersion:  payload.SchemaVersion,
		PayloadKind:           payload.PayloadKind,
		CallerRef:             manifest.CallerRef,
		PurposeCode:           manifest.PurposeCode,
		Symbol:                manifest.Symbol,
		ObservedAt:            payload.ObservedAt.UTC().Format(time.RFC3339Nano),
		SystemAsOf:            manifest.SystemAsOf,
		TradableVolumeShares:  payload.TradableVolumeShares,
		BidLevels:             payload.BidLevels,
		AskLevels:             payload.AskLevels,
		Source:                payload.Source,
		QualityStatus:         string(payload.QualityStatus),
	}, nil
}

var _ quantapplication.OrderBookFactsReader = (*quantOrderBookFactsAdapter)(nil)
