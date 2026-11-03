package service

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type priceLimitEventTestProvider struct {
	name     string
	evidence *model.PriceLimitEventEvidence
	err      error
}

func (p priceLimitEventTestProvider) Name() string { return p.name }
func (p priceLimitEventTestProvider) FetchPriceLimitEventEvidence(context.Context, string, string) (*model.PriceLimitEventEvidence, error) {
	return p.evidence, p.err
}

func validPriceLimitEventEvidence() *model.PriceLimitEventEvidence {
	return &model.PriceLimitEventEvidence{
		Symbol: "600519.SH", Status: model.PriceLimitExceptionNone,
		EffectiveFrom: "2026-10-29", Source: "exchange-event-test",
		SourceVersion: "2026-10-29-v1", QualityStatus: model.QualityValid,
		SourceURL: "https://example.com/events", ObservedAt: time.Now().UTC(),
		CoverageConfirmed: true,
	}
}

func TestPriceLimitEventEvidenceFailsClosedWithoutProvider(t *testing.T) {
	svc := &DataService{}
	if _, err := svc.PriceLimitEventEvidence(context.Background(), "600519.SH", "2026-10-29"); err == nil {
		t.Fatal("expected missing event provider to fail closed")
	}
}

func TestPriceLimitEventEvidenceAcceptsValidatedProviderEvidence(t *testing.T) {
	svc := &DataService{}
	svc.SetPriceLimitEventProviders([]provider.PriceLimitEventProvider{priceLimitEventTestProvider{name: "exchange-event-test", evidence: validPriceLimitEventEvidence()}})
	got, err := svc.PriceLimitEventEvidence(context.Background(), "600519.SH", "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != model.PriceLimitExceptionNone || !got.CoverageConfirmed {
		t.Fatalf("unexpected event evidence: %+v", got)
	}
}

func TestPriceLimitEventEvidenceRejectsMismatchedAndIncompleteEvidence(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*model.PriceLimitEventEvidence)
	}{
		{name: "symbol mismatch", mutate: func(e *model.PriceLimitEventEvidence) { e.Symbol = "000001.SZ" }},
		{name: "source mismatch", mutate: func(e *model.PriceLimitEventEvidence) { e.Source = "other-source" }},
		{name: "missing version", mutate: func(e *model.PriceLimitEventEvidence) { e.SourceVersion = "" }},
		{name: "unconfirmed absence", mutate: func(e *model.PriceLimitEventEvidence) { e.CoverageConfirmed = false }},
		{name: "unknown quality", mutate: func(e *model.PriceLimitEventEvidence) { e.QualityStatus = model.QualityUnknown }},
		{name: "date not covered", mutate: func(e *model.PriceLimitEventEvidence) { e.EffectiveFrom = "2026-10-30" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			evidence := validPriceLimitEventEvidence()
			tc.mutate(evidence)
			svc := &DataService{}
			svc.SetPriceLimitEventProviders([]provider.PriceLimitEventProvider{priceLimitEventTestProvider{name: "exchange-event-test", evidence: evidence}})
			if _, err := svc.PriceLimitEventEvidence(context.Background(), "600519.SH", "2026-10-29"); err == nil {
				t.Fatal("expected invalid evidence to be rejected")
			}
		})
	}
}
