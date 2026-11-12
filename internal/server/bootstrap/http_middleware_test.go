package bootstrap

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/observability"
)

func TestHTTPMiddlewareRegeneratesUntrustedRequestID(t *testing.T) {
	handler, err := newHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := observability.FromContext(r.Context()); got == "" || got == "caller-controlled" {
			t.Fatalf("request ID = %q, want generated ID", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}), httpMiddlewareConfig{maxRequestBodyBytes: 1024, maxConcurrentRequests: 2})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api", nil)
	request.Header.Add("X-Request-ID", "caller-controlled")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
	if got := response.Header().Get("X-Request-ID"); got == "" || got == "caller-controlled" {
		t.Fatalf("response request ID = %q, want generated ID", got)
	}
}

func TestHTTPMiddlewareRejectsDeclaredOversizedBody(t *testing.T) {
	handler, err := newHTTPMiddleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not receive oversized request")
	}), httpMiddlewareConfig{maxRequestBodyBytes: 1024, maxConcurrentRequests: 2})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(strings.Repeat("x", 1025)))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusRequestEntityTooLarge)
	}
	if !strings.Contains(response.Body.String(), "server.request_too_large") {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
}

func TestHTTPMiddlewareWrapsChunkedBodyWithMaxBytesReader(t *testing.T) {
	handler, err := newHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, readErr := io.ReadAll(r.Body)
		var maxBytesErr *http.MaxBytesError
		if !errors.As(readErr, &maxBytesErr) {
			t.Errorf("ReadAll error = %v, want MaxBytesError", readErr)
		}
		w.WriteHeader(http.StatusNoContent)
	}), httpMiddlewareConfig{maxRequestBodyBytes: 1024, maxConcurrentRequests: 2})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api", strings.NewReader(strings.Repeat("x", 2048)))
	request.ContentLength = -1
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestHTTPMiddlewareKeepsHeadLivenessAvailableDuringOverload(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	handler, err := newHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/slow" {
			close(entered)
			<-release
		}
		w.WriteHeader(http.StatusNoContent)
	}), httpMiddlewareConfig{maxRequestBodyBytes: 1024, maxConcurrentRequests: 1})
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
	}()
	<-entered
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodHead, "/healthz", nil))
	if response.Code != http.StatusNoContent {
		t.Fatalf("HEAD liveness status = %d, want %d", response.Code, http.StatusNoContent)
	}
	close(release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("blocked request did not finish")
	}
}

func TestHTTPMiddlewareBoundsConcurrentRequestsButKeepsLivenessAvailable(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	handler, err := newHTTPMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path == "/slow" {
			close(entered)
			<-release
		}
		w.WriteHeader(http.StatusNoContent)
	}), httpMiddlewareConfig{maxRequestBodyBytes: 1024, maxConcurrentRequests: 1})
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan struct{})
	go func() {
		defer close(firstDone)
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/slow", nil))
	}()
	<-entered
	overloaded := httptest.NewRecorder()
	handler.ServeHTTP(overloaded, httptest.NewRequest(http.MethodGet, "/api", nil))
	if overloaded.Code != http.StatusServiceUnavailable {
		t.Fatalf("overload status = %d, want %d", overloaded.Code, http.StatusServiceUnavailable)
	}
	liveness := httptest.NewRecorder()
	handler.ServeHTTP(liveness, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if liveness.Code != http.StatusNoContent {
		t.Fatalf("liveness status = %d, want %d", liveness.Code, http.StatusNoContent)
	}
	close(release)
	select {
	case <-firstDone:
	case <-time.After(time.Second):
		t.Fatal("first request did not finish")
	}
	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2 (slow + liveness)", calls.Load())
	}
}

func TestWritePlatformFaultMapsStableClassesAndHidesInternals(t *testing.T) {
	tests := []struct {
		class fault.Class
		code  int
	}{
		{fault.ClassValidation, http.StatusBadRequest},
		{fault.ClassUnauthenticated, http.StatusUnauthorized},
		{fault.ClassForbidden, http.StatusForbidden},
		{fault.ClassNotFound, http.StatusNotFound},
		{fault.ClassConflict, http.StatusConflict},
		{fault.ClassUnavailable, http.StatusServiceUnavailable},
		{fault.ClassTimeout, http.StatusGatewayTimeout},
		{fault.ClassInternal, http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(string(test.class), func(t *testing.T) {
			f := &fault.Fault{Class: test.class, Code: "server.test.failure", SafeMessage: "safe failure", Retryable: test.class == fault.ClassUnavailable}
			f.Dependency = "private.database"
			f.Cause = errors.New("password=must-not-leak")
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/api", nil)
			request = request.WithContext(observability.WithRequestID(request.Context(), "req-contract-001"))
			if !writePlatformFault(response, request, f) {
				t.Fatal("writePlatformFault() = false")
			}
			if response.Code != test.code {
				t.Fatalf("status = %d, want %d", response.Code, test.code)
			}
			if strings.Contains(response.Body.String(), "private.database") || strings.Contains(response.Body.String(), "must-not-leak") {
				t.Fatalf("internal fault fields leaked: %s", response.Body.String())
			}
			for _, field := range []string{"class", "code", "message", "retryable"} {
				if !strings.Contains(response.Body.String(), `"`+field+`"`) {
					t.Fatalf("response missing %s: %s", field, response.Body.String())
				}
			}
		})
	}
}


func TestWritePlatformFaultMatchesPLATCTR001EnvelopeAndRequestID(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/api", nil)
	request = request.WithContext(observability.WithRequestID(request.Context(), "req-contract-001"))
	response := httptest.NewRecorder()
	faultValue := &fault.Fault{Class: fault.ClassUnavailable, Code: "server.dependency_unavailable", SafeMessage: "dependency unavailable", Retryable: true}
	if !writePlatformFault(response, request, faultValue) {
		t.Fatal("writePlatformFault() = false")
	}
	var body struct {
		SchemaVersion string `json:"schema_version"`
		Error         struct {
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode ErrorEnvelope: %v", err)
	}
	if body.SchemaVersion != "1.0.0" || body.Error.RequestID != "req-contract-001" {
		t.Fatalf("ErrorEnvelope contract fields = %+v", body)
	}
	if response.Header().Get(observability.RequestIDHeader) != "" {
		t.Fatal("writePlatformFault must not manufacture a request ID response header")
	}
}

func TestWritePlatformFaultRejectsInvalidMetadata(t *testing.T) {
	response := httptest.NewRecorder()
	f := &fault.Fault{Class: fault.ClassValidation, Code: "Invalid Error", SafeMessage: "unsafe"}
	if !writePlatformFault(response, nil, f) {
		t.Fatal("writePlatformFault() = false")
	}
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), "Invalid Error") {
		t.Fatalf("invalid metadata leaked: %s", response.Body.String())
	}
}
