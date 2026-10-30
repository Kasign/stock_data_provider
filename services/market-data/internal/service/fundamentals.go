package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

func (s *DataService) SetFundamentalsProviders(providers []provider.FundamentalsProvider) {
	s.fundamentalsProviders = append([]provider.FundamentalsProvider(nil), providers...)
}
func (s *DataService) FundamentalsProviders() []provider.FundamentalsProvider {
	return append([]provider.FundamentalsProvider(nil), s.fundamentalsProviders...)
}
func fundamentalPeriodEnd(year, quarter int) string {
	return time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

func validateFundamentalsSnapshot(symbol string, year, quarter int, snapshot *model.FundamentalsSnapshot) error {
	if snapshot == nil {
		return errors.New("fundamentals provider returned an empty snapshot")
	}
	got, err := normalize.NormalizeSymbol(snapshot.Symbol)
	if err != nil || got != symbol || snapshot.Year != year || snapshot.Quarter != quarter {
		return errors.New("fundamentals snapshot identity/period mismatch")
	}
	if strings.TrimSpace(snapshot.Source) == "" || !hasIdentifiableSourceVersion(snapshot.SourceVersion) || snapshot.FetchedAt.IsZero() {
		return errors.New("fundamentals source, source_version or fetched_at is missing")
	}
	// 六个标准指标分区始终必需；同花顺可额外返回三张独立财务报表。
	if len(snapshot.Sections) < 6 || len(snapshot.Sections) > 9 {
		return fmt.Errorf("fundamentals snapshot has %d sections, expected six indicator groups and up to three statement groups", len(snapshot.Sections))
	}
	allowed := map[string]bool{
		"profitability": true, "operating": true, "growth": true,
		"solvency": true, "cash_flow": true, "dupont": true,
		"income_statement": true, "balance_sheet": true, "cash_flow_statement": true,
	}
	standardCategories := []string{"profitability", "operating", "growth", "solvency", "cash_flow", "dupont"}
	seen := make(map[string]bool, len(snapshot.Sections))
	hasData, allValid := false, true
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	for _, section := range snapshot.Sections {
		if !allowed[section.Category] || seen[section.Category] {
			return fmt.Errorf("unsupported or duplicate fundamentals section %q", section.Category)
		}
		seen[section.Category] = true
		if section.HasData != (len(section.Metrics) > 0) {
			return fmt.Errorf("fundamentals section %s has inconsistent has_data/metrics", section.Category)
		}
		if section.QualityStatus != model.QualityValid && section.QualityStatus != model.QualityDegraded && section.QualityStatus != model.QualityUnknown {
			return fmt.Errorf("fundamentals section %s has unsupported quality %q", section.Category, section.QualityStatus)
		}
		if section.HasData {
			hasData = true
			if section.ReportDate != fundamentalPeriodEnd(year, quarter) {
				return fmt.Errorf("fundamentals section %s report date mismatches requested period", section.Category)
			}
			if section.ReportDate > today {
				return fmt.Errorf("fundamentals section %s report period is in the future", section.Category)
			}
			if section.PublicationDate == "" {
				if section.AvailableAt == nil && section.QualityStatus == model.QualityValid {
					return fmt.Errorf("fundamentals section %s cannot be valid without publication_date or available_at", section.Category)
				}
			} else {
				publicationDate, dateErr := time.Parse("2006-01-02", section.PublicationDate)
				if dateErr != nil || publicationDate.Format("2006-01-02") != section.PublicationDate {
					return fmt.Errorf("fundamentals section %s has invalid publication date", section.Category)
				}
				if section.PublicationDate < section.ReportDate {
					return fmt.Errorf("fundamentals section %s publication date precedes the report period end", section.Category)
				}
				if section.PublicationDate > today {
					return fmt.Errorf("fundamentals section %s publication date is in the future", section.Category)
				}
			}
			if section.AvailableAt != nil {
				if section.AvailableAt.IsZero() || section.AvailableAt.After(time.Now().UTC()) {
					return fmt.Errorf("fundamentals section %s has invalid or future available_at", section.Category)
				}
				availableDate := section.AvailableAt.In(marketLocation()).Format("2006-01-02")
				if availableDate < section.ReportDate || (section.PublicationDate != "" && availableDate != section.PublicationDate) {
					return fmt.Errorf("fundamentals section %s available_at conflicts with report/publication date", section.Category)
				}
			}
			if section.QualityStatus != model.QualityValid {
				allValid = false
			}
		} else {
			allValid = false
		}
		seenMetricKeys := make(map[string]struct{}, len(section.Metrics))
		for _, metric := range section.Metrics {
			key := strings.TrimSpace(metric.Key)
			if key == "" || strings.TrimSpace(metric.Name) == "" || strings.TrimSpace(metric.Unit) == "" ||
				strings.TrimSpace(metric.SourceField) == "" || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
				return fmt.Errorf("fundamentals section %s has invalid metric metadata/value", section.Category)
			}
			if _, exists := seenMetricKeys[key]; exists {
				return fmt.Errorf("fundamentals section %s has duplicate metric key %q", section.Category, key)
			}
			seenMetricKeys[key] = struct{}{}
		}
	}
	for _, category := range standardCategories {
		if !seen[category] {
			return fmt.Errorf("fundamentals snapshot is missing required indicator section %q", category)
		}
	}
	if snapshot.HasData != hasData {
		return errors.New("fundamentals snapshot has inconsistent has_data")
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded && snapshot.QualityStatus != model.QualityUnknown {
		return fmt.Errorf("fundamentals snapshot has unsupported quality %q", snapshot.QualityStatus)
	}
	if snapshot.QualityStatus == model.QualityValid && (!hasData || !allValid) {
		return errors.New("fundamentals snapshot cannot be valid with missing/incomplete sections")
	}
	return nil
}

func fundamentalsCacheTTL(snapshot *model.FundamentalsSnapshot) time.Duration {
	if snapshot == nil || !snapshot.HasData || snapshot.QualityStatus == model.QualityUnknown {
		return 5 * time.Minute
	}
	if snapshot.QualityStatus == model.QualityDegraded {
		return time.Hour
	}
	return 12 * time.Hour
}

// betterUnknownFundamentalsSnapshot 在回退耗尽后优先保留已填充分区与指标更多的财务候选；同覆盖时保留先注册的来源。
func betterUnknownFundamentalsSnapshot(candidate, current *model.FundamentalsSnapshot) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if candidate.HasData != current.HasData {
		return candidate.HasData
	}
	type coverage struct {
		dataSections     int
		metricCount      int
		publicationDates int
	}
	score := func(snapshot *model.FundamentalsSnapshot) coverage {
		result := coverage{}
		for _, section := range snapshot.Sections {
			if section.HasData {
				result.dataSections++
			}
			result.metricCount += len(section.Metrics)
			if strings.TrimSpace(section.PublicationDate) != "" {
				result.publicationDates++
			}
		}
		return result
	}
	left, right := score(candidate), score(current)
	if left.dataSections != right.dataSections {
		return left.dataSections > right.dataSections
	}
	if left.metricCount != right.metricCount {
		return left.metricCount > right.metricCount
	}
	if left.publicationDates != right.publicationDates {
		return left.publicationDates > right.publicationDates
	}
	return false
}

