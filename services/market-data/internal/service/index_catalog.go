package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

func (s *DataService) SetIndexCatalogProviders(providers []provider.IndexCatalogProvider) {
	s.indexCatalogProviders = append([]provider.IndexCatalogProvider(nil), providers...)
}

func (s *DataService) IndexCatalogProviders() []provider.IndexCatalogProvider {
	return append([]provider.IndexCatalogProvider(nil), s.indexCatalogProviders...)
}

func canonicalIndexSymbol(raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	parts := strings.Split(value, ".")
	if len(parts) != 2 || (parts[1] != "TI" && parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "CSI") || len(parts[0]) != 6 {
		return "", fmt.Errorf("index symbol must be six digits with .TI/.SH/.SZ/.CSI suffix: %q", raw)
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("index symbol must use six numeric digits: %q", raw)
		}
	}
	return value, nil
}

func validIndexCatalogTag(raw string) (string, error) {
	tag := strings.ToLower(strings.TrimSpace(raw))
	if tag == "" {
		tag = "cn_concept"
	}
	switch tag {
	case "cn_concept", "region", "tszs", "industry", "tushare":
		return tag, nil
	default:
		return "", fmt.Errorf("tag must be cn_concept, region, tszs, industry or tushare")
	}
}

func (s *DataService) indexCatalogProvidersForSource(rawSource string) ([]provider.IndexCatalogProvider, string, error) {
	source := strings.ToLower(strings.TrimSpace(rawSource))
	if source == "" {
		return s.indexCatalogProviders, "auto", nil
	}
	for _, candidate := range s.indexCatalogProviders {
		name := strings.ToLower(strings.TrimSpace(candidate.Name()))
		if name == source {
			return []provider.IndexCatalogProvider{candidate}, name, nil
		}
	}
	return nil, source, fmt.Errorf("no enabled index-catalog provider named %q", source)
}

func validSnapshotQuality(q model.QualityStatus) bool {
	return q == model.QualityValid || q == model.QualityDegraded || q == model.QualityUnknown
}

func validateIndexCatalogSnapshot(tag string, snapshot *model.IndexCatalogSnapshot) error {
	if snapshot == nil || snapshot.Tag != tag || snapshot.Total != len(snapshot.Items) {
		return errors.New("index catalog snapshot identity/count mismatch")
	}
	if snapshot.Source == "" || snapshot.FetchedAt.IsZero() || !validSnapshotQuality(snapshot.QualityStatus) {
		return errors.New("index catalog snapshot source, timestamp or quality invalid")
	}
	if snapshot.ProviderTime != nil && (snapshot.ProviderTime.IsZero() || snapshot.ProviderTime.After(time.Now().Add(5*time.Minute))) {
		return errors.New("index catalog provider_time is zero or in the future")
	}
	if snapshot.QualityStatus == model.QualityValid && snapshot.ProviderTime == nil {
		return errors.New("index catalog cannot be valid without a provider snapshot timestamp")
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		symbol, err := canonicalIndexSymbol(item.Symbol)
		if err != nil || symbol != item.Symbol || strings.TrimSpace(item.Name) == "" || item.Tag != tag {
			return errors.New("index catalog contains invalid symbol/name/tag")
		}
		if _, exists := seen[symbol]; exists {
			return fmt.Errorf("duplicate index catalog symbol %s", symbol)
		}
		seen[symbol] = struct{}{}
	}
	if len(snapshot.Items) == 0 && snapshot.QualityStatus != model.QualityUnknown {
		return errors.New("empty index catalog must be marked unknown")
	}
	return nil
}

