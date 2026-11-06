package security

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/platform/config"
)

const ownerToken = "owner-token-0123456789-0123456789-abcdef"
const dataToken = "data-token-0123456789-0123456789-abcdef"

func newTestAuthenticator(t *testing.T) *Authenticator {
	t.Helper()
	auth, err := NewAuthenticator(
		config.NewEnvWithLookup(func(key string) (string, bool) {
			if key == "OWNER" {
				return ownerToken, true
			}
			return "", false
		}).Secret("OWNER"),
		[]ServiceCredential{{
			Name: "data-service",
			Secret: config.NewEnvWithLookup(func(key string) (string, bool) {
				if key == "SERVICE" {
					return dataToken, true
				}
				return "", false
			}).Secret("SERVICE"),
			Scopes: []string{ScopeDataRead, ScopeDataDiagnose},
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	return auth
}

func requestWithToken(token string) *http.Request {
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

func TestAuthenticateOwnerAndService(t *testing.T) {
	auth := newTestAuthenticator(t)
	owner, err := auth.Authenticate(requestWithToken(ownerToken))
	if err != nil || owner.Kind != IdentityOwner || owner.Subject != "owner" {
		t.Fatalf("owner = %+v, err=%v", owner, err)
	}
	service, err := auth.Authenticate(requestWithToken(dataToken))
	if err != nil || service.Kind != IdentityService || service.Subject != "data-service" {
		t.Fatalf("service = %+v, err=%v", service, err)
	}
	if !contains(service.Scopes, ScopeDataRead) {
		t.Fatalf("service scopes = %v", service.Scopes)
	}
}

func TestAuthenticateRejectsMissingInvalidAndMalformedCredentials(t *testing.T) {
	auth := newTestAuthenticator(t)
	anonymous, err := auth.Authenticate(requestWithToken(""))
	if err != nil || anonymous.Kind != IdentityAnonymous {
		t.Fatalf("missing credentials should resolve to anonymous: %+v, %v", anonymous, err)
	}
	if _, err := auth.Authenticate(requestWithToken("incorrect-token-0123456789-0123456789")); err == nil {
		t.Fatal("expected invalid credentials to fail")
	}
	malformed := requestWithToken("")
	malformed.Header.Set("Authorization", "Basic "+ownerToken)
	if _, err := auth.Authenticate(malformed); err == nil {
		t.Fatal("expected malformed authorization to fail")
	}
	duplicate := requestWithToken(ownerToken)
	duplicate.Header.Add("Authorization", "Bearer "+dataToken)
	if _, err := auth.Authenticate(duplicate); err == nil {
		t.Fatal("expected duplicate authorization headers to fail")
	}
}

func TestGuardEnforcesIdentityAndScope(t *testing.T) {
	auth := newTestAuthenticator(t)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		principal, ok := PrincipalFromContext(r)
		if !ok || principal.Kind != IdentityService {
			t.Errorf("principal missing from request context: %+v", principal)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	handler := Guard(auth, ServiceAccess(ScopeDataRead), next)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithToken(""))
	if response.Code != http.StatusUnauthorized || called {
		t.Fatalf("missing credentials must return 401: status=%d called=%v", response.Code, called)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithToken(dataToken))
	if response.Code != http.StatusNoContent || !called {
		t.Fatalf("service access status=%d called=%v", response.Code, called)
	}

	called = false
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithToken(ownerToken))
	if response.Code != http.StatusForbidden || called {
		t.Fatalf("owner must not impersonate service: status=%d called=%v", response.Code, called)
	}
}

func TestGuardAnonymousOwnerAndDefaultDeny(t *testing.T) {
	auth := newTestAuthenticator(t)
	handler := Guard(auth, AnonymousAccess(), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithToken(""))
	if response.Code != http.StatusNoContent {
		t.Fatalf("anonymous status=%d", response.Code)
	}

	response = httptest.NewRecorder()
	handler.ServeHTTP(response, requestWithToken(ownerToken))
	if response.Code != http.StatusForbidden {
		t.Fatalf("owner should not use anonymous-only route: %d", response.Code)
	}

	denied := Guard(auth, RoutePolicy{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unconfigured route policy must not invoke handler")
	}))
	response = httptest.NewRecorder()
	denied.ServeHTTP(response, requestWithToken(ownerToken))
	if response.Code != http.StatusForbidden {
		t.Fatalf("default deny status=%d", response.Code)
	}
}

func TestGuardOwnerOnlyAndOwnerOrService(t *testing.T) {
	auth := newTestAuthenticator(t)
	ownerOnly := Guard(auth, OwnerAccess(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	response := httptest.NewRecorder()
	ownerOnly.ServeHTTP(response, requestWithToken(ownerToken))
	if response.Code != http.StatusNoContent {
		t.Fatalf("owner status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	ownerOnly.ServeHTTP(response, requestWithToken(dataToken))
	if response.Code != http.StatusForbidden {
		t.Fatalf("service status=%d", response.Code)
	}

	both := Guard(auth, OwnerOrServiceAccess(ScopeDataRead), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	for _, token := range []string{ownerToken, dataToken} {
		response = httptest.NewRecorder()
		both.ServeHTTP(response, requestWithToken(token))
		if response.Code != http.StatusNoContent {
			t.Fatalf("allowed identity status=%d", response.Code)
		}
	}
}

func TestNewAuthenticatorRejectsTooManyServiceCredentials(t *testing.T) {
	makeSecret := func(value string) config.Secret {
		return config.NewEnvWithLookup(func(string) (string, bool) { return value, true }).Secret("TOKEN")
	}
	services := make([]ServiceCredential, maximumServiceCredentials+1)
	for index := range services {
		services[index] = ServiceCredential{
			Name: "service-" + strconv.Itoa(index),
			Secret: makeSecret(dataToken),
			Scopes: []string{ScopeDataRead},
		}
	}
	if _, err := NewAuthenticator(makeSecret(ownerToken), services); err == nil {
		t.Fatal("expected service credential count limit to be enforced")
	}
}

func TestNewAuthenticatorRejectsWeakDuplicateAndUnknownScopes(t *testing.T) {
	makeSecret := func(value string) config.Secret {
		return config.NewEnvWithLookup(func(string) (string, bool) { return value, true }).Secret("TOKEN")
	}
	for _, services := range [][]ServiceCredential{
		{{Name: "weak", Secret: makeSecret("short"), Scopes: []string{ScopeDataRead}}},
		{{Name: "invalid-format", Secret: makeSecret("token.with.invalid.punctuation-0123456789"), Scopes: []string{ScopeDataRead}}},
		{{Name: "too-long", Secret: makeSecret(strings.Repeat("a", maximumTokenBytes+1)), Scopes: []string{ScopeDataRead}}},
		{{Name: "duplicate", Secret: makeSecret(ownerToken), Scopes: []string{ScopeDataRead}}},
		{{Name: "unknown-scope", Secret: makeSecret(dataToken), Scopes: []string{"admin:*"}}},
	} {
		if _, err := NewAuthenticator(makeSecret(ownerToken), services); err == nil {
			t.Fatal("expected invalid credentials configuration")
		}
	}
}

func TestAuthenticateRejectsInvalidTokenSyntaxAndOversizedToken(t *testing.T) {
	auth := newTestAuthenticator(t)
	for _, token := range []string{
		"invalid.token-0123456789-0123456789",
		"invalid+token-0123456789-0123456789",
		"令牌令牌令牌令牌令牌令牌令牌令牌令牌令牌令牌令牌",
		strings.Repeat("a", maximumTokenBytes+1),
	} {
		if _, err := auth.Authenticate(requestWithToken(token)); err == nil {
			t.Errorf("expected token syntax rejection for %q", token)
		}
	}
}

func TestClientSuppliedIdentityHeadersAreIgnored(t *testing.T) {
	auth := newTestAuthenticator(t)
	handler := Guard(auth, ServiceAccess(ScopeDataRead), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, _ := PrincipalFromContext(r)
		if principal.Subject != "data-service" {
			t.Errorf("subject=%q", principal.Subject)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := requestWithToken(dataToken)
	request.Header.Set("X-Identity", "owner")
	request.Header.Set("X-Principal", "owner")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestAuthorizationFailureDoesNotLeakToken(t *testing.T) {
	auth := newTestAuthenticator(t)
	request := requestWithToken("wrong-secret-token-0123456789-0123456789")
	response := httptest.NewRecorder()
	Guard(auth, OwnerAccess(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})).ServeHTTP(response, request)
	if strings.Contains(response.Body.String(), "wrong-secret") || strings.Contains(response.Body.String(), ownerToken) {
		t.Fatalf("credential leaked: %s", response.Body.String())
	}
}

func TestPrincipalContextIsRequestScoped(t *testing.T) {
	auth := newTestAuthenticator(t)
	handler := Guard(auth, ServiceAccess(ScopeDataRead), http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if _, ok := ctx.Value(principalContextKey{}).(Principal); !ok {
			t.Fatal("principal missing")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	request := requestWithToken(dataToken).WithContext(context.Background())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d", response.Code)
	}
}

func TestRouterRequiresExplicitPolicyAndDefaultsUnknownRoutesToNotFound(t *testing.T) {
	auth := newTestAuthenticator(t)
	router := NewRouter()
	if err := router.Handle("GET /public", auth, RoutePolicy{}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("zero policy must not register")
	})); err == nil {
		t.Fatal("expected zero policy registration to fail")
	}
	if err := router.HandleFunc("GET /healthz", auth, AnonymousAccess(), func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	router.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("public route status=%d", response.Code)
	}
	response = httptest.NewRecorder()
	router.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unregistered", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unregistered route status=%d", response.Code)
	}
}

func TestRouterRejectsRootCatchAllPatterns(t *testing.T) {
	auth := newTestAuthenticator(t)
	for _, pattern := range []string{"/", "GET /", "example.com/", "GET example.com/", "/{path...}", "GET /{path...}", "example.com/{path...}", "GET example.com/{path...}"} {
		router := NewRouter()
		if err := router.Handle(pattern, auth, AnonymousAccess(), http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})); err == nil {
			t.Errorf("pattern %q must not register a root catch-all route", pattern)
		}
	}

	// An exact root route remains valid and does not capture other paths.
	router := NewRouter()
	if err := router.Handle("GET /{$}", auth, AnonymousAccess(), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})); err != nil {
		t.Fatalf("exact root route should be allowed: %v", err)
	}
	response := httptest.NewRecorder()
	router.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/other", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unregistered path status=%d, want 404", response.Code)
	}
}

