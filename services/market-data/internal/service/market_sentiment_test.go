package service

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type sentimentBreadthProvider struct{ snapshot *model.MarketBreadthSnapshot }
func (p sentimentBreadthProvider) Name() string { return "eastmoney" }
func (p sentimentBreadthProvider) FetchMarketBreadth(context.Context) (*model.MarketBreadthSnapshot, error) {
	return p.snapshot, nil
}

func TestMarketSentimentKeepsUnavailablePoolCountsNull(t *testing.T) {
	now := time.Now().UTC()
	reportDate := time.Now().In(marketLocation()).Format("2006-01-02")
	breadth := &model.MarketBreadthSnapshot{
		Market: "cn_a_share", ProviderReportDate: reportDate,
		TotalSecurities: 3, CoveredSecurities: 3,
		AdvancingCount: 1, DecliningCount: 1, UnchangedCount: 1,
		UnclassifiedCount: 0, QuoteTimestampCoverageCount: 0,
		Source: "eastmoney", SourceVersion: "eastmoney-test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
	ds := NewWithCapabilities([]provider.NamedProvider{sentimentBreadthProvider{snapshot: breadth}}, nil, nil,
		cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	ds.SetMarketBreadthProviders([]provider.MarketBreadthProvider{sentimentBreadthProvider{snapshot: breadth}})
	snapshot, err := ds.MarketSentiment(context.Background(), "")
	if err != nil { t.Fatalf("MarketSentiment() error = %v", err) }
	if snapshot.TotalSecurities != 3 || snapshot.AdvancingCount != 1 { t.Fatalf("unexpected breadth values: %+v", snapshot) }
	if snapshot.LimitUpCount != nil || snapshot.LimitDownCount != nil || snapshot.BrokenBoardCount != nil {
		t.Fatalf("unavailable pool counts must remain null: %+v", snapshot)
	}
	for _, reason := range []string{"limit_pool_up_unavailable", "limit_pool_down_unavailable", "limit_pool_broken_unavailable"} {
		found := false
		for _, got := range snapshot.QualityReasons { if got == reason { found = true; break } }
		if !found { t.Errorf("missing quality reason %q in %v", reason, snapshot.QualityReasons) }
	}
	if snapshot.QualityStatus != model.QualityDegraded { t.Fatalf("quality_status = %q, want degraded", snapshot.QualityStatus) }
}

func TestMarketSentimentDoesNotCollectHistoricalDate(t *testing.T) {
	ds := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	yesterday := time.Now().In(marketLocation()).AddDate(0, 0, -1).Format("2006-01-02")
	if _, err := ds.MarketSentiment(context.Background(), yesterday); err == nil {
		t.Fatal("historical date must not trigger a live collection")
	}
}
