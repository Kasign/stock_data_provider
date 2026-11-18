package diagnostics

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
)

func TestNewStageCheckMapsFourAssessmentStates(t *testing.T) {
	now := time.Date(2026, 11, 9, 13, 0, 0, 0, time.UTC)
	cases := []struct {
		status AssessmentStatus
		reason string
		want   platformdiagnostics.Status
	}{
		{AssessmentPass, "", platformdiagnostics.StatusHealthy},
		{AssessmentDegraded, "quality.coverage_low", platformdiagnostics.StatusDegraded},
		{AssessmentFail, "stage.provider_failed", platformdiagnostics.StatusFailed},
		{AssessmentNotComparable, "comparison.input_mismatch", platformdiagnostics.StatusDegraded},
	}
	for _, testCase := range cases {
		t.Run(string(testCase.status), func(t *testing.T) {
			check, err := NewStageCheck(StageAssessment{
				StageRef: "stage-1", StageCode: "market", Kind: KindQuality,
				Required: true, Status: testCase.status, ReasonCode: testCase.reason,
				SafeMessage: "Researchflow diagnostic result", EvidenceRef: "evidence-1",
				CheckedAt: now, Duration: 25 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}
			observation, err := check.Run(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if observation.Status != testCase.want ||
				observation.Attributes["assessment_status"] != string(testCase.status) {
				t.Fatalf("status projection = %#v, want %s / %s", observation, testCase.want, testCase.status)
			}
		})
	}
}

func TestStageChecksAggregateThroughPlatformRunner(t *testing.T) {
	now := time.Now().UTC()
	assessments := []StageAssessment{
		{StageRef: "stage-pass", StageCode: "market", Kind: KindStageExecution, Required: true, Status: AssessmentPass, SafeMessage: "stage completed", CheckedAt: now},
		{StageRef: "stage-nc", StageCode: "comparison", Kind: KindQuality, Required: true, Status: AssessmentNotComparable, ReasonCode: "comparison.not_comparable", SafeMessage: "inputs cannot be compared", CheckedAt: now},
	}
	checks := make([]platformdiagnostics.Check, 0, len(assessments))
	for _, assessment := range assessments {
		check, err := NewStageCheck(assessment)
		if err != nil {
			t.Fatal(err)
		}
		checks = append(checks, check)
	}
	report, err := (platformdiagnostics.Runner{}).Run(context.Background(), checks)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Status != platformdiagnostics.StatusDegraded ||
		report.Summary.Counts.Healthy != 1 || report.Summary.Counts.Degraded != 1 {
		t.Fatalf("unexpected platform diagnostic summary: %#v", report.Summary)
	}
}

func TestNewStageCheckRejectsInvalidOrUnsafeProjection(t *testing.T) {
	base := StageAssessment{
		StageRef: "stage-1", StageCode: "market", Kind: KindStageExecution,
		Status: AssessmentPass, SafeMessage: "completed",
		CheckedAt: time.Now().UTC(),
	}
	cases := []struct {
		name string
		edit func(*StageAssessment)
	}{
		{name: "unknown status", edit: func(v *StageAssessment) { v.Status = "unknown" }},
		{name: "missing stage ref", edit: func(v *StageAssessment) { v.StageRef = " " }},
		{name: "invalid stage code with spaces", edit: func(v *StageAssessment) { v.StageCode = "market search" }},
		{name: "invalid uppercase stage code", edit: func(v *StageAssessment) { v.StageCode = "Market" }},
		{name: "missing safe message", edit: func(v *StageAssessment) { v.SafeMessage = "" }},
		{name: "missing reason", edit: func(v *StageAssessment) { v.Status = AssessmentFail }},
		{name: "invalid reason", edit: func(v *StageAssessment) { v.Status = AssessmentFail; v.ReasonCode = "Provider unavailable" }},
		{name: "invalid kind", edit: func(v *StageAssessment) { v.Kind = "provider_raw_error" }},
		{name: "negative duration", edit: func(v *StageAssessment) { v.Duration = -time.Second }},
		{name: "unapproved attribute", edit: func(v *StageAssessment) { v.Attributes = map[string]string{"api_key": "secret"} }},
		{name: "reserved projection attribute", edit: func(v *StageAssessment) { v.Attributes = map[string]string{"stage_ref": "spoofed"} }},
		{name: "free-form attribute value", edit: func(v *StageAssessment) { v.Attributes = map[string]string{"source": "raw provider error"} }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value := base
			testCase.edit(&value)
			if _, err := NewStageCheck(value); err == nil {
				t.Fatal("invalid diagnostic projection was accepted")
			}
		})
	}
}

func TestNewStageCheckCopiesAttributesAndReservesProjectionKeys(t *testing.T) {
	attributes := map[string]string{"source": "research"}
	check, err := NewStageCheck(StageAssessment{
		StageRef: "stage-1", StageCode: "market", Kind: KindDependency,
		Status: AssessmentPass, SafeMessage: "dependency checked",
		CheckedAt: time.Now().UTC(), Attributes: attributes,
	})
	if err != nil {
		t.Fatal(err)
	}
	attributes["source"] = "mutated"
	observation, err := check.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.Attributes["source"] != "research" ||
		observation.Attributes["assessment_status"] != "pass" ||
		observation.Attributes["stage_ref"] != "stage-1" {
		t.Fatalf("diagnostic projection leaked or lost attributes: %#v", observation.Attributes)
	}
	observation.Attributes["source"] = "returned_mutation"
	again, err := check.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if again.Attributes["source"] != "research" {
		t.Fatal("repeated check shares mutable observation attributes")
	}
}

func TestNewStageCheckNormalizesCheckedAtToUTC(t *testing.T) {
	local := time.Date(2026, 11, 18, 18, 30, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))
	check, err := NewStageCheck(StageAssessment{
		StageRef: "stage-utc", StageCode: "market", Kind: KindStageExecution,
		Status: AssessmentPass, SafeMessage: "stage checked",
		CheckedAt: local,
	})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := check.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if observation.CheckedAt.Location() != time.UTC || !observation.CheckedAt.Equal(local.UTC()) {
		t.Fatalf("checked_at was not normalized to UTC: %#v", observation.CheckedAt)
	}
	raw, err := json.Marshal(observation)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	checkedAt, ok := payload["checked_at"].(string)
	if !ok || !strings.HasSuffix(checkedAt, "Z") {
		t.Fatalf("checked_at must serialize with UTC Z, got %#v", payload["checked_at"])
	}
}


