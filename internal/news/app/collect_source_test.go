package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type sourceCatalogStub struct {
	registration SourceEndpointRegistration
	err          error
}

func (s sourceCatalogStub) LookupEndpoint(context.Context, string, string) (SourceEndpointRegistration, error) {
	return s.registration, s.err
}

type sourceTransportStub struct {
	response   *http.Response
	err        error
	calls      int
	sourceID   string
	endpointID string
}

func (s *sourceTransportStub) Do(_ context.Context, sourceID, endpointID string, request *http.Request) (*http.Response, error) {
	s.calls++
	s.sourceID, s.endpointID = sourceID, endpointID
	if request.Method != http.MethodGet || request.URL.Host != "publisher.example" {
		return nil, errors.New("unexpected request")
	}
	return s.response, s.err
}

func approvedSourceRegistration() SourceEndpointRegistration {
	expires := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	return SourceEndpointRegistration{
		SourceID: "source-1", EndpointID: "rss-main", Enabled: true,
		URL: "https://publisher.example/feed.xml", PublisherName: "登记媒体",
		PublisherDomain: "publisher.example", AccessMethod: domain.AccessRSS,
		SourcePolicyVersionID: "source-policy-v3",
		SourcePolicy: domain.SourcePolicy{
			Status:               domain.AuthorizationApproved,
			AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
			AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:  []domain.ContentKind{domain.ContentMetadata, domain.ContentFullText},
			AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient},
			MaxRetention:         time.Minute, EffectiveAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			ExpiresAt: &expires, EvidenceRef: "license-source-1",
		},
		URLPolicy: domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"publisher.example"}},
	}
}

func feedResponse(body string, contentType string, status int) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": []string{contentType}},
		Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)),
	}
}

func validCollectRequest() CollectSourceRequest {
	return CollectSourceRequest{
		SourceID: "source-1", EndpointID: "rss-main",
		Purpose: domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentFullText,
		StorageMode: domain.StorageTransient, Retention: time.Second, MaxEntries: 10,
	}
}

func TestCollectSourceFeedSuccessUsesRegisteredEndpointAndPolicy(t *testing.T) {
	body := `<?xml version="1.0"?><rss version="2.0"><channel><title>媒体 Feed</title><item><guid>item-1</guid><title>新闻标题</title><link>https://publisher.example/story/1</link><description>来源摘要</description><pubDate>Wed, 04 Nov 2026 10:30:00 +0800</pubDate></item></channel></rss>`
	transport := &sourceTransportStub{response: feedResponse(body, "application/rss+xml; charset=utf-8", http.StatusOK)}
	useCase := NewCollectSource(sourceCatalogStub{registration: approvedSourceRegistration()}, transport)
	useCase.now = func() time.Time { return time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC) }

	result, err := useCase.Execute(context.Background(), validCollectRequest())
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 || transport.sourceID != "source-1" || transport.endpointID != "rss-main" {
		t.Fatalf("transport selection=%+v", transport)
	}
	if result.SourcePolicyVersionID != "source-policy-v3" || result.EndpointPolicyVersionID != "" ||
		!result.PolicyEvaluatedAt.Equal(time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("policy trace=%+v", result)
	}
	if result.FeedFormat != "rss" || result.FeedTitle != "媒体 Feed" || len(result.Entries) != 1 {
		t.Fatalf("result=%+v", result)
	}
	item := result.Entries[0]
	if item.Candidate.SourceItemID != "item-1" || item.Candidate.PublisherName != "登记媒体" ||
		item.Candidate.CanonicalURL != "https://publisher.example/story/1" ||
		item.Content.SourceSummary != "来源摘要" {
		t.Fatalf("collected entry=%+v", item)
	}
}