func validateIndexConstituentsSnapshot(indexSymbol string, snapshot *model.IndexConstituentSnapshot) error {
	if snapshot == nil || snapshot.IndexSymbol != indexSymbol || snapshot.Total != len(snapshot.Items) {
		return errors.New("index constituent snapshot identity/count mismatch")
	}
	if snapshot.Source == "" || snapshot.FetchedAt.IsZero() || !validSnapshotQuality(snapshot.QualityStatus) {
		return errors.New("index constituent snapshot source, timestamp or quality invalid")
	}
	if snapshot.ProviderReturned < 0 || snapshot.ExcludedCount < 0 ||
		(snapshot.ProviderReturned > 0 && snapshot.ProviderReturned != snapshot.Total+snapshot.ExcludedCount) ||
		(snapshot.ProviderReturned == 0 && snapshot.ExcludedCount > 0) {
		return errors.New("index constituent source/accepted/excluded row counts are inconsistent")
	}
	if snapshot.ExcludedCount > 0 && snapshot.QualityStatus == model.QualityValid {
		return errors.New("index constituent snapshot cannot be valid when source rows were excluded")
	}
	if snapshot.ProviderReportDate != "" {
		date, err := time.Parse("2006-01-02", snapshot.ProviderReportDate)
		if err != nil || date.Format("2006-01-02") != snapshot.ProviderReportDate {
			return errors.New("index constituent provider_report_date is invalid")
		}
		if snapshot.ProviderReportDate > time.Now().In(marketLocation()).Format("2006-01-02") {
			return errors.New("index constituent provider_report_date is in the future")
		}
	}
	if snapshot.ProviderTime != nil && (snapshot.ProviderTime.IsZero() || snapshot.ProviderTime.After(time.Now().Add(5*time.Minute))) {
		return errors.New("index constituent provider_time is zero or in the future")
	}
	if snapshot.QualityStatus == model.QualityValid && snapshot.ProviderTime == nil && snapshot.ProviderReportDate == "" {
		return errors.New("index constituents cannot be valid without a provider time or report date")
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		symbol, err := normalize.NormalizeSymbol(item.Symbol)
		if err != nil || symbol != item.Symbol || strings.TrimSpace(item.Name) == "" || item.Code != strings.Split(symbol, ".")[0] {
			return errors.New("index constituent contains invalid symbol/code/name")
		}
		if item.WeightPercent != nil && (math.IsNaN(*item.WeightPercent) || math.IsInf(*item.WeightPercent, 0) || *item.WeightPercent < 0 || *item.WeightPercent > 100) {
			return errors.New("index constituent weight_percent must be between 0 and 100 when present")
		}
		if _, exists := seen[symbol]; exists {
			return fmt.Errorf("duplicate index constituent %s", symbol)
		}
		seen[symbol] = struct{}{}
	}
	if len(snapshot.Items) == 0 && snapshot.QualityStatus != model.QualityUnknown {
		return errors.New("empty index constituents must be marked unknown")
	}
	return nil
}


func markIndexConstituentCoverage(snapshot *model.IndexConstituentSnapshot) {
	if snapshot == nil || snapshot.Total == 0 || snapshot.ProviderReturned > 0 {
		return
	}

	// 兼容没有覆盖计数的旧缓存/旧 Provider，但不再把它们当作完整有效快照。
	snapshot.QualityStatus = model.QualityDegraded
	const reason = "provider_constituent_source_coverage_unreported"
	for _, existing := range snapshot.QualityReasons {
		if existing == reason {
			return
		}
	}
	snapshot.QualityReasons = append(snapshot.QualityReasons, reason)
}

