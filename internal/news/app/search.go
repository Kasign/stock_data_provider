package app

import (
	"context"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

// SearchCandidates 执行各 Provider 查询并保留 Provider 级结果；不持久化、不缓存。
type SearchCandidates struct {
	providers ProviderSet
	now       func() time.Time
}

func NewSearchCandidates(providers ProviderSet) *SearchCandidates {
	return &SearchCandidates{providers: providers, now: time.Now}
}

func (s *SearchCandidates) Execute(ctx context.Context, request domain.SearchRequest) domain.SearchResult {
	request.Query = strings.TrimSpace(request.Query)
	if request.Query == "" || request.Limit < 1 || request.Limit > 100 || s == nil || s.providers == nil {
		return domain.SearchResult{Status: domain.SearchFailed}
	}
	providers := s.providers.All()
	if len(providers) == 0 {
		return domain.SearchResult{Status: domain.SearchSkipped}
	}

	result := domain.SearchResult{Status: domain.SearchEmpty}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		started := s.now()
		candidates, err := provider.Search(ctx, request)
		finished := s.now()
		outcome := domain.ProviderOutcome{
			ProviderCode: provider.Code(),
			Status: domain.ProviderSuccess,
			CandidateCount: len(candidates),
			ObservedAt: finished,
			Duration: finished.Sub(started),
		}
		if err != nil {
			outcome.Status = domain.ProviderFailed
			outcome.ErrorCode = "provider_request_failed"
		} else if len(candidates) == 0 {
			outcome.Status = domain.ProviderEmpty
		}
		result.Outcomes = append(result.Outcomes, outcome)
		for i := range candidates {
			candidates[i].ProviderCode = provider.Code()
			if candidates[i].ObservedAt.IsZero() {
				candidates[i].ObservedAt = finished
			}
			if candidates[i].QueryDimension == "" {
				candidates[i].QueryDimension = request.QueryDimension
			}
		}
		result.Candidates = append(result.Candidates, candidates...)
	}
	successes, failures := 0, 0
	for _, outcome := range result.Outcomes {
		switch outcome.Status {
		case domain.ProviderSuccess:
			successes++
		case domain.ProviderFailed:
			failures++
		}
	}
	switch {
	case successes > 0 && failures > 0:
		result.Status = domain.SearchPartial
	case successes > 0 && len(result.Candidates) > 0:
		result.Status = domain.SearchSuccess
	case successes > 0:
		result.Status = domain.SearchEmpty
	case failures > 0:
		result.Status = domain.SearchFailed
	default:
		result.Status = domain.SearchSkipped
	}
	return result
}
