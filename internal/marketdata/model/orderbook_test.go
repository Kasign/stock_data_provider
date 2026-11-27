package model

import (
	"testing"
	"time"
)

// TestOrderBookValidatesFiveLevels 证明盘口模型要求买卖各五档、价格与量
// 均为正、且时间语义明确，避免把不完整盘口当作可成交量事实。
func TestOrderBookValidatesFiveLevels(t *testing.T) {
	book := validOrderBook()
	if err := book.Validate(); err != nil {
		t.Fatalf("valid order book: %v", err)
	}
	book.Bids = book.Bids[:4]
	if err := book.Validate(); err == nil {
		t.Fatal("order book with fewer than five bid levels must fail")
	}
}

// TestOrderBookRejectsNonPositivePriceOrVolume 证明价格与量必须为正，
// 杜绝用零值盘口伪装成"可成交量可核验"。
func TestOrderBookRejectsNonPositivePriceOrVolume(t *testing.T) {
	book := validOrderBook()
	book.Bids[0].Price = 0
	if err := book.Validate(); err == nil {
		t.Fatal("zero bid price must fail")
	}
	book = validOrderBook()
	book.Asks[0].Volume = -1
	if err := book.Validate(); err == nil {
		t.Fatal("negative ask volume must fail")
	}
}

// TestOrderBookTradableVolumeUsesBidDepth 证明可成交量定义为买盘五档挂单量
// 之和（零假设路径），不是从成交量或参与率推导；且只统计明确有效的档位。
func TestOrderBookTradableVolumeUsesBidDepth(t *testing.T) {
	book := validOrderBook()
	got, err := book.TradableVolumeShares()
	if err != nil {
		t.Fatal(err)
	}
	want := int64(100 + 200 + 300 + 400 + 500)
	if got != want {
		t.Fatalf("tradable volume = %d, want %d", got, want)
	}
}

// TestOrderBookRequiresObservedTimeNotBeforeAsOf 证明盘口观测时间必须明确，
// 且不得晚于系统观测时间，保持 PIT 语义。
func TestOrderBookRequiresObservedTimeNotBeforeAsOf(t *testing.T) {
	book := validOrderBook()
	book.ObservedAt = book.SystemObservedAt.Add(time.Second)
	if err := book.Validate(); err == nil {
		t.Fatal("observed_at after system_observed_at must fail")
	}
}

func validOrderBook() OrderBook {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	levels := func(volumes ...int64) []OrderBookLevel {
		result := make([]OrderBookLevel, 0, len(volumes))
		for i, volume := range volumes {
			result = append(result, OrderBookLevel{
				Price:  float64(10 - i), // 买盘递减；卖盘测试里单独覆盖
				Volume: volume,
			})
		}
		return result
	}
	return OrderBook{
		Symbol:           "600000.SH",
		Bids:             levels(100, 200, 300, 400, 500),
		Asks:             levels(100, 200, 300, 400, 500),
		ObservedAt:       now,
		SystemObservedAt: now,
		Source:           "eastmoney",
		QualityStatus:    QualityValid,
	}
}
