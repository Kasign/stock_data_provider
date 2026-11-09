package template

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type testEvaluator map[string]bool

func (e testEvaluator) Evaluate(_ context.Context, r ApplicabilityRule, _ json.RawMessage) (bool, string, error) {
	if e[r.RuleRef] {
		return true, "", nil
	}
	return false, "not_applicable", nil
}

type testGates map[string]bool

func (g testGates) Contains(code string) bool { return g[code] }

type testParameterSchemas struct{}

func (testParameterSchemas) Validate(_ context.Context, ref string, _ json.RawMessage) (ParameterSchemaSnapshot, error) {
	return ParameterSchemaSnapshot{Ref: ref, Version: "1", Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, nil
}

func testPolicy() PropagationPolicy {
	return PropagationPolicy{OnDegraded: domain.PropagateAccept, OnSkipped: domain.PropagatePartial, OnFailed: domain.PropagateFail, OnTimedOut: domain.PropagateFail, OnBlocked: domain.PropagateFail}
}
func testRetry() RetryPolicy {
	return RetryPolicy{Ref: "retry-v1", MaxAttempts: 2, RetryableReasonCodes: []string{"provider_unavailable"}, Backoff: BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000}, AttemptTimeoutMS: 5000}
}
func testTemplate() WorkflowTemplate {
	return WorkflowTemplate{Ref: "stock_research", Version: "1", ParameterSchemaRef: "stock-research-params-v1", OutputRegistryVersion: "outputs-v1", ResourcePolicyRef: "resource-v1", Stages: []StageDefinition{
		{Code: "market", Required: true, Applicability: ApplicabilityRule{RuleRef: "always", RuleVersion: "1"}, Retry: testRetry(), Propagation: testPolicy()},
		{Code: "summary", Required: true, Dependencies: []string{"market"}, Applicability: ApplicabilityRule{RuleRef: "always", RuleVersion: "1"}, Retry: testRetry(), Propagation: testPolicy()},
	}}
}

func TestValidateRejectsCyclesAndWildcardRetryCodes(t *testing.T) {
	tmpl := testTemplate()
	tmpl.Stages[0].Dependencies = []string{"summary"}
	if err := Validate(tmpl, nil); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Fatalf("expected cycle error, got %v", err)
	}
	tmpl = testTemplate()
	tmpl.Stages[0].Retry.RetryableReasonCodes = []string{"provider_*"}
	if err := Validate(tmpl, nil); err == nil {
		t.Fatal("wildcard retry code accepted")
	}
}

func TestValidateRejectsUnknownGateType(t *testing.T) {
	for _, gateType := range []string{"hard-risk", "soft_risk", "unknown"} {
		t.Run(gateType, func(t *testing.T) {
			tmpl := testTemplate()
			tmpl.Stages[0].GateType = gateType
			if err := Validate(tmpl, nil); err == nil {
				t.Fatalf("unsupported gate type %q was accepted", gateType)
			}
		})
	}
}

func TestValidateRejectsRetryDelayExceedingTimeDurationRange(t *testing.T) {
	tmpl := testTemplate()
	tmpl.Stages[0].Retry.Backoff.MaxDelayMS = maxRetryDelayMS + 1
	if err := Validate(tmpl, nil); err == nil {
		t.Fatal("retry delay exceeding time.Duration range was accepted")
	}
}

func TestValidateRejectsAttemptTimeoutExceedingTimeDurationRange(t *testing.T) {
	tmpl := testTemplate()
	tmpl.Stages[0].Retry.AttemptTimeoutMS = maxRetryDelayMS + 1
	if err := Validate(tmpl, nil); err == nil {
		t.Fatal("attempt timeout exceeding time.Duration range was accepted")
	}
}

func TestResolveOrdersDAGAndRejectsExcludedDependency(t *testing.T) {
	tmpl := testTemplate()
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage(`{}`), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Stages) != 2 || resolved.Stages[0].Definition.Code != "market" || resolved.Stages[1].Definition.Code != "summary" {
		t.Fatalf("unexpected order: %#v", resolved.Stages)
	}
	tmpl.Stages[0].Applicability.RuleRef = "market-only"
	_, err = Resolve(context.Background(), tmpl, json.RawMessage(`{}`), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err == nil {
		t.Fatal("applicable stage depending on excluded stage was accepted")
	}
}

