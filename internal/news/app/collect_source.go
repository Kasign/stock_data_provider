package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
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
	SourceID                string
	EndpointID              string
	Enabled                 bool
	URL                     string
	PublisherName           string
	PublisherDomain         string
	AccessMethod            domain.AccessMethod
	SourcePolicy            domain.SourcePolicy
	SourcePolicyVersionID   string
	EndpointPolicy          *domain.EndpointPolicy
	EndpointPolicyVersionID string
	// ProviderCode/ProviderOutputPolicyRef/ProviderOutputPolicyVersionID 是
	// official_api 直接来源自身的输出策略绑定。三者必须同时存在或同时为空；
	// 为空时沿用既有直接来源语义，不写入 Provider 授权痕迹。
	ProviderCode                  string
	ProviderOutputPolicyRef       string
	ProviderOutputPolicyVersionID string
	URLPolicy                     domain.SourceURLPolicy
	MaxResponseBytes              int64
	// RequestParams 是 official_api 启动时锁定的固定 POST 表单参数。
	// 运行时调用方不可注入或覆盖；为 nil/空时 official_api 采集 fail-closed。
	// RSS/LicensedFeed 路径不使用该字段。
	RequestParams map[string]string
}

// SourceCatalog 只解析已登记的 Source/Endpoint，不接受调用方提供任意 URL。
type SourceCatalog interface {
	LookupEndpoint(context.Context, string, string) (SourceEndpointRegistration, error)
}

// SourceTransport 必须按 source_id + endpoint_id 选择固定策略的传输器。
// 生产实现应为每个 endpoint 注入独立 DestinationPolicy 适配器；不得回退到裸 http.Client。
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

// collectionSeal 绑定完整采集结果，防止调用方修改已采集的策略版本或候选载荷。
type CollectSourceResult struct {
	collectionSeal          [sha256.Size]byte
	CollectionPurpose       domain.Purpose
	SourceID                string
	EndpointID              string
	SourcePolicyVersionID   string
	EndpointPolicyVersionID string
	PolicyEvaluatedAt       time.Time
	FeedFormat              parser.FeedFormat
	FeedTitle               string
	ObservedAt              time.Time
	FetchedAt               time.Time
	NormalizedAt            time.Time
	// ResponseHash 是本次成功响应原始字节的 SHA-256，小写十六进制。
	// EvidenceRef 是不含 URL/凭据的内部响应快照定位符；G3 只用于内存资格判定，
	// 不声称已持久化原始响应，也不生成 available_at。
	ResponseHash string
	EvidenceRef  string
	Entries      []CollectedSourceEntry
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
	catalog        SourceCatalog
	transport      SourceTransport
	policyResolver CurrentSourceCollectionPolicyResolver
	now            func() time.Time
}

func NewCollectSource(catalog SourceCatalog, transport SourceTransport) *CollectSource {
	resolver, _ := catalog.(CurrentSourceCollectionPolicyResolver)
	return &CollectSource{catalog: catalog, transport: transport, policyResolver: resolver, now: time.Now}
}

// NewCollectSourceWithPolicyResolver 创建生产采集用例；没有当前策略解析器时不得用于生产装配。
func NewCollectSourceWithPolicyResolver(catalog SourceCatalog, transport SourceTransport, resolver CurrentSourceCollectionPolicyResolver) (*CollectSource, error) {
	if catalog == nil || transport == nil || resolver == nil {
		return nil, ErrSourceCollectionPolicyUnavailable
	}
	return &CollectSource{catalog: catalog, transport: transport, policyResolver: resolver, now: time.Now}, nil
}

// encodeRequestParams 将启动锁定的参数表编码为稳定的表单串：按键排序后逐项 url 编码。
// 相同参数集合永远产生相同字节，便于审计与幂等；调用方运行时输入不参与此函数。
func encodeRequestParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for key := range params {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	values := url.Values{}
	for _, key := range keys {
		values.Set(key, params[key])
	}
	return values.Encode()
}

func isOfficialAPIAccessMethod(method domain.AccessMethod) bool {
	return method == domain.AccessOfficialAPI
}

