package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	marketdataauthorization "example.com/stock-analysis-flow/internal/marketdata/authorization"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

type dataPolicyHTTPAuthenticatorFake struct {
	principal platformsecurity.Principal
	err       error
	calls     int
	method    string
}

func (fake *dataPolicyHTTPAuthenticatorFake) Authenticate(request *http.Request) (*http.Request, *http.Cookie, error) {
	fake.calls++
	fake.method = request.Method
	if fake.err != nil {
		return nil, nil, fake.err
	}
	ctx, err := platformsecurity.WithPrincipal(request.Context(), fake.principal)
	if err != nil {
		return nil, nil, err
	}
	return request.WithContext(ctx), nil, nil
}

type dataPolicyHTTPManagerFake struct {
	current           marketdataauthorization.CurrentPolicy
	currentErr        error
	publishErr        error
	publishedExpected string
	publishedVersion  string
	publishedPolicy   marketdataauthorization.DataAccessPolicy
	publishCalls      int
}

func (fake *dataPolicyHTTPManagerFake) Current(context.Context) (marketdataauthorization.CurrentPolicy, error) {
	return fake.current, fake.currentErr
}

func (fake *dataPolicyHTTPManagerFake) Publish(
	_ context.Context, expected, version string, policy marketdataauthorization.DataAccessPolicy,
) (marketdataauthorization.CurrentPolicy, error) {
	fake.publishCalls++
	fake.publishedExpected = expected
	fake.publishedVersion = version
	fake.publishedPolicy = policy
	if fake.publishErr != nil {
		return marketdataauthorization.CurrentPolicy{}, fake.publishErr
	}
	fake.current.Version.PolicyVersion = version
	fake.current.Policy = policy
	return fake.current, nil
}

// dataPolicyHTTPRefManagerFake 在旧接口之上实现按数据类读写，
// 用于验证交易状态事实策略不会复用日线策略。
type dataPolicyHTTPRefManagerFake struct {
	*dataPolicyHTTPManagerFake
	currentRefs      []string
	publishedRefs    []string
	refCurrent       marketdataauthorization.CurrentPolicy
	refCurrentErr    error
	refPublishErr    error
	refPublishPolicy marketdataauthorization.DataAccessPolicy
	refPublishCalled int
}

func (fake *dataPolicyHTTPRefManagerFake) CurrentFor(
	_ context.Context, policyRef string,
) (marketdataauthorization.CurrentPolicy, error) {
	fake.currentRefs = append(fake.currentRefs, policyRef)
	return fake.refCurrent, fake.refCurrentErr
}

func (fake *dataPolicyHTTPRefManagerFake) PublishFor(
	_ context.Context, policyRef, expected, version string, policy marketdataauthorization.DataAccessPolicy,
) (marketdataauthorization.CurrentPolicy, error) {
	fake.refPublishCalled++
	fake.publishedRefs = append(fake.publishedRefs, policyRef)
	fake.publishedExpected = expected
	fake.publishedVersion = version
	fake.refPublishPolicy = policy
	if fake.refPublishErr != nil {
		return marketdataauthorization.CurrentPolicy{}, fake.refPublishErr
	}
	fake.refCurrent.Version.PolicyVersion = version
	fake.refCurrent.Policy = policy
	return fake.refCurrent, nil
}

func newDataPolicyHTTPTestHandler(t *testing.T, auth *dataPolicyHTTPAuthenticatorFake, manager any) http.Handler {
	t.Helper()
	// 显式断言接口实现：新增 fake 若漏了方法，编译期就该失败。
	resolved, ok := manager.(dataAuthorizationPolicyManager)
	if !ok {
		t.Fatal("test manager must implement dataAuthorizationPolicyManager")
	}
	handler, err := newDataPolicyHTTPHandlerWithDependencies(auth, "owner_session", resolved)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func dataPolicyHTTPTestPrincipal(method platformsecurity.AuthMethod) platformsecurity.Principal {
	principal := platformsecurity.Principal{Kind: platformsecurity.IdentityOwner, AuthMethod: method, Subject: "owner"}
	if method == platformsecurity.AuthMethodOwnerBearer {
		principal.Scopes = []string{platformsecurity.ScopePlatformAdmin}
	}
	return principal
}

func dataPolicyHTTPTestCurrent() marketdataauthorization.CurrentPolicy {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	policy := marketdataauthorization.DataAccessPolicy{
		SchemaVersion:   marketdataauthorization.DataAccessPolicySchemaVersion,
		Status:          marketdataauthorization.PolicyApproved,
		AllowedSubjects: []string{"principal:service:quant"},
		AllowedPurposes: []string{"quant_analysis"},
		RequiredScopes:  []string{"data:read"},
		EffectiveAt:     now.Add(-time.Minute),
	}
	return marketdataauthorization.CurrentPolicy{
		Version: marketdataauthorization.PolicyVersion{
			PolicyType:    marketdataauthorization.DataAccessPolicyType,
			PolicyRef:     marketdataauthorization.DailyBarsV2PolicyRef,
			PolicyVersion: "policy-v1",
			PayloadHash:   strings.Repeat("a", 64),
			CreatedAt:     now,
		},
		Policy: policy,
	}
}

func TestDataPolicyHTTPGetCurrentPolicy(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerSession)}
	manager := &dataPolicyHTTPManagerFake{current: dataPolicyHTTPTestCurrent()}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/owner/data/daily-bars-access-policy", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" ||
		response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security headers missing: %+v", response.Header())
	}
	var body dataPolicyResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.PolicyVersion != "policy-v1" || body.Policy.Status != marketdataauthorization.PolicyApproved {
		t.Fatalf("unexpected policy response: %+v", body)
	}
	if auth.calls != 1 || auth.method != http.MethodGet {
		t.Fatalf("Owner Session authenticator calls=%d method=%q", auth.calls, auth.method)
	}
}

