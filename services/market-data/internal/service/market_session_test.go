package service

import (
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestScheduledMarketSessionBoundaries(t *testing.T) {
	cases := []struct {
		name       string
		tradingDay bool
		hour       int
		minute     int
		phase      string
		inSession  bool
		continuous bool
		auction    bool
	}{
		{name:"weekend ignores clock",tradingDay:false,hour:10,minute:0,phase:model.MarketSessionNonTradingDay},
		{name:"before opening auction",tradingDay:true,hour:9,minute:14,phase:model.MarketSessionPreOpen},
		{name:"opening auction start",tradingDay:true,hour:9,minute:15,phase:model.MarketSessionOpeningAuction,inSession:true,auction:true},
		{name:"opening auction end",tradingDay:true,hour:9,minute:25,phase:model.MarketSessionAuctionPause},
		{name:"continuous morning start",tradingDay:true,hour:9,minute:30,phase:model.MarketSessionContinuousAM,inSession:true,continuous:true},
		{name:"morning session end",tradingDay:true,hour:11,minute:30,phase:model.MarketSessionLunchBreak},
		{name:"afternoon session start",tradingDay:true,hour:13,minute:0,phase:model.MarketSessionContinuousPM,inSession:true,continuous:true},
		{name:"closing auction start",tradingDay:true,hour:14,minute:57,phase:model.MarketSessionClosingAuction,inSession:true,auction:true},
		{name:"closing auction end",tradingDay:true,hour:15,minute:0,phase:model.MarketSessionClosed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			phase, inSession, continuous, auction := scheduledMarketSession(tc.tradingDay, tc.hour, tc.minute)
			if phase != tc.phase || inSession != tc.inSession || continuous != tc.continuous || auction != tc.auction {
				t.Fatalf("scheduledMarketSession(%v,%02d:%02d)=(%q,%v,%v,%v), want (%q,%v,%v,%v)",
					tc.tradingDay, tc.hour, tc.minute, phase, inSession, continuous, auction,
					tc.phase, tc.inSession, tc.continuous, tc.auction)
			}
		})
	}
}
