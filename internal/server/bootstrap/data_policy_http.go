package bootstrap

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	marketdataauthorization "example.com/stock-analysis-flow/internal/marketdata/authorization"
	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	serversecurity "example.com/stock-analysis-flow/internal/server/security"
)

type dataPolicyHTTPHandler struct {
	authenticator ownerSessionRequestAuthenticator
	cookieName    string
	manager       dataAuthorizationPolicyManager
}

type dataPolicyPublishRequest struct {
	SchemaVersion          string          `json:"schema_version"`
	ExpectedCurrentVersion json.RawMessage `json:"expected_current_version"`
	PolicyVersion          string          `json:"policy_version"`
	Policy                 marketdataauthorization.DataAccessPolicy `json:"policy"`
}

type dataPolicyResponse struct {
	SchemaVersion string                                      `json:"schema_version"`
	PolicyType    string                                      `json:"policy_type"`
	PolicyRef     string                                      `json:"policy_ref"`
	PolicyVersion string                                      `json:"policy_version"`
	PayloadHash   string                                      `json:"payload_hash"`
	CreatedAt     string                                      `json:"created_at"`
	Policy        marketdataauthorization.DataAccessPolicy   `json:"policy"`
}

func newDataPolicyHTTPHandler(composition *ownerSessionComposition) (http.Handler, error) {
	if composition == nil || composition.authenticator == nil || composition.dataPolicyManager == nil {
		return nil, errors.New("Owner Session and Data authorization policy Application are required")
	}
	handler := &dataPolicyHTTPHandler{
		authenticator: composition.authenticator,
		cookieName:    composition.cookie.Name,
		manager:       composition.dataPolicyManager,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/owner/data/daily-bars-access-policy", handler.current)
	mux.HandleFunc("POST /api/v1/owner/data/daily-bars-access-policy/versions", handler.publish)
	return mux, nil
}

func (h *dataPolicyHTTPHandler) current(w http.ResponseWriter, r *http.Request) {
	authenticated, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	current, err := h.manager.Current(authenticated.Context())
	if err != nil {
		h.writePolicyError(w, r, err)
		return
	}
	h.writePolicy(w, http.StatusOK, current)
}

func (h *dataPolicyHTTPHandler) publish(w http.ResponseWriter, r *http.Request) {
	authenticated, ok := h.authenticate(w, r)
	if !ok {
		return
	}
	var input dataPolicyPublishRequest
	if err := decodeOwnerJSON(r, &input); err != nil || input.SchemaVersion != "1.0.0" ||
		input.ExpectedCurrentVersion == nil || input.PolicyVersion == "" {
		writePublicError(w, r, http.StatusBadRequest, publicError{
			Class: "validation", Code: "server.invalid_data_policy_request",
			Message: "Data policy publication request is invalid",
		})
		return
	}
	expectedVersion := ""
	if string(input.ExpectedCurrentVersion) != "null" {
		if err := json.Unmarshal(input.ExpectedCurrentVersion, &expectedVersion); err != nil || expectedVersion == "" {
			writePublicError(w, r, http.StatusBadRequest, publicError{
				Class: "validation", Code: "server.invalid_data_policy_request",
				Message: "Data policy publication request is invalid",
			})
			return
		}
	}
	published, err := h.manager.Publish(
		authenticated.Context(), expectedVersion, input.PolicyVersion, input.Policy,
	)
	if err != nil {
		h.writePolicyError(w, r, err)
		return
	}
	h.writePolicy(w, http.StatusCreated, published)
}

func (h *dataPolicyHTTPHandler) authenticate(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if len(r.Header.Values("Authorization")) > 0 {
		status := http.StatusUnauthorized
		code := "server.session_unauthenticated"
		message := "Owner Session authentication is required"
		if hasCookie(r, h.cookieName) {
			status = http.StatusBadRequest
			code = "server.ambiguous_authentication"
			message = "provide Owner Session only"
		}
		writePublicError(w, r, status, publicError{
			Class: "unauthenticated", Code: code, Message: message,
		})
		return nil, false
	}
	authenticated, cookie, err := h.authenticator.Authenticate(r)
	if err != nil {
		if !writePlatformFault(w, r, err) {
			writePublicError(w, r, http.StatusInternalServerError, publicError{
				Class: "internal", Code: "server.session_authentication_failed",
				Message: "authentication is unavailable",
			})
		}
		return nil, false
	}
	if authenticated == nil {
		writePublicError(w, r, http.StatusInternalServerError, publicError{
			Class: "internal", Code: "server.session_authentication_failed",
			Message: "authentication is unavailable",
		})
		return nil, false
	}
	principal, ok := platformsecurity.PrincipalFromContextValue(authenticated.Context())
	if !ok || principal.Kind != platformsecurity.IdentityOwner ||
		principal.AuthMethod != platformsecurity.AuthMethodOwnerSession || principal.Subject != "owner" {
		writePublicError(w, r, http.StatusForbidden, publicError{
			Class: "forbidden", Code: "server.data_policy_owner_required",
			Message: "Data policy administration requires an Owner Session",
		})
		return nil, false
	}
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	return authenticated, true
}

func (h *dataPolicyHTTPHandler) writePolicyError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, marketdataapplication.ErrDataPolicyNotPublished):
		writePublicError(w, r, http.StatusNotFound, publicError{
			Class: "not_found", Code: "server.data_policy_not_published",
			Message: "No Data authorization policy has been published",
		})
	case errors.Is(err, marketdataapplication.ErrDataPolicyVersionConflict):
		writePublicError(w, r, http.StatusConflict, publicError{
			Class: "conflict", Code: "server.data_policy_version_conflict",
			Message: "Data policy version has changed; reload the current version",
		})
	case errors.Is(err, marketdataapplication.ErrDataPolicyInvalidChange),
		errors.Is(err, marketdataauthorization.ErrPolicyInvalid):
		writePublicError(w, r, http.StatusBadRequest, publicError{
			Class: "validation", Code: "server.invalid_data_policy_request",
			Message: "Data policy change is invalid",
		})
	case errors.Is(err, marketdataapplication.ErrDataPolicyOwnerDenied):
		writePublicError(w, r, http.StatusForbidden, publicError{
			Class: "forbidden", Code: "server.data_policy_owner_required",
			Message: "Data policy administration requires an Owner Session",
		})
	case errors.Is(err, marketdataapplication.ErrDataPolicyOwnerUnavailable):
		writePublicError(w, r, http.StatusServiceUnavailable, publicError{
			Class: "unavailable", Code: "server.data_policy_unavailable",
			Message: "Data authorization policy service is unavailable", Retryable: true,
		})
	default:
		if !writePlatformFault(w, r, err) {
			writePublicError(w, r, http.StatusInternalServerError, publicError{
				Class: "internal", Code: "server.data_policy_failed",
				Message: "Data policy operation failed",
			})
		}
	}
}

func (h *dataPolicyHTTPHandler) writePolicy(w http.ResponseWriter, status int, current marketdataauthorization.CurrentPolicy) {
	response := dataPolicyResponse{
		SchemaVersion: "1.0.0",
		PolicyType: current.Version.PolicyType,
		PolicyRef: current.Version.PolicyRef,
		PolicyVersion: current.Version.PolicyVersion,
		PayloadHash: current.Version.PayloadHash,
		CreatedAt: current.Version.CreatedAt.UTC().Format(time.RFC3339Nano),
		Policy: current.Policy,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(response)
}

var _ = serversecurity.ErrSessionUnauthenticated