func TestCollectSourceDoesNotReturnContentAboveRequestedMetadata(t *testing.T) {
	body := `<?xml version="1.0"?><rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/"><channel><title>媒体 Feed</title><item><guid>item-1</guid><title>新闻标题</title><link>https://publisher.example/story/1</link><description>来源摘要</description><content:encoded><![CDATA[正文内容不应返回]]></content:encoded><enclosure url="https://publisher.example/file.pdf" type="application/pdf" length="10"/></item></channel></rss>`
	transport := &sourceTransportStub{response: feedResponse(body, "application/rss+xml", http.StatusOK)}
	useCase := NewCollectSource(sourceCatalogStub{registration: approvedSourceRegistration()}, transport)
	request := validCollectRequest()
	request.ContentKind = domain.ContentMetadata

	result, err := useCase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("entries=%+v", result.Entries)
	}
	content := result.Entries[0].Content
	if content.SourceSummary != "" || content.Body != nil ||
		content.BodyDeclaredTypeRaw != "" || content.ExternalBodyReference != "" ||
		len(content.Attachments) != 0 {
		t.Fatalf("metadata-only request returned higher-level payload: %+v", content)
	}
}

func TestCollectSourceRejectsUnauthorizedSourceBeforeNetwork(t *testing.T) {
	registration := approvedSourceRegistration()
	registration.SourcePolicy.Status = domain.AuthorizationPending
	transport := &sourceTransportStub{}
	useCase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)

	_, err := useCase.Execute(context.Background(), validCollectRequest())
	if SourceCollectionErrorCode(err) != "news.policy.source_pending" || transport.calls != 0 {
		t.Fatalf("error=%v calls=%d", err, transport.calls)
	}
}

func TestCollectSourceRejectsUnregisteredHostBeforeNetwork(t *testing.T) {
	registration := approvedSourceRegistration()
	registration.URL = "https://other.example/feed.xml"
	transport := &sourceTransportStub{}
	useCase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)

	_, err := useCase.Execute(context.Background(), validCollectRequest())
	if SourceCollectionErrorCode(err) != "news.source_url.host_not_allowed" || transport.calls != 0 {
		t.Fatalf("error=%v calls=%d", err, transport.calls)
	}
}

func TestCollectSourceRejectsOversizedBody(t *testing.T) {
	registration := approvedSourceRegistration()
	registration.MaxResponseBytes = 8
	transport := &sourceTransportStub{response: feedResponse("<rss>too large</rss>", "application/rss+xml", http.StatusOK)}
	useCase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)

	_, err := useCase.Execute(context.Background(), validCollectRequest())
	if SourceCollectionErrorCode(err) != "source_response_too_large" {
		t.Fatalf("error=%v", err)
	}
}

func TestCollectSourceRejectsContentTypeAndFormatMismatch(t *testing.T) {
	body := `<?xml version="1.0"?><rss version="2.0"><channel/></rss>`
	for _, testCase := range []struct {
		contentType string
		code        string
	}{
		{"text/xml", "source_content_type_unsupported"},
		{"application/atom+xml", "source_feed_format_mismatch"},
	} {
		transport := &sourceTransportStub{response: feedResponse(body, testCase.contentType, http.StatusOK)}
		useCase := NewCollectSource(sourceCatalogStub{registration: approvedSourceRegistration()}, transport)
		_, err := useCase.Execute(context.Background(), validCollectRequest())
		if SourceCollectionErrorCode(err) != testCase.code {
			t.Errorf("content type %q: error=%v", testCase.contentType, err)
		}
	}
}

func TestCollectSourceMapsHTTPStatusWithoutLeakingBody(t *testing.T) {
	transport := &sourceTransportStub{response: feedResponse("sensitive upstream body", "application/rss+xml", http.StatusTooManyRequests)}
	useCase := NewCollectSource(sourceCatalogStub{registration: approvedSourceRegistration()}, transport)

	_, err := useCase.Execute(context.Background(), validCollectRequest())
	var collectionErr *SourceCollectionError
	if !errors.As(err, &collectionErr) || collectionErr.Code != "source_http_status_429" || !collectionErr.Retryable {
		t.Fatalf("error=%v", err)
	}
	if strings.Contains(err.Error(), "sensitive") {
		t.Fatalf("error leaked response body: %v", err)
	}
}

