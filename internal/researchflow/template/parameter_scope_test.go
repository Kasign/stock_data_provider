package template

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// fingerprintBaselineFixture is a fully specified template whose fingerprint is
// pinned below. Any change to the serialized shape of WorkflowTemplate — adding
// a field without omitempty, renaming a key, reordering nothing because JCS
// sorts keys — moves this constant and fails here, before it can break the
// startup-time Publish comparison in production.
//
// The value was cross-checked against a structurally identical copy of the
// pre-ParameterScope WorkflowTemplate layout: both serialize to
// 0e57907d2a8f96a15bc6930bd8305cc886e9789c2774b81fb50d0c0a7a3c808e, which is
// the evidence that adding parameter_scope did not move any published template.
const fingerprintBaselineFixture = "0e57907d2a8f96a15bc6930bd8305cc886e9789c2774b81fb50d0c0a7a3c808e"

func baselineTemplate() WorkflowTemplate {
	retry := RetryPolicy{
		Ref: "retry-v1", MaxAttempts: 2, RetryableReasonCodes: []string{"provider_unavailable"},
		Backoff:          BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 10, MaxDelayMS: 100},
		AttemptTimeoutMS: 1000,
	}
	propagation := PropagationPolicy{
		OnDegraded: domain.PropagateAccept, OnSkipped: domain.PropagatePartial,
		OnFailed: domain.PropagateFail, OnTimedOut: domain.PropagateFail, OnBlocked: domain.PropagateFail,
	}
	return WorkflowTemplate{
		Ref: "daily", Version: "1.0.0", ParameterSchemaRef: "daily-params-v1",
		OutputRegistryVersion: "outputs-v1", ResourcePolicyRef: "resource-v1",
		Stages: []StageDefinition{{
			Code: "quote", Required: true,
			Applicability: ApplicabilityRule{RuleRef: "always", RuleVersion: "1"},
			Retry:         retry, Propagation: propagation,
		}},
	}
}

func TestWorkflowTemplateOmitsParameterScopeWhenUnset(t *testing.T) {
	raw, err := json.Marshal(baselineTemplate())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("parameter_scope")) {
		t.Fatalf("template scope must not emit parameter_scope; got %s", raw)
	}
	// omitempty plus a zero value is the whole mechanism that keeps every
	// already-published template byte-identical, so assert it on bytes rather
	// than on the struct.
	if !bytes.Contains(raw, []byte(`"parameter_schema_ref":"daily-params-v1"`)) ||
		!bytes.Contains(raw, []byte(`"output_registry_version":"outputs-v1"`)) {
		t.Fatalf("unexpected template serialization: %s", raw)
	}
}

func TestWorkflowTemplateFingerprintBaselineIsStable(t *testing.T) {
	fingerprint, err := Fingerprint(baselineTemplate())
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint != fingerprintBaselineFixture {
		t.Fatalf("template fingerprint drifted: got %s want %s; a changed value means every published template's identity changed and startup Publish will conflict", fingerprint, fingerprintBaselineFixture)
	}
}

func TestWorkflowTemplateSettingScopeChangesFingerprint(t *testing.T) {
	plain, err := Fingerprint(baselineTemplate())
	if err != nil {
		t.Fatal(err)
	}
	scoped := baselineTemplate()
	scoped.ParameterScope = ParameterScopePerStage
	perStage, err := Fingerprint(scoped)
	if err != nil {
		t.Fatal(err)
	}
	if perStage == plain {
		t.Fatal("declaring a parameter scope must change the template fingerprint: execution semantics changed")
	}
}

func TestValidateRejectsUnsupportedParameterScope(t *testing.T) {
	for _, scope := range []string{"per-stage", "STAGES", "per stage", "nested"} {
		value := baselineTemplate()
		value.ParameterScope = scope
		err := Validate(value, nil)
		if !errors.Is(err, ErrInvalidTemplate) || !strings.Contains(err.Error(), "parameter scope") {
			t.Fatalf("scope %q was not rejected with a parameter scope error: %v", scope, err)
		}
	}
}

func TestValidateAcceptsDeclaredParameterScopes(t *testing.T) {
	for _, scope := range []string{"", ParameterScopeTemplate, ParameterScopePerStage} {
		value := baselineTemplate()
		value.ParameterScope = scope
		if err := Validate(value, nil); err != nil {
			t.Fatalf("scope %q must be accepted: %v", scope, err)
		}
	}
}