func (s *DataService) Fundamentals(ctx context.Context, rawSymbol string, year, quarter int) (*model.FundamentalsSnapshot, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	if year < 2007 || year > time.Now().Year() {
		return nil, false, fmt.Errorf("fundamental year must be between 2007 and %d", time.Now().Year())
	}
	if quarter < 1 || quarter > 4 {
		return nil, false, errors.New("fundamental quarter must be between 1 and 4")
	}
	periodEnd := fundamentalPeriodEnd(year, quarter)
	if periodEnd > time.Now().In(marketLocation()).Format("2006-01-02") {
		return nil, false, errors.New("fundamental report period must not be in the future")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key := fmt.Sprintf("md:v1:fundamentals:%s:%d:%d", symbol, year, quarter)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.FundamentalsSnapshot
		if json.Unmarshal(data, &cached) == nil && validateFundamentalsSnapshot(symbol, year, quarter, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := fundamentalsCacheTTL(&cached)
			cacheEligible := cached.QualityStatus != model.QualityUnknown
			if cached.QualityStatus == model.QualityUnknown {
				fallbackExhausted := false
				for _, reason := range cached.QualityReasons {
					if reason == "fundamentals_fallback_exhausted_without_confirmed_data" ||
						reason == "fundamentals_fallback_exhausted_without_usable_quality" {
						fallbackExhausted = true
						break
					}
				}
				// 多 Provider 场景忽略未记录回退耗尽标记的 unknown 缓存，让更可靠来源有机会刷新结果。
				cacheEligible = fallbackExhausted || len(s.fundamentalsProviders) <= 1
			}
			if cacheEligible && time.Since(cached.FetchedAt) < ttl {
				s.saveFundamentalsRevision(ctx, &cached)
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("fundamentals cache read failed", "symbol", symbol, "year", year, "quarter", quarter, "error", cacheErr)
	}
	if len(s.fundamentalsProviders) == 0 {
		return nil, false, errors.New("no enabled fundamentals provider")
	}
	var failures []string
	var unknownCandidate *model.FundamentalsSnapshot
	for _, p := range s.fundamentalsProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchFundamentals(ctx, symbol, year, quarter)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if snapshot != nil {
			// fetched_at 是本服务收到 Provider 回包的时间，不能沿用上游传入值。
			snapshot.FetchedAt = time.Now().UTC()
		}
		if err := validateFundamentalsSnapshot(symbol, year, quarter, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if !snapshot.HasData || snapshot.QualityStatus == model.QualityUnknown {
			// Unknown-quality metrics may lack a provable publication date; keep them as a final candidate,
			// but allow later providers to return a point-in-time safer snapshot first.
			if betterUnknownFundamentalsSnapshot(snapshot, unknownCandidate) {
				unknownCandidate = snapshot
			}
			if !snapshot.HasData {
				failures = append(failures, p.Name()+": no report-period metrics")
			} else {
				failures = append(failures, p.Name()+": fundamentals quality is unknown")
			}
			continue
		}
		s.saveFundamentalsRevision(ctx, snapshot)
		payload, err := json.Marshal(snapshot)
		if err != nil {
			s.logger.Warn("fundamentals cache encode failed", "symbol", symbol, "year", year, "quarter", quarter, "error", err)
		} else {
			ttl := fundamentalsCacheTTL(snapshot)
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("fundamentals cache write failed", "symbol", symbol, "year", year, "quarter", quarter, "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	if unknownCandidate != nil {
		unknownCandidate.QualityStatus = model.QualityUnknown
		if unknownCandidate.HasData {
			unknownCandidate.QualityReasons = appendReason(unknownCandidate.QualityReasons, "fundamentals_fallback_exhausted_without_usable_quality")
		} else {
			unknownCandidate.QualityReasons = appendReason(unknownCandidate.QualityReasons, "fundamentals_fallback_exhausted_without_confirmed_data")
		}
		s.saveFundamentalsRevision(ctx, unknownCandidate)
		if payload, err := json.Marshal(unknownCandidate); err == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, fundamentalsCacheTTL(unknownCandidate)); cacheErr != nil {
				s.logger.Warn("fundamentals unknown cache write failed", "symbol", symbol, "year", year, "quarter", quarter, "error", cacheErr)
			}
		}
		return unknownCandidate, false, nil
	}
	return nil, false, fmt.Errorf("all fundamentals providers failed: %s", strings.Join(failures, "; "))
}

var (
	ErrFundamentalsAsOfUnavailable = errors.New("fundamentals point-in-time storage is not enabled")
	ErrFundamentalsAsOfNotFound    = errors.New("no usable fundamentals revision was recorded by the requested as_of time")
)

func (s *DataService) saveFundamentalsRevision(ctx context.Context, snapshot *model.FundamentalsSnapshot) {
	if s == nil || s.store == nil || snapshot == nil {
		return
	}
	if err := s.store.SaveFundamentals(ctx, snapshot); err != nil && s.logger != nil {
		s.logger.Warn("fundamentals revision write failed", "symbol", snapshot.Symbol, "year", snapshot.Year,
			"quarter", snapshot.Quarter, "source", snapshot.Source, "error", err)
	}
}

// FundamentalsAsOf reads only locally recorded revisions and never calls a live Provider.
// A missing or same-day disclosure date is not sufficient evidence for strict timestamp PIT.
func (s *DataService) FundamentalsAsOf(ctx context.Context, rawSymbol string, year, quarter int, asOf time.Time) (*model.FundamentalsSnapshot, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if year < 2007 || year > time.Now().Year() || quarter < 1 || quarter > 4 {
		return nil, errors.New("fundamental report year/quarter is invalid")
	}
	if asOf.IsZero() || asOf.After(time.Now().UTC()) {
		return nil, errors.New("fundamentals as_of must be a valid non-future timestamp")
	}
	if fundamentalPeriodEnd(year, quarter) > time.Now().In(marketLocation()).Format("2006-01-02") {
		return nil, errors.New("fundamental report period must not be in the future")
	}
	if s == nil || s.store == nil {
		return nil, ErrFundamentalsAsOfUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	snapshots, err := s.store.ListFundamentalsAsOf(ctx, symbol, year, quarter, asOf)
	if err != nil {
		return nil, err
	}
	var best *model.FundamentalsSnapshot
	bestScore := -1
	for index := range snapshots {
		candidate := &snapshots[index]
		if validateFundamentalsSnapshot(symbol, year, quarter, candidate) != nil ||
			candidate.FetchedAt.After(asOf) || candidate.SystemObservedAt == nil ||
			candidate.SystemObservedAt.After(asOf) || !candidate.HasData ||
			candidate.QualityStatus == model.QualityUnknown {
			continue
		}
		filtered, usable := filterFundamentalsSnapshotAsOf(candidate, asOf)
		if !usable || filtered == nil {
			continue
		}
		score := 0
		if filtered.QualityStatus == model.QualityValid {
			score = 2
		} else if filtered.QualityStatus == model.QualityDegraded {
			score = 1
		}
		if best == nil || score > bestScore ||
			(score == bestScore && filtered.SystemObservedAt != nil && best.SystemObservedAt != nil &&
				filtered.SystemObservedAt.After(*best.SystemObservedAt)) {
			best, bestScore = filtered, score
		}
	}
	if best == nil {
		return nil, ErrFundamentalsAsOfNotFound
	}
	return best, nil
}

func filterFundamentalsSnapshotAsOf(snapshot *model.FundamentalsSnapshot, asOf time.Time) (*model.FundamentalsSnapshot, bool) {
	if snapshot == nil || asOf.IsZero() {
		return nil, false
	}
	asOfDate := asOf.In(marketLocation()).Format("2006-01-02")
	filtered := *snapshot
	filtered.Sections = make([]model.FundamentalSection, 0, len(snapshot.Sections))
	hasUsableData, excludedSection := false, false
	for _, original := range snapshot.Sections {
		section := original
		if section.HasData {
			availableBeforeAsOf := false
			if section.AvailableAt != nil && !section.AvailableAt.IsZero() {
				// 精确披露时间允许同一自然日内进行 PIT 判断，边界采用闭区间。
				availableBeforeAsOf = !section.AvailableAt.After(asOf)
			} else {
				// 只有披露日期没有披露时刻时，必须早于 as_of 所在自然日才可证明已公开。
				availableBeforeAsOf = strings.TrimSpace(section.PublicationDate) != "" && section.PublicationDate < asOfDate
			}
			if !availableBeforeAsOf {
				section.HasData = false
				section.Metrics = []model.FundamentalMetric{}
				section.QualityStatus = model.QualityUnknown
				section.QualityReasons = appendReason(section.QualityReasons, "pit_section_excluded_available_at_not_proven_before_as_of")
				excludedSection = true
			}
		}
		if section.HasData && len(section.Metrics) > 0 {
			hasUsableData = true
		}
		filtered.Sections = append(filtered.Sections, section)
	}
	filtered.HasData = hasUsableData
	if excludedSection {
		filtered.QualityStatus = model.QualityDegraded
		filtered.QualityReasons = appendReason(filtered.QualityReasons, "pit_excluded_undated_or_not_yet_proven_publication_section")
	}
	return &filtered, hasUsableData
}
