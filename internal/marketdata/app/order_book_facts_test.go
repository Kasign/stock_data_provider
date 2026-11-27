package app

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func validOrderBookRecord() model.OrderBook {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	levels := func() []model.OrderBookLevel {
		return []model.OrderBookLevel{
			{Price: 10.0, Volume: 100}, {Price: 9.9, Volume: 200},
			{Price: 9.8, Volume: 300}, {Price: 9.7, Volume: 400}, {Price: 9.6, Volume: 500},
		}
	}
	return model.OrderBook{
		Symbol: "600000.SH", Bids: levels(), Asks: levels(),
		ObservedAt: now, SystemObservedAt: now, Source: "eastmoney", QualityStatus: model.QualityValid,
	}
}

// TestBuildOrderBookFactsPayloadBindsTradableVolume 证明 payload 承载的
// tradable_volume 来自盘口买盘五档挂单量之和，且带观测时间与来源。
func TestBuildOrderBookFactsPayloadBindsTradableVolume(t *testing.T) {
	payload, err := BuildOrderBookFactsPayload(validOrderBookRecord())
	if err != nil {
		t.Fatalf("build order book facts payload: %v", err)
	}
	if payload.PayloadKind != OrderBookFactsPayloadKind || payload.SchemaVersion != OrderBookFactsPayloadSchemaVersion {
		t.Fatalf("payload identity mismatch: %+v", payload)
	}
	if payload.TradableVolumeShares != 1500 {
		t.Fatalf("tradable_volume = %d, want 1500", payload.TradableVolumeShares)
	}
	if payload.ObservedAt.IsZero() || !payload.ObservedAt.Equal(payload.SystemObservedAt) {
		t.Fatalf("observed_at not bound: %+v", payload)
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("payload should validate: %v", err)
	}
}

// TestBuildOrderBookFactsPayloadRejectsInvalidRecord 证明非法盘口不能生成事实，
// 杜绝用不完整盘口伪装成可成交量证据。
func TestBuildOrderBookFactsPayloadRejectsInvalidRecord(t *testing.T) {
	record := validOrderBookRecord()
	record.Bids = record.Bids[:3]
	if _, err := BuildOrderBookFactsPayload(record); err == nil {
		t.Fatal("incomplete order book must be rejected")
	}
}

// TestOrderBookFactsPayloadValidation 证明 payload 自身校验拒绝零可成交量
// 与不一致的时间语义。
func TestOrderBookFactsPayloadValidation(t *testing.T) {
	payload, err := BuildOrderBookFactsPayload(validOrderBookRecord())
	if err != nil {
		t.Fatal(err)
	}
	payload.TradableVolumeShares = 0
	if err := payload.Validate(); err == nil {
		t.Fatal("zero tradable volume must fail validation")
	}
	payload = mustOrderBookPayload(t)
	payload.ObservedAt = payload.SystemObservedAt.Add(time.Second)
	if err := payload.Validate(); err == nil {
		t.Fatal("observed_at after system_observed_at must fail")
	}
}

func mustOrderBookPayload(t *testing.T) OrderBookFactsPayload {
	t.Helper()
	payload, err := BuildOrderBookFactsPayload(validOrderBookRecord())
	if err != nil {
		t.Fatal(err)
	}
	return payload
}
