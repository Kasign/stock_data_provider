package app

import (
	"context"
	"errors"
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
	if request.Query == "" || request.Limit < 1 || request.Limit > 100 {
		return domain.SearchResult{Status: domain.SearchFailed, FailureCode: "invalid_request"}
	}
	if s == nil || s.providers == nil {
		return domain.SearchResult{Status: domain.SearchSkipped, FailureCode: "provider_set_not_configured"}
	}
	if ctx == nil {
		return domain.SearchResult{Status: domain.SearchFailed, FailureCode: "invalid_context"}
	}

	providers := s.providers.All()
	if len(providers) == 0 {
		return domain.SearchResult{Status: domain.SearchSkipped, FailureCode: "no_provider_configured"}
	}

	result := domain.SearchResult{Status: domain.SearchEmpty}
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		code := strings.TrimSpace(provider.Code())
		started := s.now()
		candidates, err := provider.Search(ctx, request)
		finished := s.now()
		outcome := domain.ProviderOutcome{
			ProviderCode:   code,
			QueryDimension: request.QueryDimension,
			Status:         domain.ProviderSuccess,
			CandidateCount: len(candidates),
			ObservedAt:     finished,
			Duration:       finished.Sub(started),
		}
		if code == "" {
			outcome.Status = domain.ProviderFailed
			outcome.ErrorCode = "provider_code_missing"
			outcome.CandidateCount = 0
			candidates = nil
		} else if err != nil {
			outcome.Status = domain.ProviderFailed
			outcome.ErrorCode = "provider_request_failed"
			var providerErr *ProviderError
			if errors.As(err, &providerErr) {
				outcome.ErrorCode = providerErr.Code
				outcome.Retryable = providerErr.Retryable
				if providerErr.Skipped {
					outcome.Status = domain.ProviderSkipped
					outcome.ErrorCode = providerErr.Code
				}
			}
			candidates = nil
		} else if len(candidates) == 0 {
			outcome.Status = domain.ProviderEmpty
		}
		result.Outcomes = append(result.Outcomes, outcome)

		for i := range candidates {
			candidates[i].ProviderCode = code
			if candidates[i].ObservedAt.IsZero() {
				candidates[i].ObservedAt = finished
			}
			if candidates[i].QueryDimension == "" {
				candidates[i].QueryDimension = request.QueryDimension
			}
		}
		result.Candidates = append(result.Candidates, candidates...)
	}

	summarizeSearchResult(&result)
	return result
}

func summarizeSearchResult(result *domain.SearchResult) {
	succeeded, failed, skipped := 0, 0, 0
	for _, outcome := range result.Outcomes {
		switch outcome.Status {
		case domain.ProviderSuccess, domain.ProviderEmpty:
			succeeded++
		case domain.ProviderFailed:
			failed++
		case domain.ProviderSkipped:
			skipped++
		}
	}

	switch {
	case succeeded == 0 && failed == 0:
		result.Status = domain.SearchSkipped
		result.FailureCode = "all_providers_skipped"
	case succeeded == 0:
		result.Status = domain.SearchFailed
		result.FailureCode = "all_provider_requests_failed"
	case failed > 0 || skipped > 0:
		result.Status = domain.SearchPartial
		result.FailureCode = "provider_partial_failure"
	case len(result.Candidates) == 0:
		result.Status = domain.SearchEmpty
		result.FailureCode = ""
	default:
		result.Status = domain.SearchSuccess
		result.FailureCode = ""
	}
}
