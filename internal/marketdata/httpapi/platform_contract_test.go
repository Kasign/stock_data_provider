package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestPlatformFaultHTTPProjectionMapsClassAndDefaultsToNonRetryable(t *testing.T) {
	tests := []struct {
		status int
		class  string
	}{
		{http.StatusBadRequest, "validation"},
		{http.StatusUnauthorized, "unauthenticated"},
		{http.StatusForbidden, "forbidden"},
		{http.StatusNotFound, "not_found"},
		{http.StatusConflict, "conflict"},
		{http.StatusBadGateway, "unavailable"},
		{http.StatusServiceUnavailable, "unavailable"},
		{http.StatusGatewayTimeout, "timeout"},
		{http.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		writeErr(rec, tt.status, "stable_test_code", "safe test message", "request-123")
		var body errorBody
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Error.Class != tt.class || body.Error.Code != "stable_test_code" ||
			body.Error.Message != "safe test message" || body.Error.Retryable {
			t.Fatalf("unexpected public fault projection for %d: %+v", tt.status, body.Error)
		}
		if body.Error.RequestID != "request-123" {
			t.Fatalf("request ID compatibility field was not preserved: %+v", body.Error)
		}
	}
}
func TestDomainErrorProjectionDoesNotExposeInternalCause(t *testing.T) {
	rec := httptest.NewRecorder()
	writeDomainErr(rec, errors.New("provider response included token=secret and raw payload"), "request-123")

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusBadGateway || body.Error.Class != "unavailable" ||
		body.Error.Code != "PROVIDER_BAD_RESPONSE" || body.Error.Retryable {
		t.Fatalf("unexpected provider failure projection: status=%d error=%+v", rec.Code, body.Error)
	}
	if strings.Contains(body.Error.Message, "secret") || strings.Contains(body.Error.Message, "raw payload") {
		t.Fatalf("public message leaked internal cause: %q", body.Error.Message)
	}
}

func TestDomainTimeoutIsMarkedAsRetryCandidateWithSafeMessage(t *testing.T) {
	rec := httptest.NewRecorder()
	writeDomainErr(rec, context.DeadlineExceeded, "request-123")

	var body errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusGatewayTimeout || body.Error.Class != "timeout" ||
		!body.Error.Retryable || body.Error.Message != "upstream data provider request timed out" {
		t.Fatalf("unexpected timeout projection: status=%d error=%+v", rec.Code, body.Error)
	}
}

func TestOpenAPIErrorSchemaDocumentsPlatformFaultProjection(t *testing.T) {
	spec, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(spec)
	for _, field := range []string{
		"version: 1.1.0",
		"class: {type: string, enum: [validation, unauthenticated, forbidden, not_found, conflict, unavailable, timeout, internal]",
		"retryable: {type: boolean",
		"Safe summary; never contains raw provider responses or internal causes",
	} {
		if !strings.Contains(text, field) {
			t.Errorf("OpenAPI Error schema is missing %q", field)
		}
	}
}
