package service

import (
	"context"
	"errors"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// MarketSession returns the scheduled CN A-share session for a timestamp.
// Calendar status comes from configured calendar Providers; session boundaries are the
// standard weekday schedule and do not account for security-level suspensions or emergency exchange notices.
func (s *DataService) MarketSession(ctx context.Context, at time.Time) (*model.MarketSessionSnapshot, error) {
	if s == nil {
		return nil, errors.New("market-data service is nil")
	}
	if at.IsZero() {
		return nil, errors.New("market-session timestamp must not be zero")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	local := at.In(marketLocation())
	date := local.Format("2006-01-02")
	calendar, _, err := s.TradingCalendar(ctx, date, date)
	if err != nil {
		return nil, err
	}
	phase, inSession, continuous, auction := scheduledMarketSession(calendar.Items[0].IsTradingDay, local.Hour(), local.Minute())
	now := time.Now().UTC()
	return &model.MarketSessionSnapshot{
		Market:              "cn_a_share",
		At:                  at.UTC(),
		LocalDate:           date,
		LocalTime:           local.Format("15:04:05"),
		IsTradingDay:        calendar.Items[0].IsTradingDay,
		Phase:               phase,
		IsTradingSession:    inSession,
		IsContinuousTrading: continuous,
		IsAuction:           auction,
		Source:              calendar.Source,
		SourceVersion:       calendar.SourceVersion,
		FetchedAt:           now,
		QualityStatus:       calendar.QualityStatus,
		QualityReasons:      append([]string(nil), calendar.QualityReasons...),
	}, nil
}

// scheduledMarketSession is intentionally deterministic and side-effect free.
// Times use minutes after midnight in Asia/Shanghai and half-open intervals.
func scheduledMarketSession(isTradingDay bool, hour, minute int) (phase string, inSession, continuous, auction bool) {
	if !isTradingDay {
		return model.MarketSessionNonTradingDay, false, false, false
	}
	minutes := hour*60 + minute
	switch {
	case minutes < 9*60+15:
		return model.MarketSessionPreOpen, false, false, false
	case minutes < 9*60+25:
		return model.MarketSessionOpeningAuction, true, false, true
	case minutes < 9*60+30:
		return model.MarketSessionAuctionPause, false, false, false
	case minutes < 11*60+30:
		return model.MarketSessionContinuousAM, true, true, false
	case minutes < 13*60:
		return model.MarketSessionLunchBreak, false, false, false
	case minutes < 14*60+57:
		return model.MarketSessionContinuousPM, true, true, false
	case minutes < 15*60:
		return model.MarketSessionClosingAuction, true, false, true
	default:
		return model.MarketSessionClosed, false, false, false
	}
}
