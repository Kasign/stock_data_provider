package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

// SetAnnouncementProviders installs providers independent from real-time quotes and daily bars.
func (s *DataService) SetAnnouncementProviders(providers []provider.AnnouncementProvider) {
	s.announcementProviders = append([]provider.AnnouncementProvider(nil), providers...)
}

func (s *DataService) AnnouncementProviders() []provider.AnnouncementProvider {
	return append([]provider.AnnouncementProvider(nil), s.announcementProviders...)
}

func normalizeAnnouncementRange(rawStart, rawEnd string) (string, string, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	startDate, endDate := strings.TrimSpace(rawStart), strings.TrimSpace(rawEnd)
	if endDate == "" {
		endDate = today
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		return "", "", fmt.Errorf("invalid announcement end_date %q: expected YYYY-MM-DD", endDate)
	}
	if startDate == "" {
		end, _ := time.Parse("2006-01-02", endDate)
		startDate = end.AddDate(0, 0, -6).Format("2006-01-02")
	}
	start, startErr := time.Parse("2006-01-02", startDate)
	end, endErr := time.Parse("2006-01-02", endDate)
	if startErr != nil || endErr != nil || start.Format("2006-01-02") != startDate ||
		end.Format("2006-01-02") != endDate || start.After(end) {
		return "", "", errors.New("announcement start_date/end_date must be valid YYYY-MM-DD dates with start_date <= end_date")
	}
	if end.Sub(start) > 365*24*time.Hour {
		return "", "", errors.New("announcement date range must not exceed 366 days")
	}
	return startDate, endDate, nil
}

func validateStockAnnouncementSnapshot(symbol, startDate, endDate string, page, pageSize int, snapshot *model.StockAnnouncementSnapshot) error {
	if snapshot == nil {
		return errors.New("announcement provider returned an empty snapshot")
	}
	if snapshot.Symbol != symbol || snapshot.StartDate != startDate || snapshot.EndDate != endDate ||
		snapshot.Page != page || snapshot.PageSize != pageSize {
		return errors.New("announcement request identity mismatch")
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("announcement source or fetched_at is missing")
	}
	if snapshot.Total < 0 || snapshot.Pages < 0 || snapshot.Returned != len(snapshot.Items) || snapshot.Returned > pageSize {
		return errors.New("announcement total/pages/returned count is invalid")
	}
	if snapshot.Total == 0 {
		if snapshot.Pages != 0 || snapshot.Returned != 0 || len(snapshot.Items) != 0 ||
			snapshot.HasData || snapshot.HasMore || snapshot.QualityStatus != model.QualityUnknown {
			return errors.New("empty announcement response must remain unknown, not a confirmed absence")
		}
		return nil
	}
	if snapshot.Pages != (snapshot.Total+pageSize-1)/pageSize || page > snapshot.Pages ||
		snapshot.Returned == 0 || snapshot.Total < snapshot.Returned || !snapshot.HasData ||
		snapshot.HasMore != (page < snapshot.Pages) {
		return errors.New("announcement page counters are inconsistent with source total")
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("announcement quality status %q is unsupported", snapshot.QualityStatus)
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	hasDegradedItem := false
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.ArticleID) == "" || strings.TrimSpace(item.Title) == "" ||
			item.Symbol != symbol || item.Source != snapshot.Source || item.FetchedAt.IsZero() {
			return errors.New("announcement item is missing ID/title or has mismatched symbol/source")
		}
		if _, ok := seen[item.ArticleID]; ok {
			return fmt.Errorf("announcement item has duplicate article ID %q", item.ArticleID)
		}
		seen[item.ArticleID] = struct{}{}
		date, err := time.Parse("2006-01-02", item.AnnouncementDate)
		if err != nil || date.Format("2006-01-02") != item.AnnouncementDate ||
			item.AnnouncementDate < startDate || item.AnnouncementDate > endDate {
			return errors.New("announcement item date is invalid or outside the requested range")
		}
		if strings.TrimSpace(item.SourceURL) == "" {
			return errors.New("announcement item has no source URL")
		}
		if item.PublishedAt == nil && item.QualityStatus == model.QualityValid {
			return errors.New("announcement without a source publishing time must not be marked valid quality")
		}
		if item.PublishedAt != nil && (item.PublishedAt.IsZero() || item.PublishedAt.After(time.Now().Add(5*time.Minute))) {
			return errors.New("announcement publishing time is zero or in the future")
		}
		if item.QualityStatus != model.QualityValid && item.QualityStatus != model.QualityDegraded {
			return fmt.Errorf("announcement item quality status %q is unsupported", item.QualityStatus)
		}
		if item.QualityStatus == model.QualityDegraded {
			hasDegradedItem = true
		}
	}
	if snapshot.QualityStatus == model.QualityValid && hasDegradedItem {
		return errors.New("announcement snapshot cannot be valid when one or more items are degraded")
	}
	return nil
}

