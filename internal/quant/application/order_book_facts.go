package application

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Quant 消费的日内盘口事实边界。
//
// tradable_volume 是 Data 从真实五档盘口读取的零假设事实：买盘五档挂单量之和。
// Quant 不接收盘口数值，不推导参与率，也不允许调用方声称某个数值；
// 只读 Data 已授权、已持久化且通过双指纹复核的盘口快照。
const (
	OrderBookFactsPayloadKind          = "order_book"
	OrderBookFactsPayloadSchemaVersion = "1.0.0"
	OrderBookLevels                    = 5
)

var ErrInvalidOrderBookFactsSnapshot = errors.New("invalid order-book facts snapshot")

// OrderBookFactsSnapshot 是 Quant 消费的可追溯盘口事实证据。
// AuthorizationRef 与 SnapshotRef 共同证明这份事实是按什么授权读到的。
type OrderBookFactsSnapshot struct {
	SnapshotRef           string
	AuthorizationRef      string
	ManifestSchemaVersion string
	PayloadSchemaVersion  string
	PayloadKind           string
	CallerRef             string
	PurposeCode           string
	Symbol                string
	ObservedAt            string
	SystemAsOf            string
	TradableVolumeShares  int64
	BidLevels             int
	AskLevels             int
	Source                string
	QualityStatus         string
}

// Validate 校验证据身份、payload 类型、盘口规模与 PIT 时间。
// 任一项不自洽都拒绝，不能把残缺盘口当作可成交量事实。
func (snapshot OrderBookFactsSnapshot) Validate() error {
	if !nonBlankTradingFacts(snapshot.SnapshotRef) || !nonBlankTradingFacts(snapshot.AuthorizationRef) ||
		!nonBlankTradingFacts(snapshot.Symbol) || !nonBlankTradingFacts(snapshot.CallerRef) ||
		!nonBlankTradingFacts(snapshot.SystemAsOf) || !nonBlankTradingFacts(snapshot.ObservedAt) ||
		!nonBlankTradingFacts(snapshot.Source) || !nonBlankTradingFacts(snapshot.ManifestSchemaVersion) {
		return fmt.Errorf("%w: identity or evidence references are missing", ErrInvalidOrderBookFactsSnapshot)
	}
	if snapshot.PayloadKind != OrderBookFactsPayloadKind ||
		snapshot.PayloadSchemaVersion != OrderBookFactsPayloadSchemaVersion {
		return fmt.Errorf("%w: payload kind or schema version mismatch", ErrInvalidOrderBookFactsSnapshot)
	}
	if snapshot.TradableVolumeShares <= 0 || snapshot.BidLevels != OrderBookLevels ||
		snapshot.AskLevels != OrderBookLevels {
		return fmt.Errorf("%w: order book is incomplete or has no tradable volume", ErrInvalidOrderBookFactsSnapshot)
	}
	if snapshot.QualityStatus != "valid" && snapshot.QualityStatus != "degraded" {
		return fmt.Errorf("%w: quality_status %q is not admissible", ErrInvalidOrderBookFactsSnapshot, snapshot.QualityStatus)
	}
	observedAt, err := time.Parse(time.RFC3339Nano, snapshot.ObservedAt)
	if err != nil || observedAt.Location() != time.UTC {
		return fmt.Errorf("%w: observed_at is not a canonical UTC time", ErrInvalidOrderBookFactsSnapshot)
	}
	systemAsOf, err := time.Parse(time.RFC3339Nano, snapshot.SystemAsOf)
	if err != nil || systemAsOf.Location() != time.UTC {
		return fmt.Errorf("%w: system_as_of is not a canonical UTC time", ErrInvalidOrderBookFactsSnapshot)
	}
	if observedAt.After(systemAsOf) {
		return fmt.Errorf("%w: observed_at is later than system_as_of", ErrInvalidOrderBookFactsSnapshot)
	}
	return nil
}

// OrderBookFactsReader 读取盘口事实授权快照。生产实现由 Server 组合根注入。
type OrderBookFactsReader interface {
	ReadOrderBookFactsSnapshot(ctx context.Context, snapshotRef, authorizationRef, purpose string) (OrderBookFactsSnapshot, error)
}

// TradabilityEvidence 是一次可交易性判定所需的全部授权事实证据。
// 交易状态事实与日内盘口事实各有独立数据类、独立授权和独立 payload kind，
// 不能互相替代，也不能由调用方只提供其中一份。
type TradabilityEvidence struct {
	TradingFacts   TradingFactsSnapshot
	OrderBookFacts OrderBookFactsSnapshot
	LimitPoolFacts LimitPoolFactsSnapshot
}

// Validate 校验两份证据本身及它们之间的绑定关系。
func (evidence TradabilityEvidence) Validate() error {
	if err := evidence.TradingFacts.Validate(); err != nil {
		return err
	}
	if err := evidence.OrderBookFacts.Validate(); err != nil {
		return err
	}
	if evidence.TradingFacts.Symbol != evidence.OrderBookFacts.Symbol {
		return fmt.Errorf("%w: order-book symbol does not match trading-status facts", ErrInvalidOrderBookFactsSnapshot)
	}
	if evidence.TradingFacts.SystemAsOf != evidence.OrderBookFacts.SystemAsOf {
		return fmt.Errorf("%w: order-book system_as_of does not match trading-status facts", ErrInvalidOrderBookFactsSnapshot)
	}
	if err := evidence.LimitPoolFacts.Validate(); err != nil {
		return err
	}
	if evidence.TradingFacts.Symbol != evidence.LimitPoolFacts.Symbol {
		return fmt.Errorf("%w: limit-pool symbol does not match trading-status facts", ErrInvalidLimitPoolFactsSnapshot)
	}
	if evidence.TradingFacts.TradeDate != evidence.LimitPoolFacts.TradeDate {
		return fmt.Errorf("%w: limit-pool trade_date does not match trading-status facts", ErrInvalidLimitPoolFactsSnapshot)
	}
	if evidence.TradingFacts.SystemAsOf != evidence.LimitPoolFacts.SystemAsOf {
		return fmt.Errorf("%w: limit-pool system_as_of does not match trading-status facts", ErrInvalidLimitPoolFactsSnapshot)
	}
	return nil
}

// evidenceRefs 固定输出顺序：交易状态快照、交易状态授权、盘口快照、盘口授权、
// 涨跌停池快照、涨跌停池授权。
func (evidence TradabilityEvidence) evidenceRefs() []string {
	return []string{
		evidence.TradingFacts.SnapshotRef,
		evidence.TradingFacts.AuthorizationRef,
		evidence.OrderBookFacts.SnapshotRef,
		evidence.OrderBookFacts.AuthorizationRef,
		evidence.LimitPoolFacts.SnapshotRef,
		evidence.LimitPoolFacts.AuthorizationRef,
	}
}
