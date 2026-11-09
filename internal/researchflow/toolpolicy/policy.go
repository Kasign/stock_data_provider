package toolpolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidPolicy      = errors.New("invalid tool policy")
	ErrToolNotAllowed     = errors.New("tool is not allowed")
	ErrScopeDenied        = errors.New("tool scope is not granted")
	ErrCallBudgetExceeded = errors.New("tool call budget exceeded")
	ErrInvalidToolIntent  = errors.New("invalid tool call intent")
)

// SchemaValidator 必须使用本地、有限制的 JSON Schema Draft 2020-12 校验器；
// Tool Policy 不加载远程 $ref，也不自行执行工具。
type SchemaValidator interface {
	Validate(schemaJSON, valueJSON []byte) error
}

type Definition struct {
	Name           string
	Description    string
	ParametersJSON json.RawMessage
	Strict         bool
	RequiredScopes []string
	Timeout        time.Duration
	MaxCallsPerRun int
}

type Registry struct {
	definitions map[string]Definition
	validator   SchemaValidator
}

func NewRegistry(definitions []Definition, validator SchemaValidator) (*Registry, error) {
	if validator == nil {
		return nil, fmt.Errorf("%w: schema validator is required", ErrInvalidPolicy)
	}
	if len(definitions) == 0 {
		return nil, fmt.Errorf("%w: registry must not be empty", ErrInvalidPolicy)
	}
	registry := &Registry{definitions: make(map[string]Definition, len(definitions)), validator: validator}
	for _, definition := range definitions {
		definition.Name = strings.TrimSpace(definition.Name)
		if definition.Name == "" || len(definition.Name) > 128 || definition.Timeout <= 0 || definition.MaxCallsPerRun <= 0 ||
			len(definition.ParametersJSON) == 0 || !json.Valid(definition.ParametersJSON) {
			return nil, fmt.Errorf("%w: tool identity, schema, timeout and call limit are required", ErrInvalidPolicy)
		}
		if _, exists := registry.definitions[definition.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate tool %q", ErrInvalidPolicy, definition.Name)
		}
		scopes := append([]string(nil), definition.RequiredScopes...)
		sort.Strings(scopes)
		for i, scope := range scopes {
			if strings.TrimSpace(scope) == "" || (i > 0 && scopes[i-1] == scope) {
				return nil, fmt.Errorf("%w: invalid or duplicate scope for tool %q", ErrInvalidPolicy, definition.Name)
			}
		}
		definition.RequiredScopes = scopes
		definition.ParametersJSON = append(json.RawMessage(nil), definition.ParametersJSON...)
		registry.definitions[definition.Name] = definition
	}
	return registry, nil
}

type Intent struct {
	CallID        string
	Name          string
	ArgumentsJSON string
}

type Budget struct {
	CallsUsed       int
	CallsByName     map[string]int
	TimeoutUsed     time.Duration
	MaxCalls        int
	MaxTotalTimeout time.Duration
	RunDeadline     time.Time
	Now             time.Time
	IdempotencyKey  string
	GrantedScopes   []string
}

type AuthorizedCall struct {
	CallID         string
	Name           string
	ArgumentsJSON  json.RawMessage
	IdempotencyKey string
	Timeout        time.Duration
	Deadline       time.Time
}

// Authorize 只将模型输出的结构化意图转换为可执行前的授权记录。
// 它不执行工具；实际执行器必须再次绑定 Run/Stage/Attempt 身份并做幂等控制。
func (r *Registry) Authorize(intent Intent, budget Budget) (AuthorizedCall, error) {
	if r == nil || r.validator == nil {
		return AuthorizedCall{}, ErrInvalidPolicy
	}
	intent.CallID = strings.TrimSpace(intent.CallID)
	intent.Name = strings.TrimSpace(intent.Name)
	budget.IdempotencyKey = strings.TrimSpace(budget.IdempotencyKey)
	if intent.CallID == "" || len(intent.CallID) > 256 || intent.Name == "" ||
		budget.IdempotencyKey == "" || len(budget.IdempotencyKey) > 256 ||
		budget.Now.IsZero() || budget.RunDeadline.IsZero() || !budget.RunDeadline.After(budget.Now) {
		return AuthorizedCall{}, ErrInvalidToolIntent
	}
	definition, ok := r.definitions[intent.Name]
	if !ok {
		return AuthorizedCall{}, fmt.Errorf("%w: %s", ErrToolNotAllowed, intent.Name)
	}
	granted := make(map[string]struct{}, len(budget.GrantedScopes))
	for _, scope := range budget.GrantedScopes {
		granted[scope] = struct{}{}
	}
	for _, required := range definition.RequiredScopes {
		if _, ok := granted[required]; !ok {
			return AuthorizedCall{}, fmt.Errorf("%w: %s", ErrScopeDenied, required)
		}
	}
	if budget.MaxCalls <= 0 || budget.CallsUsed < 0 || budget.CallsUsed >= budget.MaxCalls ||
		budget.CallsByName[intent.Name] < 0 || budget.CallsByName[intent.Name] >= definition.MaxCallsPerRun || budget.MaxTotalTimeout <= 0 ||
		budget.TimeoutUsed < 0 || budget.TimeoutUsed >= budget.MaxTotalTimeout {
		return AuthorizedCall{}, ErrCallBudgetExceeded
	}
	if len(intent.ArgumentsJSON) == 0 || !json.Valid([]byte(intent.ArgumentsJSON)) {
		return AuthorizedCall{}, ErrInvalidToolIntent
	}
	if err := r.validator.Validate(definition.ParametersJSON, []byte(intent.ArgumentsJSON)); err != nil {
		return AuthorizedCall{}, fmt.Errorf("%w: arguments do not match registered schema", ErrInvalidToolIntent)
	}
	remaining := budget.MaxTotalTimeout - budget.TimeoutUsed
	timeout := definition.Timeout
	if timeout > remaining {
		timeout = remaining
	}
	deadline := budget.Now.Add(timeout)
	if deadline.After(budget.RunDeadline) {
		deadline = budget.RunDeadline
	}
	timeout = deadline.Sub(budget.Now)
	if timeout <= 0 {
		return AuthorizedCall{}, ErrCallBudgetExceeded
	}
	return AuthorizedCall{
		CallID: intent.CallID, Name: definition.Name,
		ArgumentsJSON: append(json.RawMessage(nil), intent.ArgumentsJSON...),
		IdempotencyKey: budget.IdempotencyKey, Timeout: timeout, Deadline: deadline,
	}, nil
}
