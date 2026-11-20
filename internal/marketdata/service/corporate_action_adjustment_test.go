package service

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestDeriveAdjustmentFactorsFromCorporateActionsIsProviderIndependent(t *testing.T) {
	fetched := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	available := fetched.Add(-time.Hour)
	yes := true
	cash, bonus, transfer := 0.60, 0.10, 0.05
	action := model.CorporateAction{Symbol:"600519.SH", ProviderEventID:"div-2026", EventType:"dividend",
		NormalizedStatus:"implemented", HolderScope:model.CorporateActionHolderScopeAllShareholders,
		ReferencePriceIncluded:&yes, TermsComplete:true, ExDate:"2026-01-05",
		CashDividendBeforeTaxCNY:&cash, BonusShareRatio:&bonus, TransferShareRatio:&transfer,
		Source:"tushare", SourceVersion:"tushare-dividend-v1", FetchedAt:fetched, AvailableAt:&available, QualityStatus:model.QualityValid}
	rawBars := func(source, version string) []model.DailyBar {
		return []model.DailyBar{
			{Symbol:"600519.SH",TradeDate:"2026-01-02",Source:source,SourceVersion:version,AdjustmentBasis:model.AdjustmentNone},
			{Symbol:"600519.SH",TradeDate:"2026-01-05",Source:source,SourceVersion:version,AdjustmentBasis:model.AdjustmentNone},
			{Symbol:"600519.SH",TradeDate:"2026-01-06",Source:source,SourceVersion:version,AdjustmentBasis:model.AdjustmentNone},
		}
	}
	for _, raw := range []struct{source,version string}{{"hithink","hithink-v1"},{"baostock","baostock-v2"}} {
		factors,err:=deriveAdjustmentFactorsFromCorporateActions(rawBars(raw.source,raw.version),[]model.CorporateAction{action},
			model.CorporateActionAdjustmentPolicyExchangeReference,true,fetched,&available)
		if err!=nil { t.Fatal(err) }
		if len(factors)!=3 { t.Fatalf("%s: got %d factors",raw.source,len(factors)) }
		if factors[0].Factor!=1 || factors[0].PriceOffsetCNY==nil || *factors[0].PriceOffsetCNY!=0 { t.Fatalf("%s: pre-event factor %+v",raw.source,factors[0]) }
		for _,factor:=range factors[1:] {
			if factor.Factor!=1.15 || factor.PriceOffsetCNY==nil || *factor.PriceOffsetCNY!=0.60 { t.Fatalf("%s: affine factor %+v",raw.source,factor) }
			if factor.Source!=raw.source || factor.SourceVersion!=raw.version || factor.Derivation!=corporateActionExchangeReferenceDerivation ||
				len(factor.EvidenceSources)!=1 || factor.EvidenceSources[0]!="tushare@tushare-dividend-v1" || len(factor.EvidenceRevisionHashes)!=1 ||
				factor.AvailableAt==nil || factor.QualityStatus!=model.QualityValid { t.Fatalf("%s: missing factor provenance %+v",raw.source,factor) }
		}
	}
}

func TestDeriveAdjustmentFactorsFromCorporateActionsCombinesSameDateRightsAndDividend(t *testing.T) {
	fetched:=time.Date(2026,11,20,8,0,0,0,time.UTC); available:=fetched.Add(-time.Hour); yes:=true
	zero,bonus,transfer,cash,rights,rightsPrice:=0.0,0.10,0.05,0.60,0.20,8.0
	base:=model.CorporateAction{Symbol:"600519.SH",NormalizedStatus:"implemented",HolderScope:model.CorporateActionHolderScopeAllShareholders,
		ReferencePriceIncluded:&yes,TermsComplete:true,ExDate:"2026-01-05",Source:"source-a",SourceVersion:"v1",
		FetchedAt:fetched,AvailableAt:&available,QualityStatus:model.QualityValid}
	distribution:=base; distribution.ProviderEventID="distribution"; distribution.EventType="dividend"
	distribution.BonusShareRatio=&bonus; distribution.TransferShareRatio=&transfer; distribution.CashDividendBeforeTaxCNY=&cash
	distribution.RightsIssueRatio=&zero; distribution.RightsIssuePriceCNY=&zero
	rightsAction:=base; rightsAction.ProviderEventID="rights"; rightsAction.EventType="rights_issue"
	rightsAction.RightsIssueRatio=&rights; rightsAction.RightsIssuePriceCNY=&rightsPrice
	factors,err:=deriveAdjustmentFactorsFromCorporateActions([]model.DailyBar{{Symbol:"600519.SH",TradeDate:"2026-01-05",Source:"raw",SourceVersion:"raw-v1",AdjustmentBasis:model.AdjustmentNone}},
		[]model.CorporateAction{distribution,rightsAction},model.CorporateActionAdjustmentPolicyExchangeReference,true,fetched,&available)
	if err!=nil { t.Fatal(err) }
	if factors[0].Factor!=1.35 || factors[0].PriceOffsetCNY==nil || *factors[0].PriceOffsetCNY != -1 { t.Fatalf("combined terms %+v",factors[0]) }
}

