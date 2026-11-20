package service

import (
	"math"
	"testing"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestComposeCorporateActionAffineTransformSeparatesExchangeReferenceAndTradableTotalReturn(t *testing.T) {
	dividend030 := 0.3
	dividend0591 := 0.591
	transfer100 := 1.0
	cashConsideration := 1.475
	shareConsideration := 0.12
	include := true
	exclude := false
	events := []model.CorporateAction{
		{
			ProviderEventID: "2006-05-19-company-distribution", EventType: "cash_dividend_and_transfer",
			ExDate: "2006-05-19", CashDividendBeforeTaxCNY: &dividend030, TransferShareRatio: &transfer100,
			HolderScope: model.CorporateActionHolderScopeAllShareholders, ReferencePriceIncluded: &include,
		},
		{
			ProviderEventID: "2006-05-24-company-dividend", EventType: "cash_dividend",
			ExDate: "2006-05-24", CashDividendBeforeTaxCNY: &dividend0591,
			HolderScope: model.CorporateActionHolderScopeAllShareholders, ReferencePriceIncluded: &include,
		},
		{
			ProviderEventID: "2006-05-24-share-reform-consideration", EventType: "share_reform_consideration",
			ExDate: "2006-05-24", CashConsiderationCNY: &cashConsideration, ShareConsiderationRatio: &shareConsideration,
			HolderScope: model.CorporateActionHolderScopeTradableShareholders, ReferencePriceIncluded: &exclude,
		},
	}

	exchange, err := composeCorporateActionAffineTransform(events, model.CorporateActionAdjustmentPolicyExchangeReference, "")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(exchange.PriceScale-2.0) > 1e-12 || math.Abs(exchange.PriceOffsetCNY-1.482) > 1e-12 {
		t.Fatalf("exchange-reference transform = %+v, want scale 2 and offset 1.482", exchange)
	}

	totalReturn, err := composeCorporateActionAffineTransform(events, model.CorporateActionAdjustmentPolicyTradableHolderTotalReturn, model.CorporateActionHolderScopeTradableShareholders)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(totalReturn.PriceScale-2.24) > 1e-12 || math.Abs(totalReturn.PriceOffsetCNY-4.432) > 1e-12 {
		t.Fatalf("tradable-holder total-return transform = %+v, want scale 2.24 and offset 4.432", totalReturn)
	}
}

func TestComposeCorporateActionAffineTransformReconstructsHithinkProviderCompatibility(t *testing.T) {
	compositeDividend := 4.132
	compositeBonus := 1.24
	separateDividend := 0.3
	events := []model.CorporateAction{
		{ProviderEventID: "hithink-2006-05-25", EventType: "provider_composite_adjustment", ExDate: "2006-05-25", CashDividendBeforeTaxCNY: &compositeDividend, BonusShareRatio: &compositeBonus},
		{ProviderEventID: "hithink-2006-05-19", EventType: "provider_composite_adjustment", ExDate: "2006-05-19", CashDividendBeforeTaxCNY: &separateDividend},
	}
	got, err := composeCorporateActionAffineTransform(events, model.CorporateActionAdjustmentPolicyProviderCompatibility, "")
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got.PriceScale-2.24) > 1e-12 || math.Abs(got.PriceOffsetCNY-4.432) > 1e-12 {
		t.Fatalf("Hithink compatibility transform = %+v, want scale 2.24 and offset 4.432", got)
	}
}

func TestComposeCorporateActionAffineTransformRejectsUnknownTreatmentAndRightsPrice(t *testing.T) {
	cash := 0.3
	unknown := []model.CorporateAction{{ProviderEventID: "unknown", ExDate: "2026-01-02", CashDividendBeforeTaxCNY: &cash}}
	if _, err := composeCorporateActionAffineTransform(unknown, model.CorporateActionAdjustmentPolicyExchangeReference, ""); err == nil {
		t.Fatal("unknown exchange-reference treatment must not be guessed")
	}
	rights := 0.2
	include := true
	missingRightsPrice := []model.CorporateAction{{
		ProviderEventID: "rights-without-price", ExDate: "2026-01-02", RightsIssueRatio: &rights,
		ReferencePriceIncluded: &include,
	}}
	if _, err := composeCorporateActionAffineTransform(missingRightsPrice, model.CorporateActionAdjustmentPolicyExchangeReference, ""); err == nil {
		t.Fatal("rights issue without subscription price must be rejected")
	}
}
