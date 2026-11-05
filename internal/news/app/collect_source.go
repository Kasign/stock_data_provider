package app

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/adapters/parser"
	"example.com/stock-analysis-flow/internal/news/domain"
)

const (
	defaultSourceResponseLimit int64 = 2 << 20
	maximumSourceResponseLimit int64 = 16 << 20
	defaultSourceEntryLimit          = 200
	maximumSourceEntryLimit          = 1000
)

var (
	ErrSourceCollectionRequest = errors.New("news: invalid source collection request")
	ErrSourceEndpointNotFound  = errors.New("news: source endpoint not found")
)

// SourceEndpointRegistration 是组合根提供的已登记来源入口快照。
// SourcePolicy 是发布者级默认授权；EndpointPolicy 仅表达入口级差异。
type SourceEndpointRegistration struct {
	SourceID         string
	EndpointID       string
	Enabled          bool
	URL              string
	PublisherName    string
	PublisherDomain  string
	AccessMethod     domain.AccessMethod
	SourcePolicy     domain.SourcePolicy
	EndpointPolicy   *domain.EndpointPolicy
	URLPolicy        domain.SourceURLPolicy
	MaxResponseBytes int64
}

// SourceCatalog 只解析已登记的 Source/Endpoint，不接受调用方提供任意 URL。
type SourceCatalog interface {
	LookupEndpoint(context.Context, string, string) (SourceEndpointRegistration, error)
}

// SourceTransport 必须按 source_id + endpoint_id 选择固定策略的传输器。
// 生产实现应为每个 endpoint 注入独立 BoundDoer；不得回退到裸 http.Client。
type SourceTransport interface {
	Do(context.Context, string, string, *http.Request) (*http.Response, error)
}

// CollectSourceRequest 描述一次受控 Feed 采集。URL 和授权策略均从 SourceCatalog 解析。
type CollectSourceRequest struct {
	SourceID       string
	EndpointID     string
	Purpose        domain.Purpose
	ContentKind    domain.ContentKind
	StorageMode    domain.StorageMode
	Retention      time.Duration
	MaxEntries     int
	QueryDimension string
}

// CollectedSourceEntry 保留 Feed 候选与分层内容，不执行身份写入或持久化。
type CollectedSourceEntry struct {
	Candidate domain.Candidate
	Content   domain.ContentLayers
}

// CollectSourceResult 是单个 Source Endpoint 的在线 Feed 采集结果。
type CollectSourceResult struct {
	SourceID   string
	EndpointID string
	FeedFormat parser.FeedFormat
	FeedTitle  string
	ObservedAt time.Time
	Entries    []CollectedSourceEntry
}

// SourceCollectionError 是可安全暴露的新闻业务错误，不包含 URL、响应体或底层错误文本。
type SourceCollectionError struct {
	Code      string
	Retryable bool
	Skipped   bool
}

func (e *SourceCollectionError) Error() string {
	if e == nil || e.Code == "" {
		return "news: source collection failed"
	}
	return "news: source " + e.Code
}

// CollectSource 仅采集已登记且授权通过的 RSS/Atom Endpoint。
// 本用例只在当前请求内处理内容，不写入 DB/Redis，不下载附件或 Atom 外部正文。
type CollectSource struct {
	catalog   SourceCatalog
	transport SourceTransport
	now       func() time.Time
}

func NewCollectSource(catalog SourceCatalog, transport SourceTransport) *CollectSource {
	return &CollectSource{catalog: catalog, transport: transport, now: time.Now}
}