func (u *CollectSource) Execute(ctx context.Context, request CollectSourceRequest) (CollectSourceResult, error) {
	if u == nil || u.catalog == nil || u.transport == nil || ctx == nil {
		return CollectSourceResult{}, ErrSourceCollectionRequest
	}
	if err := ctx.Err(); err != nil {
		return CollectSourceResult{}, err
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
	if ctxErr := ctx.Err(); ctxErr != nil {
		return CollectSourceResult{}, ctxErr
	}
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
	if registration.AccessMethod != domain.AccessRSS &&
		registration.AccessMethod != domain.AccessLicensedFeed &&
		!isOfficialAPIAccessMethod(registration.AccessMethod) {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_access_method_unsupported", Skipped: true}
	}
	// official_api 必须携带启动时锁定的固定 POST 参数；缺参数即 fail-closed，且不得触网。
	officialAPI := isOfficialAPIAccessMethod(registration.AccessMethod)
	if officialAPI && len(registration.RequestParams) == 0 {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_request_params_missing"}
	}
	if strings.TrimSpace(registration.SourcePolicyVersionID) == "" ||
		(registration.EndpointPolicy != nil && strings.TrimSpace(registration.EndpointPolicyVersionID) == "") ||
		(registration.EndpointPolicy == nil && strings.TrimSpace(registration.EndpointPolicyVersionID) != "") {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_policy_version_reference_invalid"}
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
	policyEvaluatedAt := u.now().UTC()
	if u.policyResolver == nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_current_policy_unavailable"}
	}
	sourcePolicy := registration.SourcePolicy
	endpointPolicy := registration.EndpointPolicy
	{
		currentSource, currentEndpoint, resolveErr := u.policyResolver.ResolveCurrentSourceCollectionPolicies(
			ctx, registration.SourceID, registration.SourcePolicyVersionID,
			registration.EndpointID, registration.EndpointPolicyVersionID,
		)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return CollectSourceResult{}, ctxErr
		}
		if resolveErr != nil {
			return CollectSourceResult{}, &SourceCollectionError{Code: "source_current_policy_unavailable"}
		}
		sourcePolicy = currentSource
		endpointPolicy = currentEndpoint
	}
	decision := domain.EvaluateSourceCollectionPolicy(sourcePolicy, endpointPolicy, policyRequest, policyEvaluatedAt)
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
	httpMethod := http.MethodGet
	var requestBody io.Reader
	if officialAPI {
		httpMethod = http.MethodPost
		requestBody = strings.NewReader(encodeRequestParams(registration.RequestParams))
	}
	httpRequest, err := http.NewRequestWithContext(ctx, httpMethod, urlDecision.CanonicalURL, requestBody)
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_endpoint_url_invalid"}
	}
	if officialAPI {
		httpRequest.Header.Set("Accept", "application/json")
		httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		httpRequest.Header.Set("Accept", "application/rss+xml, application/atom+xml")
	}
	response, err := u.transport.Do(ctx, request.SourceID, request.EndpointID, httpRequest)
	if ctxErr := ctx.Err(); ctxErr != nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return CollectSourceResult{}, ctxErr
	}
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
	if ctxErr := ctx.Err(); ctxErr != nil {
		return CollectSourceResult{}, ctxErr
	}
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_response_read_failed", Retryable: sourceTransportRetryable(err)}
	}
	if int64(len(payload)) > limit {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_response_too_large"}
	}
	var declaredFormat parser.FeedFormat
	if officialAPI {
		declaredFormat, err = parser.DetectOfficialAPIFormat(response.Header.Get("Content-Type"))
	} else {
		declaredFormat, err = parser.DetectFeedFormat(response.Header.Get("Content-Type"))
	}
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_content_type_unsupported"}
	}

	responseDigest := sha256.Sum256(payload)
	responseHash := hex.EncodeToString(responseDigest[:])

	fetchedAt := u.now().UTC()
	var feed parser.FeedDocument
	if officialAPI {
		feed, err = parser.ParseAPIJSON(payload, fetchedAt)
	} else {
		feed, err = parser.ParseFeed(payload, fetchedAt)
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return CollectSourceResult{}, ctxErr
	}
	if err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_feed_parse_failed"}
	}
	if feed.Format != declaredFormat {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_feed_format_mismatch"}
	}

	result := CollectSourceResult{
		CollectionPurpose:       request.Purpose,
		SourceID:                request.SourceID,
		EndpointID:              request.EndpointID,
		SourcePolicyVersionID:   registration.SourcePolicyVersionID,
		EndpointPolicyVersionID: registration.EndpointPolicyVersionID,
		PolicyEvaluatedAt:       policyEvaluatedAt,
		FeedFormat:              feed.Format,
		FeedTitle:               feed.Title,
		FetchedAt:               fetchedAt,
		ResponseHash:            responseHash,
		EvidenceRef:             "response-snapshot:" + request.SourceID + "/" + request.EndpointID + "@" + responseHash,
		Entries:                 make([]CollectedSourceEntry, 0, min(len(feed.Entries), request.MaxEntries)),
	}
	for _, entry := range feed.Entries {
		if err := ctx.Err(); err != nil {
			return CollectSourceResult{}, err
		}
		if len(result.Entries) >= request.MaxEntries {
			break
		}
		candidate := domain.NormalizeCandidate(entry.ToCandidate(registration.PublisherName, registration.PublisherDomain), fetchedAt, request.QueryDimension)
		if candidate.RawURL != "" {
			itemURL := domain.EvaluateSourceURL(candidate.RawURL, registration.URLPolicy)
			if itemURL.Allowed {
				candidate.CanonicalURL = itemURL.CanonicalURL
			}
		}
		result.Entries = append(result.Entries, CollectedSourceEntry{
			Candidate: candidate,
			Content:   domain.RestrictContentLayers(entry.ContentLayers(""), request.ContentKind),
		})
	}
	normalizedAt := u.now().UTC()
	if err := ctx.Err(); err != nil {
		return CollectSourceResult{}, err
	}
	result.NormalizedAt = normalizedAt
	result.ObservedAt = normalizedAt
	for index := range result.Entries {
		fetched := fetchedAt
		normalized := normalizedAt
		result.Entries[index].Candidate.FetchedAt = &fetched
		result.Entries[index].Candidate.NormalizedAt = &normalized
		result.Entries[index].Candidate.ObservedAt = normalizedAt
	}
	if err := ctx.Err(); err != nil {
		return CollectSourceResult{}, err
	}
	if err := sealCollectSourceResult(&result); err != nil {
		return CollectSourceResult{}, &SourceCollectionError{Code: "source_result_seal_failed"}
	}
	if err := ctx.Err(); err != nil {
		return CollectSourceResult{}, err
	}
	return result, nil
}