func validateIndexDailyBarsSnapshot(indexSymbol, startDate, endDate string, snapshot *model.IndexDailyBarsSnapshot) error {
	if snapshot == nil || snapshot.IndexSymbol != indexSymbol || snapshot.StartDate != startDate || snapshot.EndDate != endDate || snapshot.Total != len(snapshot.Items) {
		return errors.New("index daily-bars snapshot identity/range/count mismatch")
	}
	if snapshot.Source == "" || snapshot.FetchedAt.IsZero() || !validSnapshotQuality(snapshot.QualityStatus) {
		return errors.New("index daily-bars snapshot source, timestamp or quality invalid")
	}
	if snapshot.ProviderTime != nil && (snapshot.ProviderTime.IsZero() || snapshot.ProviderTime.After(time.Now().Add(5*time.Minute))) {
		return errors.New("index daily-bars provider_time is zero or in the future")
	}
	if snapshot.QualityStatus == model.QualityValid && snapshot.ProviderTime == nil {
		return errors.New("index daily-bars snapshot cannot be valid without a provider snapshot timestamp")
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	previousDate := ""
	hasDegradedItem := false
	for _, item := range snapshot.Items {
		if item.IndexSymbol != indexSymbol || item.TradeDate < startDate || item.TradeDate > endDate || item.Source != snapshot.Source || item.FetchedAt.IsZero() {
			return errors.New("index daily bar identity, date or source invalid")
		}
		parsedDate, dateErr := time.Parse("2006-01-02", item.TradeDate)
		if dateErr != nil || parsedDate.Format("2006-01-02") != item.TradeDate {
			return errors.New("index daily bar has invalid trade date")
		}
		if _, duplicate := seen[item.TradeDate]; duplicate {
			return fmt.Errorf("duplicate index daily bar date %s", item.TradeDate)
		}
		seen[item.TradeDate] = struct{}{}
		if previousDate != "" && item.TradeDate < previousDate {
			return errors.New("index daily bars are not in ascending date order")
		}
		previousDate = item.TradeDate
		if math.IsNaN(item.Open) || math.IsInf(item.Open, 0) || math.IsNaN(item.High) || math.IsInf(item.High, 0) ||
			math.IsNaN(item.Low) || math.IsInf(item.Low, 0) || math.IsNaN(item.Close) || math.IsInf(item.Close, 0) ||
			item.Open <= 0 || item.High <= 0 || item.Low <= 0 || item.Close <= 0 ||
			item.Low > item.High || item.Open < item.Low || item.Open > item.High || item.Close < item.Low || item.Close > item.High {
			return fmt.Errorf("index daily bar %s has invalid OHLC values", item.TradeDate)
		}
		if item.ChangePercent != nil && (math.IsNaN(*item.ChangePercent) || math.IsInf(*item.ChangePercent, 0)) {
			return fmt.Errorf("index daily bar %s has a non-finite change_percent", item.TradeDate)
		}
		if item.Volume == nil {
			if item.VolumeUnit != "unknown" {
				return fmt.Errorf("index daily bar %s has no volume but volume_unit is not unknown", item.TradeDate)
			}
		} else {
			if *item.Volume < 0 {
				return fmt.Errorf("index daily bar %s has negative volume", item.TradeDate)
			}
			if item.VolumeUnit != "shares" && item.VolumeUnit != "hands" {
				return fmt.Errorf("index daily bar %s has unsupported volume_unit %q", item.TradeDate, item.VolumeUnit)
			}
		}
		if item.Amount == nil {
			if item.AmountUnit != "unknown" {
				return fmt.Errorf("index daily bar %s has no amount but amount_unit is not unknown", item.TradeDate)
			}
		} else {
			if math.IsNaN(*item.Amount) || math.IsInf(*item.Amount, 0) || *item.Amount < 0 {
				return fmt.Errorf("index daily bar %s has invalid amount", item.TradeDate)
			}
			if item.AmountUnit != "CNY" && item.AmountUnit != "provider_defined" {
				return fmt.Errorf("index daily bar %s has unsupported amount_unit %q", item.TradeDate, item.AmountUnit)
			}
			if item.AmountUnit == "provider_defined" && item.QualityStatus == model.QualityValid {
				return fmt.Errorf("index daily bar %s cannot be valid with provider_defined amount unit", item.TradeDate)
			}
		}
		if (item.Volume == nil || item.Amount == nil) && item.QualityStatus == model.QualityValid {
			return fmt.Errorf("index daily bar %s cannot be valid with missing volume or amount", item.TradeDate)
		}
		if item.ProviderTime != nil && (item.ProviderTime.IsZero() || item.ProviderTime.After(time.Now().Add(5*time.Minute))) {
			return fmt.Errorf("index daily bar %s has zero or future provider_time", item.TradeDate)
		}
		if !validSnapshotQuality(item.QualityStatus) || item.QualityStatus == model.QualityUnknown {
			return fmt.Errorf("index daily bar %s has unsupported quality", item.TradeDate)
		}
		if item.QualityStatus == model.QualityDegraded {
			hasDegradedItem = true
		}
	}
	if len(snapshot.Items) == 0 && snapshot.QualityStatus != model.QualityUnknown {
		return errors.New("empty index daily-bars snapshot must be marked unknown")
	}
	if snapshot.QualityStatus == model.QualityValid && hasDegradedItem {
		return errors.New("index daily-bars snapshot cannot be valid when one or more items are degraded")
	}
	return nil
}

func indexCatalogCacheTTL(snapshot *model.IndexCatalogSnapshot) time.Duration {
	if snapshot == nil || snapshot.QualityStatus == model.QualityUnknown || len(snapshot.Items) == 0 {
		return 5 * time.Minute
	}
	if snapshot.QualityStatus != model.QualityValid {
		return time.Hour
	}
	return 12 * time.Hour
}

func (s *DataService) IndexCatalog(ctx context.Context, rawTag string) (*model.IndexCatalogSnapshot, bool, error) {
	tag, err := validIndexCatalogTag(rawTag)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key := "md:v1:index-catalog:" + tag
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.IndexCatalogSnapshot
		if json.Unmarshal(data, &cached) == nil && validateIndexCatalogSnapshot(tag, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := indexCatalogCacheTTL(&cached)
			cacheEligible := len(cached.Items) > 0 && cached.QualityStatus != model.QualityUnknown
			if cached.QualityStatus == model.QualityUnknown {
				fallbackExhausted := false
				for _, reason := range cached.QualityReasons {
					if reason == "index_catalog_fallback_exhausted_without_confirmed_data" ||
						reason == "index_catalog_fallback_exhausted_without_usable_quality" {
						fallbackExhausted = true
						break
					}
				}
				cacheEligible = fallbackExhausted || len(s.indexCatalogProviders) <= 1
			}
			if cacheEligible && time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("index catalog cache read failed", "tag", tag, "error", cacheErr)
	}
	providers := s.indexCatalogProviders
	if len(providers) == 0 {
		return nil, false, errors.New("no enabled index-catalog provider")
	}
	var failures []string
	var unknownCandidate *model.IndexCatalogSnapshot
	for _, p := range providers {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchIndexCatalog(ctx, tag)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateIndexCatalogSnapshot(tag, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if snapshot.QualityStatus == model.QualityUnknown {
			if betterUnknownIndexDailyBarsSnapshot(snapshot, unknownCandidate) {
				unknownCandidate = snapshot
			}
			if len(snapshot.Items) == 0 {
				failures = append(failures, p.Name()+": index catalog is empty and cannot be confirmed")
			} else {
				failures = append(failures, p.Name()+": index catalog contains entries but source quality is unknown")
			}
			continue
		}
		if len(snapshot.Items) == 0 {
			failures = append(failures, p.Name()+": index catalog is empty and cannot be confirmed")
			continue
		}
		if payload, err := json.Marshal(snapshot); err == nil {
			ttl := indexCatalogCacheTTL(snapshot)
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("index catalog cache write failed", "tag", tag, "error", cacheErr)
			}
		} else {
			s.logger.Warn("index catalog cache encode failed", "tag", tag, "error", err)
		}
		return snapshot, false, nil
	}
	if unknownCandidate != nil {
		unknownCandidate.QualityStatus = model.QualityUnknown
		reason := "index_catalog_fallback_exhausted_without_usable_quality"
		if len(unknownCandidate.Items) == 0 {
			reason = "index_catalog_fallback_exhausted_without_confirmed_data"
		}
		unknownCandidate.QualityReasons = appendReason(unknownCandidate.QualityReasons, reason)
		if payload, err := json.Marshal(unknownCandidate); err == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, indexCatalogCacheTTL(unknownCandidate)); cacheErr != nil {
				s.logger.Warn("index-catalog unknown cache write failed", "tag", tag, "error", cacheErr)
			}
		}
		return unknownCandidate, false, nil
	}
	return nil, false, fmt.Errorf("all index-catalog providers failed: %s", strings.Join(failures, "; "))
}

