package bootstrap

import (
	"encoding/json"
	"errors"
	"net/http"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	serversecurity "example.com/stock-analysis-flow/internal/server/security"
)

type quantPolicyHTTPHandler struct {
	authenticator *serversecurity.SessionAuthenticator
	cookieName    string
	manager       quantResultAccessPolicyManager
}

type quantPolicyChangeRequest struct {
	ExpectedVersion *uint64 `json:"expected_version"`
}

type quantPolicyResponse struct {
	SchemaVersion string `json:"schema_version"`
	Version       uint64 `json:"version"`
	Enabled       bool   `json:"enabled"`
	SnapshotRef   string `json:"snapshot_ref,omitempty"`
}

func newQuantPolicyHTTPHandler(composition *ownerSessionComposition) (http.Handler, error) {
	if composition == nil || composition.authenticator == nil || composition.policyManager == nil {
		return nil, errors.New("Owner Session and Quant result access policy manager are required")
	}
	handler := &quantPolicyHTTPHandler{
		authenticator: composition.authenticator,
		cookieName: composition.cookie.Name,
		manager: composition.policyManager,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/owner/quant/result-access-policy", handler.current)
	mux.HandleFunc("PUT /api/v1/owner/quant/result-access-policy", handler.publish)
	mux.HandleFunc("POST /api/v1/owner/quant/result-access-policy/revoke", handler.revoke)
	return mux, nil
}

func (h *quantPolicyHTTPHandler) current(w http.ResponseWriter, r *http.Request) {
	_, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	state, err := h.manager.Current(r.Context())
	if err != nil {
		h.writePolicyError(w, r, err)
		return
	}
	h.writeState(w, http.StatusOK, state)
}

func (h *quantPolicyHTTPHandler) publish(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input quantPolicyChangeRequest
	if err := decodeOwnerJSON(r, &input); err != nil || input.ExpectedVersion == nil {
		writePublicError(w, r, http.StatusBadRequest, publicError{
			Class: "validation", Code: "server.invalid_quant_policy_request",
			Message: "policy change request is invalid",
		})
		return
	}
	state, err := h.manager.Publish(r.Context(), actor, *input.ExpectedVersion)
	if err != nil {
		h.writePolicyError(w, r, err)
		return
	}
	h.writeState(w, http.StatusOK, state)
}

func (h *quantPolicyHTTPHandler) revoke(w http.ResponseWriter, r *http.Request) {
	actor, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input quantPolicyChangeRequest
	if err := decodeOwnerJSON(r, &input); err != nil || input.ExpectedVersion == nil {
		writePublicError(w, r, http.StatusBadRequest, publicError{
			Class: "validation", Code: "server.invalid_quant_policy_request",
			Message: "policy change request is invalid",
		})
		return
	}
	state, err := h.manager.Revoke(r.Context(), actor, *input.ExpectedVersion)
	if err != nil {
		h.writePolicyError(w, r, err)
		return
	}
	h.writeState(w, http.StatusOK, state)
}

func (h *quantPolicyHTTPHandler) authenticate(w http.ResponseWriter, r *http.Request) (string, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if len(r.Header.Values("Authorization")) > 0 {
		status, code, message := http.StatusUnauthorized, "server.session_unauthenticated", "Owner Session authentication is required"
		if hasCookie(r, h.cookieName) {
			status, code, message = http.StatusBadRequest, "server.ambiguous_authentication", "provide Owner Session only"
		}
		writePublicError(w, r, status, publicError{Class: "unauthenticated", Code: code, Message: message})
		return "", false
	}
	authenticated, cookie, err := h.authenticator.Authenticate(r)
	if err != nil {
		if !writePlatformFault(w, r, err) {
			writePublicError(w, r, http.StatusInternalServerError, publicError{
				Class: "internal", Code: "server.session_authentication_failed",
				Message: "authentication is unavailable",
			})
		}
		return "", false
	}
	if authenticated == nil {
		writePublicError(w, r, http.StatusInternalServerError, publicError{
			Class: "internal", Code: "server.session_authentication_failed",
			Message: "authentication is unavailable",
		})
		return "", false
	}
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	principal, ok := platformsecurity.PrincipalFromContextValue(authenticated.Context())
	if !ok || principal.Kind != platformsecurity.IdentityOwner ||
		principal.AuthMethod != platformsecurity.AuthMethodOwnerSession || principal.Subject != "owner" {
		writePublicError(w, r, http.StatusForbidden, publicError{
			Class: "forbidden", Code: "server.quant_policy_owner_required",
			Message: "Quant policy administration requires an Owner Session",
		})
		return "", false
	}
	return principal.Subject, true
}

func (h *quantPolicyHTTPHandler) writePolicyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, quantapplication.ErrInvalidPolicyChange):
		writePublicError(w, r, http.StatusBadRequest, publicError{
			Class: "validation", Code: "server.invalid_quant_policy_change",
			Message: "Quant policy change is invalid",
		})
	case errors.Is(err, quantapplication.ErrPolicyVersionConflict):
		writePublicError(w, r, http.StatusConflict, publicError{
			Class: "conflict", Code: "server.quant_policy_version_conflict",
			Message: "Quant policy version has changed; reload the current version",
		})
	case errors.Is(err, quantapplication.ErrPolicyOwnerUnavailable):
		writePublicError(w, r, http.StatusServiceUnavailable, publicError{
			Class: "unavailable", Code: "server.quant_policy_unavailable",
			Message: "Quant policy service is unavailable", Retryable: true,
		})
	default:
		if !writePlatformFault(w, r, err) {
			writePublicError(w, r, http.StatusInternalServerError, publicError{
				Class: "internal", Code: "server.quant_policy_failed",
				Message: "Quant policy operation failed",
			})
		}
	}
}

func (h *quantPolicyHTTPHandler) writeState(w http.ResponseWriter, status int, state quantapplication.ResultAccessPolicyState) {
	response := quantPolicyResponse{
		SchemaVersion: "1.0.0", Version: state.Version, Enabled: state.Enabled,
	}
	if state.Snapshot != nil {
		response.SnapshotRef = state.Snapshot.SnapshotRef
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}
