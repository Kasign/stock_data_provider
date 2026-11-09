package application

import (
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

func TestMapAuthorizationErrorPreservesFailureSemantics(t *testing.T) {
	tests := []struct {
		name string
		kind AuthorizationFailureKind
		class fault.Class
		code string
	}{
		{name: "明确拒绝", kind: AuthorizationDenied, class: fault.ClassForbidden, code: "evidence_authorization_rejected"},
		{name: "依赖不可用", kind: AuthorizationUnavailable, class: fault.ClassUnavailable, code: "evidence_authorization_unavailable"},
		{name: "检查超时", kind: AuthorizationTimeout, class: fault.ClassTimeout, code: "evidence_authorization_timeout"},
		{name: "适配器内部故障", kind: AuthorizationInternal, class: fault.ClassInternal, code: "evidence_authorization_failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cause := errors.New("sensitive owner adapter detail")
			got := mapAuthorizationError(NewAuthorizationError(tt.kind, cause))
			var gotFault *fault.Fault
			if !errors.As(got, &gotFault) {
				t.Fatalf("expected Platform Fault, got %T", got)
			}
			if gotFault.Class != tt.class || gotFault.Code != tt.code {
				t.Fatalf("unexpected mapping: class=%q code=%q", gotFault.Class, gotFault.Code)
			}
			if !errors.Is(got, cause) {
				t.Fatal("authorization cause was not preserved")
			}
			if got.Error() == "" || got.Error() == cause.Error() {
				t.Fatal("public error message must not expose adapter detail")
			}
		})
	}
}

func TestMapAuthorizationErrorTreatsUntypedFailureAsInternal(t *testing.T) {
	cause := errors.New("unclassified owner failure")
	got := mapAuthorizationError(cause)
	var gotFault *fault.Fault
	if !errors.As(got, &gotFault) {
		t.Fatalf("expected Platform Fault, got %T", got)
	}
	if gotFault.Class != fault.ClassInternal || gotFault.Code != "evidence_authorization_failed" {
		t.Fatalf("untyped failure must fail closed as internal: %+v", gotFault)
	}
	if !errors.Is(got, cause) {
		t.Fatal("untyped authorization cause was not preserved")
	}
}
