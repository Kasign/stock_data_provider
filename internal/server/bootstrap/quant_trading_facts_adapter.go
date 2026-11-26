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

// quantTradingFactsPort 是 Data 侧交易事实快照的窄边界。
// 具体实现是 marketdata/bootstrap.Module；此处只依赖两个方法，避免 Quant 认识 Data 内部类型。
type quantTradingFactsPort interface {
	CreateSecurityTradingStatusFactsSnapshot(
		context.Context, string, string, time.Time, string,
	) (marketdataapplication.SecurityTradingStatusFactsSnapshotReferences, error)
	ReadAuthorizedSecurityTradingStatusFactsSnapshot(
		context.Context, string, string, string,
	) (marketdataapp.SecurityTradingStatusFactsSnapshot, error)
}

// quantTradingFactsAdapter 把 Data 的授权交易事实快照转换为 Quant 的消费 DTO。
//
// 它是组合根桥接：Data 的 payload 结构与 Quant 的消费结构刻意不同名，
// 使任一侧变更都必须显式改到这里，而不是靠类型别名静默传播。
// 当前由 1.4 的契约测试驱动；quant.risk Stage 装配时复用同一实现。
type quantTradingFactsAdapter struct {
	data quantTradingFactsPort
}

func newQuantTradingFactsAdapter(data quantTradingFactsPort) (*quantTradingFactsAdapter, error) {
	if data == nil {
		return nil, errors.New("Quant trading-status facts adapter requires the Data Module")
	}
	return &quantTradingFactsAdapter{data: data}, nil
}

// CreateTradingFactsSnapshot 创建并授权一个交易事实快照，返回可被授权读取的两个引用。
func (adapter *quantTradingFactsAdapter) CreateTradingFactsSnapshot(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (string, string, error) {
	if adapter == nil || adapter.data == nil || ctx == nil {
		return "", "", errors.New("Quant trading-status facts creator is unavailable")
	}
	refs, err := adapter.data.CreateSecurityTradingStatusFactsSnapshot(ctx, symbol, tradeDate, systemAsOf.UTC(), purpose)
	if err != nil {
		return "", "", fmt.Errorf("Data CreateSecurityTradingStatusFactsSnapshot: %w", err)
	}
	if refs.SnapshotRef == "" || refs.AuthorizationRef == "" {
		return "", "", errors.New("Data returned empty trading-status facts references")
	}
	return refs.SnapshotRef, refs.AuthorizationRef, nil
}

// ReadTradingFactsSnapshot 读取授权快照并转换为 Quant DTO。
// 它在转换前重新校验快照自身与身份，避免把"Data 校验过"当作传输层可信。
func (adapter *quantTradingFactsAdapter) ReadTradingFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (quantapplication.TradingFactsSnapshot, error) {
	if adapter == nil || adapter.data == nil || ctx == nil {
		return quantapplication.TradingFactsSnapshot{}, errors.New("Quant trading-status facts reader is unavailable")
	}
	snapshot, err := adapter.data.ReadAuthorizedSecurityTradingStatusFactsSnapshot(ctx, snapshotRef, authorizationRef, purpose)
	if err != nil {
		return quantapplication.TradingFactsSnapshot{}, fmt.Errorf("Data ReadAuthorizedSecurityTradingStatusFactsSnapshot: %w", err)
	}
	if err := snapshot.Verify(); err != nil {
		return quantapplication.TradingFactsSnapshot{}, fmt.Errorf("Data returned an invalid trading-status facts Snapshot: %w", err)
	}
	manifest := snapshot.Manifest
	payload := snapshot.Payload
	if manifest.SnapshotRef != snapshotRef || manifest.PurposeCode != purpose {
		return quantapplication.TradingFactsSnapshot{}, errors.New("Data authorized Snapshot identity does not match the requested reference and purpose")
	}
	if payload.PayloadKind != quantapplication.TradingFactsPayloadKind {
		return quantapplication.TradingFactsSnapshot{}, errors.New("Data authorized Snapshot payload kind is not trading-status facts")
	}
	facts := make([]quantapplication.TradingFact, len(payload.Facts))
	for index, fact := range payload.Facts {
		facts[index] = quantapplication.TradingFact{
			Key:               fact.Key,
			Availability:      fact.Availability,
			Value:             fact.Value,
			UnavailableReason: fact.UnavailableReason,
		}
	}
	return quantapplication.TradingFactsSnapshot{
		SnapshotRef:           manifest.SnapshotRef,
		AuthorizationRef:      authorizationRef,
		ManifestSchemaVersion: manifest.SchemaVersion,
		PayloadSchemaVersion:  payload.SchemaVersion,
		PayloadKind:           payload.PayloadKind,
		CallerRef:             manifest.CallerRef,
		PurposeCode:           manifest.PurposeCode,
		SystemAsOf:            manifest.SystemAsOf,
		Symbol:                manifest.Symbol,
		TradeDate:             manifest.TradeDate,
		QualityStatus:         string(payload.QualityStatus),
		Facts:                 facts,
	}, nil
}

var _ quantapplication.TradingFactsReader = (*quantTradingFactsAdapter)(nil)
