package signal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const schemaFileName = "QNT-CTR-006-signal-1.0.0.schema.json"

func compileSignalSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "quant", "schema", schemaFileName)
	compiled, err := jsonschema.Compile(path)
	if err != nil {
		t.Fatalf("compile %s: %v", schemaFileName, err)
	}
	return compiled
}

func readSignalGolden(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "quant", "golden", "signal", "signal-1.0.0.golden.json")
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v", path, err)
	}
	return document
}

func validSignal() Signal {
	return Signal{
		SignalID:  "signal-quant-trend-20261126-0001",
		Symbol:    "600000.SH",
		Direction: DirectionLong,
		Status:    StatusCandidate,
		StrategyRef: StrategyRef{
			ID:      "quant.trend.close_only",
			Version: "1.0.0",
		},
		SystemAsOf:             "2026-11-26T07:05:00Z",
		TradeDate:              "2026-11-26",
		EvidenceRefs:           []string{"quant_result:trend:sha256:0a1b2c3d"},
		StrategyRuleSetVersion: "quant-trend-close-only/1.0.0",
	}
}

func TestSignalGoldenMatchesSchemaAndDTO(t *testing.T) {
	document := readSignalGolden(t)
	schema := compileSignalSchema(t)

	var instance any
	if err := json.Unmarshal(document, &instance); err != nil {
		t.Fatalf("decode golden: %v", err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("golden violates %s: %v", schemaFileName, err)
	}

	var decoded Signal
	if err := json.Unmarshal(document, &decoded); err != nil {
		t.Fatalf("decode golden into DTO: %v", err)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("golden fails DTO validation: %v", err)
	}
	reencoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("re-encode golden: %v", err)
	}
	if err := schema.Validate(decodeAny(t, reencoded)); err != nil {
		t.Fatalf("DTO round-trip violates schema: %v", err)
	}
}

func TestSignalDTOSerializationMatchesSchema(t *testing.T) {
	encoded, err := json.Marshal(validSignal())
	if err != nil {
		t.Fatalf("marshal DTO: %v", err)
	}
	if err := compileSignalSchema(t).Validate(decodeAny(t, encoded)); err != nil {
		t.Fatalf("serialized DTO violates %s: %v", schemaFileName, err)
	}
}

// 结构违规必须同时被 Schema 与 DTO 拒绝，避免两套判定漂移；
// Schema 独有的约束（额外字段）与 Go 独有的约束（真实日历日期）分别单独覆盖。
func TestSignalValidateMatchesSchemaOnStructuralViolations(t *testing.T) {
	schema := compileSignalSchema(t)
	mutations := map[string]func(*Signal){
		"blank signal_id":          func(s *Signal) { s.SignalID = " " },
		"padding in symbol":        func(s *Signal) { s.Symbol = " 600000.SH " },
		"unknown direction":        func(s *Signal) { s.Direction = "short" },
		"unknown status":           func(s *Signal) { s.Status = "pending" },
		"missing strategy id":      func(s *Signal) { s.StrategyRef.ID = "" },
		"missing strategy version": func(s *Signal) { s.StrategyRef.Version = "" },
		"offset timestamp":         func(s *Signal) { s.SystemAsOf = "2026-11-26T15:05:00+08:00" },
		"trailing zero fraction":   func(s *Signal) { s.SystemAsOf = "2026-11-26T07:05:00.120Z" },
		"date-time in trade_date":  func(s *Signal) { s.TradeDate = "2026-11-26T00:00:00Z" },
		"empty evidence":           func(s *Signal) { s.EvidenceRefs = []string{} },
		"nil evidence":             func(s *Signal) { s.EvidenceRefs = nil },
		"duplicate evidence":       func(s *Signal) { s.EvidenceRefs = []string{"ref", "ref"} },
		"blank evidence":           func(s *Signal) { s.EvidenceRefs = []string{""} },
		"missing rule set version": func(s *Signal) { s.StrategyRuleSetVersion = "" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			candidate := validSignal()
			mutate(&candidate)
			encoded, err := json.Marshal(candidate)
			if err != nil {
				t.Fatalf("marshal mutated DTO: %v", err)
			}
			if err := schema.Validate(decodeAny(t, encoded)); err == nil {
				t.Fatal("Schema accepted structurally invalid signal")
			}
			if err := candidate.Validate(); err == nil {
				t.Fatal("DTO accepted structurally invalid signal")
			}
		})
	}
}

func TestSignalSchemaRejectsUnknownFields(t *testing.T) {
	schema := compileSignalSchema(t)
	instance := decodeAny(t, readSignalGolden(t)).(map[string]any)
	instance["reference_price"] = "12.34"
	if err := schema.Validate(instance); err == nil {
		t.Fatal("Schema accepted undeclared field reference_price")
	}
}

// Schema 只校验 trade_date 形状，真实日历日期由 Go 校验；
// 这不是漂移，而是 Schema 无法表达日历合法性。
func TestSignalValidateRejectsImpossibleTradeDate(t *testing.T) {
	candidate := validSignal()
	candidate.TradeDate = "2026-02-30"
	if err := candidate.Validate(); err == nil {
		t.Fatal("DTO accepted impossible trade date")
	}
}

func TestNewSignalCopiesEvidenceRefs(t *testing.T) {
	source := validSignal()
	constructed, err := NewSignal(source)
	if err != nil {
		t.Fatalf("NewSignal() error = %v", err)
	}
	source.EvidenceRefs[0] = "mutated"
	if constructed.EvidenceRefs[0] != "quant_result:trend:sha256:0a1b2c3d" {
		t.Fatalf("evidence_refs aliased caller slice: %v", constructed.EvidenceRefs)
	}
}

func TestSignalContractVersionMatchesSchemaFile(t *testing.T) {
	if ContractVersion != "1.0.0" {
		t.Fatalf("ContractVersion = %q, want 1.0.0", ContractVersion)
	}
	if _, err := os.Stat(filepath.Join("..", "..", "..", "doc", "quant", "schema",
		"QNT-CTR-006-signal-"+ContractVersion+".schema.json")); err != nil {
		t.Fatalf("schema file for contract version missing: %v", err)
	}
}

func decodeAny(t *testing.T, document []byte) any {
	t.Helper()
	var instance any
	if err := json.Unmarshal(document, &instance); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	return instance
}
