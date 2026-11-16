package bootstrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	"example.com/stock-analysis-flow/internal/platform/observability"
	serversecurity "example.com/stock-analysis-flow/internal/server/security"
)

type ownerSessionHTTPHandler struct {
	lifecycle     *serversecurity.SessionLifecycleService
	authenticator *serversecurity.SessionAuthenticator
	cookie        platformsecurity.SessionCookieConfig
}

type ownerSessionResponse struct {
	SchemaVersion string `json:"schema_version"`
	Authenticated bool   `json:"authenticated"`
	CSRFToken     string `json:"csrf_token,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
}

type ownerLoginRequest struct {
	Password string `json:"password"`
}

type ownerPasswordChangeRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func newOwnerSessionHTTPHandler(composition *ownerSessionComposition, cookie platformsecurity.SessionCookieConfig) (http.Handler, error) {
	if composition == nil || composition.lifecycle == nil || composition.authenticator == nil {
		return nil, errors.New("Owner Session composition is required")
	}
	h := &ownerSessionHTTPHandler{
		lifecycle: composition.lifecycle, authenticator: composition.authenticator, cookie: cookie,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/owner/session", h.login)
	mux.HandleFunc("GET /api/v1/owner/session", h.current)
	mux.HandleFunc("DELETE /api/v1/owner/session", h.logout)
	mux.HandleFunc("POST /api/v1/owner/session/rotate", h.rotate)
	mux.HandleFunc("POST /api/v1/owner/session/csrf", h.refreshCSRF)
	mux.HandleFunc("PUT /api/v1/owner/password", h.changePassword)
	return mux, nil
}

func (h *ownerSessionHTTPHandler) login(w http.ResponseWriter, r *http.Request) {
	h.noStore(w)
	if err := h.authenticator.ValidateLoginOrigin(r); err != nil {
		h.writeError(w, r, err)
		return
	}
	var input ownerLoginRequest
	if err := decodeOwnerJSON(r, &input); err != nil {
		h.writeError(w, r, platformfault.New(platformfault.ClassValidation, "server.invalid_owner_login_request", "login request is invalid", err))
		return
	}
	sourceIP, err := remoteClientIP(r.RemoteAddr)
	if err != nil {
		h.writeError(w, r, platformfault.New(platformfault.ClassValidation, "server.invalid_login_source", "login source is invalid", err))
		return
	}
	issue, err := h.lifecycle.Login(r.Context(), serversecurity.LoginInput{
		Password: []byte(input.Password), SourceIP: sourceIP, RequestID: observability.FromContext(r.Context()),
	})
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	http.SetCookie(w, issue.Cookie)
	h.writeJSON(w, http.StatusOK, ownerSessionResponse{
		SchemaVersion: "1.0.0", Authenticated: true, CSRFToken: issue.CSRFToken,
		ExpiresAt: issue.Cookie.Expires.UTC().Format(time.RFC3339Nano),
	})
}

func (h *ownerSessionHTTPHandler) current(w http.ResponseWriter, r *http.Request) {
	h.noStore(w)
	authenticated, cookie, err := h.authenticator.Authenticate(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	_ = authenticated
	http.SetCookie(w, cookie)
	h.writeJSON(w, http.StatusOK, ownerSessionResponse{
		SchemaVersion: "1.0.0", Authenticated: true,
		ExpiresAt: cookie.Expires.UTC().Format(time.RFC3339Nano),
	})
}

func (h *ownerSessionHTTPHandler) refreshCSRF(w http.ResponseWriter, r *http.Request) {
	h.noStore(w)
	_, cookie, csrfToken, err := h.authenticator.RefreshCSRFToken(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	http.SetCookie(w, cookie)
	h.writeJSON(w, http.StatusOK, ownerSessionResponse{
		SchemaVersion: "1.0.0", Authenticated: true, CSRFToken: csrfToken,
	})
}

func (h *ownerSessionHTTPHandler) logout(w http.ResponseWriter, r *http.Request) {
	h.noStore(w)
	_, _, err := h.authenticator.Authenticate(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	rawToken, err := h.rawSessionToken(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	cookie, err := h.lifecycle.Logout(r.Context(), rawToken, observability.FromContext(r.Context()))
	if cookie != nil {
		http.SetCookie(w, cookie)
	}
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ownerSessionHTTPHandler) rotate(w http.ResponseWriter, r *http.Request) {
	h.noStore(w)
	_, _, err := h.authenticator.Authenticate(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	rawToken, err := h.rawSessionToken(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	issue, err := h.lifecycle.RotateSession(r.Context(), rawToken, observability.FromContext(r.Context()))
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	http.SetCookie(w, issue.Cookie)
	h.writeJSON(w, http.StatusOK, ownerSessionResponse{
		SchemaVersion: "1.0.0", Authenticated: true, CSRFToken: issue.CSRFToken,
		ExpiresAt: issue.Cookie.Expires.UTC().Format(time.RFC3339Nano),
	})
}

func (h *ownerSessionHTTPHandler) changePassword(w http.ResponseWriter, r *http.Request) {
	h.noStore(w)
	_, _, err := h.authenticator.Authenticate(r)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	var input ownerPasswordChangeRequest
	if err := decodeOwnerJSON(r, &input); err != nil {
		h.writeError(w, r, platformfault.New(platformfault.ClassValidation, "server.invalid_password_change", "password change request is invalid", err))
		return
	}
	if err := h.lifecycle.ChangePassword(r.Context(), []byte(input.CurrentPassword), []byte(input.NewPassword),
		observability.FromContext(r.Context())); err != nil {
		h.writeError(w, r, err)
		return
	}
	cookie, err := platformsecurity.ClearSessionCookie(h.cookie)
	if err != nil {
		h.writeError(w, r, err)
		return
	}
	http.SetCookie(w, cookie)
	w.WriteHeader(http.StatusNoContent)
}

func (h *ownerSessionHTTPHandler) rawSessionToken(r *http.Request) (string, error) {
	var value string
	count := 0
	for _, cookie := range r.Cookies() {
		if cookie.Name == h.cookie.Name {
			count++
			value = cookie.Value
		}
	}
	if count != 1 || value == "" {
		return "", platformfault.New(platformfault.ClassUnauthenticated, "server.session_unauthenticated", "authentication required", nil)
	}
	return value, nil
}

func (h *ownerSessionHTTPHandler) writeError(w http.ResponseWriter, r *http.Request, err error) {
	var value *platformfault.Fault
	if errors.As(err, &value) && value != nil && value.Code == "server.owner_login_rate_limited" {
		writePublicError(w, r, http.StatusTooManyRequests, publicError{
			Class: string(value.Class), Code: value.Code, Message: value.SafeMessage,
		})
		return
	}
	if writePlatformFault(w, r, err) {
		return
	}
	writePublicError(w, r, http.StatusInternalServerError, publicError{
		Class: string(platformfault.ClassInternal), Code: "server.owner_session_failed",
		Message: "owner session operation failed",
	})
}

func (h *ownerSessionHTTPHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func (h *ownerSessionHTTPHandler) noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func decodeOwnerJSON(r *http.Request, target any) error {
	if r == nil || r.Body == nil {
		return errors.New("application/json request body is required")
	}
	mediaType, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || !strings.EqualFold(mediaType, "application/json") {
		return errors.New("application/json request body is required")
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("request body must contain exactly one JSON value")
		}
		return err
	}
	return nil
}

func remoteClientIP(remoteAddr string) (net.IP, error) {
	host, _, err := net.SplitHostPort(strings.TrimSpace(remoteAddr))
	if err != nil {
		return nil, fmt.Errorf("invalid direct peer address: %w", err)
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return nil, errors.New("direct peer is not an IP address")
	}
	return ip, nil
}
