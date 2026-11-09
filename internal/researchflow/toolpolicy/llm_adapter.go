package toolpolicy

import (
	"encoding/json"
	"errors"
	"strings"

	"example.com/stock-analysis-flow/internal/llm"
)

var ErrInvalidModelToolCall = errors.New("invalid model tool call")

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
	if strings.TrimSpace(call.ArgumentsJSON) == "" || !json.Valid([]byte(call.ArgumentsJSON)) {
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
