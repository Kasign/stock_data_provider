package fault

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"unicode"
)

// Class 标识平台或领域错误的稳定类别。
type Class string

const (
	ClassValidation      Class = "validation"
	ClassUnauthenticated Class = "unauthenticated"
	ClassForbidden       Class = "forbidden"
	ClassNotFound        Class = "not_found"
	ClassConflict        Class = "conflict"
	ClassUnavailable     Class = "unavailable"
	ClassTimeout         Class = "timeout"
	ClassInternal        Class = "internal"
)

// Valid reports whether c belongs to the stable public Fault class set.
func (c Class) Valid() bool {
	switch c {
	case ClassValidation, ClassUnauthenticated, ClassForbidden, ClassNotFound,
		ClassConflict, ClassUnavailable, ClassTimeout, ClassInternal:
		return true
	default:
		return false
	}
}

// Fault 是仅供进程内部使用的错误结构。所有字段均禁止 JSON 序列化；
// HTTP transport 必须显式映射到独立公开 DTO。
type Fault struct {
	Class       Class  `json:"-"`
	Code        string `json:"-"`
	SafeMessage string `json:"-"`
	Dependency  string `json:"-"`
	Retryable   bool   `json:"-"`
	Cause       error  `json:"-"`
}

func (f *Fault) Error() string {
	if f == nil {
		return ""
	}
	if f.SafeMessage != "" {
		return f.SafeMessage
	}
	if f.Code != "" {
		return f.Code
	}
	return fmt.Sprintf("platform fault (%s)", f.Class)
}

func (f *Fault) Unwrap() error {
	if f == nil {
		return nil
	}
	return f.Cause
}

// New 创建包含稳定错误码和安全摘要的 Fault。
func New(class Class, code, safeMessage string, cause error) *Fault {
	return &Fault{
		Class:       class,
		Code:        code,
		SafeMessage: safeMessage,
		Cause:       cause,
	}
}


// Validate checks the stable public metadata invariants of a Fault. Cause is
// intentionally excluded because it is an internal error chain.
func (f *Fault) Validate() error {
	if f == nil {
		return errors.New("fault is required")
	}
	if !f.Class.Valid() {
		return errors.New("fault class is invalid")
	}
	if len(f.Code) > 160 || !validIdentifier(f.Code) {
		return errors.New("fault code must be a stable lowercase identifier")
	}
	if strings.TrimSpace(f.SafeMessage) == "" || len(f.SafeMessage) > 512 ||
		strings.IndexFunc(f.SafeMessage, unicode.IsControl) >= 0 {
		return errors.New("fault safe message is invalid")
	}
	if f.Dependency != "" && (len(f.Dependency) > 128 ||
		!validIdentifier(f.Dependency) || net.ParseIP(f.Dependency) != nil) {
		return errors.New("fault dependency must be a logical identifier")
	}
	if f.Retryable && f.Class != ClassUnavailable && f.Class != ClassTimeout {
		return errors.New("only unavailable and timeout faults may be retryable")
	}
	return nil
}

func validIdentifier(value string) bool {
	if value == "" {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') ||
			(index > 0 && (char == '.' || char == '_' || char == '-')) {
			continue
		}
		return false
	}
	return true
}