func TestHardRiskGateMustBeRegisteredAndFailClosed(t *testing.T) {
	tmpl := testTemplate()
	tmpl.RequiresHardRiskGate = true
	tmpl.Stages[0].GateType = "hard_risk"
	if err := Validate(tmpl, testGates{}); err == nil {
		t.Fatal("unregistered gate accepted")
	}
	if err := Validate(tmpl, testGates{"market": true}); err == nil {
		t.Fatal("non-fail-closed gate accepted")
	}
	p := PropagationPolicy{OnDegraded: domain.PropagateFail, OnSkipped: domain.PropagateFail, OnFailed: domain.PropagateFail, OnTimedOut: domain.PropagateFail, OnBlocked: domain.PropagateFail}
	tmpl.Stages[0].Propagation = p
	if err := Validate(tmpl, testGates{"market": true}); err != nil {
		t.Fatal(err)
	}
}

func TestFingerprintCanonicalizesJSONAndSetLikeCollections(t *testing.T) {
	a := testTemplate()
	b := testTemplate()
	b.Stages[0], b.Stages[1] = b.Stages[1], b.Stages[0]
	b.Stages[1].Dependencies = nil
	b.Stages[0].Dependencies = []string{"market"}
	fa, err := Fingerprint(a)
	if err != nil {
		t.Fatal(err)
	}
	fb, err := Fingerprint(b)
	if err != nil {
		t.Fatal(err)
	}
	if fa != fb {
		t.Fatalf("equivalent template collection order changed fingerprint: %s != %s", fa, fb)
	}
}

func TestResolveRequiresImmutableParameterSchemaSnapshot(t *testing.T) {
	tmpl := testTemplate()
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ParameterSchema.Ref != tmpl.ParameterSchemaRef || resolved.ParameterSchema.Version != "1" || resolved.ParameterSchema.Fingerprint == "" {
		t.Fatalf("parameter schema snapshot not preserved: %#v", resolved.ParameterSchema)
	}
	_, err = Resolve(context.Background(), tmpl, json.RawMessage("{}"), nil, testEvaluator{"always": true}, nil)
	if err == nil {
		t.Fatal("missing parameter schema validator accepted")
	}
}

type mismatchedParameterSchemas struct{}

func (mismatchedParameterSchemas) Validate(_ context.Context, _ string, _ json.RawMessage) (ParameterSchemaSnapshot, error) {
	return ParameterSchemaSnapshot{Ref: "other-schema", Version: "1", Fingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, nil
}

func TestResolveRejectsMismatchedParameterSchemaRef(t *testing.T) {
	_, err := Resolve(context.Background(), testTemplate(), json.RawMessage("{}"), mismatchedParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err == nil {
		t.Fatal("mismatched parameter schema ref accepted")
	}
}

type versionedParameterSchemas struct{ version, fingerprint string }

func (s versionedParameterSchemas) Validate(_ context.Context, ref string, _ json.RawMessage) (ParameterSchemaSnapshot, error) {
	return ParameterSchemaSnapshot{Ref: ref, Version: s.version, Fingerprint: s.fingerprint}, nil
}

func TestResolvedFingerprintBindsParameterSchemaVersionAndDigest(t *testing.T) {
	tmpl := testTemplate()
	base := versionedParameterSchemas{"1", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	first, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), base, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), versionedParameterSchemas{"2", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	third, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), versionedParameterSchemas{"1", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.Fingerprint == second.Fingerprint || first.Fingerprint == third.Fingerprint {
		t.Fatal("resolved template fingerprint must change when immutable schema version or content digest changes")
	}
}

func TestResolveRejectsNonHexSchemaFingerprint(t *testing.T) {
	schemas := versionedParameterSchemas{"1", "not-a-sha256"}
	_, err := Resolve(context.Background(), testTemplate(), json.RawMessage("{}"), schemas, testEvaluator{"always": true}, nil)
	if err == nil {
		t.Fatal("non-hex schema fingerprint accepted")
	}
}

func TestResolvePreservesExecutionPolicyInImmutableSnapshot(t *testing.T) {
	tmpl := testTemplate()
	tmpl.RequiresHardRiskGate = true
	tmpl.Stages[0].GateType = "hard_risk"
	tmpl.Stages[0].Propagation = PropagationPolicy{OnDegraded: domain.PropagateFail, OnSkipped: domain.PropagateFail, OnFailed: domain.PropagateFail, OnTimedOut: domain.PropagateFail, OnBlocked: domain.PropagateFail}
	tmpl.AllowEmptyInputs = true
	tmpl.OutputRegistryVersion = "outputs-v7"
	tmpl.ResourcePolicyRef = "resource-v9"
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), testParameterSchemas{}, testEvaluator{"always": true}, testGates{"market": true})
	if err != nil {
		t.Fatal(err)
	}
	if !resolved.RequiresHardRiskGate || !resolved.AllowEmptyInputs || resolved.OutputRegistryVersion != "outputs-v7" || resolved.ResourcePolicyRef != "resource-v9" {
		t.Fatalf("resolved template snapshot lost execution policy: %#v", resolved)
	}
}

