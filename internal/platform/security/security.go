// Package security authenticates service credentials and enforces explicit
// HTTP route access policies.
package security

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"example.com/stock-analysis-flow/internal/platform/config"
)

type IdentityKind string

const (
	IdentityAnonymous IdentityKind = "anonymous"
	IdentityOwner     IdentityKind = "owner"
	IdentityService   IdentityKind = "service"

	ScopePlatformAdmin = "platform:admin"
	ScopeDataRead      = "data:read"
	ScopeDataDiagnose  = "data:diagnose"
	ScopeResearchRun   = "research:run"
	ScopeResearchRead  = "research:read"
)

const minimumTokenBytes = 32

type Principal struct {
	Kind   IdentityKind
	Subject string
	Scopes []string
}

type ServiceCredential struct {
	Name   string
	Secret config.Secret
	Scopes []string
}

type credentialDigest struct {
	kind    IdentityKind
	subject string
	digest  [sha256.Size]byte
	scopes  []string
}

type Authenticator struct {
	credentials []credentialDigest
}

// Router registers only routes with an explicit access policy. Unregistered
// routes retain ServeMux's 404 behavior and cannot accidentally become public.
type Router struct {
	mux *http.ServeMux
}

// NewRouter creates a policy-enforcing router.
func NewRouter() *Router {
	return &Router{mux: http.NewServeMux()}
}

// Handle registers a route only when its access policy is explicit and valid.
func (r *Router) Handle(pattern string, authenticator *Authenticator, policy RoutePolicy, handler http.Handler) (err error) {
	if r == nil || r.mux == nil || authenticator == nil || handler == nil || !policy.valid() {
		return errors.New("route, authenticator, handler and explicit access policy are required")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("route pattern is invalid or already registered")
		}
	}()
	r.mux.Handle(pattern, Guard(authenticator, policy, handler))
	return nil
}

// HandleFunc registers a handler function with an explicit access policy.
func (r *Router) HandleFunc(pattern string, authenticator *Authenticator, policy RoutePolicy, handler http.HandlerFunc) error {
	if handler == nil {
		return errors.New("route handler is required")
	}
	return r.Handle(pattern, authenticator, policy, handler)
}

// Handler returns the router's HTTP handler.
func (r *Router) Handler() http.Handler {
	if r == nil || r.mux == nil {
		return http.NotFoundHandler()
	}
	return r.mux
}

func NewAuthenticator(ownerToken config.Secret, services []ServiceCredential) (*Authenticator, error) {
	if !ownerToken.Configured() {
		return nil, errors.New("owner credential is required")
	}
	if len(ownerToken.Value()) < minimumTokenBytes {
		return nil, errors.New("owner credential must contain at least 32 bytes")
	}

	auth := &Authenticator{}
	ownerDigest := sha256.Sum256([]byte(ownerToken.Value()))
	auth.credentials = append(auth.credentials, credentialDigest{
		kind: IdentityOwner, subject: "owner", digest: ownerDigest,
		scopes: []string{ScopePlatformAdmin},
	})

	seenNames := map[string]struct{}{}
	for _, service := range services {
		name := strings.TrimSpace(service.Name)
		if name == "" || !service.Secret.Configured() {
			return nil, errors.New("service credential name and secret are required")
		}
		if len(service.Secret.Value()) < minimumTokenBytes {
			return nil, errors.New("service credential must contain at least 32 bytes")
		}
		if _, exists := seenNames[name]; exists {
			return nil, errors.New("service credential names must be unique")
		}
		seenNames[name] = struct{}{}

		if len(service.Scopes) == 0 {
			return nil, errors.New("service credential must have at least one scope")
		}
		scopes, err := normalizeScopes(service.Scopes)
		if err != nil {
			return nil, err
		}
		for _, scope := range scopes {
			if scope == ScopePlatformAdmin {
				return nil, errors.New("service credentials cannot receive the owner admin scope")
			}
		}
		digest := sha256.Sum256([]byte(service.Secret.Value()))
		for _, existing := range auth.credentials {
			if subtle.ConstantTimeCompare(existing.digest[:], digest[:]) == 1 {
				return nil, errors.New("credentials must be unique")
			}
		}
		auth.credentials = append(auth.credentials, credentialDigest{
			kind: IdentityService, subject: name, digest: digest, scopes: scopes,
		})
	}
	return auth, nil
}

