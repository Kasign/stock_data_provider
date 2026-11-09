package bootstrap

import (
	"errors"
	"net/http"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

// newHTTPHandler registers every currently supported endpoint through the
// Platform policy-enforcing Router. No legacy catch-all route is retained.
func newHTTPHandler(authenticator *platformsecurity.Authenticator, health, legacy http.Handler) (http.Handler, error) {
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
		{pattern: "GET /{$}", policy: platformsecurity.OwnerAccess(), handler: legacy},
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
	return router.Handler(), nil
}
