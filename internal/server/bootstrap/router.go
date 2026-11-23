package bootstrap

import (
	"errors"
	"net/http"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

// ownerSessionRequestAuthenticator is the narrow Server-side boundary used by
// Data routes. The concrete implementation is Platform-backed SessionAuthenticator.
type ownerSessionRequestAuthenticator interface {
	Authenticate(*http.Request) (*http.Request, *http.Cookie, error)
}

// newHTTPHandler registers every currently supported endpoint through explicit
// route policies. Data routes use a dedicated Server dispatcher only when Owner
// Session is enabled; otherwise they remain Owner Bearer-only.
func newHTTPHandler(authenticator *platformsecurity.Authenticator, health, legacy, marketData http.Handler, ownerSession *ownerSessionComposition) (http.Handler, error) {
	if authenticator == nil || health == nil || legacy == nil || marketData == nil {
		return nil, errors.New("HTTP authenticator, health, legacy and Market Data handlers are required")
	}
	router := platformsecurity.NewRouter()
	dataPatterns := []string{
		"/api/v1/quotes/",
		"/api/v1/bars/",
		"/api/v1/bundles/",
		"/api/v1/data-sources",
		"/api/v1/data-sources/",
		"/api/v1/market/",
		"/api/v1/stocks/",
	}
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
		{pattern: "POST /api/v1/analysis/runs", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "GET /api/v1/analysis/runs/{run_id}", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "GET /api/v1/analysis/runs/{run_id}/events", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "POST /api/v1/analysis/tasks", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "GET /api/v1/analysis/tasks/{task_id}", policy: platformsecurity.OwnerAccess(), handler: legacy},
		{pattern: "GET /api/v1/analysis/tasks/{task_id}/events", policy: platformsecurity.OwnerAccess(), handler: legacy},
	}
	// The browser-visible readiness alias stays under /api so the HttpOnly
	// Owner Session cookie remains scoped to the API path.
	if ownerSession == nil {
		routes = append(routes, struct {
			pattern string
			policy  platformsecurity.RoutePolicy
			handler http.Handler
		}{pattern: "GET /api/readyz", policy: platformsecurity.OwnerAccess(), handler: rewriteURLPath(health, "/readyz")})
	}
	for _, route := range routes {
		if err := router.Handle(route.pattern, authenticator, route.policy, route.handler); err != nil {
			return nil, err
		}
	}
	legacyHandler := router.Handler()
	mux := http.NewServeMux()
	if ownerSession == nil {
		// No Owner Session means no browser identity is available; retain the
		// explicit Owner Bearer policy for every Data route.
		for _, pattern := range dataPatterns {
			if err := router.Handle(pattern, authenticator, platformsecurity.OwnerAccess(), marketData); err != nil {
				return nil, err
			}
		}
		legacyHandler = router.Handler()
	} else {
		ownerHandler, err := newOwnerSessionHTTPHandler(ownerSession, ownerSessionCookie(ownerSession))
		if err != nil {
			return nil, err
		}
		dataHandler := newOwnerSessionOrBearerDataHandler(
			authenticator, ownerSession.authenticator, ownerSession.cookie.Name, marketData,
		)
		for _, pattern := range dataPatterns {
			mux.Handle(pattern, dataHandler)
		}
		readinessHandler := newOwnerSessionOrBearerDataHandler(
			authenticator, ownerSession.authenticator, ownerSession.cookie.Name,
			rewriteURLPath(health, "/readyz"),
		)
		mux.Handle("/api/readyz", readinessHandler)
		if ownerSession.policyManager != nil {
			policyHandler, policyErr := newQuantPolicyHTTPHandler(ownerSession)
			if policyErr != nil {
				return nil, policyErr
			}
			mux.Handle("/api/v1/owner/quant/result-access-policy", policyHandler)
			mux.Handle("/api/v1/owner/quant/result-access-policy/", policyHandler)
		}
		mux.Handle("/api/v1/owner/", ownerHandler)
		if ownerSession.researchflow != nil {
			researchflowHandler, handlerErr := newResearchflowHTTPHandler(ownerSession)
			if handlerErr != nil {
				return nil, handlerErr
			}
			mux.Handle("/api/v1/researchflow/runs", researchflowHandler)
			mux.Handle("/api/v1/researchflow/runs/", researchflowHandler)
		}
	}
	mux.Handle("/", legacyHandler)
	return mux, nil
}

// newOwnerSessionOrBearerDataHandler implements the explicit SRV-CTR-002 Data
// route policy. Bearer calls are checked by the Platform OwnerAccess guard;
// browser calls are independently authenticated as owner_session. The Session
// authenticator performs Host/Origin and atomic CSRF checks for unsafe methods.
func newOwnerSessionOrBearerDataHandler(
	bearerAuthenticator *platformsecurity.Authenticator,
	sessionAuthenticator ownerSessionRequestAuthenticator,
	sessionCookieName string,
	next http.Handler,
) http.Handler {
	bearerHandler := platformsecurity.Guard(bearerAuthenticator, platformsecurity.OwnerAccess(), next)
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if len(request.Header.Values("Authorization")) > 0 {
			if hasCookie(request, sessionCookieName) {
				writePublicError(w, request, http.StatusBadRequest, publicError{
					Class: "validation", Code: "server.ambiguous_authentication",
					Message: "provide either Owner Bearer or Owner Session, not both",
				})
				return
			}
			bearerHandler.ServeHTTP(w, request)
			return
		}
		if sessionAuthenticator == nil {
			writePublicError(w, request, http.StatusUnauthorized, publicError{
				Class: "unauthenticated", Code: "server.session_unauthenticated",
				Message: "authentication required",
			})
			return
		}
		authenticated, cookie, err := sessionAuthenticator.Authenticate(request)
		if err != nil {
			if !writePlatformFault(w, request, err) {
				writePublicError(w, request, http.StatusInternalServerError, publicError{
					Class: "internal", Code: "server.session_authentication_failed",
					Message: "authentication is unavailable",
				})
			}
			return
		}
		if authenticated == nil {
			writePublicError(w, request, http.StatusInternalServerError, publicError{
				Class: "internal", Code: "server.session_authentication_failed",
				Message: "authentication is unavailable",
			})
			return
		}
		if cookie != nil {
			http.SetCookie(w, cookie)
		}
		next.ServeHTTP(w, authenticated)
	})
}

func hasCookie(request *http.Request, name string) bool {
	if request == nil || name == "" {
		return false
	}
	for _, cookie := range request.Cookies() {
		if cookie.Name == name {
			return true
		}
	}
	return false
}

func ownerSessionCookie(composition *ownerSessionComposition) platformsecurity.SessionCookieConfig {
	return composition.cookie
}

// rewriteURLPath adapts the browser-visible /api/readyz alias to the existing
// Root readiness handler without duplicating readiness logic or diagnostics.
func rewriteURLPath(next http.Handler, path string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if next == nil {
			http.NotFound(w, request)
			return
		}
		clone := request.Clone(request.Context())
		urlCopy := *request.URL
		urlCopy.Path = path
		urlCopy.RawPath = ""
		clone.URL = &urlCopy
		next.ServeHTTP(w, clone)
	})
}
