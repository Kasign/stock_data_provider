package model

import (
	"errors"
	"time"
)

var ErrInvalidOrderBook = errors.New("invalid order book")

// OrderBookLevel 是盘口的单一档位。价格与成交量都是来源提供的原始事实，
// 不做单位换算或参与率假设。
type OrderBookLevel struct {
	Price  float64 `json:"price"`
	Volume int64   `json:"volume"`
}

// OrderBook 是某一时刻的完整五档盘口快照。
// 它是 tradable_volume 的唯一事实来源：可成交量 = 买盘五档挂单量之和，
// 不使用"当日成交量 × 参与率"一类无权威出处的假设。
type OrderBook struct {
	Symbol           string           `json:"symbol"`
	Bids             []OrderBookLevel `json:"bids"`
	Asks             []OrderBookLevel `json:"asks"`
	ObservedAt       time.Time        `json:"observed_at"`
	SystemObservedAt time.Time        `json:"system_observed_at"`
	Source           string           `json:"source"`
	QualityStatus    QualityStatus    `json:"quality_status"`
}

const orderBookDepth = 5

// Validate 要求完整五档、正价格/成交量、明确的时间语义与有效质量状态。
func (book OrderBook) Validate() error {
	if book.Symbol == "" || book.Source == "" {
		return ErrInvalidOrderBook
	}
	if len(book.Bids) != orderBookDepth || len(book.Asks) != orderBookDepth {
		return ErrInvalidOrderBook
	}
	if book.ObservedAt.IsZero() || book.SystemObservedAt.IsZero() ||
		book.ObservedAt.After(book.SystemObservedAt) {
		return ErrInvalidOrderBook
	}
	if book.QualityStatus != QualityValid && book.QualityStatus != QualityDegraded {
		return ErrInvalidOrderBook
	}
	for _, side := range [][]OrderBookLevel{book.Bids, book.Asks} {
		for _, level := range side {
			if level.Price <= 0 || level.Volume <= 0 {
				return ErrInvalidOrderBook
			}
		}
	}
	return nil
}

// TradableVolumeShares 返回买盘五档挂单量之和。这是零假设的直接事实：
// 在盘口可见范围内可立即成交的股数上限，不引入参与率或模型参数。
func (book OrderBook) TradableVolumeShares() (int64, error) {
	if err := book.Validate(); err != nil {
		return 0, err
	}
	var total int64
	for _, level := range book.Bids {
		total += level.Volume
	}
	return total, nil
}
