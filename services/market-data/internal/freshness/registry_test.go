package freshness

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestPolicyForDomain(t *testing.T) {
	tests := []struct {
		domain string
		age    time.Duration
	}{
		{DomainQuote, 90 * time.Second}, {DomainIndex, 120 * time.Second},
		{DomainMarketBreadth, 5 * time.Minute}, {DomainSector, 5 * time.Minute},
		{DomainFundFlow, 5 * time.Minute},
	}
	for _, tt := range tests {
		policy, ok := PolicyForDomain(tt.domain)
		if !ok || policy.Domain != tt.domain || policy.MaxAge != tt.age {
			t.Errorf("PolicyForDomain(%q) = %+v, %v", tt.domain, policy, ok)
		}
	}
	if _, ok := PolicyForDomain("unknown"); ok {
		t.Fatal("unknown domain unexpectedly has a policy")
	}
}

func TestPolicyForDomainReturnsIndependentPhaseSlice(t *testing.T) {
	first, _ := PolicyForDomain(DomainQuote)
	first.ActivePhases[0] = "corrupted"
	second, _ := PolicyForDomain(DomainQuote)
	if second.ActivePhases[0] == "corrupted" {
		t.Fatal("registry policy was mutated by caller")
	}
}

func TestQuoteEvidenceUsesProviderTimeOnly(t *testing.T) {
	sourceTime := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	fetchedAt := sourceTime.Add(time.Minute)
	evidence := QuoteEvidence(&model.Quote{QuoteTime: &sourceTime,
		QuoteTimeSemantics: "provider_snapshot_time", FetchedAt: fetchedAt})
	if evidence.DataTime == nil || !evidence.DataTime.Equal(sourceTime) ||
		evidence.DataTimeType != "provider_snapshot_time" {
		t.Fatalf("unexpected quote evidence: %+v", evidence)
	}
	if evidence.FetchedAt == nil || !evidence.FetchedAt.Equal(fetchedAt) {
		t.Fatalf("unexpected fetched time: %+v", evidence.FetchedAt)
	}
}

func TestMarketBreadthUsesOldestComponentTimestamp(t *testing.T) {
	oldest := time.Date(2026, 11, 3, 9, 59, 0, 0, time.UTC)
	newest := oldest.Add(4 * time.Minute)
	evidence := MarketBreadthEvidence(&model.MarketBreadthSnapshot{
		OldestQuoteTime: &oldest, NewestQuoteTime: &newest, FetchedAt: newest.Add(time.Second)})
	if evidence.DataTime == nil || !evidence.DataTime.Equal(oldest) ||
		evidence.DataTimeType != "oldest_component_quote_time" {
		t.Fatalf("unexpected breadth evidence: %+v", evidence)
	}
}

func TestSectorAndFundFlowDoNotPromoteBusinessDateOrFetchedAt(t *testing.T) {
	fetchedAt := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	sector := SectorEvidence(&model.SectorRankingSnapshot{ProviderReportDate: "2026-11-03", FetchedAt: fetchedAt})
	flow := FundFlowEvidence(&model.MarketCapitalFlowSnapshot{
		StartDate: "2026-11-03", EndDate: "2026-11-03", FetchedAt: fetchedAt})
	for name, evidence := range map[string]Evidence{"sector": sector, "fund_flow": flow} {
		if evidence.DataTime != nil || evidence.DataTimeType != "" {
			t.Errorf("%s fabricated source time: %+v", name, evidence)
		}
		if evidence.FetchedAt == nil || !evidence.FetchedAt.Equal(fetchedAt) {
			t.Errorf("%s lost collection time: %+v", name, evidence.FetchedAt)
		}
	}
}
