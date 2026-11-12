package diagnostics

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	"example.com/stock-analysis-flow/internal/platform/fault"
)

func TestDiagnosticsHTTPReportSchemaAndGolden(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "..", "doc", "server", "schema", "diagnostics-report-1.0.0.schema.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read diagnostics schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("diagnostics-report-1.0.0.schema.json", bytes.NewReader(schemaBytes)); err != nil {
		t.Fatalf("add diagnostics schema: %v", err)
	}
	schema, err := compiler.Compile("diagnostics-report-1.0.0.schema.json")
	if err != nil {
		t.Fatalf("compile diagnostics schema: %v", err)
	}

	goldenPath := filepath.Join("..", "..", "..", "doc", "server", "golden", "diagnostics-report-healthy-1.0.0.golden.json")
	goldenBytes, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read diagnostics golden: %v", err)
	}
	var golden any
	if err := json.Unmarshal(goldenBytes, &golden); err != nil {
		t.Fatalf("decode diagnostics golden: %v", err)
	}
	if err := schema.Validate(golden); err != nil {
		t.Fatalf("diagnostics golden violates Schema: %v", err)
	}

	report := Report{
		SchemaVersion: ReportSchemaVersion,
		Diagnostics: platformdiagnostics.Report{
			Summary: platformdiagnostics.Summary{
				Status: platformdiagnostics.StatusHealthy,
				Counts: platformdiagnostics.Counts{Healthy: 1},
			},
			Results: []platformdiagnostics.Result{{
				CheckID: "server.storage.mariadb", Name: "MariaDB", Required: true,
				Observation: platformdiagnostics.Observation{
					Status: platformdiagnostics.StatusHealthy,
					SafeMessage: "MariaDB connection is healthy",
					CheckedAt: time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC),
					Duration: 1200000,
					Attributes: map[string]string{"dependency": "mariadb"},
				},
				Fault: &fault.Fault{},
			}},
		},
		Readiness: Readiness{Ready: true, SafeMessage: "Required dependencies are healthy"},
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal diagnostics report: %v", err)
	}
	var instance any
	if err := json.Unmarshal(encoded, &instance); err != nil {
		t.Fatalf("decode diagnostics report: %v", err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("runtime diagnostics report violates Schema: %v\n%s", err, encoded)
	}
	if containsJSONKey(encoded, "fault") {
		t.Fatalf("internal Fault must not be serialized: %s", encoded)
	}
}

func containsJSONKey(encoded []byte, key string) bool {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return false
	}
	diagnostics, ok := fields["diagnostics"]
	if !ok {
		return false
	}
	var report map[string]json.RawMessage
	if err := json.Unmarshal(diagnostics, &report); err != nil {
		return false
	}
	var results []map[string]json.RawMessage
	if err := json.Unmarshal(report["results"], &results); err != nil || len(results) == 0 {
		return false
	}
	_, exists := results[0][key]
	return exists
}
