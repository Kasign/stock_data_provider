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

// quantLimitPoolFactsPort 是 Data 侧涨跌停事实快照的窄边界。
type quantLimitPoolFactsPort interface {
	CreateLimitPoolFactsSnapshot(
		context.Context, string, string, time.Time, string,
	) (marketdataapplication.LimitPoolFactsSnapshotReferences, error)
	ReadAuthorizedLimitPoolFactsSnapshot(
		context.Context, string, string, string,
	) (marketdataapp.LimitPoolFactsSnapshot, error)
}

// quantLimitPoolFactsAdapter 把 Data 的授权涨跌停快照转换为 Quant 的消费 DTO。
// 两侧类型刻意不同名：任一侧变更都必须显式落到这里。
type quantLimitPoolFactsAdapter struct {
	data quantLimitPoolFactsPort
}

func newQuantLimitPoolFactsAdapter(data quantLimitPoolFactsPort) (*quantLimitPoolFactsAdapter, error) {
	if data == nil {
		return nil, errors.New("Quant limit-pool facts adapter requires the Data Module")
	}
	return &quantLimitPoolFactsAdapter{data: data}, nil
}

func (adapter *quantLimitPoolFactsAdapter) CreateLimitPoolFactsSnapshot(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (string, string, error) {
	if adapter == nil || adapter.data == nil || ctx == nil {
		return "", "", errors.New("Quant limit-pool facts creator is unavailable")
	}
	refs, err := adapter.data.CreateLimitPoolFactsSnapshot(ctx, symbol, tradeDate, systemAsOf.UTC(), purpose)
	if err != nil {
		return "", "", fmt.Errorf("Data CreateLimitPoolFactsSnapshot: %w", err)
	}
	if refs.SnapshotRef == "" || refs.AuthorizationRef == "" {
		return "", "", errors.New("Data returned empty limit-pool facts references")
	}
	return refs.SnapshotRef, refs.AuthorizationRef, nil
}

func (adapter *quantLimitPoolFactsAdapter) ReadLimitPoolFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (quantapplication.LimitPoolFactsSnapshot, error) {
	if adapter == nil || adapter.data == nil || ctx == nil {
		return quantapplication.LimitPoolFactsSnapshot{}, errors.New("Quant limit-pool facts reader is unavailable")
	}
	snapshot, err := adapter.data.ReadAuthorizedLimitPoolFactsSnapshot(ctx, snapshotRef, authorizationRef, purpose)
	if err != nil {
		return quantapplication.LimitPoolFactsSnapshot{}, fmt.Errorf("Data ReadAuthorizedLimitPoolFactsSnapshot: %w", err)
	}
	if err := snapshot.Verify(); err != nil {
		return quantapplication.LimitPoolFactsSnapshot{}, fmt.Errorf("Data returned an invalid limit-pool facts Snapshot: %w", err)
	}
	manifest := snapshot.Manifest
	payload := snapshot.Payload
	if manifest.SnapshotRef != snapshotRef || manifest.PurposeCode != purpose {
		return quantapplication.LimitPoolFactsSnapshot{},
			errors.New("Data authorized limit-pool Snapshot identity does not match the requested reference and purpose")
	}
	if payload.PayloadKind != quantapplication.LimitPoolFactsPayloadKind {
		return quantapplication.LimitPoolFactsSnapshot{},
			errors.New("Data authorized Snapshot payload kind is not limit-pool facts")
	}
	return quantapplication.LimitPoolFactsSnapshot{
		SnapshotRef:           manifest.SnapshotRef,
		AuthorizationRef:      authorizationRef,
		ManifestSchemaVersion: manifest.SchemaVersion,
		PayloadSchemaVersion:  payload.SchemaVersion,
		PayloadKind:           payload.PayloadKind,
		CallerRef:             manifest.CallerRef,
		PurposeCode:           manifest.PurposeCode,
		Symbol:                manifest.Symbol,
		TradeDate:             manifest.TradeDate,
		ObservedAt:            payload.ObservedAt.UTC().Format(time.RFC3339Nano),
		SystemAsOf:            manifest.SystemAsOf,
		LimitStatus:           string(payload.LimitStatus),
		UpPoolTotal:           payload.UpPoolTotal,
		DownPoolTotal:         payload.DownPoolTotal,
		UpPoolComplete:        payload.UpPoolComplete,
		DownPoolComplete:      payload.DownPoolComplete,
		Source:                payload.Source,
		QualityStatus:         string(payload.QualityStatus),
	}, nil
}

var _ quantapplication.LimitPoolFactsReader = (*quantLimitPoolFactsAdapter)(nil)
