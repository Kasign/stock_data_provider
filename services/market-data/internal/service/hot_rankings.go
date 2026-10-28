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

func (s *DataService) SetHotRankingProviders(providers []provider.HotRankingProvider) {
	s.hotRankingProviders = append([]provider.HotRankingProvider(nil), providers...)
}

func (s *DataService) HotRankingProviders() []provider.HotRankingProvider {
	return append([]provider.HotRankingProvider(nil), s.hotRankingProviders...)
}

func validateHotRankingSnapshot(rankingType string, snapshot *model.HotRankingSnapshot) error {
	if snapshot == nil || snapshot.RankingType != rankingType || snapshot.Total != len(snapshot.Items) {
		return errors.New("hot-ranking snapshot identity/count mismatch")
	}
	if strings.TrimSpace(snapshot.Source) == "" || strings.TrimSpace(snapshot.SourceVersion) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("hot-ranking source, source_version or fetched_at is missing")
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded && snapshot.QualityStatus != model.QualityUnknown {
		return fmt.Errorf("unsupported hot-ranking quality %q", snapshot.QualityStatus)
	}
	if snapshot.ProviderTime == nil {
		if snapshot.QualityStatus == model.QualityValid {
			return errors.New("hot-ranking snapshot cannot be valid without provider time")
		}
	} else if snapshot.ProviderTime.IsZero() || snapshot.ProviderTime.After(time.Now().Add(5*time.Minute)) {
		return errors.New("hot-ranking provider time is zero or in the future")
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	seenRanks := make(map[int]struct{}, len(snapshot.Items))
	hasDegradedItem := false
	for _, item := range snapshot.Items {
		symbol, err := normalize.NormalizeSymbol(item.Symbol)
		if err != nil || symbol != item.Symbol || item.Code != strings.Split(symbol, ".")[0] || item.Rank < 1 {
			return errors.New("hot-ranking item has invalid symbol/code/rank")
		}
		if _, duplicate := seen[symbol]; duplicate {
			return fmt.Errorf("hot-ranking snapshot contains duplicate symbol %s", symbol)
		}
		seen[symbol] = struct{}{}
		if _, duplicate := seenRanks[item.Rank]; duplicate {
			return fmt.Errorf("hot-ranking snapshot contains duplicate rank %d", item.Rank)
		}
		seenRanks[item.Rank] = struct{}{}
		if item.Source != snapshot.Source || item.SourceVersion != snapshot.SourceVersion || item.FetchedAt.IsZero() {
			return errors.New("hot-ranking item source/version/fetched_at mismatch")
		}
		if item.QualityStatus != model.QualityValid && item.QualityStatus != model.QualityDegraded {
			return fmt.Errorf("hot-ranking item has unsupported quality %q", item.QualityStatus)
		}
		if item.QualityStatus == model.QualityDegraded {
			hasDegradedItem = true
		}
		for _, value := range []*float64{item.LatestPrice, item.Change, item.ChangePercent} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return errors.New("hot-ranking item contains a non-finite quote field")
			}
		}
		if item.LatestPrice != nil && *item.LatestPrice <= 0 {
			return errors.New("hot-ranking item contains a non-positive latest price")
		}
	}
	if len(snapshot.Items) == 0 && snapshot.QualityStatus != model.QualityUnknown {
		return errors.New("empty hot-ranking snapshot must be marked unknown")
	}
	if snapshot.QualityStatus == model.QualityValid && hasDegradedItem {
		return errors.New("hot-ranking snapshot cannot be valid when one or more items are degraded")
	}
	return nil
}