// sealCollectSourceResult binds all exported result fields into an unexported digest.
// The digest is intentionally package-private so external callers cannot mint a valid result.
func sealCollectSourceResult(result *CollectSourceResult) error {
	if result == nil {
		return ErrSourceCollectionRequest
	}
	result.collectionSeal = [sha256.Size]byte{}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	result.collectionSeal = sha256.Sum256(payload)
	return nil
}

func (r CollectSourceResult) hasValidCollectionSeal() bool {
	expected := r.collectionSeal
	if expected == [sha256.Size]byte{} {
		return false
	}
	r.collectionSeal = [sha256.Size]byte{}
	payload, err := json.Marshal(r)
	return err == nil && sha256.Sum256(payload) == expected
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

// sourceTransportRetryable accepts an explicit adapter classification first, then
// recognizes timeout errors. Unknown transport errors remain non-retryable.
func sourceTransportRetryable(err error) bool {
	var classified interface{ Retryable() bool }
	if errors.As(err, &classified) {
		return classified.Retryable()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func (r CollectSourceResult) ObservationPolicyTrace() ObservationPolicyTrace {
	trace := ObservationPolicyTrace{
		SourcePolicyVersionID: r.SourcePolicyVersionID,
		PolicyEvaluatedAt:     r.PolicyEvaluatedAt,
	}
	// EndpointRef 代表经过单独评估的 Endpoint Policy，而不是仅仅存在一个采集入口。
	if r.EndpointPolicyVersionID != "" {
		trace.EndpointID = r.EndpointID
		trace.EndpointPolicyVersionID = r.EndpointPolicyVersionID
	}
	return trace
}