func TestResolveCanonicalizesDependencySetInSnapshot(t *testing.T) {
	tmpl := testTemplate()
	audit := StageDefinition{Code: "audit", Required: false, Applicability: ApplicabilityRule{RuleRef: "always", RuleVersion: "1"}, Retry: testRetry(), Propagation: testPolicy()}
	tmpl.Stages = append(tmpl.Stages, audit)
	tmpl.Stages[1].Dependencies = []string{"summary", "audit"}
	// Replace the dependent stage with a valid DAG node that depends on both roots.
	tmpl.Stages[1].Dependencies = []string{"audit", "market"}
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var final *ResolvedStage
	for i := range resolved.Stages {
		if resolved.Stages[i].Definition.Code == "summary" {
			final = &resolved.Stages[i]
			break
		}
	}
	if final == nil {
		t.Fatal("summary stage missing")
	}
	if len(final.Dependencies) != 2 || final.Dependencies[0] != "audit" || final.Dependencies[1] != "market" {
		t.Fatalf("resolved dependency set is not canonical: %#v", final.Dependencies)
	}
	if len(final.Definition.Dependencies) != 2 || final.Definition.Dependencies[0] != "audit" || final.Definition.Dependencies[1] != "market" {
		t.Fatalf("resolved definition snapshot is not canonical: %#v", final.Definition.Dependencies)
	}
}

func TestResolveMaterializesNonApplicableOptionalStageAsSkipped(t *testing.T) {
	tmpl := testTemplate()
	tmpl.Stages = append(tmpl.Stages, StageDefinition{
		Code: "optional_news", Required: false,
		Applicability: ApplicabilityRule{RuleRef: "optional-news", RuleVersion: "v3"},
		Retry:         testRetry(), Propagation: testPolicy(),
	})
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage(`{}`), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Stages) != 3 {
		t.Fatalf("expected two executable stages and one skipped optional stage, got %d", len(resolved.Stages))
	}
	last := resolved.Stages[2]
	if last.Definition.Code != "optional_news" || last.InitialStatus != domain.StageSkipped || len(last.ReasonCodes) != 1 || last.ReasonCodes[0] != "not_applicable" {
		t.Fatalf("optional non-applicable stage was not resolved as skipped: %#v", last)
	}
	if len(last.Dependencies) != 0 {
		t.Fatalf("terminal skipped stage must not carry executable dependencies: %#v", last.Dependencies)
	}
	if len(resolved.Excluded) != 0 {
		t.Fatalf("optional skipped stage must not be recorded as an excluded required stage: %#v", resolved.Excluded)
	}
}

func TestResolveExcludesNonApplicableRequiredStage(t *testing.T) {
	tmpl := testTemplate()
	tmpl.Stages = append(tmpl.Stages, StageDefinition{
		Code: "required_optional_dependency", Required: true,
		Applicability: ApplicabilityRule{RuleRef: "required-off", RuleVersion: "v2"},
		Retry:         testRetry(), Propagation: testPolicy(),
	})
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage(`{}`), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved.Stages) != 2 || len(resolved.Excluded) != 1 || resolved.Excluded[0].StageCode != "required_optional_dependency" || resolved.Excluded[0].RuleVersion != "v2" {
		t.Fatalf("required non-applicable stage must be excluded from Run DAG: stages=%#v excluded=%#v", resolved.Stages, resolved.Excluded)
	}
}

