package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type limitPoolFallbackTestProvider struct {
	name     string
	snapshot *model.LimitPoolSnapshot
	err      error
	calls    int
}

func (p *limitPoolFallbackTestProvider) Name() string { return p.name }

func (p *limitPoolFallbackTestProvider) FetchLimitPool(_ context.Context, poolType, tradeDate string, _ int) (*model.LimitPoolSnapshot, error) {
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

func newLimitPoolFallbackService(providers ...provider.LimitPoolProvider) *DataService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, logger)
	svc.SetLimitPoolProviders(providers)
	return svc
}

func unknownEmptyLimitPoolForTest(source, tradeDate string) *model.LimitPoolSnapshot {
	return &model.LimitPoolSnapshot{
		PoolType: "up", TradeDate: tradeDate, Total: 0, HasData: false,
		Items: []model.LimitPoolItem{}, Source: source, SourceVersion: "test-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityUnknown,
		QualityReasons: []string{"empty_limit_pool_cannot_confirm_zero_events_or_unavailable_source"},
	}
}

func dataLimitPoolForTest(source, tradeDate string) *model.LimitPoolSnapshot {
	change := 10.0
	now := time.Now().UTC()
	return &model.LimitPoolSnapshot{
		PoolType: "up", TradeDate: tradeDate, ProviderReportDate: tradeDate,
		Total: 1, HasData: true,
		Items: []model.LimitPoolItem{{
			Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", ChangePercent: &change,
			Source: source, SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"test_provider_timestamp_not_exposed"},
		}},
		Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"test_provider_timestamp_not_exposed"},
	}
}

func TestLimitPoolFallsBackAfterUnknownEmptyProvider(t *testing.T) {
	tradeDate := time.Now().In(marketLocation()).Format("2006-01-02")
	empty := &limitPoolFallbackTestProvider{name: "eastmoney", snapshot: unknownEmptyLimitPoolForTest("eastmoney", tradeDate)}
	fallback := &limitPoolFallbackTestProvider{name: "hithink", snapshot: dataLimitPoolForTest("hithink", tradeDate)}
	svc := newLimitPoolFallbackService(empty, fallback)

	got, cacheHit, err := svc.LimitPool(context.Background(), "up", tradeDate, 100)
	if err != nil {
		t.Fatalf("LimitPool returned error: %v", err)
	}
	if cacheHit {
		t.Fatal("disabled-cache fixture must not report a cache hit")
	}
	if got == nil || !got.HasData || got.Source != "hithink" || len(got.Items) != 1 {
		t.Fatalf("expected fallback provider data after unknown empty response, got %+v", got)
	}
	if empty.calls != 1 || fallback.calls != 1 {
		t.Fatalf("expected each provider to be called once, eastmoney=%d hithink=%d", empty.calls, fallback.calls)
	}
}

func TestLimitPoolReturnsUnknownWhenAllProvidersExhaustWithoutConfirmedData(t *testing.T) {
	tradeDate := time.Now().In(marketLocation()).Format("2006-01-02")
	empty := &limitPoolFallbackTestProvider{name: "eastmoney", snapshot: unknownEmptyLimitPoolForTest("eastmoney", tradeDate)}
	failed := &limitPoolFallbackTestProvider{name: "hithink", err: errors.New("test endpoint unavailable")}
	svc := newLimitPoolFallbackService(empty, failed)

	got, cacheHit, err := svc.LimitPool(context.Background(), "up", tradeDate, 100)
	if err != nil {
		t.Fatalf("unknown empty response should be returned explicitly after fallback exhaustion: %v", err)
	}
	if cacheHit {
		t.Fatal("unknown empty result must not be a cache hit")
	}
	if got == nil || got.HasData || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected an unknown empty result rather than a confirmed zero pool: %+v", got)
	}
	foundReason := false
	for _, reason := range got.QualityReasons {
		if reason == "limit_pool_fallback_exhausted_without_confirmed_data" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Fatalf("missing fallback-exhausted quality reason: %+v", got.QualityReasons)
	}
	if empty.calls != 1 || failed.calls != 1 {
		t.Fatalf("expected all providers to be tried once, eastmoney=%d hithink=%d", empty.calls, failed.calls)
	}
	if strings.TrimSpace(got.Source) == "" {
		t.Fatal("unknown result must preserve the source that returned the unknown empty snapshot")
	}
}
