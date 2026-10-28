package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

const maxTradingCalendarDays = 3660

func parseTradingCalendarRange(startDate, endDate string) (time.Time, time.Time, int, error) {
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid trading-calendar start date %q: expected YYYY-MM-DD", startDate)
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid trading-calendar end date %q: expected YYYY-MM-DD", endDate)
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, 0, errors.New("trading-calendar start date must not be after end date")
	}
	totalDays := int(end.Sub(start)/(24*time.Hour)) + 1
	if totalDays > maxTradingCalendarDays {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("trading-calendar range exceeds %d calendar days", maxTradingCalendarDays)
	}
	return start, end, totalDays, nil
}

func validateTradingCalendarSnapshot(snapshot *model.TradingCalendarSnapshot, startDate, endDate string) error {
	if snapshot == nil {
		return errors.New("trading-calendar provider returned an empty snapshot")
	}
	start, _, expectedDays, err := parseTradingCalendarRange(startDate, endDate)
	if err != nil {
		return err
	}
	if snapshot.Market != "cn_a_share" || snapshot.StartDate != startDate || snapshot.EndDate != endDate {
		return fmt.Errorf("trading-calendar identity mismatch: requested cn_a_share/%s..%s, got %s/%s..%s", startDate, endDate, snapshot.Market, snapshot.StartDate, snapshot.EndDate)
	}
	if snapshot.TotalDays != expectedDays || len(snapshot.Items) != expectedDays {
		return fmt.Errorf("trading-calendar coverage mismatch: total_days=%d items=%d expected=%d", snapshot.TotalDays, len(snapshot.Items), expectedDays)
	}
	count := 0
	for index, item := range snapshot.Items {
		expectedDate := start.AddDate(0, 0, index).Format("2006-01-02")
		if item.Date != expectedDate {
			return fmt.Errorf("trading-calendar row %d is missing, duplicate or out of order: got %q, want %q", index, item.Date, expectedDate)
		}
		if item.IsTradingDay {
			count++
		}
	}
	if snapshot.TradingDays != count {
		return fmt.Errorf("trading-calendar trading_days mismatch: declared=%d observed=%d", snapshot.TradingDays, count)
	}
	if strings.TrimSpace(snapshot.Source) == "" || strings.TrimSpace(snapshot.SourceVersion) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("trading-calendar source, source_version or fetched_at is missing")
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("trading-calendar quality status %q is unsupported", snapshot.QualityStatus)
	}
	return nil
}

func (s *DataService) TradingCalendar(ctx context.Context, rawStartDate, rawEndDate string) (*model.TradingCalendarSnapshot, bool, error) {
	start, end, totalDays, err := parseTradingCalendarRange(rawStartDate, rawEndDate)
	if err != nil {
		return nil, false, err
	}
	startDate, endDate := start.Format("2006-01-02"), end.Format("2006-01-02")
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	const ttl = 12 * time.Hour
	key := fmt.Sprintf("md:v1:trading-calendar:cn-a-share:%s:%s", startDate, endDate)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.TradingCalendarSnapshot
		if json.Unmarshal(data, &cached) == nil && validateTradingCalendarSnapshot(&cached, startDate, endDate) == nil &&
			time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < ttl {
			return &cached, true, nil
		}
	} else if cacheErr != nil {
		s.logger.Warn("trading-calendar cache read failed", "start_date", startDate, "end_date", endDate, "error", cacheErr)
	}
	if len(s.tradingCalendarProviders) == 0 {
		return nil, false, errors.New("no enabled trading-calendar provider")
	}
	var failures []string
	for _, p := range s.tradingCalendarProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchTradingCalendar(ctx, startDate, endDate)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateTradingCalendarSnapshot(snapshot, startDate, endDate); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if snapshot.TotalDays != totalDays {
			failures = append(failures, p.Name()+": returned date count does not match request")
			continue
		}
		if data, marshalErr := json.Marshal(snapshot); marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, data, ttl); cacheErr != nil {
				s.logger.Warn("trading-calendar cache write failed", "start_date", startDate, "end_date", endDate, "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all trading-calendar providers failed: %s", strings.Join(failures, "; "))
}