func TestDeriveAdjustmentFactorsFromCorporateActionsExcludesShareReformConsideration(t *testing.T) {
	fetched:=time.Date(2026,11,20,8,0,0,0,time.UTC); yes,no:=true,false; cash,zero:=0.60,0.0
	distribution:=model.CorporateAction{Symbol:"600519.SH",ProviderEventID:"cash",EventType:"dividend",NormalizedStatus:"implemented",
		HolderScope:model.CorporateActionHolderScopeAllShareholders,ReferencePriceIncluded:&yes,TermsComplete:true,ExDate:"2026-01-05",
		CashDividendBeforeTaxCNY:&cash,BonusShareRatio:&zero,TransferShareRatio:&zero,Source:"tushare",SourceVersion:"v1",FetchedAt:fetched,QualityStatus:model.QualityValid}
	consideration:=model.CorporateAction{Symbol:"600519.SH",ProviderEventID:"reform",EventType:"share_reform_consideration",NormalizedStatus:"implemented",
		HolderScope:model.CorporateActionHolderScopeTradableShareholders,ReferencePriceIncluded:&no,TermsComplete:true,ExDate:"2026-01-05",
		CashConsiderationCNY:corporateActionFloatPointer(1.475),ShareConsiderationRatio:corporateActionFloatPointer(0.12),Source:"pytdx",SourceVersion:"v1",FetchedAt:fetched,QualityStatus:model.QualityValid}
	factors,err:=deriveAdjustmentFactorsFromCorporateActions([]model.DailyBar{{Symbol:"600519.SH",TradeDate:"2026-01-05",Source:"raw",SourceVersion:"raw-v1",AdjustmentBasis:model.AdjustmentNone}},
		[]model.CorporateAction{distribution,consideration},model.CorporateActionAdjustmentPolicyExchangeReference,true,fetched,nil)
	if err!=nil { t.Fatal(err) }
	if factors[0].Factor!=1 || factors[0].PriceOffsetCNY==nil || *factors[0].PriceOffsetCNY!=0.60 { t.Fatalf("consideration leaked into exchange reference: %+v",factors[0]) }
	if factors[0].QualityStatus!=model.QualityDegraded { t.Fatal("unknown coverage availability must degrade quality") }
}

func TestDeriveAdjustmentFactorsFromCorporateActionsRejectsAmbiguousTerms(t *testing.T) {
	fetched:=time.Date(2026,11,20,8,0,0,0,time.UTC); yes:=true; cash,zero:=0.5,0.0
	base:=model.CorporateAction{Symbol:"600519.SH",ProviderEventID:"event",EventType:"dividend",NormalizedStatus:"implemented",
		HolderScope:model.CorporateActionHolderScopeAllShareholders,ReferencePriceIncluded:&yes,TermsComplete:true,ExDate:"2026-01-05",
		CashDividendBeforeTaxCNY:&cash,BonusShareRatio:&zero,TransferShareRatio:&zero,Source:"tushare",SourceVersion:"v1",FetchedAt:fetched,QualityStatus:model.QualityValid}
	incomplete:=base; incomplete.TermsComplete=false
	unknown:=base; unknown.ReferencePriceIncluded=nil
	for name,action:=range map[string]model.CorporateAction{"incomplete":incomplete,"unknown_eligibility":unknown} {
		t.Run(name,func(t *testing.T) {
			_,err:=deriveAdjustmentFactorsFromCorporateActions([]model.DailyBar{{Symbol:"600519.SH",TradeDate:"2026-01-05",Source:"raw",SourceVersion:"raw-v1",AdjustmentBasis:model.AdjustmentNone}},
				[]model.CorporateAction{action},model.CorporateActionAdjustmentPolicyExchangeReference,true,fetched,nil)
			if err==nil { t.Fatal("ambiguous event must be rejected") }
		})
	}
}

func TestDeriveAdjustmentFactorsFromCorporateActionsMarksIncompleteCoverageDegraded(t *testing.T) {
	fetched:=time.Date(2026,11,20,8,0,0,0,time.UTC); yes:=true; cash,zero:=0.5,0.0
	action:=model.CorporateAction{Symbol:"600519.SH",ProviderEventID:"event",EventType:"dividend",NormalizedStatus:"implemented",
		HolderScope:model.CorporateActionHolderScopeAllShareholders,ReferencePriceIncluded:&yes,TermsComplete:true,ExDate:"2026-01-05",
		CashDividendBeforeTaxCNY:&cash,BonusShareRatio:&zero,TransferShareRatio:&zero,Source:"tushare",SourceVersion:"v1",FetchedAt:fetched,QualityStatus:model.QualityValid}
	factors,err:=deriveAdjustmentFactorsFromCorporateActions([]model.DailyBar{{Symbol:"600519.SH",TradeDate:"2026-01-05",Source:"raw",SourceVersion:"raw-v1",AdjustmentBasis:model.AdjustmentNone}},
		[]model.CorporateAction{action},model.CorporateActionAdjustmentPolicyExchangeReference,false,fetched,nil)
	if err!=nil { t.Fatal(err) }
	if factors[0].QualityStatus!=model.QualityDegraded || !containsString(factors[0].QualityReasons,"corporate_action_history_coverage_unverified") {
		t.Fatalf("coverage degradation missing: %+v",factors[0])
	}
}
