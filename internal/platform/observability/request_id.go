// Package observability provides small request-correlation primitives for
// HTTP boundaries and structured logs.
package observability

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const RequestIDHeader = "X-Request-ID"

type requestIDKey struct{}

var fallbackIDCounter atomic.Uint64

// Middleware creates a fresh Request ID at the HTTP boundary by default.
// Incoming client headers are not trusted unless the caller explicitly uses
// MiddlewareWithOptions after establishing a trusted, non-bypassable gateway.
func Middleware(next http.Handler) http.Handler {
	return MiddlewareWithOptions(next, MiddlewareOptions{})
}

// MiddlewareOptions controls which incoming correlation metadata may be trusted.
type MiddlewareOptions struct {
	// TrustIncomingRequestID must only be enabled when a trusted gateway
	// overwrites X-Request-ID and the service cannot be reached around it.
	TrustIncomingRequestID bool
}

// MiddlewareWithOptions validates or creates a Request ID, stores it in the
// request context, and returns it to the caller in the response header.
func MiddlewareWithOptions(next http.Handler, options MiddlewareOptions) http.Handler {
	if next == nil {
		next = http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := ""
		if options.TrustIncomingRequestID {
			// Reject duplicate values rather than selecting an ambiguous ID.
			values := r.Header.Values(RequestIDHeader)
			if len(values) == 1 {
				requestID = strings.TrimSpace(values[0])
			}
		}
		if !ValidRequestID(requestID) {
			requestID = newRequestID()
		}
		w.Header().Set(RequestIDHeader, requestID)
		ctx := context.WithValue(r.Context(), requestIDKey{}, requestID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// FromContext returns the request ID associated with ctx, if any.
func FromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	requestID, _ := ctx.Value(requestIDKey{}).(string)
	return requestID
}

// WithRequestID associates a previously validated request ID with ctx.
// Invalid or empty IDs leave the context unchanged.
func WithRequestID(ctx context.Context, requestID string) context.Context {
	if ctx == nil || !ValidRequestID(requestID) {
		return ctx
	}
	return context.WithValue(ctx, requestIDKey{}, requestID)
}

// PropagateRequestID makes the validated context request ID the sole source
// for the outbound header. A caller-supplied header is removed when the
// context has no request ID, preventing untrusted values from leaking upstream.
func PropagateRequestID(ctx context.Context, request *http.Request) {
	if request == nil {
		return
	}
	if requestID := FromContext(ctx); requestID != "" {
		request.Header.Set(RequestIDHeader, requestID)
		return
	}
	request.Header.Del(RequestIDHeader)
}

// ValidRequestID accepts a bounded ASCII token safe for an HTTP header and logs.
func ValidRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}

func newRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err == nil {
		return hex.EncodeToString(value[:])
	}
	return fmt.Sprintf("%x-%x", time.Now().UTC().UnixNano(), fallbackIDCounter.Add(1))
}
