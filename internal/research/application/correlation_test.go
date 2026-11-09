package application

import (
	"context"
	"testing"

	"example.com/stock-analysis-flow/internal/platform/observability"
)

func TestCorrelationIDIsIndependentFromPlatformRequestID(t *testing.T) {
	ctx := context.Background()
	ctx = observability.WithRequestID(ctx, "request_123")
	got, err := correlationIDForPublication(ctx, &fixedIDs{})
	if err != nil { t.Fatal(err) }
	if got != "cor_test-1" { t.Fatalf("Request ID must not be reused as Correlation ID; got %q", got) }
}

func TestCorrelationIDInheritsTrustedParentContext(t *testing.T) {
	ctx, err := WithCorrelationID(context.Background(), "corr_parent_123")
	if err != nil { t.Fatal(err) }
	got, err := correlationIDForPublication(ctx, &fixedIDs{})
	if err != nil { t.Fatal(err) }
	if got != "corr_parent_123" { t.Fatalf("correlation ID = %q, want trusted parent ID", got) }
}

func TestWithCorrelationIDRejectsInvalidValue(t *testing.T) {
	if _, err := WithCorrelationID(context.Background(), "client supplied id"); err == nil {
		t.Fatal("invalid correlation ID must be rejected")
	}
}
