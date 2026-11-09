package toolpolicy

import (
	"errors"\n\t"encoding/json"
	"testing"
	"time"
)

type schemaValidatorFunc func(schemaJSON, valueJSON []byte) error

func (f schemaValidatorFunc) ValidateSchema(schemaJSON []byte) error {
	if !json.Valid(schemaJSON) {
		return errors.New("invalid schema")
	}
	return nil
}

func (f schemaValidatorFunc) Validate(schemaJSON, valueJSON []byte) error {
	return f(schemaJSON, valueJSON)
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry([]Definition{{
		Name: "research.lookup", ParametersJSON: []byte(`{"type":"object","required":["symbol"]}`),
		RequiredScopes: []string{"research:read"}, Timeout: 5 * time.Second, MaxCallsPerRun: 2,
	}}, schemaValidatorFunc(func(_, value []byte) error {
		if string(value) != `{"symbol":"600000.SH"}` {
			return errors.New("schema mismatch")
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func validBudget(now time.Time) Budget {
	return Budget{
		MaxCalls: 3, MaxTotalTimeout: 10 * time.Second, RunDeadline: now.Add(time.Minute),
		Now: now, IdempotencyKey: "run-1/stage-1/call-1", GrantedScopes: []string{"research:read"},
	}
}

func TestAuthorizeReturnsBoundedCallWithoutExecuting(t *testing.T) {
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	budget := validBudget(now)
	call, err := testRegistry(t).Authorize(Intent{
		CallID: "call-1", Name: "research.lookup", ArgumentsJSON: `{"symbol":"600000.SH"}`,
	}, budget)
	if err != nil {
		t.Fatal(err)
	}
	if call.Timeout != 5*time.Second || !call.Deadline.Equal(now.Add(5*time.Second)) {
		t.Fatalf("unexpected bounded timeout/deadline: %+v", call)
	}
	if call.IdempotencyKey != budget.IdempotencyKey || string(call.ArgumentsJSON) != `{"symbol":"600000.SH"}` {
		t.Fatalf("authorized call lost intent metadata: %+v", call)
	}
}

func TestAuthorizeRejectsUnknownToolAndMissingScope(t *testing.T) {
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	registry := testRegistry(t)
	intent := Intent{CallID: "call-1", Name: "shell.exec", ArgumentsJSON: `{}`}
	if _, err := registry.Authorize(intent, validBudget(now)); !errors.Is(err, ErrToolNotAllowed) {
		t.Fatalf("unknown tool error = %v", err)
	}
	intent.Name = "research.lookup"
	budget := validBudget(now)
	budget.GrantedScopes = nil
	if _, err := registry.Authorize(intent, budget); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("missing scope error = %v", err)
	}
}

func TestAuthorizeRejectsSchemaMismatchAndExhaustedBudget(t *testing.T) {
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	registry := testRegistry(t)
	intent := Intent{CallID: "call-1", Name: "research.lookup", ArgumentsJSON: `{"symbol":"000001.SZ"}`}
	if _, err := registry.Authorize(intent, validBudget(now)); !errors.Is(err, ErrInvalidToolIntent) {
		t.Fatalf("schema mismatch error = %v", err)
	}
	intent.ArgumentsJSON = `{"symbol":"600000.SH"}`
	budget := validBudget(now)
	budget.CallsUsed = budget.MaxCalls
	if _, err := registry.Authorize(intent, budget); !errors.Is(err, ErrCallBudgetExceeded) {
		t.Fatalf("exhausted budget error = %v", err)
	}
}

func TestAuthorizeClampsTimeoutToRunDeadlineAndRemainingBudget(t *testing.T) {
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	budget := validBudget(now)
	budget.MaxTotalTimeout = 3 * time.Second
	budget.RunDeadline = now.Add(2 * time.Second)
	call, err := testRegistry(t).Authorize(Intent{
		CallID: "call-1", Name: "research.lookup", ArgumentsJSON: `{"symbol":"600000.SH"}`,
	}, budget)
	if err != nil {
		t.Fatal(err)
	}
	if call.Timeout != 2*time.Second || !call.Deadline.Equal(budget.RunDeadline) {
		t.Fatalf("timeout was not clamped: %+v", call)
	}
}

func TestAuthorizeEnforcesPerToolCallLimit(t *testing.T) {
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	budget := validBudget(now)
	budget.CallsUsed = 1
	budget.CallsByName = map[string]int{"research.lookup": 2}
	intent := Intent{CallID: "call-3", Name: "research.lookup", ArgumentsJSON: `{"symbol":"600000.SH"}`}
	if _, err := testRegistry(t).Authorize(intent, budget); !errors.Is(err, ErrCallBudgetExceeded) {
		t.Fatalf("per-tool call limit error = %v", err)
	}
}

func TestToolDefinitionsForOnlyAdvertisesScopedToolsWithinBudget(t *testing.T) {
	registry := testRegistry(t)
	now := time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC)
	budget := validBudget(now)
	definitions := registry.ToolDefinitionsFor(budget)
	if len(definitions) != 1 || definitions[0].Name != "research.lookup" {
		t.Fatalf("unexpected advertised tools: %+v", definitions)
	}
	budget.GrantedScopes = nil
	if got := registry.ToolDefinitionsFor(budget); len(got) != 0 {
		t.Fatalf("tool without scope was advertised: %+v", got)
	}
	budget = validBudget(now)
	budget.CallsUsed = budget.MaxCalls
	if got := registry.ToolDefinitionsFor(budget); len(got) != 0 {
		t.Fatalf("tool advertised after global budget exhausted: %+v", got)
	}
}
