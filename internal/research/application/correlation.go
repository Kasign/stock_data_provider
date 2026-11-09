package application

import (
	"context"
	"fmt"
)

// correlationIDKey is intentionally distinct from Platform Request ID context.
// Server may inject a correlation ID only after validating the trusted parent
// workflow/request context; a client-provided X-Request-ID is not a correlation ID.
type correlationIDKey struct{}

// WithCorrelationID attaches a trusted cross-module correlation identifier.
// Callers must pass a value inherited from a trusted parent execution context.
func WithCorrelationID(ctx context.Context, correlationID string) (context.Context, error) {
	if ctx == nil || !validCorrelationID(correlationID) {
		return nil, fmt.Errorf("valid context and correlation ID are required")
	}
	return context.WithValue(ctx, correlationIDKey{}, correlationID), nil
}

func correlationIDForPublication(ctx context.Context, ids IDGenerator) (string, error) {
	if ctx != nil {
		if value, ok := ctx.Value(correlationIDKey{}).(string); ok && validCorrelationID(value) {
			return value, nil
		}
	}
	if ids == nil { return "", fmt.Errorf("Research ID generator is not configured") }
	value, err := ids.NewID("cor_")
	if err != nil { return "", err }
	if !validCorrelationID(value) { return "", fmt.Errorf("Research ID generator returned an invalid correlation ID") }
	return value, nil
}

func validCorrelationID(value string) bool {
	if value == "" || len(value) > 128 { return false }
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') { return false }
	}
	return true
}
