package app

import (
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func limitPoolSnapshot(poolType, tradeDate string, total int, symbols ...string) *model.LimitPoolSnapshot {
	fetchedAt := time.Date(2026, 11, 27, 7, 1, 0, 0, time.UTC)
	items := make([]model.LimitPoolItem, 0, len(symbols))
	for _, symbol := range symbols {
		items = append(items, model.LimitPoolItem{
			Symbol: symbol, Code: strings.Split(symbol, ".")[0], Name: symbol,
			FetchedAt: fetchedAt, QualityStatus: model.QualityValid, Source: "eastmoney",
		})
	}
	return &model.LimitPoolSnapshot{
		PoolType: poolType, TradeDate: tradeDate, Total: total, HasData: true,
		Items: items, Source: "eastmoney", FetchedAt: fetchedAt, QualityStatus: model.QualityValid,
	}
}

func TestBuildLimitPoolFactsPayloadNoneRequiresBothCompletePools(t *testing.T) {
	up := limitPoolSnapshot("up", "2026-11-27", 1, "600519.SH")
	down := limitPoolSnapshot("down", "2026-11-27", 1, "000001.SZ")
	payload, err := BuildLimitPoolFactsPayload(up, down, "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if payload.LimitStatus != LimitPoolStatusNone {
		t.Fatalf("status = %q, want none", payload.LimitStatus)
	}
	if !payload.UpPoolComplete || !payload.DownPoolComplete {
		t.Fatal("complete pools must be recorded as complete")
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
}

func TestBuildLimitPoolFactsPayloadDetectsLimitUpAndLimitDown(t *testing.T) {
	up := limitPoolSnapshot("up", "2026-11-27", 1, "600036.SH")
	down := limitPoolSnapshot("down", "2026-11-27", 0)
	payload, err := BuildLimitPoolFactsPayload(up, down, "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if payload.LimitStatus != LimitPoolStatusUp {
		t.Fatalf("status = %q, want limit_up", payload.LimitStatus)
	}

	up2 := limitPoolSnapshot("up", "2026-11-27", 0)
	down2 := limitPoolSnapshot("down", "2026-11-27", 1, "600036.SH")
	payload2, err := BuildLimitPoolFactsPayload(up2, down2, "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if payload2.LimitStatus != LimitPoolStatusDown {
		t.Fatalf("status = %q, want limit_down", payload2.LimitStatus)
	}
}

func TestBuildLimitPoolFactsPayloadPartialPoolCannotClaimNone(t *testing.T) {
	up := limitPoolSnapshot("up", "2026-11-27", 10, "600519.SH") // Total 10 but only 1 item
	down := limitPoolSnapshot("down", "2026-11-27", 0)
	payload, err := BuildLimitPoolFactsPayload(up, down, "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if payload.LimitStatus != LimitPoolStatusUnknown {
		t.Fatalf("status = %q, want unknown when a pool is incomplete", payload.LimitStatus)
	}
	if payload.UpPoolComplete {
		t.Fatal("partial pool must not be marked complete")
	}
}

func TestBuildLimitPoolFactsPayloadPartialDownPoolCannotClaimNone(t *testing.T) {
	up := limitPoolSnapshot("up", "2026-11-27", 0)
	down := limitPoolSnapshot("down", "2026-11-27", 10, "000001.SZ") // Total 10 but only 1 item
	payload, err := BuildLimitPoolFactsPayload(up, down, "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if payload.DownPoolComplete {
		t.Fatal("partial down pool must not be marked complete")
	}
	if payload.LimitStatus != LimitPoolStatusUnknown {
		t.Fatalf("status = %q, want unknown when the down pool is incomplete", payload.LimitStatus)
	}
}

func TestBuildLimitPoolFactsPayloadLowerBoundTotalIsNotComplete(t *testing.T) {
	up := limitPoolSnapshot("up", "2026-11-27", 1, "600519.SH")
	up.TotalIsLowerBound = true
	down := limitPoolSnapshot("down", "2026-11-27", 0)
	payload, err := BuildLimitPoolFactsPayload(up, down, "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	if payload.UpPoolComplete || payload.LimitStatus != LimitPoolStatusUnknown {
		t.Fatalf("lower-bound total must stay incomplete: complete=%v status=%q", payload.UpPoolComplete, payload.LimitStatus)
	}
}

func TestBuildLimitPoolFactsPayloadRejectsInvalidInputs(t *testing.T) {
	base := limitPoolSnapshot("up", "2026-11-27", 0)
	down := limitPoolSnapshot("down", "2026-11-27", 0)

	cases := []struct {
		name string
		up   *model.LimitPoolSnapshot
		down *model.LimitPoolSnapshot
		sym  string
	}{
		{"nil up", nil, down, "600036.SH"},
		{"nil down", base, nil, "600036.SH"},
		{"trade date mismatch", base, limitPoolSnapshot("down", "2026-11-28", 0), "600036.SH"},
		{"blank symbol", base, down, "  "},
		{"no data", func() *model.LimitPoolSnapshot {
			s := limitPoolSnapshot("up", "2026-11-27", 0)
			s.HasData = false
			return s
		}(), down, "600036.SH"},
		{"wrong pool type", limitPoolSnapshot("down", "2026-11-27", 0), down, "600036.SH"},
		{"invalid trade date", limitPoolSnapshot("up", "11/27/2026", 0), limitPoolSnapshot("down", "11/27/2026", 0), "600036.SH"},
		{"unusable quality", func() *model.LimitPoolSnapshot {
			s := limitPoolSnapshot("up", "2026-11-27", 0)
			s.QualityStatus = model.QualityStatus("unknown")
			return s
		}(), down, "600036.SH"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildLimitPoolFactsPayload(tc.up, tc.down, tc.sym); err == nil {
				t.Fatal("invalid input was accepted")
			}
		})
	}
}

func TestLimitPoolFactsPayloadValidateRejectsNoneWithoutCompletePools(t *testing.T) {
	up := limitPoolSnapshot("up", "2026-11-27", 0)
	down := limitPoolSnapshot("down", "2026-11-27", 0)
	payload, err := BuildLimitPoolFactsPayload(up, down, "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	payload.DownPoolComplete = false
	if err := payload.Validate(); err == nil {
		t.Fatal("none with an incomplete pool must be rejected")
	}
}
