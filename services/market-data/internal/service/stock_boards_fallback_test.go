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

type stockBoardsFallbackTestProvider struct {
	name     string
	snapshot *model.StockBoardSnapshot
	err      error
	calls    int
}

func (p *stockBoardsFallbackTestProvider) Name() string { return p.name }

func (p *stockBoardsFallbackTestProvider) FetchStockBoards(_ context.Context, symbol string) (*model.StockBoardSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, errors.New("test stock-board snapshot is nil")
	}
	out := *p.snapshot
	out.Symbol = symbol
	out.Items = append([]model.StockBoard(nil), p.snapshot.Items...)
	return &out, nil
}

func stockBoardsFallbackService(providers ...provider.StockBoardsProvider) *DataService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, logger)
	svc.SetStockBoardsProviders(providers)
	return svc
}

func unknownStockBoardsForFallbackTest(source string) *model.StockBoardSnapshot {
	return &model.StockBoardSnapshot{
		Symbol: "600519.SH", Total: 0, HasData: false, Items: []model.StockBoard{},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityUnknown,
		QualityReasons: []string{"provider_empty_membership_unconfirmed"},
	}
}

func validStockBoardsForFallbackTest(source string) *model.StockBoardSnapshot {
	now := time.Now().UTC()
	return &model.StockBoardSnapshot{
		Symbol: "600519.SH", Total: 1, ProviderReturned: 1, HasData: true,
		Items: []model.StockBoard{{
			Code: "BK0450", Name: "白酒", Source: source, SourceVersion: "test-v1",
			FetchedAt: now, QualityStatus: model.QualityDegraded,
			QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		}},
		Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
}

func TestStockBoardsFallsBackFromUnknownQualityRows(t *testing.T) {
	firstSnapshot := validStockBoardsForFallbackTest("eastmoney")
	firstSnapshot.QualityStatus = model.QualityUnknown
	firstSnapshot.QualityReasons = []string{"provider_board_quality_unknown"}
	first := &stockBoardsFallbackTestProvider{name: "eastmoney", snapshot: firstSnapshot}
	fallback := &stockBoardsFallbackTestProvider{name: "hithink", snapshot: validStockBoardsForFallbackTest("hithink")}
	svc := stockBoardsFallbackService(first, fallback)
	got, hit, err := svc.StockBoards(context.Background(), "600519.SH")
	if err != nil || hit { t.Fatalf("StockBoards returned hit=%v err=%v", hit, err) }
	if got == nil || got.Source != "hithink" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected degraded fallback rather than unknown board rows, got %+v", got)
	}
	if first.calls != 1 || fallback.calls != 1 { t.Fatalf("expected both providers to be attempted, calls=%d/%d", first.calls, fallback.calls) }
}

func TestStockBoardsReturnsUnknownRowsOnlyAfterFallbackExhaustion(t *testing.T) {
	candidate := validStockBoardsForFallbackTest("eastmoney")
	candidate.QualityStatus = model.QualityUnknown
	candidate.QualityReasons = []string{"provider_board_quality_unknown"}
	first := &stockBoardsFallbackTestProvider{name: "eastmoney", snapshot: candidate}
	failed := &stockBoardsFallbackTestProvider{name: "hithink", err: errors.New("test provider unavailable")}
	svc := stockBoardsFallbackService(first, failed)
	got, hit, err := svc.StockBoards(context.Background(), "600519.SH")
	if err != nil || hit { t.Fatalf("StockBoards returned hit=%v err=%v", hit, err) }
	if got == nil || !got.HasData || got.QualityStatus != model.QualityUnknown ||
		!strings.Contains(strings.Join(got.QualityReasons, ","), "stock_boards_fallback_exhausted_with_unconfirmed_quality") {
		t.Fatalf("expected retained unknown rows after fallback exhaustion, got %+v", got)
	}
	if first.calls != 1 || failed.calls != 1 { t.Fatalf("expected all providers to be attempted, calls=%d/%d", first.calls, failed.calls) }
}

func TestStockBoardsFallsBackAfterUnknownEmptyProvider(t *testing.T) {
	empty := &stockBoardsFallbackTestProvider{name: "eastmoney", snapshot: unknownStockBoardsForFallbackTest("eastmoney")}
	fallback := &stockBoardsFallbackTestProvider{name: "hithink", snapshot: validStockBoardsForFallbackTest("hithink")}
	svc := stockBoardsFallbackService(empty, fallback)

	got, hit, err := svc.StockBoards(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("StockBoards returned hit=%v err=%v", hit, err)
	}
	if got == nil || !got.HasData || got.Source != "hithink" || len(got.Items) != 1 {
		t.Fatalf("expected fallback board membership, got %+v", got)
	}
	if empty.calls != 1 || fallback.calls != 1 {
		t.Fatalf("expected both sources to be tried once, eastmoney=%d hithink=%d", empty.calls, fallback.calls)
	}
}

func TestStockBoardsCachesUnknownOnlyAfterFallbackExhaustion(t *testing.T) {
	empty := &stockBoardsFallbackTestProvider{name: "eastmoney", snapshot: unknownStockBoardsForFallbackTest("eastmoney")}
	failed := &stockBoardsFallbackTestProvider{name: "hithink", err: errors.New("test provider unavailable")}
	mem := newMemoryCache()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, logger)
	svc.SetStockBoardsProviders([]provider.StockBoardsProvider{empty, failed})

	got, hit, err := svc.StockBoards(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("StockBoards returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.HasData || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected unknown empty result after fallback exhaustion, got %+v", got)
	}
	hasReason := false
	for _, reason := range got.QualityReasons {
		if reason == "stock_boards_fallback_exhausted_without_confirmed_membership" {
			hasReason = true
		}
	}
	if !hasReason {
		t.Fatalf("missing fallback exhaustion reason: %+v", got.QualityReasons)
	}
	if empty.calls != 1 || failed.calls != 1 {
		t.Fatalf("expected both sources to be tried once, eastmoney=%d hithink=%d", empty.calls, failed.calls)
	}
	if strings.TrimSpace(got.Source) == "" {
		t.Fatal("unknown result must preserve its provider source")
	}
}
