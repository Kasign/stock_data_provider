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

// emptyLimitPoolSnapshot 复现真实 Provider 的空池形态：强市日跌停数为 0 时，
// Eastmoney 返回 tc=0 / pool=[]，客户端据此给出 HasData=false、QualityStatus=unknown，
// 并带上"无法区分零事件与日期不可用"的质量原因。
func emptyLimitPoolSnapshot(poolType, tradeDate string) *model.LimitPoolSnapshot {
	snapshot := limitPoolSnapshot(poolType, tradeDate, 0)
	snapshot.HasData = false
	snapshot.QualityStatus = model.QualityUnknown
	snapshot.QualityReasons = []string{"provider_returned_empty_pool_cannot_distinguish_zero_events_from_unavailable_date"}
	return snapshot
}

// TestBuildLimitPoolFactsPayloadTreatsEmptySiblingPoolAsZeroEvents 覆盖真实强势交易日：
// 涨停池有 400 多行、跌停池 tc=0。若把空池一律当成"数据不可用"，price_limit 在强势日
// 直接不可核验，整个 Run 会失败。空池的歧义是"该交易日是否可用"，而同一天另一个池
// 已返回真实行，说明日期可用，因此空池就是零事件。
func TestBuildLimitPoolFactsPayloadTreatsEmptySiblingPoolAsZeroEvents(t *testing.T) {
	up := limitPoolSnapshot("up", "2026-11-27", 3, "600519.SH", "600036.SH", "000001.SZ")
	down := emptyLimitPoolSnapshot("down", "2026-11-27")

	payload, err := BuildLimitPoolFactsPayload(up, down, "601398.SH")
	if err != nil {
		t.Fatalf("empty limit-down pool next to a populated limit-up pool must be usable: %v", err)
	}
	if payload.LimitStatus != LimitPoolStatusNone {
		t.Fatalf("status = %q, want none", payload.LimitStatus)
	}
	if !payload.UpPoolComplete || !payload.DownPoolComplete {
		t.Fatalf("complete=%v/%v, want both complete", payload.UpPoolComplete, payload.DownPoolComplete)
	}
	if payload.DownPoolTotal != 0 {
		t.Fatalf("down pool total = %d, want 0", payload.DownPoolTotal)
	}
	// 零事件是由同池兄弟存活性推断出来的，不是源端直接声明的完整池，质量必须降级。
	if payload.QualityStatus != QualityDegraded {
		t.Fatalf("quality = %q, want degraded", payload.QualityStatus)
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("payload rejected: %v", err)
	}
}

// TestBuildLimitPoolFactsPayloadWithBothPoolsEmptyStaysUnknown 覆盖两个池都为空的情况：
// 此时无法区分"当日零涨跌停"与"该交易日不可用"，只能 unknown，绝不允许 none。
func TestBuildLimitPoolFactsPayloadWithBothPoolsEmptyStaysUnknown(t *testing.T) {
	up := emptyLimitPoolSnapshot("up", "2026-11-27")
	down := emptyLimitPoolSnapshot("down", "2026-11-27")

	payload, err := BuildLimitPoolFactsPayload(up, down, "601398.SH")
	if err != nil {
		t.Fatalf("two empty pools must yield an unknown payload, not an error: %v", err)
	}
	if payload.LimitStatus != LimitPoolStatusUnknown {
		t.Fatalf("status = %q, want unknown", payload.LimitStatus)
	}
	if payload.UpPoolComplete || payload.DownPoolComplete {
		t.Fatalf("complete=%v/%v, want both incomplete when neither pool has rows",
			payload.UpPoolComplete, payload.DownPoolComplete)
	}
	if err := payload.Validate(); err != nil {
		t.Fatalf("payload rejected: %v", err)
	}
}

// TestBuildLimitPoolFactsPayloadRejectsEmptyPoolWithContradictoryQuality 锁定质量枚举的
// 唯一放宽点：空池只能由源端声明歧义（unknown）。声称有行却 unknown、或空池却声称
// valid/degraded，都属于自相矛盾，必须拒绝。
func TestBuildLimitPoolFactsPayloadRejectsEmptyPoolWithContradictoryQuality(t *testing.T) {
	up := limitPoolSnapshot("up", "2026-11-27", 1, "600519.SH")
	up.HasData = true
	up.QualityStatus = model.QualityUnknown
	down := limitPoolSnapshot("down", "2026-11-27", 0)
	if _, err := BuildLimitPoolFactsPayload(up, down, "600036.SH"); err == nil {
		t.Fatal("a populated pool with unknown quality must be rejected")
	}

	populated := limitPoolSnapshot("up", "2026-11-27", 1, "600519.SH")
	emptyButValid := limitPoolSnapshot("down", "2026-11-27", 0)
	emptyButValid.HasData = false
	if _, err := BuildLimitPoolFactsPayload(populated, emptyButValid, "600036.SH"); err == nil {
		t.Fatal("an empty pool claiming valid quality must be rejected")
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
		// 空池不再是非法输入：它表示当日零事件或日期不可用，由状态与完整性表达。
		// 真正非法的总量是否定值。
		{"negative total", func() *model.LimitPoolSnapshot {
			s := limitPoolSnapshot("up", "2026-11-27", -1)
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