func TestSplitPerStageParametersPreservesSubtreeBytes(t *testing.T) {
	// Canonical envelope, as created by CreateRun's JCS canonicalization.
	parameters := json.RawMessage(`{"stages":{"quant.risk":{"symbol":"600036.SH","trade_date":"2026-11-25"},"quant.sma20":{"limit":20,"symbol":"600036.SH","start_date":"2026-10-01"}}}`)
	split, err := SplitPerStageParameters(parameters)
	if err != nil {
		t.Fatal(err)
	}
	if len(split) != 2 {
		t.Fatalf("split stages = %d, want 2", len(split))
	}
	want := `{"symbol":"600036.SH","trade_date":"2026-11-25"}`
	if got := string(split["quant.risk"]); got != want {
		t.Fatalf("risk subtree = %s, want %s", got, want)
	}
	if _, ok := split["quant.trend"]; ok {
		t.Fatal("split must not synthesize subtrees")
	}
}

func TestSplitPerStageParametersRejectsMalformedEnvelopes(t *testing.T) {
	for name, body := range map[string]string{
		"root array":            `[]`,
		"empty document":        `{}`,
		"missing stages":        `{"stage":"quant.risk"}`,
		"extra root key":        `{"stages":{"a":{"x":1}},"symbol":"600036.SH"}`,
		"stages not object":     `{"stages":[]}`,
		"stages null":           `{"stages":null}`,
		"empty subtree":         `{"stages":{"a":{}}}`,
		"scalar subtree":        `{"stages":{"a":1}}`,
		"array subtree":         `{"stages":{"a":[1]}}`,
		"null subtree":          `{"stages":{"a":null}}`,
		"blank stage code":      `{"stages":{" ":{}}}`,
		"non-object stages str": `{"stages":"a"}`,
	} {
		if _, err := SplitPerStageParameters(json.RawMessage(body)); !errors.Is(err, ErrStageParameters) {
			t.Fatalf("%s must be rejected as stage parameters: %v", name, err)
		}
	}
}

func TestSplitPerStageParametersAcceptsEmptyStageSet(t *testing.T) {
	// An empty stage set is structurally valid; the caller reports the missing
	// executable Stage codes, which produces a far better error than a parse
	// failure would.
	split, err := SplitPerStageParameters(json.RawMessage(`{"stages":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(split) != 0 {
		t.Fatalf("split stage count = %d, want 0", len(split))
	}
}

func TestStageParametersHonoursScope(t *testing.T) {
	envelope := json.RawMessage(`{"stages":{"a":{"x":1}}}`)
	// Empty scope is template scope: the document is delivered verbatim.
	for _, scope := range []string{"", ParameterScopeTemplate} {
		got, err := StageParameters(scope, envelope, "a")
		if err != nil {
			t.Fatalf("scope %q: %v", scope, err)
		}
		if !bytes.Equal(got, envelope) {
			t.Fatalf("scope %q must pass parameters verbatim: got %s want %s", scope, got, envelope)
		}
	}
	got, err := StageParameters(ParameterScopePerStage, envelope, "a")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"x":1}` {
		t.Fatalf("per-stage subtree = %s, want {\"x\":1}", got)
	}
	if _, err := StageParameters(ParameterScopePerStage, envelope, "b"); !errors.Is(err, ErrStageParameters) {
		t.Fatalf("missing subtree must be rejected: %v", err)
	}
}

func TestStageParametersDoesNotAliasTheEnvelope(t *testing.T) {
	envelope := json.RawMessage(`{"stages":{"a":{"x":1},"b":{"y":2}}}`)
	first, err := StageParameters(ParameterScopePerStage, envelope, "a")
	if err != nil {
		t.Fatal(err)
	}
	first[0] = 'X'
	second, err := StageParameters(ParameterScopePerStage, envelope, "b")
	if err != nil {
		t.Fatal(err)
	}
	if string(second) != `{"y":2}` {
		t.Fatalf("mutating one subtree must not affect another: %s", second)
	}
	if got := string(envelope); got != `{"stages":{"a":{"x":1},"b":{"y":2}}}` {
		t.Fatalf("envelope was mutated: %s", got)
	}
}
