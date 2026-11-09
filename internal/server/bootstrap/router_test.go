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
	handler, err := newHTTPHandler(auth, health, legacy)
	if err != nil {
		t.Fatalf("newHTTPHandler() error = %v", err)
	}
	tests := []struct {
		name, method, path, authorization string
		want                              int
	}{
		{name: "匿名存活检查", method: http.MethodGet, path: "/healthz", want: http.StatusOK},
		{name: "匿名 HEAD 存活检查", method: http.MethodHead, path: "/healthz", want: http.StatusOK},
		{name: "Readiness 未认证", method: http.MethodGet, path: "/readyz", want: http.StatusUnauthorized},
		{name: "业务路由未认证", method: http.MethodPost, path: "/api/v1/analysis/runs", want: http.StatusUnauthorized},
		{name: "错误 Owner 凭证", method: http.MethodPost, path: "/api/v1/analysis/runs", authorization: "Bearer " + strings.Repeat("c", 43), want: http.StatusUnauthorized},
		{name: "Service Scope 不得访问 Owner 路由", method: http.MethodPost, path: "/api/v1/analysis/runs", authorization: "Bearer " + serviceToken, want: http.StatusForbidden},
		{name: "Service Scope 不得访问 Owner Readiness", method: http.MethodGet, path: "/readyz", authorization: "Bearer " + serviceToken, want: http.StatusForbidden},
		{name: "Owner 创建 Run", method: http.MethodPost, path: "/api/v1/analysis/runs", authorization: "Bearer " + token, want: http.StatusNoContent},
		{name: "Owner 查询事件", method: http.MethodGet, path: "/api/v1/analysis/runs/run-1/events", authorization: "Bearer " + token, want: http.StatusNoContent},
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
