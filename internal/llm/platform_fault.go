package llm

import (
	"context"
	"errors"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

// ModelErrorFromPlatformFault projects an internal Platform Fault into the
// stable LLM error contract. Dependency and Cause are intentionally omitted.
func ModelErrorFromPlatformFault(err error) *ModelErrorInfo {
	if err == nil {
		return nil
	}

	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault == nil {
		return nil
	}

	kind := ErrorUpstream
	switch {
	case errors.Is(err, context.Canceled):
		kind = ErrorCancelled
	case errors.Is(err, context.DeadlineExceeded), platformFault.Class == fault.ClassTimeout:
		kind = ErrorTimeout
	default:
		switch platformFault.Class {
		case fault.ClassValidation:
			kind = ErrorInvalidRequest
		case fault.ClassUnauthenticated, fault.ClassForbidden:
			kind = ErrorUnauthorized
		case fault.ClassUnavailable:
			kind = ErrorUpstreamUnavailable
		default:
			kind = ErrorUpstream
		}
	}

	code := platformFault.Code
	if code == "" {
		code = "platform.fault." + string(platformFault.Class)
	}
	message := platformFault.SafeMessage
	if message == "" {
		message = "model provider request failed"
	}

	return &ModelErrorInfo{
		Kind: kind, StableCode: code, Retryable: platformFault.Retryable,
		SafeMessage: message,
	}
}
