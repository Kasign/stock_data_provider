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

func newDataPolicyHTTPTestHandler(t *testing.T, auth *dataPolicyHTTPAuthenticatorFake, manager *dataPolicyHTTPManagerFake) http.Handler {
	t.Helper()
	handler, err := newDataPolicyHTTPHandlerWithDependencies(auth, "owner_session", manager)
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