func (s *DataService) IndexConstituents(ctx context.Context, rawIndexSymbol string) (*model.IndexConstituentSnapshot, bool, error) {
	return s.IndexConstituentsFromSource(ctx, rawIndexSymbol, "")
}

func (s *DataService) IndexConstituentsFromSource(ctx context.Context, rawIndexSymbol, rawSource string) (*model.IndexConstituentSnapshot, bool, error) {
	indexSymbol, err := canonicalIndexSymbol(rawIndexSymbol)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	providers, sourceKey, sourceErr := s.indexCatalogProvidersForSource(rawSource)
	if sourceErr != nil {
		return nil, false, sourceErr
	}
	if len(providers) == 0 {
		return nil, false, errors.New("no enabled index-catalog provider")
	}
	key := fmt.Sprintf("md:v2:index-constituents:%s:%s", sourceKey, indexSymbol)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.IndexConstituentSnapshot
		if json.Unmarshal(data, &cached) == nil && validateIndexConstituentsSnapshot(indexSymbol, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := 30 * time.Minute
			if cached.QualityStatus != model.QualityValid {
				ttl = 5 * time.Minute
			}
			cacheEligible := len(cached.Items) > 0 && cached.QualityStatus != model.QualityUnknown
			if cached.QualityStatus == model.QualityUnknown {
				fallbackExhausted := false
				for _, reason := range cached.QualityReasons {
					if reason == "index_constituents_fallback_exhausted_without_confirmed_data" ||
						reason == "index_constituents_fallback_exhausted_without_usable_quality" {
						fallbackExhausted = true
						break
					}
				}
				cacheEligible = fallbackExhausted || len(providers) <= 1
			}
			if cacheEligible && time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("index constituents cache read failed", "symbol", indexSymbol, "error", cacheErr)
	}
	var failures []string
	var unknownCandidate *model.IndexConstituentSnapshot
	for _, p := range providers {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchIndexConstituents(ctx, indexSymbol)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateIndexConstituentsSnapshot(indexSymbol, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if snapshot.QualityStatus == model.QualityUnknown || len(snapshot.Items) == 0 {
			if unknownCandidate == nil || len(snapshot.Items) > len(unknownCandidate.Items) {
				unknownCandidate = snapshot
			}
			if len(snapshot.Items) == 0 {
				failures = append(failures, p.Name()+": index constituent membership is empty and cannot be confirmed")
			} else {
				failures = append(failures, p.Name()+": index constituent membership is present but source quality is unknown")
			}
			continue
		}
		markIndexConstituentCoverage(snapshot)
		if payload, err := json.Marshal(snapshot); err == nil {
			ttl := 30 * time.Minute
			if snapshot.QualityStatus != model.QualityValid {
				ttl = 5 * time.Minute
			}
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("index constituents cache write failed", "symbol", indexSymbol, "error", cacheErr)
			}
		} else {
			s.logger.Warn("index constituents cache encode failed", "symbol", indexSymbol, "error", err)
		}
		return snapshot, false, nil
	}
	if unknownCandidate != nil {
		unknownCandidate.QualityStatus = model.QualityUnknown
		reason := "index_constituents_fallback_exhausted_without_usable_quality"
		if len(unknownCandidate.Items) == 0 {
			reason = "index_constituents_fallback_exhausted_without_confirmed_data"
		}
		unknownCandidate.QualityReasons = appendReason(unknownCandidate.QualityReasons, reason)
		if payload, err := json.Marshal(unknownCandidate); err == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, 5*time.Minute); cacheErr != nil {
				s.logger.Warn("index constituents unknown cache write failed", "symbol", indexSymbol, "error", cacheErr)
			}
		}
		return unknownCandidate, false, nil
	}
	return nil, false, fmt.Errorf("all index-constituent providers failed: %s", strings.Join(failures, "; "))
}

