package observability

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMiddlewarePreservesValidRequestID(t *testing.T) {
	const requestID = "request-123.test:abc"
	var contextID string
	handler := Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contextID = FromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, requestID)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if contextID != requestID {
		t.Fatalf("context request ID = %q", contextID)
	}
	if got := response.Header().Get(RequestIDHeader); got != requestID {
		t.Fatalf("response request ID = %q", got)
	}
}

func TestMiddlewareReplacesInvalidRequestID(t *testing.T) {
	var contextID string
	handler := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contextID = FromContext(r.Context())
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "bad\r\ninjected")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if !ValidRequestID(contextID) || contextID == "bad\r\ninjected" {
		t.Fatalf("invalid request ID was not replaced: %q", contextID)
	}
	if got := response.Header().Get(RequestIDHeader); got != contextID {
		t.Fatalf("response request ID = %q, want %q", got, contextID)
	}
}

func TestWithRequestIDAndOutboundPropagation(t *testing.T) {
	ctx := WithRequestID(context.Background(), "task-456")
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.com", nil)
	PropagateRequestID(ctx, request)
	if got := request.Header.Get(RequestIDHeader); got != "task-456" {
		t.Fatalf("outbound request ID = %q", got)
	}

	invalid := WithRequestID(context.Background(), "bad value")
	if FromContext(invalid) != "" {
		t.Fatal("invalid request ID must not be stored")
	}
}

func TestPropagateRequestIDRemovesCallerSuppliedHeaderWithoutContext(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Add(RequestIDHeader, "untrusted-client-value")
	request.Header.Add(RequestIDHeader, "duplicate-value")

	PropagateRequestID(context.Background(), request)

	if values := request.Header.Values(RequestIDHeader); len(values) != 0 {
		t.Fatalf("caller-supplied request ID must be removed, got %q", values)
	}
}

func TestPropagateRequestIDOverwritesCallerSuppliedHeader(t *testing.T) {
	request, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set(RequestIDHeader, "untrusted-client-value")
	ctx := WithRequestID(context.Background(), "trusted-context-id")

	PropagateRequestID(ctx, request)

	if values := request.Header.Values(RequestIDHeader); len(values) != 1 || values[0] != "trusted-context-id" {
		t.Fatalf("outbound request ID = %q, want one trusted context value", values)
	}
}

func TestRequestIDValidation(t *testing.T) {
	for _, value := range []string{"", "has space", "line\nbreak", strings.Repeat("a", 129)} {
		if ValidRequestID(value) {
			t.Errorf("expected %q to be invalid", value)
		}
	}
	for _, value := range []string{"abc", "a-b_c.d:e", strings.Repeat("a", 128)} {
		if !ValidRequestID(value) {
			t.Errorf("expected %q to be valid", value)
		}
	}
}

func TestMiddlewareReplacesDuplicateRequestIDHeaders(t *testing.T) {
	var contextID string
	handler := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		contextID = FromContext(r.Context())
	}))

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Add(RequestIDHeader, "client-id-one")
	request.Header.Add(RequestIDHeader, "client-id-two")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if !ValidRequestID(contextID) || contextID == "client-id-one" || contextID == "client-id-two" {
		t.Fatalf("duplicate request IDs were not replaced: %q", contextID)
	}
	if got := response.Header().Get(RequestIDHeader); got != contextID {
		t.Fatalf("response request ID = %q, want %q", got, contextID)
	}
}
