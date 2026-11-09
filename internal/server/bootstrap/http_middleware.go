package bootstrap

import (
	"encoding/json"
	"errors"
	"net/http"

	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/observability"
)

type httpMiddlewareConfig struct {
	maxRequestBodyBytes   int64
	maxConcurrentRequests int
}

type publicError struct {
	Class     string `json:"class"`
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type publicErrorEnvelope struct {
	Error publicError `json:"error"`
}

func newHTTPMiddleware(next http.Handler, cfg httpMiddlewareConfig) (http.Handler, error) {
	if next == nil || cfg.maxRequestBodyBytes <= 0 || cfg.maxConcurrentRequests <= 0 {
		return nil, errors.New("HTTP handler and positive request limits are required")
	}
	next = limitRequestBody(next, cfg.maxRequestBodyBytes)
	next = limitConcurrentRequests(next, cfg.maxConcurrentRequests)
	// Root Server can be reached without the reverse proxy, so incoming IDs
	// are regenerated instead of trusting caller-supplied X-Request-ID values.
	next = observability.MiddlewareWithOptions(next, observability.MiddlewareOptions{})
	return next, nil
}

func limitRequestBody(next http.Handler, maximum int64) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength > maximum {
			writePublicError(w, http.StatusRequestEntityTooLarge, publicError{
				Class: "validation", Code: "server.request_too_large",
				Message: "request body exceeds the configured limit",
			})
			return
		}
		if r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, maximum)
		}
		next.ServeHTTP(w, r)
	})
}

func limitConcurrentRequests(next http.Handler, maximum int) http.Handler {
	semaphore := make(chan struct{}, maximum)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Keep liveness available during overload; readiness remains bounded
		// because it may execute dependency checks.
		if r.URL.Path == "/healthz" && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			next.ServeHTTP(w, r)
			return
		}
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }()
			next.ServeHTTP(w, r)
		default:
			w.Header().Set("Retry-After", "1")
			writePublicError(w, http.StatusServiceUnavailable, publicError{
				Class: "unavailable", Code: "server.overloaded",
				Message: "server is temporarily overloaded", Retryable: true,
			})
		}
	})
}

func writePlatformFault(w http.ResponseWriter, err error) bool {
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault == nil {
		return false
	}
	if validationErr := platformFault.Validate(); validationErr != nil {
		writePublicError(w, http.StatusInternalServerError, publicError{
			Class: string(fault.ClassInternal), Code: "server.invalid_fault",
			Message: "internal server error",
		})
		return true
	}
	status := faultHTTPStatus(platformFault.Class)
	writePublicError(w, status, publicError{
		Class: string(platformFault.Class), Code: platformFault.Code,
		Message: platformFault.SafeMessage, Retryable: platformFault.Retryable,
	})
	return true
}

func faultHTTPStatus(class fault.Class) int {
	switch class {
	case fault.ClassValidation:
		return http.StatusBadRequest
	case fault.ClassUnauthenticated:
		return http.StatusUnauthorized
	case fault.ClassForbidden:
		return http.StatusForbidden
	case fault.ClassNotFound:
		return http.StatusNotFound
	case fault.ClassConflict:
		return http.StatusConflict
	case fault.ClassUnavailable:
		return http.StatusServiceUnavailable
	case fault.ClassTimeout:
		return http.StatusGatewayTimeout
	default:
		return http.StatusInternalServerError
	}
}

func writePublicError(w http.ResponseWriter, status int, value publicError) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if value.Code == "server.overloaded" {
		w.Header().Set("Retry-After", "1")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(publicErrorEnvelope{Error: value})
}