// StockAnnouncements returns paginated announcements for one A-share and an explicit date range.
func (s *DataService) StockAnnouncements(ctx context.Context, rawSymbol, rawStartDate, rawEndDate string, page, pageSize int) (*model.StockAnnouncementSnapshot, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	startDate, endDate, err := normalizeAnnouncementRange(rawStartDate, rawEndDate)
	if err != nil {
		return nil, false, err
	}
	if page < 1 || page > 1000 {
		return nil, false, errors.New("announcement page must be between 1 and 1000")
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, false, errors.New("announcement page_size must be between 1 and 100")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	const positiveTTL = 10 * time.Minute
	const emptyTTL = 30 * time.Second
	key := fmt.Sprintf("md:v1:stock-announcements:%s:%s:%s:%d:%d", symbol, startDate, endDate, page, pageSize)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.StockAnnouncementSnapshot
		if json.Unmarshal(data, &cached) == nil &&
			validateStockAnnouncementSnapshot(symbol, startDate, endDate, page, pageSize, &cached) == nil &&
			time.Since(cached.FetchedAt) >= 0 {
			ttl := positiveTTL
			cacheEligible := cached.HasData
			if !cached.HasData {
				ttl = emptyTTL
				fallbackExhausted := false
				for _, reason := range cached.QualityReasons {
					if reason == "announcements_fallback_exhausted_without_confirmed_data" {
						fallbackExhausted = true
						break
					}
				}
				// 多 Provider 场景忽略旧版首源 unknown 空缓存，确保重新尝试后备源。
				cacheEligible = fallbackExhausted || len(s.announcementProviders) <= 1
			}
			if cacheEligible && time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("stock-announcement cache read failed", "symbol", symbol, "error", cacheErr)
	}
	if len(s.announcementProviders) == 0 {
		return nil, false, errors.New("no enabled stock-announcement provider")
	}
	var failures []string
	var unknownEmpty *model.StockAnnouncementSnapshot
	for _, p := range s.announcementProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchStockAnnouncements(ctx, symbol, startDate, endDate, page, pageSize)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateStockAnnouncementSnapshot(symbol, startDate, endDate, page, pageSize, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if !snapshot.HasData {
			unknownEmpty = snapshot
			failures = append(failures, p.Name()+": announcement absence is not confirmed")
			continue
		}
		if encoded, marshalErr := json.Marshal(snapshot); marshalErr == nil {
			ttl := positiveTTL
			if !snapshot.HasData {
				ttl = emptyTTL
			}
			if cacheErr := s.cache.Set(ctx, key, encoded, ttl); cacheErr != nil {
				s.logger.Warn("stock-announcement cache write failed", "symbol", symbol, "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	if unknownEmpty != nil {
		unknownEmpty.QualityStatus = model.QualityUnknown
		unknownEmpty.QualityReasons = appendReason(unknownEmpty.QualityReasons, "announcements_fallback_exhausted_without_confirmed_data")
		if encoded, marshalErr := json.Marshal(unknownEmpty); marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, encoded, emptyTTL); cacheErr != nil {
				s.logger.Warn("stock-announcement unknown cache write failed", "symbol", symbol, "error", cacheErr)
			}
		}
		return unknownEmpty, false, nil
	}
	return nil, false, fmt.Errorf("all stock-announcement providers failed: %s", strings.Join(failures, "; "))
}
