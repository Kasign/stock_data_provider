package observability

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

type requestIDGolden struct {
	Header                              string   `json:"header"`
	MaxLength                           int      `json:"max_length"`
	GeneratedRandomBytes                int      `json:"generated_random_bytes"`
	TrustIncomingRequiresExplicitOption bool     `json:"trust_incoming_requires_explicit_option"`
	ValidValues                         []string `json:"valid_values"`
	InvalidValues                       []string `json:"invalid_values"`
	DuplicateValues                     []string `json:"duplicate_values"`
	OutboundWithContext                 struct {
		ContextID      string `json:"context_id"`
		ExistingHeader string `json:"existing_header"`
		ExpectedHeader string `json:"expected_header"`
	} `json:"outbound_with_context"`
	OutboundWithoutContext struct {
		ExistingHeader string  `json:"existing_header"`
		ExpectedHeader *string `json:"expected_header"`
	} `json:"outbound_without_context"`
}

func TestRequestIDContractGoldenVectors(t *testing.T) {
	data, err := os.ReadFile("../../../doc/platform/golden/PLAT-CTR-002-request-id.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden requestIDGolden
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	if golden.Header != RequestIDHeader || golden.MaxLength != 128 || golden.GeneratedRandomBytes != 16 || !golden.TrustIncomingRequiresExplicitOption {
		t.Fatalf("unexpected Request ID contract metadata: %+v", golden)
	}
	for _, value := range golden.ValidValues {
		if !ValidRequestID(value) {
			t.Errorf("Golden valid value rejected: %q", value)
		}
	}
	for _, value := range golden.InvalidValues {
		if ValidRequestID(value) {
			t.Errorf("Golden invalid value accepted: %q", value)
		}
	}

	var contextID string
	handler := Middleware(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		contextID = FromContext(request.Context())
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	generated := response.Header().Get(RequestIDHeader)
	decoded, err := hex.DecodeString(generated)
	if err != nil || len(decoded) != golden.GeneratedRandomBytes || contextID != generated {
		t.Fatalf("generated Request ID does not match Golden: header=%q context=%q decoded=%d err=%v", generated, contextID, len(decoded), err)
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	for _, value := range golden.DuplicateValues {
		request.Header.Add(RequestIDHeader, value)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if !ValidRequestID(contextID) {
		t.Fatalf("duplicate Header did not produce a valid replacement: %q", contextID)
	}
	for _, value := range golden.DuplicateValues {
		if contextID == value {
			t.Fatalf("duplicate Header value was preserved: %q", contextID)
		}
	}
	if response.Header().Get(RequestIDHeader) != contextID {
		t.Fatal("response Header does not echo replacement Request ID")
	}

	// A valid incoming ID is accepted only under the explicit trusted-gateway option.
	trustedHandler := MiddlewareWithOptions(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		contextID = FromContext(request.Context())
	}), MiddlewareOptions{TrustIncomingRequestID: true})
	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, golden.ValidValues[2])
	response = httptest.NewRecorder()
	trustedHandler.ServeHTTP(response, request)
	if contextID != golden.ValidValues[2] || response.Header().Get(RequestIDHeader) != contextID {
		t.Fatal("trusted gateway Request ID did not pass through the explicit option")
	}

	outbound, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if err != nil {
		t.Fatal(err)
	}
	outbound.Header.Set(RequestIDHeader, golden.OutboundWithContext.ExistingHeader)
	ctx := WithRequestID(context.Background(), golden.OutboundWithContext.ContextID)
	PropagateRequestID(ctx, outbound)
	if got := outbound.Header.Get(RequestIDHeader); got != golden.OutboundWithContext.ExpectedHeader {
		t.Fatalf("outbound Request ID = %q, want %q", got, golden.OutboundWithContext.ExpectedHeader)
	}

	outbound.Header.Set(RequestIDHeader, golden.OutboundWithoutContext.ExistingHeader)
	PropagateRequestID(context.Background(), outbound)
	if got := outbound.Header.Get(RequestIDHeader); got != "" || golden.OutboundWithoutContext.ExpectedHeader != nil {
		t.Fatalf("outbound Request ID without Context = %q, expected removal", got)
	}
}
