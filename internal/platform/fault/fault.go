package fault

import "fmt"

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
