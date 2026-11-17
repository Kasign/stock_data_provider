package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func commandAt(_ int) CreateRunCommand {
	return CreateRunCommand{
		SchemaVersion:     "1.0.0",
		IdempotencyKey:    "key-1",
		TemplateRef:       "news-search",
		TemplateVersion:   "1.0.0",
		InputSnapshotRefs: []string{"snapshot-b", "snapshot-a"},
		Parameters:        json.RawMessage(`{"query":"market","limit":20}`),
	}
}

func TestValidateCreateRunCommandBoundaries(t *testing.T) {
	cmd := commandAt(0)
	if err := ValidateCreateRunCommand(cmd, false); err != nil {
		t.Fatal(err)
	}
	cmd.InputSnapshotRefs = []string{}
	if err := ValidateCreateRunCommand(cmd, false); !errors.Is(err, ErrInvalidCommand) {
		t.Fatalf("expected empty snapshot rejection, got %v", err)
	}
	if err := ValidateCreateRunCommand(cmd, true); err != nil {
		t.Fatal(err)
	}
	cmd = commandAt(0)
	cmd.Parameters = json.RawMessage(`[]`)
	if err := ValidateCreateRunCommand(cmd, false); err == nil {
		t.Fatal("non-object parameters accepted")
	}
	cmd = commandAt(0)
	cmd.InputSnapshotRefs = []string{" snapshot-a"}
	if err := ValidateCreateRunCommand(cmd, false); err == nil {
		t.Fatal("snapshot reference with surrounding whitespace accepted")
	}
	cmd = commandAt(0)
	cmd.InputSnapshotRefs = []string{"snapshot-a", "snapshot-a"}
	if err := ValidateCreateRunCommand(cmd, false); err == nil {
		t.Fatal("duplicate snapshot references accepted")
	}
	cmd = commandAt(0)
	cmd.SchemaVersion = ""
	if err := ValidateCreateRunCommand(cmd, false); err == nil {
		t.Fatal("missing request schema version accepted")
	}
}

func TestRequestFingerprintCanonicalizesParametersAndIncludesSchemaVersion(t *testing.T) {
	a, b := commandAt(0), commandAt(1)
	b.Parameters = json.RawMessage(`{ "limit":20, "query" : "market" }`)
	fa, err := RequestFingerprint("caller-a", a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := RequestFingerprint("caller-a", b)
	if err != nil {
		t.Fatal(err)
	}
	if fa != fb {
		t.Fatal("semantic-equivalent request produced different fingerprint")
	}
	fc, err := RequestFingerprint("caller-b", a)
	if err != nil {
		t.Fatal(err)
	}
	if fa == fc {
		t.Fatal("caller scope missing from fingerprint")
	}
	if err := ValidateIdempotentReplay(fa, fc); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	b = a
	b.SchemaVersion = "2.0.0"
	fd, err := RequestFingerprint("caller-a", b)
	if err != nil {
		t.Fatal(err)
	}
	if fa == fd {
		t.Fatal("request schema version must participate in fingerprint")
	}
}

func TestRequestFingerprintUsesJCSNumberAndUnicodeRules(t *testing.T) {
	a, b := commandAt(0), commandAt(1)
	a.Parameters = json.RawMessage(`{"value":1.0,"negative_zero":-0,"unicode":"\u0065"}`)
	b.Parameters = json.RawMessage(`{ "unicode":"e", "negative_zero":0.0, "value":1 }`)
	fa, err := RequestFingerprint("caller-a", a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := RequestFingerprint("caller-a", b)
	if err != nil {
		t.Fatal(err)
	}
	if fa != fb {
		t.Fatalf("JCS-equivalent parameters differ: %s != %s", fa, fb)
	}
}

func TestRequestFingerprintRejectsDuplicateJSONKeys(t *testing.T) {
	cmd := commandAt(0)
	cmd.Parameters = json.RawMessage(`{"query":"market","query":"other","limit":20}`)
	if _, err := RequestFingerprint("caller-a", cmd); err == nil {
		t.Fatal("duplicate JSON keys must be rejected by JCS")
	}
}

func TestRequestFingerprintCanonicalizesSnapshotSetAndRejectsInvalidRefs(t *testing.T) {
	a, b := commandAt(0), commandAt(1)
	a.InputSnapshotRefs = []string{"snapshot-b", "snapshot-a"}
	b.InputSnapshotRefs = []string{"snapshot-a", "snapshot-b"}
	fa, err := RequestFingerprint("caller-a", a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := RequestFingerprint("caller-a", b)
	if err != nil {
		t.Fatal(err)
	}
	if fa != fb {
		t.Fatal("snapshot set order changed request fingerprint")
	}
	invalid := [][]string{nil, {""}, {" snapshot-a"}, {"snapshot-a", "snapshot-a"}}
	for i, refs := range invalid {
		cmd := commandAt(0)
		cmd.InputSnapshotRefs = refs
		if _, err := RequestFingerprint("caller-a", cmd); err == nil {
			t.Fatalf("invalid input_snapshot_refs case %d accepted", i)
		}
	}
}
