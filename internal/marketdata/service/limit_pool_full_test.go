package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

// fullLimitPoolTestProvider 只实现全量接口，用于验证 DataService 的全量入口语义。
type fullLimitPoolTestProvider struct {
	name     string
	snapshot *model.LimitPoolSnapshot
	err      error
	calls    int
}

func (p *fullLimitPoolTestProvider) Name() string { return p.name }

func (p *fullLimitPoolTestProvider) FetchLimitPool(
	context.Context, string, string, int,
) (*model.LimitPoolSnapshot, error) {
	return nil, errors.New("capped interface must not be used by the full entry point")
}

func (p *fullLimitPoolTestProvider) FetchFullLimitPool(
	_ context.Context, poolType, tradeDate string,
) (*model.LimitPoolSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, errors.New("test snapshot is nil")
	}
	out := *p.snapshot
	out.PoolType = poolType
	out.TradeDate = tradeDate
	return &out, nil
}

// TestFetchFullLimitPoolReturnsZeroEventPool 覆盖真实强势交易日：跌停池 tc=0、pool=[]，
// 客户端给出 HasData=false。这不是"源不可用"，而是"当日该池零事件"。
// 该歧义必须留给载荷层结合同日兄弟池判定；取数层把它升级成硬错误会让 price_limit
// 在没有跌停的日子整体不可用。
func TestFetchFullLimitPoolReturnsZeroEventPool(t *testing.T) {
	tradeDate := time.Now().In(marketLocation()).Format("2006-01-02")
	svc := newLimitPoolFallbackService(&fullLimitPoolTestProvider{
		name:     "eastmoney",
		snapshot: unknownEmptyLimitPoolForTest("eastmoney", tradeDate),
	})

	got, err := svc.FetchFullLimitPool(context.Background(), "down", tradeDate)
	if err != nil {
		t.Fatalf("zero-event pool must be returned instead of rejected: %v", err)
	}
	if got == nil || got.Total != 0 || len(got.Items) != 0 {
		t.Fatalf("expected an empty pool snapshot, got %+v", got)
	}
	if got.HasData {
		t.Fatal("empty pool must not claim data")
	}
	if got.TradeDate != tradeDate || got.Source != "eastmoney" {
		t.Fatalf("empty pool lost identity: %+v", got)
	}
}

// TestFetchFullLimitPoolRejectsPartialPool 确认「部分池冒充完整池」仍是硬错误。
func TestFetchFullLimitPoolRejectsPartialPool(t *testing.T) {
	tradeDate := time.Now().In(marketLocation()).Format("2006-01-02")
	partial := dataLimitPoolForTest("eastmoney", tradeDate)
	partial.Total = 2 // 只返回 1 条
	svc := newLimitPoolFallbackService(&fullLimitPoolTestProvider{name: "eastmoney", snapshot: partial})

	if _, err := svc.FetchFullLimitPool(context.Background(), "up", tradeDate); err == nil {
		t.Fatal("partial pool must be rejected")
	} else if !errors.Is(err, ErrLimitPoolSourceIncomplete) {
		t.Fatalf("partial pool error = %v, want ErrLimitPoolSourceIncomplete", err)
	}
}

// TestFetchFullLimitPoolClassifiesUnavailableSource 确认网络/协议失败被归类为「源不可用」，
// 与「源给了脏数据」区分开：前者重试有意义，后者没有。
func TestFetchFullLimitPoolClassifiesUnavailableSource(t *testing.T) {
	tradeDate := time.Now().In(marketLocation()).Format("2006-01-02")
	svc := newLimitPoolFallbackService(&fullLimitPoolTestProvider{
		name: "eastmoney",
		err:  errors.New("dial tcp: connection reset by peer"),
	})

	if _, err := svc.FetchFullLimitPool(context.Background(), "up", tradeDate); err == nil {
		t.Fatal("unavailable source must return an error")
	} else if !errors.Is(err, ErrLimitPoolSourceUnavailable) {
		t.Fatalf("unavailable source error = %v, want ErrLimitPoolSourceUnavailable", err)
	}
}

// TestFetchFullLimitPoolRequiresFullProvider 确认只有限额接口的来源不能被当作全量来源。
func TestFetchFullLimitPoolRequiresFullProvider(t *testing.T) {
	tradeDate := time.Now().In(marketLocation()).Format("2006-01-02")
	svc := newLimitPoolFallbackService(&limitPoolFallbackTestProvider{
		name:     "capped-only",
		snapshot: dataLimitPoolForTest("capped-only", tradeDate),
	})

	if _, err := svc.FetchFullLimitPool(context.Background(), "up", tradeDate); err == nil {
		t.Fatal("capped-only provider must not satisfy the full entry point")
	} else if !errors.Is(err, ErrLimitPoolSourceUnavailable) {
		t.Fatalf("capped-only error = %v, want ErrLimitPoolSourceUnavailable", err)
	}
}
