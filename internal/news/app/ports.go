package app

import (
	"context"
	"errors"
	"net/http"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrInvalidRequest = errors.New("news: invalid search request")
	ErrProviderConfig = errors.New("news: invalid provider configuration")
)

// Provider 错误码是 News 内部稳定分类；不得直接透传任意上游 HTTP 状态或 Platform Fault Code。
const (
	ProviderErrorUnauthorized       = "provider_unauthorized"
	ProviderErrorRateLimited        = "provider_rate_limited"
	ProviderErrorTimeout            = "provider_timeout"
	ProviderErrorTransportFailed    = "provider_transport_failed"
	ProviderErrorRequestInvalid     = "provider_request_invalid"
	ProviderErrorResponseInvalid    = "provider_response_invalid"
	ProviderErrorBusinessRejected   = "provider_business_rejected"
	ProviderErrorUpstreamUnavailable = "provider_upstream_unavailable"
	ProviderErrorUnknown            = "provider_unknown"
	ProviderErrorNotConfigured      = "provider_not_configured"
)

// ProviderError 表达 Provider 层可安全透传的业务错误分类。
// Retryable 仅是技术重试候选，不代表 Application 会自动重试。
type ProviderError struct {
	Code      string
	Retryable bool
	Skipped   bool
}

func (e *ProviderError) Error() string {
	if e == nil || e.Code == "" {
		return "news: provider request failed"
	}
	return "news: provider " + e.Code
}

// HTTPDoer 是窄传输端口；生产环境由组合根注入绑定了 Platform DestinationPolicy 的 Doer。
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// Provider 只负责一个 Provider 的协议适配，不做跨 Provider 聚合、缓存或排序。
type Provider interface {
	Code() string
	Search(context.Context, domain.SearchRequest) ([]domain.Candidate, error)
}

// ProviderSet 是应用层使用的 Provider 集合。
type ProviderSet interface {
	All() []Provider
}

// StaticProviderSet 用于组合根和测试。
type StaticProviderSet struct {
	Providers []Provider
}

func (s StaticProviderSet) All() []Provider {
	return append([]Provider(nil), s.Providers...)
}
