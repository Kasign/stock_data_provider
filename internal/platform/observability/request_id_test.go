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
