package provider

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// TestLiveTickFlowCapabilities 是 TickFlow 已实现但默认关闭能力的独立真实请求验收入口。
// 仅在显式设置 MARKET_DATA_LIVE_TICKFLOW_TESTS=1 且提供 API Key 时执行，避免普通 CI 依赖外部服务。
func TestLiveTickFlowCapabilities(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_TICKFLOW_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_TICKFLOW_TESTS=1 to call TickFlow live endpoints")
	}

	apiKey := os.Getenv("MARKET_DATA_TICKFLOW_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("TICKFLOW_API_KEY")
	}
	if apiKey == "" {
		t.Fatal("MARKET_DATA_LIVE_TICKFLOW_TESTS=1 requires MARKET_DATA_TICKFLOW_API_KEY or TICKFLOW_API_KEY")
	}

	baseURL := os.Getenv("MARKET_DATA_TICKFLOW_API_URL")
	if baseURL == "" {
		baseURL = os.Getenv("TICKFLOW_BASE_URL")
	}
	freeURL := os.Getenv("MARKET_DATA_TICKFLOW_FREE_API_URL")
	if freeURL == "" {
		freeURL = os.Getenv("TICKFLOW_FREE_BASE_URL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client := NewTickFlowConfigured(25*time.Second, baseURL, freeURL, apiKey)
	type result struct {
		Capability   string   `json:"capability"`
		Status       string   `json:"status"`
		ElapsedMS    int64    `json:"elapsed_ms"`
		Quality      string   `json:"quality,omitempty"`
		QualityNotes []string `json:"quality_notes,omitempty"`
		Rows         int      `json:"rows,omitempty"`
		Error        string   `json:"error,omitempty"`
	}

	results := make([]result, 0, 4)
	failures := make([]string, 0, 4)
	run := func(name string, call func() (int, string, []string, error)) {
		started := time.Now()
		rows, quality, notes, err := call()
		item := result{Capability: name, Status: "passed", ElapsedMS: time.Since(started).Milliseconds(), Rows: rows, Quality: quality, QualityNotes: notes}
		if err != nil {
			_, safeMessage := liveProbeErrorSummary(err)
			item.Status, item.Error = "failed", safeMessage
			failures = append(failures, name+": "+safeMessage)
		}
		results = append(results, item)
	}

	run("quote", func() (int, string, []string, error) {
		item, err := client.FetchQuote(ctx, "600519.SH")
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Symbol != "600519.SH" || item.Source != "tickflow" || item.FetchedAt.IsZero() || item.CurrentPrice == nil {
			return 0, "", nil, errLiveProbeContract("quote identity/source/time/price mismatch")
		}
		return 1, string(item.QualityStatus), item.QualityReasons, nil
	})

	run("index_quotes", func() (int, string, []string, error) {
		item, err := client.FetchIndexQuotes(ctx)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Source != "tickflow" || item.FetchedAt.IsZero() || len(item.Items) != len(tickflowCoreIndexDefinitions) {
			return 0, "", nil, errLiveProbeContract("core index snapshot coverage mismatch")
		}
		return len(item.Items), string(item.QualityStatus), item.QualityReasons, nil
	})

	run("market_breadth", func() (int, string, []string, error) {
		item, err := client.FetchMarketBreadth(ctx)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Market != "cn_a_share" || item.Source != "tickflow" ||
			item.TotalSecurities <= 0 || item.CoveredSecurities != item.TotalSecurities ||
			item.AdvancingCount+item.DecliningCount+item.UnchangedCount+item.UnclassifiedCount != item.CoveredSecurities {
			return 0, "", nil, errLiveProbeContract("market breadth universe/coverage/count contract mismatch")
		}
		return item.CoveredSecurities, string(item.QualityStatus), item.QualityReasons, nil
	})

	run("sector_rankings_industry", func() (int, string, []string, error) {
		item, err := client.FetchSectorRankings(ctx, "industry", 10)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Category != "industry" || item.Source != "tickflow" ||
			item.Total <= 0 || len(item.Items) == 0 || item.FetchedAt.IsZero() {
			return 0, "", nil, errLiveProbeContract("industry ranking identity/coverage mismatch")
		}
		// 当前 TickFlow 行业涨幅由申万一级成分股行情聚合计算，预期为 degraded，不得标记为源端完整排名。
		if item.QualityStatus != model.QualityDegraded {
			return 0, "", nil, errLiveProbeContract("constituent-derived industry ranking must remain degraded")
		}
		return len(item.Items), string(item.QualityStatus), item.QualityReasons, nil
	})

	encoded, _ := json.Marshal(results)
	t.Logf("LIVE_TICKFLOW_RESULT %s", encoded)
	if len(failures) > 0 {
		t.Fatalf("TickFlow live capability acceptance failed: %v", failures)
	}
}

func errLiveProbeContract(message string) error {
	return &liveProbeContractError{message: message}
}

type liveProbeContractError struct {
	message string
}

func (e *liveProbeContractError) Error() string { return e.message }
