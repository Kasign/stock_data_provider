package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func compileNewsEvidencePageSchemaV2(t *testing.T) *jsonschema.Schema {
	t.Helper()
	schemaPath := filepath.Join("..", "..", "..", "doc", "news", "schema", "NEWS-CTR-001-research-evidence-page-2.0.0.schema.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(schemaBytes, &document); err != nil {
		t.Fatal(err)
	}
	schemaID, ok := document["$id"].(string)
	if !ok || schemaID == "" {
		t.Fatalf("schema %s has no $id", schemaPath)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	if err := compiler.AddResource(schemaID, bytes.NewReader(schemaBytes)); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(schemaID)
	if err != nil {
		t.Fatal(err)
	}
	return compiled
}

func TestNewsEvidenceQueryContractV2Golden(t *testing.T) {
	goldenPath := filepath.Join("..", "..", "..", "doc", "news", "golden", "NEWS-CTR-001-research-evidence-page-2.0.0.golden.json")
	payload, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var instance any
	if err := json.Unmarshal(payload, &instance); err != nil {
		t.Fatal(err)
	}
	if err := compileNewsEvidencePageSchemaV2(t).Validate(instance); err != nil {
		t.Fatalf("NEWS-CTR-001 2.0.0 schema rejected golden: %v", err)
	}

	var page ResearchEvidencePage
	if err := json.Unmarshal(payload, &page); err != nil {
		t.Fatal(err)
	}
	if page.Items == nil || len(page.Items) != 1 || page.Items[0].AvailableAt == nil ||
		page.Items[0].AvailableAt.Nanosecond() != 0 {
		t.Fatalf("2.0.0 golden must carry one non-null second-precision available_at: %+v", page.Items)
	}
}

func TestNewsEvidenceQueryContractV2RejectsFractionalAvailableAt(t *testing.T) {
	goldenPath := filepath.Join("..", "..", "..", "doc", "news", "golden", "NEWS-CTR-001-research-evidence-page-2.0.0.golden.json")
	payload, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var instance map[string]any
	if err := json.Unmarshal(payload, &instance); err != nil {
		t.Fatal(err)
	}
	items, ok := instance["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("golden has no items: %+v", instance)
	}
	item, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("golden item is not an object: %+v", items[0])
	}
	item["available_at"] = "2026-11-06T08:30:00.000000000Z"
	if err := compileNewsEvidencePageSchemaV2(t).Validate(instance); err == nil {
		t.Fatal("NEWS-CTR-001 2.0.0 must reject fractional available_at")
	}
}

func TestNewsEvidenceQueryContractGoldens(t *testing.T) {
	goldenDir := filepath.Join("..", "..", "..", "doc", "news", "golden")
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)

	getPayload, err := os.ReadFile(filepath.Join(goldenDir, "NEWS-CTR-001-get-evidence-request.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var getRequest GetEvidenceRequest
	if err := json.Unmarshal(getPayload, &getRequest); err != nil {
		t.Fatal(err)
	}
	if err := getRequest.Validate(now); err != nil {
		t.Fatalf("GetEvidence golden rejected: %v", err)
	}

	listPayload, err := os.ReadFile(filepath.Join(goldenDir, "NEWS-CTR-001-list-evidence-request.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var listRequest ListEvidenceRequest
	if err := json.Unmarshal(listPayload, &listRequest); err != nil {
		t.Fatal(err)
	}
	if err := listRequest.Validate(now); err != nil {
		t.Fatalf("ListEvidence golden rejected: %v", err)
	}

	pagePayload, err := os.ReadFile(filepath.Join(goldenDir, "NEWS-CTR-001-research-evidence-page.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var page ResearchEvidencePage
	if err := json.Unmarshal(pagePayload, &page); err != nil {
		t.Fatal(err)
	}
	if page.SnapshotRef == "" || page.Items == nil || page.Diagnostics == nil {
		t.Fatalf("page golden has missing required values: %+v", page)
	}
	if page.Items[0].AvailableAt != nil || page.NextCursor != nil {
		t.Fatalf("nullable fields were not preserved: %+v", page.Items[0])
	}
	if page.Items[0].RelationStatus != EvidenceRelationNotProvided || len(page.Items[0].RelationRefs) != 0 {
		t.Fatalf("relation status must distinguish an unqueried relation set: %+v", page.Items[0])
	}
}
