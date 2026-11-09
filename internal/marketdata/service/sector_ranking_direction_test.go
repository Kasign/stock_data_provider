package service

import (
	"context"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

type legacySectorRankingProviderForDirectionTest struct {
	calls int
}

func (p *legacySectorRankingProviderForDirectionTest) Name() string { return "legacy-sector" }

func (p *legacySectorRankingProviderForDirectionTest) FetchSectorRankings(_ context.Context, category string, _ int) (*model.SectorRankingSnapshot, error) {
	p.calls++
	return &model.SectorRankingSnapshot{Category: category}, nil
}

func TestSectorRankingsByDirectionDoesNotUseLegacyProviderForBottomRanking(t *testing.T) {
	legacy := &legacySectorRankingProviderForDirectionTest{}
	svc := &DataService{
		cache:                  cache.NewDisabled(),
		sectorRankingProviders: []provider.SectorRankingProvider{legacy},
	}
	_, _, err := svc.SectorRankingsByDirection(context.Background(), "industry", "down", 10)
	if err == nil || !strings.Contains(err.Error(), "does not support down sector ranking") {
		t.Fatalf("legacy provider must not be misrepresented as a bottom-ranking source, got %v", err)
	}
	if legacy.calls != 0 {
		t.Fatalf("legacy top-only Provider must not be called for a bottom ranking, calls=%d", legacy.calls)
	}
}
