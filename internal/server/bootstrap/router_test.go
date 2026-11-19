package bootstrap

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

func TestHTTPHandlerEnforcesExplicitRoutePolicies(t *testing.T) {
	token := strings.Repeat("a", 43)
	serviceToken := strings.Repeat("b", 43)
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "PLATFORM_ADMIN_TOKEN":
			return token, true
		case "RESEARCHFLOW_SERVICE_TOKEN":
			return serviceToken, true
		default:
			return "", false
		}
	})
	auth, err := platformsecurity.NewAuthenticator(env.Secret("PLATFORM_ADMIN_TOKEN"), []platformsecurity.ServiceCredential{
		{Name: "researchflow", Secret: env.Secret("RESEARCHFLOW_SERVICE_TOKEN"), Scopes: []string{platformsecurity.ScopeResearchRun}},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	legacy := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) })
	health := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	data := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusAccepted) })
	handler, err := newHTTPHandler(auth, health, legacy, data, nil)
	if err != nil {
		t.Fatalf("newHTTPHandler() error = %v", err)
	}
	tests := []struct {
		name, method, path, authorization string
		want                              int
	}{
		{name: "匿名存活检查", method: http.MethodGet, path: "/healthz", want: http.StatusOK},
		{name: "匿名 HEAD 存活检查", method: http.MethodHead, path: "/healthz", want: http.StatusOK},
		{name: "匿名 HEAD Readiness", method: http.MethodHead, path: "/readyz", want: http.StatusUnauthorized},
		{name: "匿名 HEAD Diagnostics", method: http.MethodHead, path: "/diagnostics", want: http.StatusUnauthorized},
		{name: "Owner HEAD Readiness", method: http.MethodHead, path: "/readyz", authorization: "Bearer " + token, want: http.StatusOK},
		{name: "Owner HEAD Diagnostics", method: http.MethodHead, path: "/diagnostics", authorization: "Bearer " + token, want: http.StatusOK},
		{name: "Readiness 未认证", method: http.MethodGet, path: "/readyz", want: http.StatusUnauthorized},
		{name: "业务路由未认证", method: http.MethodPost, path: "/api/v1/analysis/runs", want: http.StatusUnauthorized},
		{name: "Data 路由未认证", method: http.MethodGet, path: "/api/v1/quotes/600519.SH", want: http.StatusUnauthorized},
		{name: "Data Source 根路由未认证", method: http.MethodGet, path: "/api/v1/data-sources", want: http.StatusUnauthorized},
		{name: "错误 Owner 凭证", method: http.MethodPost, path: "/api/v1/analysis/runs", authorization: "Bearer " + strings.Repeat("c", 43), want: http.StatusUnauthorized},
		{name: "Service Scope 不得访问 Owner 路由", method: http.MethodPost, path: "/api/v1/analysis/runs", authorization: "Bearer " + serviceToken, want: http.StatusForbidden},
		{name: "Service Scope 不得访问 Owner Readiness", method: http.MethodGet, path: "/readyz", authorization: "Bearer " + serviceToken, want: http.StatusForbidden},
		{name: "Diagnostics 未认证", method: http.MethodGet, path: "/diagnostics", want: http.StatusUnauthorized},
		{name: "Service Scope 不得访问 Diagnostics", method: http.MethodGet, path: "/diagnostics", authorization: "Bearer " + serviceToken, want: http.StatusForbidden},
		{name: "Owner 读取 Diagnostics", method: http.MethodGet, path: "/diagnostics", authorization: "Bearer " + token, want: http.StatusOK},
		{name: "Owner 创建 Run", method: http.MethodPost, path: "/api/v1/analysis/runs", authorization: "Bearer " + token, want: http.StatusNoContent},
		{name: "Owner 访问 Data Handler", method: http.MethodGet, path: "/api/v1/quotes/600519.SH", authorization: "Bearer " + token, want: http.StatusAccepted},
		{name: "Owner 访问 Data Source 根路由", method: http.MethodGet, path: "/api/v1/data-sources", authorization: "Bearer " + token, want: http.StatusAccepted},
		{name: "Owner 查询事件", method: http.MethodGet, path: "/api/v1/analysis/runs/run-1/events", authorization: "Bearer " + token, want: http.StatusNoContent},
		{name: "Owner Session 未启用时不注册登录路由", method: http.MethodPost, path: "/api/v1/owner/session", want: http.StatusNotFound},
		{name: "未注册路由", method: http.MethodGet, path: "/internal/debug", authorization: "Bearer " + token, want: http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(tt.method, tt.path, nil)
			if tt.authorization != "" {
				request.Header.Set("Authorization", tt.authorization)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.want, response.Body.String())
			}
		})
	}
}

