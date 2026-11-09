package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)


const maxDailyBarSecurityStatusLookupsPerProbe = 10

var errDailyBarContextStatusBudgetExceeded = errors.New("daily-bar context security-status lookup budget exceeded")

type dailyBarContextCacheKey struct{}

type dailyBarContextCacheEntry struct {
	ready  chan struct{}
	status *model.SecurityTradingStatusSnapshot
	err    error
}

type dailyBarContextCache struct {
	mu      sync.Mutex
	entries map[string]*dailyBarContextCacheEntry
	used    int
	budget  int
}

func withDailyBarContextCache(ctx context.Context, budget int) context.Context {
	if ctx == nil || budget <= 0 {
		return ctx
	}
	return context.WithValue(ctx, dailyBarContextCacheKey{}, &dailyBarContextCache{
		entries: make(map[string]*dailyBarContextCacheEntry), budget: budget,
	})
}

func loadDailyBarContextSecurityStatus(
	ctx context.Context,
	symbol, tradeDate string,
	loader func(context.Context) (*model.SecurityTradingStatusSnapshot, error),
) (*model.SecurityTradingStatusSnapshot, error) {
	cache, _ := ctx.Value(dailyBarContextCacheKey{}).(*dailyBarContextCache)
	if cache == nil {
		return loader(ctx)
	}
	key := symbol + "|" + tradeDate
	cache.mu.Lock()
	if entry, exists := cache.entries[key]; exists {
		cache.mu.Unlock()
		select {
		case <-entry.ready:
			return entry.status, entry.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if cache.used >= cache.budget {
		cache.mu.Unlock()
		return nil, errDailyBarContextStatusBudgetExceeded
	}
	entry := &dailyBarContextCacheEntry{ready: make(chan struct{})}
	cache.entries[key] = entry
	cache.used++
	cache.mu.Unlock()

	entry.status, entry.err = loader(ctx)
	close(entry.ready)
	return entry.status, entry.err
}

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
		status, statusErr := loadDailyBarContextSecurityStatus(ctx, symbol, day.Date, func(loadCtx context.Context) (*model.SecurityTradingStatusSnapshot, error) {
			return s.SecurityTradingStatus(loadCtx, symbol, day.Date)
		})
		if statusErr != nil {
			statusErrors[day.Date] = statusErr
			continue
		}
		statuses[day.Date] = status
	}
	return assessDailyBarTradingContext(calendar, bars, statuses, statusErrors)
}

// historicalCalendarTimestampOnlyDegradation accepts a complete historical date calendar
// when the only provider quality gap is an unavailable source publication timestamp.
func historicalCalendarTimestampOnlyDegradation(calendar *model.TradingCalendarSnapshot) bool {
	if calendar == nil || calendar.QualityStatus != model.QualityDegraded || calendar.EndDate == "" {
		return false
	}
	endDate, err := time.ParseInLocation("2006-01-02", calendar.EndDate, marketLocation())
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	if err != nil || endDate.Format("2006-01-02") >= today {
		return false
	}
	if len(calendar.QualityReasons) == 0 {
		return false
	}
	for _, reason := range calendar.QualityReasons {
		switch reason {
		case "provider_snapshot_timestamp_missing", "provider_snapshot_timestamp_not_exposed":
		default:
			return false
		}
	}
	return true
}

// assessDailyBarTradingContext never treats an unknown security state as a confirmed missing bar.
func assessDailyBarTradingContext(calendar *model.TradingCalendarSnapshot, bars []model.DailyBar, statuses map[string]*model.SecurityTradingStatusSnapshot, statusErrors map[string]error) dailyBarTradingContextResult {
	result := dailyBarTradingContextResult{QualityStatus: model.QualityValid}
	if calendar == nil {
		result.QualityStatus = model.QualityDegraded
		result.Reasons = append(result.Reasons, "daily_bar_context_calendar_unavailable")
		return result
	}
	if calendar.QualityStatus != model.QualityValid {
		if historicalCalendarTimestampOnlyDegradation(calendar) {
			// 历史交易日历日期是稳定事实；缺少上游发布时间不应单独污染历史 K 线质量。
			// 该元数据缺口仍显式保留，当前/未来日历则继续降级。
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_snapshot_time_unverified")
		} else {
			result.QualityStatus = model.QualityDegraded
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_degraded")
			for _, reason := range calendar.QualityReasons {
				if reason == "provider_trading_calendar_sources_disagree" {
					result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_calendar_sources_disagree")
				}
			}
			// 日历存在实质质量缺陷时，不能继续用其中的交易日标记确认证券漏数。
			// 保留不可比较结论，避免把推断或不完整的日历证据升级为确定缺口。
			return result
		}
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
			if errors.Is(statusErrors[day.Date], errDailyBarContextStatusBudgetExceeded) {
				result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_context_security_status_budget_exceeded:"+day.Date)
			} else if statusErrors[day.Date] != nil {
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
		case model.SuspensionStatusSuspended:
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_excluded_suspended:"+day.Date)
		case model.SuspensionStatusIntraday:
			// An intraday suspension does not explain a missing full-day bar:
			// the security was not suspended for the entire trading session.
			result.QualityStatus = model.QualityDegraded
			result.Reasons = appendDailyBarContextReason(result.Reasons, "daily_bar_missing_during_intraday_suspension:"+day.Date)
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
