package service

import (
	"math"
	"testing"
	"time"

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

func TestBuildCorporateActionAdjustmentFactorsIsProviderIndependentAndPreservesRawVolume(t *testing.T) {
	fetched := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	available := fetched.Add(-time.Hour)
	observed := fetched.Add(time.Minute)
	yes := true
	cash, bonus, transfer := 0.60, 0.10, 0.05
	event := model.CorporateAction{
		Symbol: "600519.SH", ProviderEventID: "div-2026", EventType: "dividend",
		NormalizedStatus: "implemented", TermsComplete: true,
		HolderScope: model.CorporateActionHolderScopeAllShareholders, ReferencePriceIncluded: &yes,
		ExDate: "2026-01-05", CashDividendBeforeTaxCNY: &cash,
		BonusShareRatio: &bonus, TransferShareRatio: &transfer,
		Source: "tushare", SourceVersion: "tushare-dividend-v1",
		FetchedAt: fetched, AvailableAt: &available, QualityStatus: model.QualityValid,
	}
	volume := int64(123456)
	amount := 456789.12
	rawBars := func(source, version string) []model.DailyBar {
		return []model.DailyBar{
			{Symbol: "600519.SH", TradeDate: "2026-01-02", Open: 100, High: 102, Low: 99, Close: 101,
				Volume: &volume, Amount: &amount, Source: source, SourceVersion: version, AdjustmentBasis: model.AdjustmentNone},
			{Symbol: "600519.SH", TradeDate: "2026-01-05", Open: 90, High: 92, Low: 89, Close: 91,
				Volume: &volume, Amount: &amount, Source: source, SourceVersion: version, AdjustmentBasis: model.AdjustmentNone},
			{Symbol: "600519.SH", TradeDate: "2026-01-06", Open: 91, High: 93, Low: 90, Close: 92,
				Volume: &volume, Amount: &amount, Source: source, SourceVersion: version, AdjustmentBasis: model.AdjustmentNone},
		}
	}
	for _, raw := range []struct{ source, version string }{{"hithink", "hithink-v1"}, {"baostock", "baostock-v2"}} {
		factors, err := buildCorporateActionAdjustmentFactors(
			rawBars(raw.source, raw.version), []model.CorporateAction{event},
			model.CorporateActionAdjustmentPolicyExchangeReference, "",
			true, fetched, &available,
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(factors) != 3 || math.Abs(factors[1].Factor-1.15) > 1e-12 ||
			factors[1].PriceOffsetCNY == nil || math.Abs(*factors[1].PriceOffsetCNY-0.60) > 1e-12 {
			t.Fatalf("%s: derived affine factor mismatch: %+v", raw.source, factors)
		}
		for i := range factors {
			factors[i].SystemObservedAt = &observed
		}
		adjusted, err := rebuildAdjustedDailyBarsWithModeAt(
			rawBars(raw.source, raw.version), factors, model.AdjustmentBackward, "",
			adjustmentFactorRebuildCurrent, observed,
		)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(adjusted[1].Open-(90*1.15+0.60)) > 1e-12 ||
			math.Abs(adjusted[1].Close-(91*1.15+0.60)) > 1e-12 {
			t.Fatalf("%s: adjusted OHLC mismatch: %+v", raw.source, adjusted[1])
		}
		for _, bar := range adjusted {
			if bar.Volume == nil || *bar.Volume != volume || bar.Amount == nil || *bar.Amount != amount {
				t.Fatalf("%s: price adjustment changed raw volume/amount: %+v", raw.source, bar)
			}
		}
		if factors[1].Source != raw.source || factors[1].SourceVersion != raw.version ||
			factors[1].Derivation == "" || len(factors[1].EvidenceSources) != 1 ||
			factors[1].EvidenceSources[0] != "tushare@tushare-dividend-v1" ||
			len(factors[1].EvidenceRevisionHashes) != 1 {
			t.Fatalf("%s: factor target binding or event provenance missing: %+v", raw.source, factors[1])
		}
	}
}

func TestBuildCorporateActionAdjustmentFactorsDeduplicatesProvidersAndRejectsConflicts(t *testing.T) {
	fetched := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	available := fetched.Add(-time.Hour)
	yes := true
	bonus, transfer := 0.10, 0.0
	newEvent := func(source, version, id string, cashValue float64) model.CorporateAction {
		return model.CorporateAction{
			Symbol: "600519.SH", ProviderEventID: id, EventType: "dividend",
			NormalizedStatus: "implemented", TermsComplete: true,
			HolderScope: model.CorporateActionHolderScopeAllShareholders, ReferencePriceIncluded: &yes,
			ExDate: "2026-01-05", CashDividendBeforeTaxCNY: &cashValue,
			BonusShareRatio: &bonus, TransferShareRatio: &transfer,
			Source: source, SourceVersion: version, FetchedAt: fetched,
			AvailableAt: &available, QualityStatus: model.QualityValid,
		}
	}
	raw := []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-01-05", Source: "hithink", SourceVersion: "hithink-v1", AdjustmentBasis: model.AdjustmentNone}}
	events := []model.CorporateAction{
		newEvent("tushare", "v1", "ts-dividend", 0.30),
		newEvent("hithink", "v2", "ht-dividend", 0.30),
	}
	factors, err := buildCorporateActionAdjustmentFactors(
		raw, events, model.CorporateActionAdjustmentPolicyExchangeReference, "",
		true, fetched, &available,
	)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(factors[0].Factor-1.10) > 1e-12 || math.Abs(*factors[0].PriceOffsetCNY-0.30) > 1e-12 {
		t.Fatalf("duplicate event was counted more than once: %+v", factors[0])
	}
	if len(factors[0].EvidenceSources) != 2 || len(factors[0].EvidenceRevisionHashes) != 2 {
		t.Fatalf("all provider evidence must remain traceable: %+v", factors[0])
	}
	conflict := newEvent("hithink", "v2", "ht-conflict", 0.40)
	if _, err := buildCorporateActionAdjustmentFactors(
		raw, []model.CorporateAction{events[0], conflict},
		model.CorporateActionAdjustmentPolicyExchangeReference, "",
		true, fetched, &available,
	); err == nil {
		t.Fatal("conflicting same-date same-type terms must fail closed")
	}
}

func TestBuildCorporateActionAdjustmentFactorsComposesEventsChronologically(t *testing.T) {
	fetched := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	available := fetched.Add(-time.Hour)
	yes := true
	zero := 0.0
	newEvent := func(id, date string, cashValue, bonusValue float64) model.CorporateAction {
		return model.CorporateAction{
			Symbol: "600519.SH", ProviderEventID: id, EventType: "dividend",
			NormalizedStatus: "implemented", TermsComplete: true,
			HolderScope: model.CorporateActionHolderScopeAllShareholders, ReferencePriceIncluded: &yes,
			ExDate: date, CashDividendBeforeTaxCNY: &cashValue,
			BonusShareRatio: &bonusValue, TransferShareRatio: &zero,
			Source: "tushare", SourceVersion: "v1", FetchedAt: fetched,
			AvailableAt: &available, QualityStatus: model.QualityValid,
		}
	}
	raw := []model.DailyBar{
		{Symbol: "600519.SH", TradeDate: "2026-01-02", Source: "hithink", SourceVersion: "v1", AdjustmentBasis: model.AdjustmentNone},
		{Symbol: "600519.SH", TradeDate: "2026-01-05", Source: "hithink", SourceVersion: "v1", AdjustmentBasis: model.AdjustmentNone},
		{Symbol: "600519.SH", TradeDate: "2026-02-05", Source: "hithink", SourceVersion: "v1", AdjustmentBasis: model.AdjustmentNone},
	}
	events := []model.CorporateAction{
		newEvent("event-2", "2026-02-05", 0.30, 0.20),
		newEvent("event-1", "2026-01-05", 0.20, 0.10),
	}
	factors, err := buildCorporateActionAdjustmentFactors(
		raw, events, model.CorporateActionAdjustmentPolicyExchangeReference, "",
		true, fetched, &available,
	)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(factors[0].Factor-1) > 1e-12 || math.Abs(*factors[0].PriceOffsetCNY) > 1e-12 {
		t.Fatalf("pre-event factor mismatch: %+v", factors[0])
	}
	if math.Abs(factors[1].Factor-1.1) > 1e-12 || math.Abs(*factors[1].PriceOffsetCNY-0.2) > 1e-12 {
		t.Fatalf("first event factor mismatch: %+v", factors[1])
	}
	if math.Abs(factors[2].Factor-1.32) > 1e-12 || math.Abs(*factors[2].PriceOffsetCNY-0.53) > 1e-12 {
		t.Fatalf("cumulative factor mismatch: %+v", factors[2])
	}
}
