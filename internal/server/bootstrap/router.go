package bootstrap

import (
	"errors"
	"net/http"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

// newHTTPHandler registers every currently supported endpoint through the
// Platform policy-enforcing Router. No legacy catch-all route is retained.
func newHTTPHandler(authenticator *platformsecurity.Authenticator, health, legacy, marketData http.Handler, ownerSession *ownerSessionComposition) (http.Handler, error) {
	if authenticator == nil || health == nil || legacy == nil {
		return nil, errors.New("HTTP authenticator, health handler and legacy handler are required")
	}
	router := platformsecurity.NewRouter()
	routes := []struct {
		pattern string
		policy  platformsecurity.RoutePolicy
		handler http.Handler
	}{
		{pattern: "GET /healthz", policy: platformsecurity.AnonymousAccess(), handler: health},
		{pattern: "HEAD /healthz", policy: platformsecurity.AnonymousAccess(), handler: health},
		{pattern: "GET /readyz", policy: platformsecurity.OwnerAccess(), handler: health},
		{pattern: "HEAD /readyz", policy: platformsecurity.OwnerAccess(), handler: health},
		{pattern: "GET /diagnostics", policy: platformsecurity.OwnerAccess(), handler: health},
		{pattern: "HEAD /diagnostics", policy: platformsecurity.OwnerAccess(), handler: health},
		{pattern: "GET /{$}", policy: platformsecurity.OwnerAccess(), handler: legacy},
		// Data 的 InProcessHandler 保留模块内完整路由；Root 统一执行 Owner 认证和入口中间件。
		{pattern: "/api/v1/", policy: platformsecurity.OwnerAccess(), handler: marketData},
		{pattern: "POST /api/v1/analysis/runs", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "GET /api/v1/analysis/runs/{run_id}", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "GET /api/v1/analysis/runs/{run_id}/events", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "POST /api/v1/analysis/tasks", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "GET /api/v1/analysis/tasks/{task_id}", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "GET /api/v1/analysis/tasks/{task_id}/events", policy: platformsecurity.OwnerAccess(), handler: legacy},
	}
	for _, route := range routes {
		if err := router.Handle(route.pattern, authenticator, route.policy, route.handler); err != nil {
			return nil, err
		}
	}
	legacyHandler := router.Handler()
	if ownerSession == nil {
		return legacyHandler, nil
	}
	ownerHandler, err := newOwnerSessionHTTPHandler(ownerSession, ownerSessionCookie(ownerSession))
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/api/v1/owner/", ownerHandler)
	mux.Handle("/", legacyHandler)
	return mux, nil
}

func ownerSessionCookie(composition *ownerSessionComposition) platformsecurity.SessionCookieConfig {
	return composition.cookie
}
