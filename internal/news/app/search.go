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
	request.QueryDimension = strings.TrimSpace(request.QueryDimension)
	if request.Query == "" || request.Limit < 1 || request.Limit > 100 {
		return domain.SearchResult{Status: domain.SearchFailed, FailureCode: "invalid_request"}
	}
	if s == nil || s.providers == nil {
		return domain.SearchResult{Status: domain.SearchSkipped, FailureCode: "provider_set_not_configured"}
	}
	if ctx == nil {
		return domain.SearchResult{Status: domain.SearchFailed, FailureCode: "invalid_context"}
	}
	if err := ctx.Err(); err != nil {
		return canceledSearchResult(domain.SearchResult{}, err)
	}

	providers := s.providers.All()
	if err := ctx.Err(); err != nil {
		return canceledSearchResult(domain.SearchResult{}, err)
	}
	if len(providers) == 0 {
		return domain.SearchResult{Status: domain.SearchSkipped, FailureCode: "no_provider_configured"}
	}

	codes, valid := validateProviderSet(providers)
	if !valid {
		return domain.SearchResult{Status: domain.SearchFailed, FailureCode: "provider_registry_invalid"}
	}

	result := domain.SearchResult{Status: domain.SearchEmpty}
	for index, provider := range providers {
		if err := ctx.Err(); err != nil {
			return canceledSearchResult(result, err)
		}
		code := codes[index]
		started := s.now()
		candidates, err := provider.Search(ctx, request)
		finished := s.now()
		if ctxErr := ctx.Err(); ctxErr != nil {
			// 当前 Provider 的结果是在请求取消后返回的，不作为已完成 Outcome；
			// 之前已完成的 Provider Outcome 和候选仍保留。
			return canceledSearchResult(result, ctxErr)
		}

		outcome := domain.ProviderOutcome{
			ProviderCode:   code,
			QueryDimension: request.QueryDimension,
			Status:         domain.ProviderSuccess,
			CandidateCount: len(candidates),
			ObservedAt:     finished,
			Duration:       finished.Sub(started),
		}
		if err != nil {
			outcome.Status = domain.ProviderFailed
			outcome.CandidateCount = 0
			outcome.ErrorCode = ProviderErrorUnknown
			var providerErr *ProviderError
			if errors.As(err, &providerErr) {
				outcome.ErrorCode = normalizeProviderErrorCode(providerErr.Code)
				outcome.Retryable = providerErrorRetryable(outcome.ErrorCode, providerErr.Retryable)
				if providerErr.Skipped && outcome.ErrorCode == ProviderErrorNotConfigured {
					outcome.Status = domain.ProviderSkipped
				}
			}
			candidates = nil
		} else if len(candidates) == 0 {
			outcome.Status = domain.ProviderEmpty
		}
		result.Outcomes = append(result.Outcomes, outcome)

		for i := range candidates {
			candidates[i].ProviderCode = code
			candidates[i] = domain.NormalizeCandidate(candidates[i], finished, request.QueryDimension)
		}
		result.Candidates = append(result.Candidates, candidates...)
	}

	if err := ctx.Err(); err != nil {
		return canceledSearchResult(result, err)
	}
	summarizeSearchResult(&result)
	return result
}

// validateProviderSet 在任何上游请求发生前校验 Provider 身份，避免空身份、重复身份
// 或 nil 实例造成结果归因歧义和不完整执行。
func validateProviderSet(providers []Provider) ([]string, bool) {
	codes := make([]string, len(providers))
	seen := make(map[string]struct{}, len(providers))
	for index, provider := range providers {
		if provider == nil {
			return nil, false
		}
		code := provider.Code()
		if code == "" || strings.TrimSpace(code) != code {
			return nil, false
		}
		if _, exists := seen[code]; exists {
			return nil, false
		}
		seen[code] = struct{}{}
		codes[index] = code
	}
	return codes, true
}

// canceledSearchResult 保留此前已完成 Provider 的诊断结果，但不把请求级取消伪装成 Provider 故障。
func canceledSearchResult(result domain.SearchResult, err error) domain.SearchResult {
	result.Status = domain.SearchFailed
	result.FailureCode = "context_canceled"
	if errors.Is(err, context.DeadlineExceeded) {
		result.FailureCode = "context_deadline_exceeded"
	}
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

// normalizeProviderErrorCode prevents adapter-specific or arbitrary error strings from
// becoming part of the stable SearchResult contract.
func normalizeProviderErrorCode(code string) string {
	switch strings.TrimSpace(code) {
	case ProviderErrorUnauthorized,
		ProviderErrorRateLimited,
		ProviderErrorTimeout,
		ProviderErrorTransportFailed,
		ProviderErrorRequestInvalid,
		ProviderErrorResponseInvalid,
		ProviderErrorBusinessRejected,
		ProviderErrorUpstreamUnavailable,
		ProviderErrorNotConfigured:
		return strings.TrimSpace(code)
	default:
		return ProviderErrorUnknown
	}
}

func providerErrorRetryable(code string, retryable bool) bool {
	switch code {
	case ProviderErrorRateLimited, ProviderErrorTimeout, ProviderErrorUpstreamUnavailable:
		return true
	case ProviderErrorTransportFailed:
		return retryable
	default:
		return false
	}
}