func TestRouterRejectsDuplicatePatterns(t *testing.T) {
	auth := newTestAuthenticator(t)
	router := NewRouter()
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	if err := router.Handle("GET /same", auth, AnonymousAccess(), handler); err != nil {
		t.Fatal(err)
	}
	if err := router.Handle("GET /same", auth, OwnerAccess(), handler); err == nil {
		t.Fatal("expected duplicate route registration to fail")
	}
}

func TestAuthenticatorCredentialSnapshotIsImmutableToCallers(t *testing.T) {
	makeSecret := func(value string) config.Secret {
		return config.NewEnvWithLookup(func(string) (string, bool) { return value, true }).Secret("TOKEN")
	}

	serviceScopes := []string{ScopeDataRead}
	auth, err := NewAuthenticator(makeSecret(ownerToken), []ServiceCredential{{
		Name:   "snapshot-service",
		Secret: makeSecret(dataToken),
		Scopes: serviceScopes,
	}})
	if err != nil {
		t.Fatal(err)
	}

	// Mutating the caller-owned configuration after construction must not
	// change the authenticator's credential or authorization snapshot.
	serviceScopes[0] = ScopeResearchRun
	serviceScopes = append(serviceScopes, ScopeResearchRead)

	principal, err := auth.Authenticate(requestWithToken(dataToken))
	if err != nil {
		t.Fatal(err)
	}
	if principal.Subject != "snapshot-service" || !contains(principal.Scopes, ScopeDataRead) || contains(principal.Scopes, ScopeResearchRun) {
		t.Fatalf("authenticator changed with caller-owned scopes: %+v", principal)
	}

	// A caller may mutate its returned Principal without changing later
	// authentication results.
	principal.Scopes[0] = ScopeResearchRun
	next, err := auth.Authenticate(requestWithToken(dataToken))
	if err != nil {
		t.Fatal(err)
	}
	if !contains(next.Scopes, ScopeDataRead) || contains(next.Scopes, ScopeResearchRun) {
		t.Fatalf("returned principal mutated authenticator state: %+v", next)
	}
}
