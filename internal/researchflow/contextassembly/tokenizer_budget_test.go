package contextassembly

import (
	"errors"
	"strings"
	"testing"
)

func TestMeasureBudgetReservationsCountsSystemAndToolSchemaWithTargetTokenizer(t *testing.T) {
	tokenizer := testTokenizer{
		modelID: "model-a", version: "v7",
		count: func(value string) (int, error) {
			switch value {
			case "system prompt":
				return 120, nil
			case "tool schema":
				return 80, nil
			default:
				return 0, errors.New("unexpected tokenizer input")
			}
		},
	}
	budget, provenance, err := MeasureBudgetReservations(tokenizer, "model-a", BudgetReservationInput{
		ModelContextTokens: 4096, SystemPrompt: "system prompt",
		ToolSchema: "tool schema", OutputReserve: 1024, SafetyMargin: 128,
	})
	if err != nil {
		t.Fatal(err)
	}
	if budget.SystemTokens != 120 || budget.ToolSchemaTokens != 80 ||
		budget.OutputReserve != 1024 || budget.SafetyMargin != 128 {
		t.Fatalf("unexpected budget reservations: %+v", budget)
	}
	if provenance.ModelID != "model-a" || provenance.Version != "v7" {
		t.Fatalf("unexpected tokenizer provenance: %+v", provenance)
	}
}

func TestMeasureBudgetReservationsAllowsNoToolSchema(t *testing.T) {
	tokenizer := testTokenizer{
		modelID: "model-a", version: "v1",
		count: func(value string) (int, error) {
			if strings.TrimSpace(value) == "" {
				t.Fatal("empty input should not be sent to tokenizer")
			}
			return 100, nil
		},
	}
	budget, _, err := MeasureBudgetReservations(tokenizer, "model-a", BudgetReservationInput{
		ModelContextTokens: 2048, SystemPrompt: "system prompt",
		OutputReserve: 512, SafetyMargin: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if budget.ToolSchemaTokens != 0 {
		t.Fatalf("empty tool schema should reserve zero tokens, got %d", budget.ToolSchemaTokens)
	}
}

func TestMeasureBudgetReservationsRejectsInvalidWindowAndExhaustedBudget(t *testing.T) {
	tokenizer := testTokenizer{modelID: "model-a", version: "v1", count: func(string) (int, error) { return 100, nil }}
	for _, input := range []BudgetReservationInput{
		{SystemPrompt: "system", ModelContextTokens: 100, OutputReserve: 1},
		{SystemPrompt: "system", ModelContextTokens: 1000, OutputReserve: 0},
		{SystemPrompt: "system", ModelContextTokens: 1000, OutputReserve: 1001},
		{SystemPrompt: " ", ModelContextTokens: 1000, OutputReserve: 100},
	} {
		if _, _, err := MeasureBudgetReservations(tokenizer, "model-a", input); !errors.Is(err, ErrInvalidBudget) {
			t.Fatalf("invalid budget %+v should be rejected, got %v", input, err)
		}
	}
}
