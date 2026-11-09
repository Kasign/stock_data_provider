package contextassembly

import (
	"errors"
	"testing"
)

func TestAvailableBudgetSubtractsAllReservations(t *testing.T) {
	got, err := AvailableBudget(BudgetInput{
		ModelContextTokens: 32768,
		SystemTokens:       2048,
		ToolSchemaTokens:   1024,
		OutputReserve:      4096,
		SafetyMargin:       512,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 25088 {
		t.Fatalf("available budget = %d, want 25088", got)
	}
}

func TestAvailableBudgetRejectsInvalidOrExhaustedWindow(t *testing.T) {
	tests := []BudgetInput{
		{ModelContextTokens: 0},
		{ModelContextTokens: 100, SystemTokens: -1},
		{ModelContextTokens: 100, SystemTokens: 101},
		{ModelContextTokens: 100, SystemTokens: 100},
	}
	for _, input := range tests {
		if _, err := AvailableBudget(input); !errors.Is(err, ErrInvalidBudget) {
			t.Fatalf("input %+v error = %v", input, err)
		}
	}
}
