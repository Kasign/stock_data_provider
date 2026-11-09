package contextassembly

import (
	"errors"
	"fmt"
)

var ErrInvalidBudget = errors.New("invalid model context budget")

// BudgetInput describes the token reservations that must be deducted from a
// target model's context window before section planning starts.
type BudgetInput struct {
	ModelContextTokens int
	SystemTokens      int
	ToolSchemaTokens  int
	OutputReserve     int
	SafetyMargin      int
}

// AvailableBudget returns the remaining input-token budget for context
// sections. All estimates must come from the selected model/tokenizer adapter;
// this function only performs bounded accounting.
func AvailableBudget(input BudgetInput) (int, error) {
	if input.ModelContextTokens <= 0 || input.SystemTokens < 0 ||
		input.ToolSchemaTokens < 0 || input.OutputReserve < 0 || input.SafetyMargin < 0 {
		return 0, ErrInvalidBudget
	}
	remaining := input.ModelContextTokens
	for _, reserved := range []int{
		input.SystemTokens, input.ToolSchemaTokens, input.OutputReserve, input.SafetyMargin,
	} {
		if reserved > remaining {
			return 0, fmt.Errorf("%w: reservations exceed model context window", ErrInvalidBudget)
		}
		remaining -= reserved
	}
	if remaining == 0 {
		return 0, fmt.Errorf("%w: no tokens remain for context sections", ErrInvalidBudget)
	}
	return remaining, nil
}
