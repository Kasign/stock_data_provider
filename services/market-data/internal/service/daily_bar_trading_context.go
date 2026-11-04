package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

type dailyBarTradingContextResult struct {
	QualityStatus model.QualityStatus
	Reasons       []string
}

func (s *DataService) assessDailyBarTradingContext(ctx context.Context, symbol string, bars []model.DailyBar, startDate, endDate string) dailyBarTradingContextResult {
	calendar, _, err := s.TradingCalendar(ctx, startDate, endDate)
	if err != nil {
		return dailyBarTradingContextResult{QualityStatus: model.QualityDegraded, Reasons: []string{"daily_bar_context_calendar_unavailable"}}
	}
	statuses := make(map[string]*model.SecurityTradingStatusSnapshot)
	statusErrors := make(map[string]error)
	barDates := make(map[string]struct{}, len(bars))
	for _, bar := range bars {
		barDates[bar.TradeDate] = struct{}{}
	}
	for _, day := range calendar.Items {
		if !day.IsTradingDay {
			continue
		}
		if _, exists := barDates[day.Date]; exists {
			continue
		}
		status, statusErr := s.SecurityTradingStatus(ctx, symbol, day.Date)
		if statusErr != nil {
			statusErrors[day.Date] = statusErr
			continue
		}
		statuses[day.Date] = status
	}
	return assessDailyBarTradingContext(calendar, bars, statuses, statusErrors)
}

// assessDailyBarTradingContext never treats an unknown security state as a confirmed missing bar.
func assessDailyBarTradingContext(calendar *model.TradingCalendarSnapshot, bars []model.DailyBar, statuses map[string]*model.SecurityTradingStatusSnapshot, statusErrors map[string]error) dailyBarTradingContextResult {
	result := dailyBarTradingContextResult{QualityStatus: model.QualityValid}
	if calendar == nil {
		result.QualityStatus = model.QualityDegraded
		result.Reasons = append(result.Reasons, "daily_bar_context_calendar_unavailable")
		return result
	}
	for _, reason := range calendar.QualityReasons {
		if reason == "provider_trading_calendar_sources_disagree" {
			result.QualityStatus = model.QualityDegraded
			result.Reasons = append(result.Reasons, "daily_bar_context_calendar_sources_disagree")
			return result
		}
	}
	barDates := make(map[string]struct{}, len(bars))
	for _, bar := range bars {
		barDates[bar.TradeDate] = struct{}{}
	}
	for _, bar := range bars {
		for _, day := range calendar.Items {
			if day.Date == bar.TradeDate && !day.IsTradingDay {
				result.QualityStatus = model.QualityDegraded
				result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_returned_on_non_trading_date:"+bar.TradeDate)
				break
			}
		}
	}
	for _, day := range calendar.Items {
		if !day.IsTradingDay {
			continue
		}
		if _, ok := barDates[day.Date]; ok {
			continue
		}
		status := statuses[day.Date]
		if status == nil {
			result.QualityStatus = model.QualityDegraded
			if statusErrors[day.Date] != nil {
				result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_security_status_unavailable:"+day.Date)
			} else {
				result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_security_status_missing:"+day.Date)
			}
			continue
		}
		if status.TradeDate != day.Date || !status.TradingDayKnown || !status.IsTradingDay {
			result.QualityStatus = model.QualityDegraded
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_trading_day_unknown_or_conflicting:"+day.Date)
			continue
		}
		if status.ListingDate != "" {
			listingDate, parseErr := time.Parse("2006-01-02", status.ListingDate)
			if parseErr != nil || listingDate.Format("2006-01-02") != status.ListingDate {
				result.QualityStatus = model.QualityDegraded
				result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_listing_date_invalid:"+day.Date)
				continue
			}
			if day.Date < status.ListingDate {
				result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_excluded_before_listing:"+day.Date)
				continue
			}
		} else if !status.ListingStatusKnown {
			result.QualityStatus = model.QualityDegraded
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_listing_status_unknown:"+day.Date)
			continue
		}
		if !status.ListingStatusKnown {
			result.QualityStatus = model.QualityDegraded
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_listing_status_unknown:"+day.Date)
			continue
		}
		switch status.SuspensionStatus {
		case model.SuspensionStatusSuspended, model.SuspensionStatusIntraday:
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_excluded_suspended:"+day.Date)
		case model.SuspensionStatusNoEvent, model.SuspensionStatusResumed:
			result.QualityStatus = model.QualityDegraded
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_missing_on_confirmed_trading_day:"+day.Date)
		default:
			result.QualityStatus = model.QualityDegraded
			result.Reasons = appendDailyBarContextReason(result.Reasons, fmt.Sprintf("daily_bar_context_suspension_status_unknown:%s", day.Date))
		}
	}
	return result
}

func appendDailyBarContextReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