func TestDataPolicyHTTPGetNotPublishedReturns404(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerSession)}
	manager := &dataPolicyHTTPManagerFake{currentErr: marketdataapplication.ErrDataPolicyNotPublished}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/owner/data/daily-bars-access-policy", nil))
	if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "server.data_policy_not_published") {
		t.Fatalf("GET not-published response = %d %s", response.Code, response.Body.String())
	}
}

func TestDataPolicyHTTPPostCreatesImmutableVersion(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerSession)}
	manager := &dataPolicyHTTPManagerFake{current: dataPolicyHTTPTestCurrent()}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)
	body := `{
		"schema_version":"1.0.0",
		"expected_current_version":null,
		"policy_version":"policy-v2",
		"policy":{
			"schema_version":"1.0.0",
			"status":"approved",
			"allowed_subjects":["principal:service:quant"],
			"allowed_purposes":["quant_analysis"],
			"required_scopes":["data:read"],
			"effective_at":"2026-11-23T11:59:00Z",
			"expires_at":null
		}
	}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/owner/data/daily-bars-access-policy/versions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body=%s", response.Code, response.Body.String())
	}
	if manager.publishCalls != 1 || manager.publishedExpected != "" || manager.publishedVersion != "policy-v2" {
		t.Fatalf("unexpected Publish call: %+v", manager)
	}
	if auth.calls != 1 || auth.method != http.MethodPost {
		t.Fatalf("Owner Session authenticator calls=%d method=%q", auth.calls, auth.method)
	}
}

func TestDataPolicyHTTPPostRequiresExpectedVersionAndMapsConflict(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerSession)}
	manager := &dataPolicyHTTPManagerFake{publishErr: marketdataapplication.ErrDataPolicyVersionConflict}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)
	missing := httptest.NewRequest(http.MethodPost, "/api/v1/owner/data/daily-bars-access-policy/versions",
		strings.NewReader(`{"schema_version":"1.0.0","policy_version":"policy-v1","policy":{}}`))
	missing.Header.Set("Content-Type", "application/json")
	missingResponse := httptest.NewRecorder()
	handler.ServeHTTP(missingResponse, missing)
	if missingResponse.Code != http.StatusBadRequest || manager.publishCalls != 0 {
		t.Fatalf("missing expected_current_version response = %d, calls=%d", missingResponse.Code, manager.publishCalls)
	}

	body := `{"schema_version":"1.0.0","expected_current_version":"policy-v1","policy_version":"policy-v2","policy":{"schema_version":"1.0.0","status":"approved","allowed_subjects":["principal:service:quant"],"allowed_purposes":["quant_analysis"],"required_scopes":["data:read"],"effective_at":"2026-11-23T11:59:00Z","expires_at":null}}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/owner/data/daily-bars-access-policy/versions", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict || !strings.Contains(response.Body.String(), "server.data_policy_version_conflict") {
		t.Fatalf("CAS conflict response = %d %s", response.Code, response.Body.String())
	}
	if manager.publishedExpected != "policy-v1" {
		t.Fatalf("expected current version = %q", manager.publishedExpected)
	}
}