func TestResolveRejectsApplicableStageDependingOnSkippedOptionalStage(t *testing.T) {
	tmpl := testTemplate()
	tmpl.Stages = append(tmpl.Stages, StageDefinition{
		Code: "optional_news", Required: false,
		Applicability: ApplicabilityRule{RuleRef: "optional-news", RuleVersion: "v3"},
		Retry:         testRetry(), Propagation: testPolicy(),
	})
	tmpl.Stages[1].Dependencies = []string{"market", "optional_news"}
	_, err := Resolve(context.Background(), tmpl, json.RawMessage(`{}`), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err == nil || !strings.Contains(err.Error(), "depends on non-applicable stage optional_news") {
		t.Fatalf("applicable Stage depending on skipped optional Stage must fail closed, got %v", err)
	}
}

func TestResolveSnapshotOwnsCanonicalStageDefinitionSlices(t *testing.T) {
	tmpl := testTemplate()
	tmpl.Stages[1].Dependencies = []string{"market"}
	tmpl.Stages[1].Retry.RetryableReasonCodes = []string{"z_provider_error", "a_provider_error"}
	tmpl.Stages = append(tmpl.Stages, StageDefinition{
		Code: "optional_news", Required: false,
		Dependencies:  []string{"summary", "market"},
		Applicability: ApplicabilityRule{RuleRef: "optional-news", RuleVersion: "v3"},
		Retry:         testRetry(), Propagation: testPolicy(),
	})

	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage(`{}`), testParameterSchemas{}, testEvaluator{
		"always": true,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var summary, skipped *ResolvedStage
	for i := range resolved.Stages {
		switch resolved.Stages[i].Definition.Code {
		case "summary":
			summary = &resolved.Stages[i]
		case "optional_news":
			skipped = &resolved.Stages[i]
		}
	}
	if summary == nil || skipped == nil {
		t.Fatalf("expected summary and optional skipped stages, got %#v", resolved.Stages)
	}
	if got := summary.Definition.Retry.RetryableReasonCodes; len(got) != 2 || got[0] != "a_provider_error" || got[1] != "z_provider_error" {
		t.Fatalf("retry reason codes are not canonical: %#v", got)
	}
	if got := skipped.Definition.Dependencies; len(got) != 2 || got[0] != "market" || got[1] != "summary" {
		t.Fatalf("skipped stage definition dependencies are not canonical: %#v", got)
	}
	if len(skipped.Dependencies) != 0 {
		t.Fatalf("skipped stage must not carry executable dependencies: %#v", skipped.Dependencies)
	}

	// Mutating the registry-owned template after resolution must not mutate the
	// resolved execution snapshot that will later be persisted with the Run.
	tmpl.Stages[1].Dependencies[0] = "mutated"
	tmpl.Stages[1].Retry.RetryableReasonCodes[0] = "mutated"
	tmpl.Stages[2].Dependencies[0] = "mutated"
	if summary.Definition.Dependencies[0] != "market" ||
		summary.Definition.Retry.RetryableReasonCodes[0] != "a_provider_error" {
		t.Fatalf("resolved active stage changed after source template mutation: %#v", summary.Definition)
	}
	if skipped.Definition.Dependencies[0] != "market" {
		t.Fatalf("resolved skipped stage changed after source template mutation: %#v", skipped.Definition)
	}
}

type mutatingParameterSchemas struct{}

func (mutatingParameterSchemas) Validate(_ context.Context, ref string, parameters json.RawMessage) (ParameterSchemaSnapshot, error) {
	parameters[0] = '['
	return ParameterSchemaSnapshot{Ref: ref, Version: "1", Fingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, nil
}

type mutatingApplicabilityEvaluator struct{ calls int }

func (e *mutatingApplicabilityEvaluator) Evaluate(_ context.Context, _ ApplicabilityRule, parameters json.RawMessage) (bool, string, error) {
	e.calls++
	if string(parameters) != `{"symbol":"600000"}` {
		return false, "", fmt.Errorf("applicability received mutated parameters: %s", parameters)
	}
	parameters[0] = '['
	return true, "", nil
}

func TestResolveIsolatesParameterBytesAcrossInjectedBoundaries(t *testing.T) {
	parameters := json.RawMessage(`{"symbol":"600000"}`)
	evaluator := &mutatingApplicabilityEvaluator{}
	resolved, err := Resolve(context.Background(), testTemplate(), parameters, mutatingParameterSchemas{}, evaluator, nil)
	if err != nil {
		t.Fatal(err)
	}
	if evaluator.calls != 2 {
		t.Fatalf("expected each Stage to receive an isolated parameter copy, got %d calls", evaluator.calls)
	}
	if string(parameters) != `{"symbol":"600000"}` {
		t.Fatalf("Resolve mutated caller-owned parameters: %s", parameters)
	}
	if len(resolved.Stages) != 2 {
		t.Fatalf("unexpected resolved stages after evaluator mutation: %#v", resolved.Stages)
	}
}