func (s *DataService) IndexDailyBars(ctx context.Context, rawIndexSymbol, rawStartDate, rawEndDate string) (*model.IndexDailyBarsSnapshot, bool, error) {
	return s.IndexDailyBarsFromSource(ctx, rawIndexSymbol, rawStartDate, rawEndDate, "")
}

// betterUnknownIndexDailyBarsSnapshot 在回退耗尽后优先保留日 K 条目数相同但成交量、成交额、涨跌幅更完整的快照。
func betterUnknownIndexDailyBarsSnapshot(candidate, current *model.IndexDailyBarsSnapshot) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if len(candidate.Items) != len(current.Items) {
		return len(candidate.Items) > len(current.Items)
	}
	coverage := func(snapshot *model.IndexDailyBarsSnapshot) int {
		count := 0
		for _, item := range snapshot.Items {
			if item.ChangePercent != nil {
				count++
			}
			if item.Volume != nil {
				count++
			}
			if item.Amount != nil {
				count++
			}
			if item.ProviderTime != nil {
				count++
			}
		}
		if snapshot.ProviderTime != nil {
			count++
		}
		return count
	}
	left, right := coverage(candidate), coverage(current)
	if left != right {
		return left > right
	}
	if candidate.ProviderTime != nil && current.ProviderTime == nil {
		return true
	}
	if candidate.ProviderTime == nil && current.ProviderTime != nil {
		return false
	}
	if candidate.ProviderTime != nil && current.ProviderTime != nil && !candidate.ProviderTime.Equal(*current.ProviderTime) {
		return candidate.ProviderTime.After(*current.ProviderTime)
	}
	return false
}

