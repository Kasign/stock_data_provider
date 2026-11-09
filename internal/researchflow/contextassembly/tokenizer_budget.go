package contextassembly

import (
	"fmt"
	"strings"
)

// BudgetReservationInput contains the exact non-context inputs that consume
// the model context window, plus configured output and safety reservations.
type BudgetReservationInput struct {
	ModelContextTokens int
	SystemPrompt       string
	ToolSchema         string
	OutputReserve      int
	SafetyMargin       int
}

// MeasureBudgetReservations counts system prompt and tool schema with the
// same target-model tokenizer used for section content. Output reserve and
// safety margin are explicit policy/configuration values, not estimates.
func MeasureBudgetReservations(
	tokenizer Tokenizer,
	expectedModelID string,
	input BudgetReservationInput,
) (BudgetInput, TokenizerProvenance, error) {
	provenance, err := validateTokenizer(tokenizer, expectedModelID)
	if err != nil {
		return BudgetInput{}, TokenizerProvenance{}, err
	}
	if input.ModelContextTokens <= 0 || strings.TrimSpace(input.SystemPrompt) == "" ||
		input.OutputReserve <= 0 || input.SafetyMargin < 0 {
		return BudgetInput{}, TokenizerProvenance{}, ErrInvalidBudget
	}
	systemTokens, err := tokenizer.CountTokens(input.SystemPrompt)
	if err != nil {
		return BudgetInput{}, TokenizerProvenance{}, fmt.Errorf("count system prompt tokens: %w", err)
	}
	if systemTokens <= 0 {
		return BudgetInput{}, TokenizerProvenance{}, fmt.Errorf("%w: system prompt returned non-positive token count", ErrInvalidTokenEstimate)
	}
	var toolTokens int
	if strings.TrimSpace(input.ToolSchema) != "" {
		toolTokens, err = tokenizer.CountTokens(input.ToolSchema)
		if err != nil {
			return BudgetInput{}, TokenizerProvenance{}, fmt.Errorf("count tool schema tokens: %w", err)
		}
		if toolTokens <= 0 {
			return BudgetInput{}, TokenizerProvenance{}, fmt.Errorf("%w: tool schema returned non-positive token count", ErrInvalidTokenEstimate)
		}
	}
	budget := BudgetInput{
		ModelContextTokens: input.ModelContextTokens,
		SystemTokens: systemTokens,
		ToolSchemaTokens: toolTokens,
		OutputReserve: input.OutputReserve,
		SafetyMargin: input.SafetyMargin,
	}
	if _, err := AvailableBudget(budget); err != nil {
		return BudgetInput{}, TokenizerProvenance{}, err
	}
	return budget, provenance, nil
}
