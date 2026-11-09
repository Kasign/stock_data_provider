package storage

import (
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

func TestResearchflowTimestampUsesFixedUTCPrecision(t *testing.T) {
	input := time.Date(2026, 11, 9, 1, 2, 3, 123456789, time.FixedZone("offset", 8*60*60))
	encoded, err := formatResearchflowTime(input)
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) != 30 || encoded != "2026-11-08T17:02:03.123456789Z" {
		t.Fatalf("formatted timestamp = %q, want fixed-width UTC timestamp", encoded)
	}
	decoded, err := parseResearchflowTime(encoded)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Equal(input) || decoded.Location() != time.UTC {
		t.Fatalf("decoded timestamp = %s (%s), want same instant in UTC", decoded, decoded.Location())
	}
}

func TestResearchflowTimestampRejectsZeroValue(t *testing.T) {
	if _, err := formatResearchflowTime(time.Time{}); err == nil {
		t.Fatal("expected zero timestamp to be rejected")
	}
}

func TestSortResearchflowStagesUsesResolvedTemplateOrder(t *testing.T) {
	stages := []domain.Stage{
		{Code: "risk"},
		{Code: "context"},
		{Code: "quant"},
	}
	sortResearchflowStages(stages, map[string]int{"context": 0, "quant": 1, "risk": 2})
	want := []string{"context", "quant", "risk"}
	for index := range want {
		if stages[index].Code != want[index] {
			t.Fatalf("stage order = %#v, want %#v", stages, want)
		}
	}
}

func TestMySQLRunRepositoryValidatesResolvedSnapshotIdentity(t *testing.T) {
	repository := &MySQLRunRepository{db: &sql.DB{}}
	record := application.CreateRunRecord{
		Scope: application.IdempotencyScope{CallerRef: "caller-1", Operation: "CreateRun", Key: "key-1"},
		Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Run: domain.Run{
			Ref: "run-1", SchemaVersion: "v1", Status: domain.RunCreated, Version: 1,
			TemplateRef: "template-1", TemplateVersion: "v1",
			TemplateFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			CorrelationID: "corr-1",
		},
		Parameters: json.RawMessage(`{}`),
		ResolvedTemplate: template.ResolvedTemplate{
			Ref: "template-1", Version: "v1",
			Fingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		},
		Stages: []domain.Stage{{Ref: "stage-1", RunRef: "run-1", Code: "context"}},
	}
	if err := repository.validateRecord(record); err != nil {
		t.Fatalf("valid create-run record rejected: %v", err)
	}
	record.ResolvedTemplate.Version = "v2"
	if err := repository.validateRecord(record); err == nil {
		t.Fatal("expected mismatched resolved-template version to be rejected")
	}
}