func TestCollectSourceRequiresTransientRequestOnlyPolicy(t *testing.T) {
	transport := &sourceTransportStub{}
	useCase := NewCollectSource(sourceCatalogStub{registration: approvedSourceRegistration()}, transport)
	request := validCollectRequest()
	request.StorageMode = domain.StoragePersistentFullText

	_, err := useCase.Execute(context.Background(), request)
	if !errors.Is(err, ErrSourceCollectionRequest) || transport.calls != 0 {
		t.Fatalf("error=%v calls=%d", err, transport.calls)
	}
}

func TestCollectSourceSkipsCatalogFailure(t *testing.T) {
	transport := &sourceTransportStub{}
	useCase := NewCollectSource(sourceCatalogStub{err: ErrSourceEndpointNotFound}, transport)

	_, err := useCase.Execute(context.Background(), validCollectRequest())
	var collectionErr *SourceCollectionError
	if !errors.As(err, &collectionErr) || !collectionErr.Skipped || transport.calls != 0 {
		t.Fatalf("error=%v calls=%d", err, transport.calls)
	}
}

func TestCollectSourceUnknownTransportErrorIsNotRetryable(t *testing.T) {
	for _, testCase := range []struct {
		err       error
		retryable bool
	}{
		{errors.New("connection refused"), false},
		{context.DeadlineExceeded, true},
	} {
		transport := &sourceTransportStub{err: testCase.err}
		useCase := NewCollectSource(sourceCatalogStub{registration: approvedSourceRegistration()}, transport)
		_, err := useCase.Execute(context.Background(), validCollectRequest())
		var collectionErr *SourceCollectionError
		if !errors.As(err, &collectionErr) || collectionErr.Retryable != testCase.retryable {
			t.Errorf("transport error %v: result=%+v", testCase.err, collectionErr)
		}
	}
}

func TestCollectSourceRequiresImmutablePolicyVersionReferences(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*SourceEndpointRegistration)
	}{
		{
			name: "missing source policy version",
			mutate: func(registration *SourceEndpointRegistration) {
				registration.SourcePolicyVersionID = ""
			},
		},
		{
			name: "endpoint policy without version",
			mutate: func(registration *SourceEndpointRegistration) {
				registration.EndpointPolicy = &domain.EndpointPolicy{Status: domain.AuthorizationApproved}
			},
		},
		{
			name: "endpoint version without policy",
			mutate: func(registration *SourceEndpointRegistration) {
				registration.EndpointPolicyVersionID = "endpoint-policy-v2"
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			registration := approvedSourceRegistration()
			testCase.mutate(&registration)
			transport := &sourceTransportStub{}
			useCase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)

			_, err := useCase.Execute(context.Background(), validCollectRequest())
			if SourceCollectionErrorCode(err) != "source_policy_version_reference_invalid" || transport.calls != 0 {
				t.Fatalf("error=%v calls=%d", err, transport.calls)
			}
		})
	}
}

func TestCollectSourceReturnsEndpointPolicyVersionWhenApplied(t *testing.T) {
	registration := approvedSourceRegistration()
	registration.EndpointPolicy = &domain.EndpointPolicy{
		Status:      domain.AuthorizationApproved,
		EvidenceRef: "endpoint-license-1",
	}
	registration.EndpointPolicyVersionID = "endpoint-policy-v2"
	body := "<?xml version=\"1.0\"?><rss version=\"2.0\"><channel><title>媒体 Feed</title></channel></rss>"
	transport := &sourceTransportStub{
		response: feedResponse(body, "application/rss+xml", http.StatusOK),
	}
	useCase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)

	result, err := useCase.Execute(context.Background(), validCollectRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.SourcePolicyVersionID != "source-policy-v3" ||
		result.EndpointPolicyVersionID != "endpoint-policy-v2" {
		t.Fatalf("policy trace=%+v", result)
	}
}
