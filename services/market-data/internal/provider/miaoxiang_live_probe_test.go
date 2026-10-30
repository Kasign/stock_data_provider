package provider

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// TestLiveMiaoXiangCapabilities 默认跳过；显式开启后使用真实 MX API Key 验收补充数据能力。
func TestLiveMiaoXiangCapabilities(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_MIAOXIANG_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_MIAOXIANG_TESTS=1 to call MiaoXiang endpoints")
	}

	apiKey := os.Getenv("MARKET_DATA_MIAOXIANG_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("MIAOXIANG_API_KEY")
	}
	if apiKey == "" {
		t.Fatal("MARKET_DATA_LIVE_MIAOXIANG_TESTS=1 requires MARKET_DATA_MIAOXIANG_API_KEY or MIAOXIANG_API_KEY")
	}

	endpoint := os.Getenv("MARKET_DATA_MIAOXIANG_API_URL")
	client, err := NewMiaoXiang(apiKey, endpoint, 45*time.Second)
	if err != nil {
		t.Fatalf("initialize MiaoXiang client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	const symbol = "600519.SH"
	results := make([]optionalProviderProbeResult, 0, 2)
	failures := make([]string, 0, 2)

	run := func(capability string, call func() (int, model.QualityStatus, []string, error)) {
		started := time.Now()
		rows, quality, notes, callErr := call()
		item := optionalProviderProbeResult{
			Provider: "miaoxiang", Capability: capability, Status: "passed",
			ElapsedMS: time.Since(started).Milliseconds(), Rows: rows,
			Quality: string(quality), QualityNote: notes,
		}
		if callErr != nil {
			_, safeMessage := liveProbeErrorSummary(callErr)
			item.Status, item.Error = "failed", safeMessage
			failures = append(failures, capability+": "+safeMessage)
		}
		results = append(results, item)
	}

	run("chip_distribution", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchChipDistribution(ctx, symbol)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Symbol != symbol || item.Source != "miaoxiang" ||
			item.FetchedAt.IsZero() || item.AverageCost <= 0 {
			return 0, "", nil, errLiveProbeContract("chip_distribution identity/source/fetched_at/average_cost mismatch")
		}
		if item.QualityStatus != model.QualityDegraded {
			return 0, "", nil, errLiveProbeContract("MiaoXiang chip data must preserve publication-availability degraded status")
		}
		return 1, item.QualityStatus, item.QualityReasons, nil
	})

	run("capital_flow", func() (int, model.QualityStatus, []string, error) {
		item, err := client.FetchCapitalFlow(ctx, symbol)
		if err != nil {
			return 0, "", nil, err
		}
		if item == nil || item.Symbol != symbol || item.Source != "miaoxiang" ||
			item.FetchedAt.IsZero() || item.Observations <= 0 || item.ProviderDate == "" {
			return 0, "", nil, errLiveProbeContract("capital_flow identity/source/date/observations/fetched_at mismatch")
		}
		if item.QualityStatus != model.QualityDegraded {
			return 0, "", nil, errLiveProbeContract("MiaoXiang capital flow must preserve publication-availability degraded status")
		}
		return item.Observations, item.QualityStatus, item.QualityReasons, nil
	})

	logOptionalProviderProbe(t, results, failures)
}
