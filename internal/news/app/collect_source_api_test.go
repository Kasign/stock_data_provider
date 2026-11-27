package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type apiTransportStub struct {
	response    *http.Response
	calls       int
	method      string
	accept      string
	contentType string
	body        string
	host        string
}

func (s *apiTransportStub) Do(_ context.Context, sourceID, endpointID string, request *http.Request) (*http.Response, error) {
	s.calls++
	s.method = request.Method
	s.accept = request.Header.Get("Accept")
	s.contentType = request.Header.Get("Content-Type")
	s.host = request.URL.Host
	if request.Body != nil {
		body, _ := io.ReadAll(request.Body)
		s.body = string(body)
	}
	return s.response, nil
}

func officialAPIRegistration() SourceEndpointRegistration {
	registration := approvedSourceRegistration()
	registration.EndpointID = "announcements"
	registration.URL = "https://www.cninfo.com.cn/new/hisAnnouncement/query"
	registration.AccessMethod = domain.AccessOfficialAPI
	registration.SourcePolicy.AllowedAccessMethods = []domain.AccessMethod{domain.AccessOfficialAPI}
	registration.URLPolicy = domain.SourceURLPolicy{
		AllowedSchemes: []string{"https"},
		AllowedHosts:   []string{"www.cninfo.com.cn"},
	}
	registration.RequestParams = map[string]string{
		"stock":  "600036,gssh0600036",
		"seDate": "2026-10-01~2026-11-27",
	}
	return registration
}

func apiJSONResponse(body string) *http.Response {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"application/json; charset=utf-8"}},
		Body:          io.NopCloser(strings.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

const apiJSONBody = `{"announcements":[
	{"announcementId":"1225649440","announcementTitle":"公告A","announcementTime":1793376000000,"adjunctUrl":"https://static.cninfo.com.cn/finalpage/2026-10-31/a.PDF"},
	{"announcementId":"1225649239","announcementTitle":"公告B","announcementTime":1793376000000,"adjunctUrl":"https://static.cninfo.com.cn/finalpage/2026-10-31/b.PDF"}
]}`

func apiCollectRequest() CollectSourceRequest {
	return CollectSourceRequest{
		SourceID: "source-1", EndpointID: "announcements",
		Purpose: domain.PurposeInternalPersonalResearch, ContentKind: domain.ContentMetadata,
		StorageMode: domain.StorageTransient, Retention: time.Second, MaxEntries: 10,
	}
}

func TestCollectOfficialAPIIssuesFixedPOSTWithParams(t *testing.T) {
	registration := officialAPIRegistration()
	transport := &apiTransportStub{response: apiJSONResponse(apiJSONBody)}
	resolver := &sourceCollectionPolicyResolverStub{
		source: registration.SourcePolicy, endpoint: registration.EndpointPolicy,
	}
	usecase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)
	usecase.policyResolver = resolver

	result, err := usecase.Execute(context.Background(), apiCollectRequest())
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 {
		t.Fatalf("transport calls=%d want 1", transport.calls)
	}
	if transport.method != http.MethodPost {
		t.Fatalf("method=%q want POST", transport.method)
	}
	if transport.host != "www.cninfo.com.cn" {
		t.Fatalf("host=%q", transport.host)
	}
	if transport.accept != "application/json" {
		t.Fatalf("accept=%q want application/json", transport.accept)
	}
	if transport.contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("content_type=%q", transport.contentType)
	}
	// 请求体必须来自启动时锁定的固定模板，排序后稳定编码。
	want := "seDate=2026-10-01~2026-11-27&stock=600036%2Cgssh0600036"
	if transport.body != want {
		t.Fatalf("body=%q want %q", transport.body, want)
	}
	if len(result.Entries) != 2 {
		t.Fatalf("entries=%d want 2", len(result.Entries))
	}
}

func TestCollectOfficialAPIPreservesDisclosureRawValue(t *testing.T) {
	registration := officialAPIRegistration()
	transport := &apiTransportStub{response: apiJSONResponse(apiJSONBody)}
	usecase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)
	usecase.policyResolver = &sourceCollectionPolicyResolverStub{source: registration.SourcePolicy}

	result, err := usecase.Execute(context.Background(), apiCollectRequest())
	if err != nil {
		t.Fatal(err)
	}
	first := result.Entries[0]
	if first.Candidate.PublishedAtRaw != "1793376000000" {
		t.Fatalf("raw=%q want millis", first.Candidate.PublishedAtRaw)
	}
	if first.Candidate.PublishedAt != nil {
		t.Fatalf("published_at must stay nil, got %v", *first.Candidate.PublishedAt)
	}
	if first.Candidate.SourceItemID != "1225649440" {
		t.Fatalf("source_item_id=%q", first.Candidate.SourceItemID)
	}
}

