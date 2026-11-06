package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

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
}
