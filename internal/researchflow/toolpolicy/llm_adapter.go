package toolpolicy

import (
	"encoding/json"
	"errors"
	"strings"

	"example.com/stock-analysis-flow/internal/llm"
)

var ErrInvalidModelToolCall = errors.New("invalid model tool call")

type llmSchemaValidator struct{}

func (llmSchemaValidator) ValidateSchema(schemaJSON []byte) error {
	_, err := llm.CompileJSONSchema(schemaJSON)
	return err
}

func (llmSchemaValidator) Validate(schemaJSON, valueJSON []byte) error {
	return llm.ValidateJSONAgainstSchema(schemaJSON, valueJSON)
}

// NewLLMRegistry 使用 LLM 模块已实现的本地 Draft 2020-12 校验器。
func NewLLMRegistry(definitions []Definition) (*Registry, error) {
	return NewRegistry(definitions, llmSchemaValidator{})
}

// IntentFromModelToolCall 将 LLM Provider 返回的工具调用转换为待授权意图。
// 这里只做结构和状态检查，不代表工具已获准，也绝不执行工具。
func IntentFromModelToolCall(call llm.ModelToolCall) (Intent, error) {
	callID := strings.TrimSpace(call.CallID)
	name := strings.TrimSpace(call.Name)
	if callID == "" || len(callID) > 256 || name == "" || len(name) > 128 {
		return Intent{}, ErrInvalidModelToolCall
	}
	if call.Status != "" && call.Status != "completed" {
		return Intent{}, ErrInvalidModelToolCall
	}
	if strings.TrimSpace(call.ArgumentsJSON) == "" || len(call.ArgumentsJSON) > llm.MaxPayloadBytes ||
		!json.Valid([]byte(call.ArgumentsJSON)) {
		return Intent{}, ErrInvalidModelToolCall
	}
	return Intent{
		CallID: callID, Name: name, ArgumentsJSON: call.ArgumentsJSON,
	}, nil
}

// IntentsFromModelResponse 只接收已完成的模型响应；每个 Tool Call 仍须单独经过 Registry.Authorize。
func IntentsFromModelResponse(response llm.ModelResponse) ([]Intent, error) {
	if response.Status != llm.ResponseCompleted {
		return nil, ErrInvalidModelToolCall
	}
	if len(response.ToolCalls) == 0 {
		return nil, nil
	}
	intents := make([]Intent, 0, len(response.ToolCalls))
	seen := make(map[string]struct{}, len(response.ToolCalls))
	for _, call := range response.ToolCalls {
		intent, err := IntentFromModelToolCall(call)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[intent.CallID]; exists {
			return nil, ErrInvalidModelToolCall
		}
		seen[intent.CallID] = struct{}{}
		intents = append(intents, intent)
	}
	return intents, nil
}

// ToolDefinitionsFor 只向模型暴露当前 Scope 和预算允许的工具。
// 返回值用于 ModelRequest.Tools；工具调用响应仍必须经过 Authorize。
func (r *Registry) ToolDefinitionsFor(budget Budget) []llm.ToolDefinition {
	if r == nil || budget.MaxCalls <= budget.CallsUsed || budget.MaxCalls <= 0 ||
		budget.MaxTotalTimeout <= budget.TimeoutUsed || budget.MaxTotalTimeout <= 0 ||
		budget.Now.IsZero() || !budget.RunDeadline.After(budget.Now) {
		return nil
	}
	granted := make(map[string]struct{}, len(budget.GrantedScopes))
	for _, scope := range budget.GrantedScopes {
		granted[scope] = struct{}{}
	}
	names := make([]string, 0, len(r.definitions))
	for name, definition := range r.definitions {
		if budget.CallsByName[name] < 0 || budget.CallsByName[name] >= definition.MaxCallsPerRun {
			continue
		}
		allowed := true
		for _, required := range definition.RequiredScopes {
			if _, exists := granted[required]; !exists {
				allowed = false
				break
			}
		}
		if allowed {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	result := make([]llm.ToolDefinition, 0, len(names))
	for _, name := range names {
		definition := r.definitions[name]
		result = append(result, llm.ToolDefinition{
			Name: definition.Name, Description: definition.Description,
			ParametersJSON: append(json.RawMessage(nil), definition.ParametersJSON...),
			Strict:         definition.Strict,
		})
	}
	return result
}
