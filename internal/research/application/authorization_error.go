package application

import (
	"errors"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

// AuthorizationFailureKind 表示来源 Owner 授权服务失败的稳定语义。
type AuthorizationFailureKind string

const (
	// AuthorizationDenied 表示 Owner 已明确判定当前调用无权访问。
	AuthorizationDenied AuthorizationFailureKind = "denied"
	// AuthorizationUnavailable 表示 Owner 授权依赖暂时不可用。
	AuthorizationUnavailable AuthorizationFailureKind = "unavailable"
	// AuthorizationTimeout 表示 Owner 授权检查超时。
	AuthorizationTimeout AuthorizationFailureKind = "timeout"
	// AuthorizationInternal 表示 Owner 授权适配器发生内部故障或返回无法识别的失败。
	AuthorizationInternal AuthorizationFailureKind = "internal"
)

// AuthorizationError 由来源 Owner Adapter 返回，避免把依赖故障误报为权限拒绝。
// Cause 仅保留在进程内部，不应直接作为对外消息。
type AuthorizationError struct {
	Kind  AuthorizationFailureKind
	Cause error
}

func (e *AuthorizationError) Error() string {
	if e == nil {
		return ""
	}
	if e.Cause != nil {
		return "Research Evidence authorization failed: " + string(e.Kind)
	}
	return "Research Evidence authorization failed: " + string(e.Kind)
}

func (e *AuthorizationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// NewAuthorizationError 构造来源 Owner Adapter 可返回的类型化授权错误。
func NewAuthorizationError(kind AuthorizationFailureKind, cause error) error {
	switch kind {
	case AuthorizationDenied, AuthorizationUnavailable, AuthorizationTimeout, AuthorizationInternal:
		return &AuthorizationError{Kind: kind, Cause: cause}
	default:
		return &AuthorizationError{Kind: AuthorizationInternal, Cause: cause}
	}
}

func mapAuthorizationError(err error) error {
	var authorizationErr *AuthorizationError
	if !errors.As(err, &authorizationErr) || authorizationErr == nil {
		return fault.New(fault.ClassInternal, "evidence_authorization_failed", "Evidence authorization check failed", err)
	}

	switch authorizationErr.Kind {
	case AuthorizationDenied:
		return fault.New(fault.ClassForbidden, "evidence_authorization_rejected", "Evidence is not authorized for this Research operation", err)
	case AuthorizationUnavailable:
		return fault.New(fault.ClassUnavailable, "evidence_authorization_unavailable", "Evidence authorization service is unavailable", err)
	case AuthorizationTimeout:
		return fault.New(fault.ClassTimeout, "evidence_authorization_timeout", "Evidence authorization check timed out", err)
	case AuthorizationInternal:
		return fault.New(fault.ClassInternal, "evidence_authorization_failed", "Evidence authorization check failed", err)
	default:
		return fault.New(fault.ClassInternal, "evidence_authorization_failed", "Evidence authorization check failed", err)
	}
}
