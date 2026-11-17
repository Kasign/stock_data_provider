package model

import "time"

// MarketSessionSnapshot 描述沪深 A 股某个时刻对应的交易时段。
// 这是基于交易日历和固定时段计算出的计划状态，不代表证券级停牌或交易所临时中断。
type MarketSessionSnapshot struct {
	Market              string        `json:"market"`
	At                  time.Time     `json:"at"`
	ServerTime          time.Time     `json:"server_time"`
	TimeZone            string        `json:"time_zone"`
	LocalDate           string        `json:"local_date"`
	LocalTime           string        `json:"local_time"`
	IsTradingDay        bool          `json:"is_trading_day"`
	Phase               string        `json:"phase"`
	IsTradingSession    bool          `json:"is_trading_session"`
	IsContinuousTrading bool          `json:"is_continuous_trading"`
	IsAuction           bool          `json:"is_auction"`
	Source              string        `json:"source"`
	SourceVersion       string        `json:"source_version,omitempty"`
	FetchedAt           time.Time     `json:"fetched_at"`
	QualityStatus       QualityStatus `json:"quality_status"`
	QualityReasons      []string      `json:"quality_reasons,omitempty"`
}

const (
	MarketSessionNonTradingDay  = "non_trading_day"
	MarketSessionPreOpen        = "pre_open"
	MarketSessionOpeningAuction = "opening_auction"
	MarketSessionAuctionPause   = "auction_pause"
	MarketSessionContinuousAM   = "continuous_morning"
	MarketSessionLunchBreak     = "lunch_break"
	MarketSessionContinuousPM   = "continuous_afternoon"
	MarketSessionClosingAuction = "closing_auction"
	MarketSessionClosed         = "closed"
)