// betterUnknownHotRankingSnapshot 在回退耗尽后优先保留条目更多、行情/热度字段更完整的榜单候选。
func betterUnknownHotRankingSnapshot(candidate, current *model.HotRankingSnapshot) bool {
	if candidate == nil {
		return false
	}
	if current == nil {
		return true
	}
	if len(candidate.Items) != len(current.Items) {
		return len(candidate.Items) > len(current.Items)
	}
	metricCount := func(snapshot *model.HotRankingSnapshot) int {
		count := 0
		for _, item := range snapshot.Items {
			for _, value := range []*float64{item.LatestPrice, item.Change, item.ChangePercent, item.Heat} {
				if value != nil {
					count++
				}
			}
			if item.RankChange != nil {
				count++
			}
			if strings.TrimSpace(item.Name) != "" {
				count++
			}
			if strings.TrimSpace(item.RankTrend) != "" {
				count++
			}
		}
		return count
	}
	left, right := metricCount(candidate), metricCount(current)
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

func (s *DataService) HotStocks(ctx context.Context, rawType string, limit int) (*model.HotRankingSnapshot, bool, error) {
	rankingType := strings.ToLower(strings.TrimSpace(rawType))
	if rankingType == "" {
		rankingType = "popular"
	}
	if rankingType != "popular" && rankingType != "surge" && rankingType != "followers" {
		return nil, false, errors.New("hot-ranking type must be popular, surge or followers")
	}
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 100 {
		return nil, false, errors.New("hot-ranking limit must be between 1 and 100")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key := fmt.Sprintf("md:v1:hot-ranking:%s:%d", rankingType, limit)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.HotRankingSnapshot
		if json.Unmarshal(data, &cached) == nil && validateHotRankingSnapshot(rankingType, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := 90 * time.Second
			if cached.QualityStatus == model.QualityDegraded {
				ttl = 30 * time.Second
			} else if cached.QualityStatus == model.QualityUnknown {
				ttl = 10 * time.Second
			}
			cacheEligible := len(cached.Items) > 0 && cached.QualityStatus != model.QualityUnknown
			if cached.QualityStatus == model.QualityUnknown {
				fallbackExhausted := false
				for _, reason := range cached.QualityReasons {
					if reason == "hot_ranking_fallback_exhausted_without_confirmed_data" ||
						reason == "hot_ranking_fallback_exhausted_without_usable_quality" {
						fallbackExhausted = true
						break
					}
				}
				// 多 Provider 场景中，unknown 即使带有条目，也必须先让后备源刷新。
				cacheEligible = fallbackExhausted || len(s.hotRankingProviders) <= 1
			}
			if cacheEligible && time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("hot-ranking cache read failed", "type", rankingType, "error", cacheErr)
	}
	if len(s.hotRankingProviders) == 0 {
		return nil, false, errors.New("no enabled hot-ranking provider")
	}
	var failures []string
	var unknownCandidate *model.HotRankingSnapshot
	for _, p := range s.hotRankingProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchHotStocks(ctx, rankingType, limit)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateHotRankingSnapshot(rankingType, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if snapshot.QualityStatus == model.QualityUnknown || len(snapshot.Items) == 0 {
			// 空榜或虽有条目但质量未知，都不能阻断后续来源。
			if betterUnknownHotRankingSnapshot(snapshot, unknownCandidate) {
				unknownCandidate = snapshot
			}
			if len(snapshot.Items) == 0 {
				failures = append(failures, p.Name()+": hot-ranking result is empty and cannot be confirmed")
			} else {
				failures = append(failures, p.Name()+": hot-ranking rows are present but snapshot quality is unknown")
			}
			continue
		}
		encoded, encodeErr := json.Marshal(snapshot)
		if encodeErr == nil {
			ttl := 90 * time.Second
			if snapshot.QualityStatus == model.QualityDegraded {
				ttl = 30 * time.Second
			}
			if cacheErr := s.cache.Set(ctx, key, encoded, ttl); cacheErr != nil {
				s.logger.Warn("hot-ranking cache write failed", "type", rankingType, "error", cacheErr)
			}
		} else {
			s.logger.Warn("hot-ranking cache encode failed", "type", rankingType, "error", encodeErr)
		}
		return snapshot, false, nil
	}
	if unknownCandidate != nil {
		unknownCandidate.QualityStatus = model.QualityUnknown
		reason := "hot_ranking_fallback_exhausted_without_usable_quality"
		if len(unknownCandidate.Items) == 0 {
			reason = "hot_ranking_fallback_exhausted_without_confirmed_data"
		}
		unknownCandidate.QualityReasons = appendReason(unknownCandidate.QualityReasons, reason)
		if encoded, encodeErr := json.Marshal(unknownCandidate); encodeErr == nil {
			if cacheErr := s.cache.Set(ctx, key, encoded, 10*time.Second); cacheErr != nil {
				s.logger.Warn("hot-ranking unknown cache write failed", "type", rankingType, "error", cacheErr)
			}
		}
		return unknownCandidate, false, nil
	}
	return nil, false, fmt.Errorf("all hot-ranking providers failed: %s", strings.Join(failures, "; "))
}
