package service

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/cache"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/provider"
)

// TestLiveHistoricalProviderProbe exercises the production Probe service path
// against public providers. It is opt-in and never gates the regular CI suite.
func TestLiveHistoricalProviderProbe(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_HISTORICAL_PROBE") != "1" {
		t.Skip("set MARKET_DATA_LIVE_HISTORICAL_PROBE=1 to run production historical Probe against live providers")
	}

	const startDate, endDate = "2025-07-07", "2025-07-17"
	sina := provider.NewSina(20*time.Second, "")
	tencent := provider.NewTencent(20 * time.Second)
	eastmoney := provider.NewEastmoney(20*time.Second, "", "")
	yahoo := provider.NewYahooFinance(20*time.Second, "")

	all := []provider.NamedProvider{sina, tencent, eastmoney, yahoo}
	bars := []provider.DailyBarProvider{sina, tencent, eastmoney, yahoo}
	ds := NewWithCapabilities(
		all, nil, bars, cache.NewDisabled(), nil,
		time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil,
	)

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	adjustments := []model.AdjustmentBasis{
		model.AdjustmentNone,
		model.AdjustmentForward,
		model.AdjustmentBackward,
	}
	var reports []*model.ProviderProbeRun
	var failures []string
	for _, symbol := range []string{"600036.SH", "002982.SZ"} {
		run, err := ds.ProbeProviderCapabilitiesWithOptions(ctx, ProviderProbeOptions{
			Scope:               "all",
			Capabilities:        []string{"daily_bars"},
			Checks:              []string{"connectivity", "contract", "quality", "cross_provider"},
			Symbols:             []string{symbol},
			DailyBarStartDate:   startDate,
			DailyBarEndDate:     endDate,
			DailyBarAdjustments: adjustments,
			TimeoutSeconds:      25,
		})
		if err != nil {
			t.Fatalf("production historical Probe failed for %s: %v", symbol, err)
		}
		reports = append(reports, run)
		t.Logf("LIVE_HISTORICAL_PROBE_SUMMARY symbol=%s status=%s total=%d returned=%d degraded=%d failed=%d not_supported=%d persisted=%t", symbol, run.Status, run.Total, run.Returned, run.Degraded, run.Failed, run.NotSupported, run.Persisted)
		if run.Failed > 0 {
			failures = append(failures, symbol+": "+strings.Join(probeRunFailureSummaries(run), "; "))
		}
	}

	encoded, err := json.MarshalIndent(reports, "", "  ")
	if err != nil {
		t.Fatalf("marshal production historical Probe report: %v", err)
	}
	t.Logf("LIVE_HISTORICAL_PROBE_REPORT_BEGIN\n%s\nLIVE_HISTORICAL_PROBE_REPORT_END", encoded)
	if outputPath := strings.TrimSpace(os.Getenv("LIVE_HISTORICAL_PROBE_REPORT_PATH")); outputPath != "" {
		if writeErr := os.WriteFile(outputPath, append(encoded, '\n'), 0o644); writeErr != nil {
			t.Errorf("write production historical Probe report: %v", writeErr)
		}
	}
	if len(failures) > 0 {
		t.Errorf("production historical Probe reported provider failures: %s", strings.Join(failures, " | "))
	}
}

func probeRunFailureSummaries(run *model.ProviderProbeRun) []string {
	if run == nil {
		return nil
	}
	var summaries []string
	for _, item := range run.Items {
		if item.Status == "failed" {
			summaries = append(summaries, item.Provider+"/"+item.Capability+"/"+string(item.AdjustmentBasis)+": "+item.ErrorCode+" "+item.ErrorMessage)
		}
	}
	return summaries
}