func (u *CollectSource) Execute(ctx context.Context, request CollectSourceRequest) (CollectSourceResult, error) {
	if u == nil || u.catalog == nil || u.transport == nil || ctx == nil {
		return CollectSourceResult{}, ErrSourceCollectionRequest
	}
	request.SourceID = strings.TrimSpace(request.SourceID)
	request.EndpointID = strings.TrimSpace(request.EndpointID)
	request.QueryDimension = strings.TrimSpace(request.QueryDimension)
	if request.SourceID == "" || request.EndpointID == "" ||
		request.Purpose == "" || request.ContentKind == "" ||
		request.StorageMode != domain.StorageTransient || request.Retention <= 0 ||
		request.MaxEntries < 0 || request.MaxEntries > maximumSourceEntryLimit {
		return CollectSourceResult{}, ErrSourceCollectionRequest
	}
	if request.MaxEntries == 0 {
		request.MaxEntries = defaultSourceEntryLimit
	}

	registration, err := u.catalog.LookupEndpoint(ctx, request.SourceID, request.EndpointID)
	if err != nil {
		if errors.Is(err, ErrSourceEndpointNotFound) {
			return CollectSourceResult{}, &SourceCollectionError{Code: "source_endpoint_not_found", Skipped: true}
		}
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_catalog_unavailable"}
	}
	if registration.SourceID != request.SourceID || registration.EndpointID != request.EndpointID ||
		!registration.Enabled || strings.TrimSpace(registration.URL) == "" {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_endpoint_unavailable", Skipped: true}
	}
	if registration.AccessMethod != domain.AccessRSS && registration.AccessMethod != domain.AccessLicensedFeed {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_access_method_unsupported", Skipped: true}
	}
	urlDecision := domain.EvaluateSourceURL(registration.URL, registration.URLPolicy)
	if !urlDecision.Allowed {
		return CollectSourceResult{}, &SourceCollectionError{Code: urlDecision.ReasonCode, Skipped: true}
	}
	policyRequest := domain.CollectionPolicyRequest{
		AccessMethod: registration.AccessMethod,
		Purpose:      request.Purpose,
		ContentKind:  request.ContentKind,
		StorageMode:  request.StorageMode,
		Retention:    request.Retention,
	}
	decision := domain.EvaluateSourceCollectionPolicy(registration.SourcePolicy, registration.EndpointPolicy, policyRequest, u.now())
	if !decision.Allowed {
		return CollectSourceResult{}, &SourceCollectionError{Code: decision.ReasonCode, Skipped: true}
	}

	limit := registration.MaxResponseBytes
	if limit == 0 {
		limit = defaultSourceResponseLimit
	}
	if limit < 1 || limit > maximumSourceResponseLimit {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_response_limit_invalid"}
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, urlDecision.CanonicalURL, nil)
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_endpoint_url_invalid"}
	}
	httpRequest.Header.Set("Accept", "application/rss+xml, application/atom+xml")
	response, err := u.transport.Do(ctx, request.SourceID, request.EndpointID, httpRequest)
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_fetch_failed", Retryable: sourceTransportRetryable(err)}
	}
	if response == nil || response.Body == nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_response_invalid"}
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return CollectSourceResult{}, &SourceCollectionError{
			Code:      "source_http_status_" + strconv.Itoa(response.StatusCode),
			Retryable: response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500,
		}
	}
	if response.ContentLength > limit {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_response_too_large"}
	}
	payload, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_response_read_failed", Retryable: sourceTransportRetryable(err)}
	}
	if int64(len(payload)) > limit {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_response_too_large"}
	}
	declaredFormat, err := parser.DetectFeedFormat(response.Header.Get("Content-Type"))
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_content_type_unsupported"}
	}

	observedAt := u.now().UTC()
	feed, err := parser.ParseFeed(payload, observedAt)
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_feed_parse_failed"}
	}
	if feed.Format != declaredFormat {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_feed_format_mismatch"}
	}

	result := CollectSourceResult{
		SourceID: request.SourceID, EndpointID: request.EndpointID,
		FeedFormat: feed.Format, FeedTitle: feed.Title, ObservedAt: observedAt,
		Entries: make([]CollectedSourceEntry, 0, min(len(feed.Entries), request.MaxEntries)),
	}
	for _, entry := range feed.Entries {
		if len(result.Entries) >= request.MaxEntries {
			break
		}
		candidate := domain.NormalizeCandidate(entry.ToCandidate(registration.PublisherName, registration.PublisherDomain), observedAt, request.QueryDimension)
		if candidate.RawURL != "" {
			itemURL := domain.EvaluateSourceURL(candidate.RawURL, registration.URLPolicy)
			if itemURL.Allowed {
				candidate.CanonicalURL = itemURL.CanonicalURL
			}
		}
		result.Entries = append(result.Entries, CollectedSourceEntry{
			Candidate: candidate,
			Content:   entry.ContentLayers(""),
		})
	}
	return result, nil
}

// SourceCollectionErrorCode returns the stable business code without exposing wrapped transport errors.
func SourceCollectionErrorCode(err error) string {
	var collectionErr *SourceCollectionError
	if errors.As(err, &collectionErr) {
		return collectionErr.Code
	}
	if errors.Is(err, ErrSourceCollectionRequest) {
		return "invalid_request"
	}
	if err == nil {
		return ""
	}
	return "internal_error"
}

// sourceTransportRetryable only marks explicit timeout failures as retry candidates.
// Unknown transport errors remain non-retryable until a domain policy classifies them.
func sourceTransportRetryable(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}