func TestCollectOfficialAPICapturesResponseProvenance(t *testing.T) {
	registration := officialAPIRegistration()
	registration.SourceID = "cninfo"
	registration.EndpointID = "cninfo-his-announcement"
	transport := &apiTransportStub{response: apiJSONResponse(apiJSONBody)}
	usecase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)
	usecase.policyResolver = &sourceCollectionPolicyResolverStub{source: registration.SourcePolicy}

	request := apiCollectRequest()
	request.SourceID = "cninfo"
	request.EndpointID = "cninfo-his-announcement"
	result, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(apiJSONBody))
	wantHash := hex.EncodeToString(digest[:])
	if result.ResponseHash != wantHash {
		t.Fatalf("response_hash=%q want %q", result.ResponseHash, wantHash)
	}
	wantRef := "response-snapshot:cninfo/cninfo-his-announcement@" + wantHash
	if result.EvidenceRef != wantRef {
		t.Fatalf("evidence_ref=%q want %q", result.EvidenceRef, wantRef)
	}
	if !result.hasValidCollectionSeal() {
		t.Fatal("response provenance must be covered by collection seal")
	}
}

func TestCollectOfficialAPIRejectsRequestWithoutFixedParams(t *testing.T) {
	registration := officialAPIRegistration()
	registration.RequestParams = nil
	transport := &apiTransportStub{response: apiJSONResponse(apiJSONBody)}
	usecase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)
	usecase.policyResolver = &sourceCollectionPolicyResolverStub{source: registration.SourcePolicy}

	_, err := usecase.Execute(context.Background(), apiCollectRequest())
	if err == nil {
		t.Fatal("official_api without fixed request params must fail closed")
	}
	var collectionErr *SourceCollectionError
	if !asCollectionError(err, &collectionErr) || collectionErr.Code != "source_request_params_missing" {
		t.Fatalf("err=%v", err)
	}
	if transport.calls != 0 {
		t.Fatalf("must not hit network, calls=%d", transport.calls)
	}
}

func TestCollectOfficialAPIRejectsNonJSONContentType(t *testing.T) {
	registration := officialAPIRegistration()
	response := apiJSONResponse(apiJSONBody)
	response.Header.Set("Content-Type", "text/html")
	transport := &apiTransportStub{response: response}
	usecase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)
	usecase.policyResolver = &sourceCollectionPolicyResolverStub{source: registration.SourcePolicy}

	_, err := usecase.Execute(context.Background(), apiCollectRequest())
	if err == nil {
		t.Fatal("non-JSON content type must fail closed")
	}
}

func TestCollectOfficialAPIIgnoresRuntimeRequestParams(t *testing.T) {
	// 运行时请求携带的任何参数都不能覆盖启动时锁定的模板。
	registration := officialAPIRegistration()
	transport := &apiTransportStub{response: apiJSONResponse(apiJSONBody)}
	usecase := NewCollectSource(sourceCatalogStub{registration: registration}, transport)
	usecase.policyResolver = &sourceCollectionPolicyResolverStub{source: registration.SourcePolicy}

	request := apiCollectRequest()
	request.QueryDimension = "injected=1"
	_, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(transport.body, "injected") {
		t.Fatalf("runtime param leaked into body: %q", transport.body)
	}
}

func asCollectionError(err error, target **SourceCollectionError) bool {
	if err == nil {
		return false
	}
	if collectionErr, ok := err.(*SourceCollectionError); ok {
		*target = collectionErr
		return true
	}
	return false
}

func TestOfficialAPIParamsEncodeStable(t *testing.T) {
	params := map[string]string{"stock": "600036,gssh0600036", "seDate": "2026-10-01~2026-11-27"}
	got := encodeRequestParams(params)
	want := "seDate=2026-10-01~2026-11-27&stock=600036%2Cgssh0600036"
	if got != want {
		t.Fatalf("encoded=%q want %q", got, want)
	}
}
