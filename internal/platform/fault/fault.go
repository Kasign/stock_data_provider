package fault

import "fmt"

// Class 标识平台或领域错误的稳定类别。
type Class string

const (
	ClassValidation       Class = "validation"
	ClassUnauthenticated  Class = "unauthenticated"
	ClassForbidden        Class = "forbidden"
	ClassNotFound         Class = "not_found"
	ClassConflict         Class = "conflict"
	ClassUnavailable      Class = "unavailable"
	ClassTimeout          Class = "timeout"
	ClassInternal         Class = "internal"
)

// Fault 是可安全序列化的错误结构；Cause 仅用于内部错误链，不输出到 JSON。
type Fault struct {
	Class       Class  `json:"class"`
	Code        string `json:"code"`
	SafeMessage string `json:"safe_message"`
	Dependency  string `json:"dependency,omitempty"`
	Retryable   bool   `json:"retryable"`
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
