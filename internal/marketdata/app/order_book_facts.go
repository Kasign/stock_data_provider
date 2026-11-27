package app

import (
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

var ErrInvalidOrderBookFacts = errors.New("invalid order book facts payload")

const (
	OrderBookFactsPayloadKind          = "order_book"
	OrderBookFactsPayloadSchemaVersion = "1.0.0"
)

// OrderBookFactsPayload 是日内盘口授权快照的可核验事实载荷。
// tradable_volume 是买盘五档挂单量之和（零假设直接事实），observed_at 绑定到
// 具体快照时刻，避免与单日交易状态混淆 PIT 精度。
type OrderBookFactsPayload struct {
	SchemaVersion        string        `json:"schema_version"`
	PayloadKind          string        `json:"payload_kind"`
	Symbol               string        `json:"symbol"`
	ObservedAt           time.Time     `json:"observed_at"`
	SystemObservedAt     time.Time     `json:"system_observed_at"`
	TradableVolumeShares int64         `json:"tradable_volume_shares"`
	BidLevels            int           `json:"bid_levels"`
	AskLevels            int           `json:"ask_levels"`
	Source               string        `json:"source"`
	QualityStatus        QualityStatus `json:"quality_status"`
}

// BuildOrderBookFactsPayload 从未校验的盘口模型构造事实载荷，先经 OrderBook.Validate
// 与 TradableVolumeShares 拿到真值，避免不完整盘口或零值被写成事实。
func BuildOrderBookFactsPayload(book model.OrderBook) (OrderBookFactsPayload, error) {
	if err := book.Validate(); err != nil {
		return OrderBookFactsPayload{}, fmt.Errorf("%w: source order book: %v", ErrInvalidOrderBookFacts, err)
	}
	volume, err := book.TradableVolumeShares()
	if err != nil {
		return OrderBookFactsPayload{}, fmt.Errorf("%w: tradable volume: %v", ErrInvalidOrderBookFacts, err)
	}
	if volume <= 0 {
		return OrderBookFactsPayload{}, fmt.Errorf("%w: non-positive tradable volume", ErrInvalidOrderBookFacts)
	}
	quality, err := orderBookQuality(book.QualityStatus)
	if err != nil {
		return OrderBookFactsPayload{}, err
	}
	return OrderBookFactsPayload{
		SchemaVersion: OrderBookFactsPayloadSchemaVersion, PayloadKind: OrderBookFactsPayloadKind,
		Symbol: book.Symbol, ObservedAt: book.ObservedAt, SystemObservedAt: book.SystemObservedAt,
		TradableVolumeShares: volume, BidLevels: len(book.Bids), AskLevels: len(book.Asks),
		Source: book.Source, QualityStatus: quality,
	}, nil
}

func orderBookQuality(status model.QualityStatus) (QualityStatus, error) {
	switch status {
	case model.QualityValid:
		return QualityValid, nil
	case model.QualityDegraded:
		return QualityDegraded, nil
	default:
		return "", fmt.Errorf("%w: unusable order book quality %q", ErrInvalidOrderBookFacts, status)
	}
}

// Validate 校验载荷身份、数量与 PIT 时间语义。
func (payload OrderBookFactsPayload) Validate() error {
	if payload.SchemaVersion != OrderBookFactsPayloadSchemaVersion ||
		payload.PayloadKind != OrderBookFactsPayloadKind ||
		payload.Symbol == "" || payload.Source == "" {
		return ErrInvalidOrderBookFacts
	}
	if payload.TradableVolumeShares <= 0 || payload.BidLevels != 5 || payload.AskLevels != 5 {
		return ErrInvalidOrderBookFacts
	}
	if payload.ObservedAt.IsZero() || payload.SystemObservedAt.IsZero() ||
		payload.ObservedAt.After(payload.SystemObservedAt) {
		return ErrInvalidOrderBookFacts
	}
	if payload.QualityStatus != QualityValid && payload.QualityStatus != QualityDegraded {
		return ErrInvalidOrderBookFacts
	}
	return nil
}