func TestDataPolicyHTTPRejectsBearerAndMixedCredentials(t *testing.T) {
	manager := &dataPolicyHTTPManagerFake{current: dataPolicyHTTPTestCurrent()}
	for _, test := range []struct {
		name          string
		principal     platformsecurity.Principal
		authorization string
		cookie        string
		want          int
	}{
		{name: "Owner Bearer", principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerBearer), want: http.StatusForbidden},
		{name: "Authorization without Session", authorization: "Bearer token", want: http.StatusUnauthorized},
		{name: "mixed Session and Authorization", authorization: "Bearer token", cookie: "owner_session=session", want: http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			auth := &dataPolicyHTTPAuthenticatorFake{principal: test.principal}
			handler := newDataPolicyHTTPTestHandler(t, auth, manager)
			request := httptest.NewRequest(http.MethodGet, "/api/v1/owner/data/daily-bars-access-policy", nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			if test.cookie != "" {
				request.Header.Set("Cookie", test.cookie)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.want {
				t.Fatalf("status=%d want=%d body=%s", response.Code, test.want, response.Body.String())
			}
			if test.authorization != "" && auth.calls != 0 {
				t.Fatalf("ambiguous/bearer request reached Session authenticator %d times", auth.calls)
			}
		})
	}
}

func TestDataPolicyHTTPAuthenticationFailureStopsApplicationCall(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{err: errors.New("session invalid")}
	manager := &dataPolicyHTTPManagerFake{current: dataPolicyHTTPTestCurrent()}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/owner/data/daily-bars-access-policy", nil))
	if response.Code != http.StatusInternalServerError || manager.publishCalls != 0 {
		t.Fatalf("authentication failure response=%d manager publish calls=%d", response.Code, manager.publishCalls)
	}
}

// 交易状态事实是独立数据类：manager 若只实现日线接口，必须报不可用，
// 绝不能回落成“把日线策略当作事实策略”返回。
func TestDataPolicyHTTPSecurityStatusFactsPolicyRejectsDayOnlyManager(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerSession)}
	manager := &dataPolicyHTTPManagerFake{current: dataPolicyHTTPTestCurrent()}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/api/v1/owner/data/security-status-facts-access-policy", nil,
	))
	if response.Code != http.StatusServiceUnavailable ||
		!strings.Contains(response.Body.String(), "server.data_policy_unavailable") {
		t.Fatalf("facts GET with day-only manager = %d %s", response.Code, response.Body.String())
	}

	publishBody := `{"schema_version":"1.0.0","expected_current_version":null,"policy_version":"policy-v1","policy":{"schema_version":"1.0.0","status":"approved","allowed_subjects":["principal:service:quant"],"allowed_purposes":["risk_review"],"required_scopes":["data:read"],"effective_at":"2026-11-23T11:59:00Z","expires_at":null}}`
	publishRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/owner/data/security-status-facts-access-policy/versions", strings.NewReader(publishBody),
	)
	publishRequest.Header.Set("Content-Type", "application/json")
	publishResponse := httptest.NewRecorder()
	handler.ServeHTTP(publishResponse, publishRequest)
	if publishResponse.Code != http.StatusServiceUnavailable ||
		!strings.Contains(publishResponse.Body.String(), "server.data_policy_unavailable") {
		t.Fatalf("facts POST with day-only manager = %d %s", publishResponse.Code, publishResponse.Body.String())
	}
	// 未支持的 manager 不得被调动日线方法，否则等于跨数据类读写。
	if manager.publishCalls != 0 {
		t.Fatalf("day-only Publish called %d times for the facts data class", manager.publishCalls)
	}
}