func TestNewStageCheckRejectsInvalidUTF8Text(t *testing.T) {
	invalid := string([]byte{0xff, 0xfe})
	base := StageAssessment{
		StageRef: "stage-utf8", StageCode: "market", Kind: KindStageExecution,
		Status: AssessmentPass, SafeMessage: "completed",
		CheckedAt: time.Now().UTC(),
	}
	cases := []struct {
		name string
		edit func(*StageAssessment)
	}{
		{name: "stage ref", edit: func(v *StageAssessment) { v.StageRef = invalid }},
		{name: "stage code", edit: func(v *StageAssessment) { v.StageCode = invalid }},
		{name: "reason code", edit: func(v *StageAssessment) { v.ReasonCode = invalid }},
		{name: "safe message", edit: func(v *StageAssessment) { v.SafeMessage = invalid }},
		{name: "evidence ref", edit: func(v *StageAssessment) { v.EvidenceRef = invalid }},
		{name: "attribute value", edit: func(v *StageAssessment) { v.Attributes = map[string]string{"source": invalid} }},
		{name: "attribute key", edit: func(v *StageAssessment) { v.Attributes = map[string]string{invalid: "research"} }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			value := base
			testCase.edit(&value)
			if _, err := NewStageCheck(value); err == nil {
				t.Fatal("invalid UTF-8 diagnostic text was accepted")
			}
		})
	}
}
