package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// limitPoolFullFixture 模拟 push2ex 的分页响应：每页最多 30 行，从未分页的 tc 声明总量。
func limitPoolFullFixture(t *testing.T, total int, tradeDateCompact string, shortenPage map[int]int, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		query := r.URL.Query()
		page, err := strconv.Atoi(query.Get("Pageindex"))
		if err != nil {
			t.Errorf("invalid Pageindex: %v", err)
			http.Error(w, "bad page index", http.StatusBadRequest)
			return
		}
		if got := query.Get("pagesize"); got != "30" {
			t.Errorf("pagesize=%q, want 30", got)
		}
		if got := query.Get("date"); got != tradeDateCompact {
			t.Errorf("date=%q, want %s", got, tradeDateCompact)
		}
		start := page * 30
		end := start + 30
		if end > total {
			end = total
		}
		if start > total {
			start = total
		}
		if short, ok := shortenPage[page]; ok {
			end = start + short
		}
		rows := make([]map[string]any, 0, end-start)
		for i := start; i < end; i++ {
			rows = append(rows, map[string]any{
				"c": fmt.Sprintf("%06d", 600001+i), "n": fmt.Sprintf("测试股票%d", i+1),
				"m": "1", "zdp": 10.0,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rc": 0, "data": map[string]any{"tc": total, "qdate": tradeDateCompact, "pool": rows},
		})
	}))
}

func limitPoolFullTradeDate(t *testing.T) (iso, compact string) {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	yesterday := time.Now().In(loc).AddDate(0, 0, -1)
	return yesterday.Format("2006-01-02"), yesterday.Format("20060102")
}

// TestEastmoneyFetchFullLimitPoolExceedsCappedLimit 覆盖真正的缺陷场景：强市日涨停数量
// 超过 100 时，FetchLimitPool 在协议层面取不到全量（limit>100 直接报错），而全量入口必须
// 分页取满并让 Total == len(Items)。
func TestEastmoneyFetchFullLimitPoolExceedsCappedLimit(t *testing.T) {
	const total = 125
	tradeDateISO, tradeDateCompact := limitPoolFullTradeDate(t)
	calls := 0
	server := limitPoolFullFixture(t, total, tradeDateCompact, nil, &calls)
	defer server.Close()

	client := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "", "", server.URL, "",
	)
	got, err := client.FetchFullLimitPool(context.Background(), "up", tradeDateISO)
	if err != nil {
		t.Fatalf("FetchFullLimitPool: %v", err)
	}
	if calls != 5 {
		t.Fatalf("page calls=%d, want 5 pages for %d rows", calls, total)
	}
	if got.Total != total || len(got.Items) != total {
		t.Fatalf("full pool total=%d items=%d, want %d for both", got.Total, len(got.Items), total)
	}
	for _, reason := range got.QualityReasons {
		if reason == "provider_total_exceeds_returned_rows" {
			t.Fatalf("full fetch must not report partial coverage: %v", got.QualityReasons)
		}
	}
	if got.Items[0].Code != "600001" || got.Items[len(got.Items)-1].Code != "600125" {
		t.Fatalf("pagination range mismatch: first=%s last=%s", got.Items[0].Code, got.Items[len(got.Items)-1].Code)
	}

	// 同一份数据走限额入口时，超过 100 必须报错——这正是"取不到全量"的证据，
	// 也是全量入口存在的理由。
	if _, err := client.FetchLimitPool(context.Background(), "up", tradeDateISO, 101); err == nil {
		t.Fatal("FetchLimitPool must reject limit above 100 instead of paginating past it")
	} else if !strings.Contains(err.Error(), "must not exceed 100") {
		t.Fatalf("unexpected limit error: %v", err)
	}
}

// TestEastmoneyFetchFullLimitPoolRejectsTotalAboveSafetyBound 确认全量模式在源端总量不可信时
// fail-closed，而不是朝一个荒谬的总数翻几千页。
func TestEastmoneyFetchFullLimitPoolRejectsTotalAboveSafetyBound(t *testing.T) {
	const implausibleTotal = 100000
	tradeDateISO, tradeDateCompact := limitPoolFullTradeDate(t)
	calls := 0
	server := limitPoolFullFixture(t, implausibleTotal, tradeDateCompact, nil, &calls)
	defer server.Close()

	client := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "", "", server.URL, "",
	)
	_, err := client.FetchFullLimitPool(context.Background(), "up", tradeDateISO)
	if err == nil {
		t.Fatal("full fetch must reject a source total above the safety bound")
	}
	if !strings.Contains(err.Error(), "exceeds full-fetch safety bound") {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("page calls=%d, want 1: the bound must be checked before paging further", calls)
	}
}

// TestEastmoneyFetchFullLimitPoolRefusesPartialCoverage 是全量入口最重要的性质：源端某页
// 少返回行时，只能报错，绝不能返回"部分但自称完整"的快照。
func TestEastmoneyFetchFullLimitPoolRefusesPartialCoverage(t *testing.T) {
	const total = 125
	tradeDateISO, tradeDateCompact := limitPoolFullTradeDate(t)
	calls := 0
	server := limitPoolFullFixture(t, total, tradeDateCompact, map[int]int{2: 20}, &calls)
	defer server.Close()

	client := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "", "", server.URL, "",
	)
	got, err := client.FetchFullLimitPool(context.Background(), "up", tradeDateISO)
	if err == nil {
		t.Fatalf("full fetch must fail on short page, got snapshot total=%d items=%d", got.Total, len(got.Items))
	}
	if !strings.Contains(err.Error(), "coverage mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}