// manager 支持按数据类读写后，两条路由必须各自绑定自己的 policy_ref。
func TestDataPolicyHTTPSecurityStatusFactsPolicyUsesOwnPolicyRef(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerSession)}
	manager := &dataPolicyHTTPRefManagerFake{
		dataPolicyHTTPManagerFake: &dataPolicyHTTPManagerFake{current: dataPolicyHTTPTestCurrent()},
		refCurrent:                dataPolicyHTTPTestCurrent(),
	}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)

	factsResponse := httptest.NewRecorder()
	handler.ServeHTTP(factsResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/owner/data/security-status-facts-access-policy", nil,
	))
	if factsResponse.Code != http.StatusOK {
		t.Fatalf("facts GET status = %d, body=%s", factsResponse.Code, factsResponse.Body.String())
	}
	if len(manager.currentRefs) != 1 ||
		manager.currentRefs[0] != marketdataauthorization.SecurityStatusFactsPolicyRef {
		t.Fatalf("facts GET current refs = %v", manager.currentRefs)
	}

	dayResponse := httptest.NewRecorder()
	handler.ServeHTTP(dayResponse, httptest.NewRequest(
		http.MethodGet, "/api/v1/owner/data/daily-bars-access-policy", nil,
	))
	if dayResponse.Code != http.StatusOK {
		t.Fatalf("daily-bars GET status = %d, body=%s", dayResponse.Code, dayResponse.Body.String())
	}
	if len(manager.currentRefs) != 1 {
		t.Fatalf("daily-bars GET must not use the ref-aware reader, refs = %v", manager.currentRefs)
	}

	publishBody := `{"schema_version":"1.0.0","expected_current_version":"policy-v1","policy_version":"policy-v2","policy":{"schema_version":"1.0.0","status":"approved","allowed_subjects":["principal:service:quant"],"allowed_purposes":["risk_review"],"required_scopes":["data:read"],"effective_at":"2026-11-23T11:59:00Z","expires_at":null}}`
	publishRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/owner/data/security-status-facts-access-policy/versions", strings.NewReader(publishBody),
	)
	publishRequest.Header.Set("Content-Type", "application/json")
	publishResponse := httptest.NewRecorder()
	handler.ServeHTTP(publishResponse, publishRequest)
	if publishResponse.Code != http.StatusCreated {
		t.Fatalf("facts POST status = %d, body=%s", publishResponse.Code, publishResponse.Body.String())
	}
	if manager.refPublishCalled != 1 || manager.publishCalls != 0 {
		t.Fatalf("ref publishes=%d day publishes=%d", manager.refPublishCalled, manager.publishCalls)
	}
	if len(manager.publishedRefs) != 1 ||
		manager.publishedRefs[0] != marketdataauthorization.SecurityStatusFactsPolicyRef {
		t.Fatalf("facts POST published refs = %v", manager.publishedRefs)
	}
	if manager.publishedExpected != "policy-v1" || manager.publishedVersion != "policy-v2" {
		t.Fatalf("facts POST expected=%q version=%q", manager.publishedExpected, manager.publishedVersion)
	}
	var body dataPolicyResponse
	if err := json.Unmarshal(publishResponse.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.PolicyVersion != "policy-v2" {
		t.Fatalf("facts POST response policy version = %q", body.PolicyVersion)
	}
}

func TestDataPolicyHTTPOrderBookFactsPolicyUsesOwnPolicyRef(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerSession)}
	manager := &dataPolicyHTTPRefManagerFake{
		dataPolicyHTTPManagerFake: &dataPolicyHTTPManagerFake{current: dataPolicyHTTPTestCurrent()},
		refCurrent:                dataPolicyHTTPTestCurrent(),
	}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(
		http.MethodGet, "/api/v1/owner/data/order-book-facts-access-policy", nil,
	))
	if response.Code != http.StatusOK {
		t.Fatalf("order-book GET status = %d, body=%s", response.Code, response.Body.String())
	}
	if len(manager.currentRefs) != 1 ||
		manager.currentRefs[0] != marketdataauthorization.OrderBookFactsPolicyRef {
		t.Fatalf("order-book GET current refs = %v", manager.currentRefs)
	}
}

// 按数据类读取时的不可用/冲突错误必须与日线共用同一套投影。
func TestDataPolicyHTTPSecurityStatusFactsPolicyProjectionErrors(t *testing.T) {
	auth := &dataPolicyHTTPAuthenticatorFake{principal: dataPolicyHTTPTestPrincipal(platformsecurity.AuthMethodOwnerSession)}
	manager := &dataPolicyHTTPRefManagerFake{
		dataPolicyHTTPManagerFake: &dataPolicyHTTPManagerFake{},
		refCurrentErr:             marketdataapplication.ErrDataPolicyNotPublished,
		refPublishErr:             marketdataapplication.ErrDataPolicyVersionConflict,
	}
	handler := newDataPolicyHTTPTestHandler(t, auth, manager)

	notPublished := httptest.NewRecorder()
	handler.ServeHTTP(notPublished, httptest.NewRequest(
		http.MethodGet, "/api/v1/owner/data/security-status-facts-access-policy", nil,
	))
	if notPublished.Code != http.StatusNotFound ||
		!strings.Contains(notPublished.Body.String(), "server.data_policy_not_published") {
		t.Fatalf("facts GET not-published = %d %s", notPublished.Code, notPublished.Body.String())
	}

	publishBody := `{"schema_version":"1.0.0","expected_current_version":"policy-v1","policy_version":"policy-v2","policy":{"schema_version":"1.0.0","status":"approved","allowed_subjects":["principal:service:quant"],"allowed_purposes":["risk_review"],"required_scopes":["data:read"],"effective_at":"2026-11-23T11:59:00Z","expires_at":null}}`
	publishRequest := httptest.NewRequest(
		http.MethodPost, "/api/v1/owner/data/security-status-facts-access-policy/versions", strings.NewReader(publishBody),
	)
	publishRequest.Header.Set("Content-Type", "application/json")
	conflict := httptest.NewRecorder()
	handler.ServeHTTP(conflict, publishRequest)
	if conflict.Code != http.StatusConflict ||
		!strings.Contains(conflict.Body.String(), "server.data_policy_version_conflict") {
		t.Fatalf("facts POST conflict = %d %s", conflict.Code, conflict.Body.String())
	}
}