func normalizeScopes(scopes []string) ([]string, error) {
	allowed := map[string]struct{}{
		ScopePlatformAdmin: {}, ScopeDataRead: {}, ScopeDataDiagnose: {},
		ScopeResearchRun: {}, ScopeResearchRead: {},
	}
	seen := make(map[string]struct{}, len(scopes))
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if _, ok := allowed[scope]; !ok {
			return nil, errors.New("service credential contains an unsupported scope")
		}
		if _, exists := seen[scope]; exists {
			return nil, errors.New("service credential contains duplicate scopes")
		}
		seen[scope] = struct{}{}
		result = append(result, scope)
	}
	return result, nil
}

func (a *Authenticator) Authenticate(request *http.Request) (Principal, error) {
	if a == nil || request == nil {
		return Principal{}, errors.New("authentication unavailable")
	}
	values := request.Header.Values("Authorization")
	if len(values) == 0 {
		return Principal{Kind: IdentityAnonymous, Subject: "anonymous"}, nil
	}
	if len(values) != 1 {
		return Principal{}, errors.New("invalid authorization header")
	}
	parts := strings.Fields(values[0])
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return Principal{}, errors.New("invalid authorization header")
	}

	presented := sha256.Sum256([]byte(parts[1]))
	matched := -1
	// Compare every configured digest; never return on the first match.
	for index := range a.credentials {
		equal := subtle.ConstantTimeCompare(presented[:], a.credentials[index].digest[:])
		if equal == 1 {
			matched = index
		}
	}
	if matched < 0 {
		return Principal{}, errors.New("invalid credentials")
	}
	credential := a.credentials[matched]
	return Principal{
		Kind: credential.kind, Subject: credential.subject,
		Scopes: append([]string(nil), credential.scopes...),
	}, nil
}

type RoutePolicy struct {
	allowAnonymous bool
	allowOwner     bool
	serviceScopes  []string
}

func AnonymousAccess() RoutePolicy {
	return RoutePolicy{allowAnonymous: true}
}

func OwnerAccess() RoutePolicy {
	return RoutePolicy{allowOwner: true}
}

func ServiceAccess(scopes ...string) RoutePolicy {
	return RoutePolicy{serviceScopes: append([]string(nil), scopes...)}
}

func OwnerOrServiceAccess(scopes ...string) RoutePolicy {
	return RoutePolicy{allowOwner: true, serviceScopes: append([]string(nil), scopes...)}
}

func (p RoutePolicy) valid() bool {
	if p.allowAnonymous && (p.allowOwner || len(p.serviceScopes) > 0) {
		return false
	}
	if !p.allowAnonymous && !p.allowOwner && len(p.serviceScopes) == 0 {
		return false
	}
	for _, scope := range p.serviceScopes {
		if _, err := normalizeScopes([]string{scope}); err != nil {
			return false
		}
	}
	return true
}

type principalContextKey struct{}

func PrincipalFromContext(request *http.Request) (Principal, bool) {
	if request == nil {
		return Principal{}, false
	}
	principal, ok := request.Context().Value(principalContextKey{}).(Principal)
	return principal, ok
}

func Guard(authenticator *Authenticator, policy RoutePolicy, next http.Handler) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if !policy.valid() || authenticator == nil {
			writeAuthError(w, http.StatusForbidden, "route_policy_denied", "route access policy is not configured")
			return
		}
		principal, err := authenticator.Authenticate(request)
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeAuthError(w, http.StatusUnauthorized, "unauthorized", "valid bearer credentials are required")
			return
		}
		if !authorized(principal, policy) {
			writeAuthError(w, http.StatusForbidden, "forbidden", "the caller is not allowed to access this route")
			return
		}
		ctx := request.Context()
		ctx = withPrincipal(ctx, principal)
		next.ServeHTTP(w, request.WithContext(ctx))
	})
}

func authorized(principal Principal, policy RoutePolicy) bool {
	switch principal.Kind {
	case IdentityAnonymous:
		return policy.allowAnonymous
	case IdentityOwner:
		return policy.allowOwner
	case IdentityService:
		if len(policy.serviceScopes) == 0 {
			return false
		}
		for _, required := range policy.serviceScopes {
			if !contains(principal.Scopes, required) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func withPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]string{"code": code, "message": message},
	})
}
