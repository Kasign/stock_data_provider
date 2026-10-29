package service

import (
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestResolvePriceLimitRuleMainboardRiskWarningChangeDate(t *testing.T) {
	before, err := ResolvePriceLimitRule("000001.SZ", "2026-07-05",
		model.RiskWarningApplied, 2000, true, model.PriceLimitExceptionNone)
	if err != nil {
		t.Fatal(err)
	}
	if before.Status != model.PriceLimitStatusLimited || before.LimitPercent == nil || *before.LimitPercent != 5 {
		t.Fatalf("expected 5%% limit before effective date, got %+v", before)
	}
	if before.RuleEffectiveTo != "2026-07-05" {
		t.Fatalf("expected the previous rule to end on 2026-07-05, got %+v", before)
	}

	after, err := ResolvePriceLimitRule("000001.SZ", "2026-07-06",
		model.RiskWarningApplied, 2000, true, model.PriceLimitExceptionNone)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != model.PriceLimitStatusLimited || after.LimitPercent == nil || *after.LimitPercent != 10 {
		t.Fatalf("expected 10%% limit from 2026-07-06, got %+v", after)
	}
	if after.RuleEffectiveFrom != "2026-07-06" {
		t.Fatalf("effective date missing from current ST rule: %+v", after)
	}
}

func TestResolvePriceLimitRuleBoardRates(t *testing.T) {
	cases := []struct {
		name, symbol, date string
		risk model.RiskWarningStatus
		want float64
	}{
		{name:"mainboard normal remains 10 percent",symbol:"600519.SH",date:"2026-07-05",risk:model.RiskWarningNormal,want:10},
		{name:"ChiNext before reform",symbol:"300750.SZ",date:"2020-08-21",risk:model.RiskWarningUnknown,want:10},
		{name:"ChiNext after reform",symbol:"300750.SZ",date:"2020-08-24",risk:model.RiskWarningUnknown,want:20},
		{name:"STAR",symbol:"688981.SH",date:"2026-07-06",risk:model.RiskWarningUnknown,want:20},
		{name:"Beijing Exchange",symbol:"430047.BJ",date:"2026-07-06",risk:model.RiskWarningUnknown,want:30},
	}
	for _, tc := range cases {
		t.Run(tc.name,func(t *testing.T){
			got,err:=ResolvePriceLimitRule(tc.symbol,tc.date,tc.risk,2000,true,model.PriceLimitExceptionNone)
			if err!=nil { t.Fatal(err) }
			if got.Status!=model.PriceLimitStatusLimited || got.LimitPercent==nil || *got.LimitPercent!=tc.want {
				t.Fatalf("got %+v; want limited at %.0f%%",got,tc.want)
			}
		})
	}
}

func TestResolvePriceLimitRuleIPOAndSpecialNoLimitEvents(t *testing.T) {
	ipo,err:=ResolvePriceLimitRule("600519.SH","2026-07-06",model.RiskWarningNormal,3,true,model.PriceLimitExceptionNone)
	if err!=nil { t.Fatal(err) }
	if ipo.Status!=model.PriceLimitStatusNoLimit || ipo.LimitPercent!=nil || ipo.NoLimitReason!="ipo_listing_day_without_price_limit" {
		t.Fatalf("expected first-five-day IPO exemption, got %+v",ipo)
	}

	relisting,err:=ResolvePriceLimitRule("600519.SH","2026-07-06",model.RiskWarningUnknown,0,false,model.PriceLimitExceptionRelistingFirst)
	if err!=nil { t.Fatal(err) }
	if relisting.Status!=model.PriceLimitStatusNoLimit || relisting.LimitPercent!=nil {
		t.Fatalf("explicit relisting-first-day event must override ordinary percentage, got %+v",relisting)
	}
}

func TestResolvePriceLimitRuleReturnsUnknownForUnverifiedInputs(t *testing.T) {
	unknownException,err:=ResolvePriceLimitRule("600519.SH","2026-07-06",model.RiskWarningNormal,2000,true,model.PriceLimitExceptionUnknown)
	if err!=nil { t.Fatal(err) }
	if unknownException.Status!=model.PriceLimitStatusUnknown || unknownException.QualityStatus!=model.QualityUnknown {
		t.Fatalf("unknown special-event state must not be treated as no exception, got %+v",unknownException)
	}

	unknownRisk,err:=ResolvePriceLimitRule("600519.SH","2026-07-05",model.RiskWarningUnknown,2000,true,model.PriceLimitExceptionNone)
	if err!=nil { t.Fatal(err) }
	if unknownRisk.Status!=model.PriceLimitStatusUnknown || unknownRisk.LimitPercent!=nil {
		t.Fatalf("unknown pre-cutover risk status must not assume normal-stock limits, got %+v",unknownRisk)
	}

	unknownListing,err:=ResolvePriceLimitRule("600519.SH","2026-07-06",model.RiskWarningNormal,0,false,model.PriceLimitExceptionNone)
	if err!=nil { t.Fatal(err) }
	if unknownListing.Status!=model.PriceLimitStatusUnknown || unknownListing.LimitPercent!=nil {
		t.Fatalf("unknown IPO/listing window must not be assumed absent, got %+v",unknownListing)
	}
}