type fakeSessionRequestAuthenticator struct {
	cookie *http.Cookie
	err    error
}

func (f fakeSessionRequestAuthenticator) Authenticate(request *http.Request) (*http.Request, *http.Cookie, error) {
	if f.err != nil {
		return nil, nil, f.err
	}
	ctx, err := platformsecurity.WithPrincipal(request.Context(), platformsecurity.Principal{
		Kind: platformsecurity.IdentityOwner, AuthMethod: platformsecurity.AuthMethodOwnerSession, Subject: "owner",
	})
	if err != nil {
		return nil, nil, err
	}
	return request.WithContext(ctx), f.cookie, nil
}

func TestOwnerSessionOrBearerDataHandlerUsesExplicitIdentityAndRejectsAmbiguousCredentials(t *testing.T) {
	ownerToken := strings.Repeat("a", 43)
	serviceToken := strings.Repeat("b", 43)
	env := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case "PLATFORM_ADMIN_TOKEN":
			return ownerToken, true
		case "RESEARCHFLOW_SERVICE_TOKEN":
			return serviceToken, true
		default:
			return "", false
		}
	})
	auth, err := platformsecurity.NewAuthenticator(env.Secret("PLATFORM_ADMIN_TOKEN"), []platformsecurity.ServiceCredential{
		{Name: "researchflow", Secret: env.Secret("RESEARCHFLOW_SERVICE_TOKEN"), Scopes: []string{platformsecurity.ScopeResearchRun}},
	})
	if err != nil {
		t.Fatalf("NewAuthenticator() error = %v", err)
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := platformsecurity.PrincipalFromContext(r)
		if !ok {
			t.Error("PrincipalFromContext() missing")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch principal.AuthMethod {
		case platformsecurity.AuthMethodOwnerBearer:
			w.WriteHeader(http.StatusNoContent)
		case platformsecurity.AuthMethodOwnerSession:
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Errorf("unexpected AuthMethod %q", principal.AuthMethod)
			w.WriteHeader(http.StatusForbidden)
		}
	})
	sessionCookie := &http.Cookie{Name: "__Secure-owner_session", Value: strings.Repeat("s", 43), HttpOnly: true, Path: "/api"}
	handler := newOwnerSessionOrBearerDataHandler(auth, fakeSessionRequestAuthenticator{cookie: sessionCookie}, sessionCookie.Name, next)

	tests := []struct {
		name          string
		authorization string
		cookie        string
		want          int
		wantSetCookie bool
	}{
		{name: "Owner Bearer 运维调用", authorization: "Bearer " + ownerToken, want: http.StatusNoContent},
		{name: "Owner Session 浏览器调用", cookie: sessionCookie.Name + "=" + sessionCookie.Value, want: http.StatusAccepted, wantSetCookie: true},
		{name: "Service Bearer 不得访问 Data Owner 路由", authorization: "Bearer " + serviceToken, want: http.StatusForbidden},
		{name: "错误 Bearer 凭证", authorization: "Bearer " + strings.Repeat("c", 43), want: http.StatusUnauthorized},
		{name: "双凭证身份歧义必须拒绝", authorization: "Bearer " + ownerToken, cookie: sessionCookie.Name + "=" + sessionCookie.Value, want: http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/quotes/600519.SH", nil)
			if tt.authorization != "" {
				request.Header.Set("Authorization", tt.authorization)
			}
			if tt.cookie != "" {
				request.Header.Set("Cookie", tt.cookie)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != tt.want {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.want, response.Body.String())
			}
			if got := response.Header().Get("Set-Cookie") != ""; got != tt.wantSetCookie {
				t.Fatalf("Set-Cookie present = %v, want %v", got, tt.wantSetCookie)
			}
		})
	}
}

func TestReadinessAliasReusesRootReadinessHandler(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/readyz" {
			t.Fatalf("rewritten path = %q, want /readyz", request.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	})
	handler := rewriteURLPath(next, "/readyz")
	request := httptest.NewRequest(http.MethodGet, "/api/readyz", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if request.URL.Path != "/api/readyz" {
		t.Fatalf("rewrite must not mutate the caller request, got %q", request.URL.Path)
	}
}