func (s *DataService) IndexDailyBarsFromSource(ctx context.Context, rawIndexSymbol, rawStartDate, rawEndDate, rawSource string) (*model.IndexDailyBarsSnapshot, bool, error) {
	indexSymbol, err := canonicalIndexSymbol(rawIndexSymbol)
	if err != nil {
		return nil, false, err
	}
	startDate, endDate := strings.TrimSpace(rawStartDate), strings.TrimSpace(rawEndDate)
	start, startErr := time.Parse("2006-01-02", startDate)
	end, endErr := time.Parse("2006-01-02", endDate)
	if startErr != nil || start.Format("2006-01-02") != startDate || endErr != nil || end.Format("2006-01-02") != endDate {
		return nil, false, errors.New("start and end must use YYYY-MM-DD")
	}
	if start.After(end) {
		return nil, false, errors.New("start must not be after end")
	}
	if endDate > time.Now().In(marketLocation()).Format("2006-01-02") {
		return nil, false, errors.New("index daily-bars end date must not be in the future")
	}
	if end.After(start.AddDate(10, 0, 0)) {
		return nil, false, errors.New("index daily-bars date range must not exceed 10 years")
	}
	if int(end.Sub(start)/(24*time.Hour))+1 > 3660 {
		return nil, false, errors.New("index daily-bars date range must not exceed 3660 calendar days")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	providers, sourceKey, sourceErr := s.indexCatalogProvidersForSource(rawSource)
	if sourceErr != nil {
		return nil, false, sourceErr
	}
	if len(providers) == 0 {
		return nil, false, errors.New("no enabled index-catalog provider")
	}
	key := fmt.Sprintf("md:v2:index-daily-bars:%s:%s:%s:%s", sourceKey, indexSymbol, startDate, endDate)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.IndexDailyBarsSnapshot
		if json.Unmarshal(data, &cached) == nil && validateIndexDailyBarsSnapshot(indexSymbol, startDate, endDate, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := 15 * time.Minute
			cacheEligible := len(cached.Items) > 0 && cached.QualityStatus != model.QualityUnknown
			if cached.QualityStatus != model.QualityValid {
				ttl = 3 * time.Minute
			}
			if cached.QualityStatus == model.QualityUnknown {
				fallbackExhausted := false
				for _, reason := range cached.QualityReasons {
					if reason == "index_daily_bars_fallback_exhausted_without_confirmed_data" ||
						reason == "index_daily_bars_fallback_exhausted_without_usable_quality" {
						fallbackExhausted = true
						break
					}
				}
				// 多 Provider 场景忽略未记录回退耗尽的 unknown 缓存，包括“有 K 线条目但质量未知”的旧缓存。
				cacheEligible = fallbackExhausted || len(providers) <= 1
			}
			if cacheEligible && time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("index daily-bars cache read failed", "symbol", indexSymbol, "error", cacheErr)
	}
	var failures []string
	var unknownCandidate *model.IndexDailyBarsSnapshot
	for _, p := range providers {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchIndexDailyBars(ctx, indexSymbol, startDate, endDate)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateIndexDailyBarsSnapshot(indexSymbol, startDate, endDate, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if snapshot.QualityStatus == model.QualityUnknown || len(snapshot.Items) == 0 {
			// 无数据，或虽有 K 线但无法确认快照质量时，都先让后续来源尝试。
			if betterUnknownIndexDailyBarsSnapshot(snapshot, unknownCandidate) {
				unknownCandidate = snapshot
			}
			if len(snapshot.Items) == 0 {
				failures = append(failures, p.Name()+": index daily bars are empty and cannot be confirmed")
			} else {
				failures = append(failures, p.Name()+": index daily bars contain rows but snapshot quality is unknown")
			}
			continue
		}
		sort.Slice(snapshot.Items, func(i, j int) bool { return snapshot.Items[i].TradeDate < snapshot.Items[j].TradeDate })
		if payload, err := json.Marshal(snapshot); err == nil {
			ttl := 15 * time.Minute
			if snapshot.QualityStatus != model.QualityValid {
				ttl = 3 * time.Minute
			}
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("index daily-bars cache write failed", "symbol", indexSymbol, "error", cacheErr)
			}
		} else {
			s.logger.Warn("index daily-bars cache encode failed", "symbol", indexSymbol, "error", err)
		}
		return snapshot, false, nil
	}
	if unknownCandidate != nil {
		unknownCandidate.QualityStatus = model.QualityUnknown
		reason := "index_daily_bars_fallback_exhausted_without_usable_quality"
		if len(unknownCandidate.Items) == 0 {
			reason = "index_daily_bars_fallback_exhausted_without_confirmed_data"
		}
		unknownCandidate.QualityReasons = appendReason(unknownCandidate.QualityReasons, reason)
		if payload, err := json.Marshal(unknownCandidate); err == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, 3*time.Minute); cacheErr != nil {
				s.logger.Warn("index daily-bars unknown cache write failed", "symbol", indexSymbol, "error", cacheErr)
			}
		}
		return unknownCandidate, false, nil
	}
	return nil, false, fmt.Errorf("all index daily-bars providers failed: %s", strings.Join(failures, "; "))
}
