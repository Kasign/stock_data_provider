package bootstrap

import (
	"testing"

	"example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

// newQuantResearchTemplateFixture builds the four single-Stage Registries and
// the combined template exactly as the composition root does.
func newQuantResearchTemplateFixture(t *testing.T) (template.WorkflowTemplate, *stageoutput.Registry) {
	t.Helper()
	sma, err := newQuantSMA20OutputRegistry()
	if err != nil {
		t.Fatal(err)
	}
	factor, err := newQuantFactorOutputRegistry()
	if err != nil {
		t.Fatal(err)
	}
	trend, err := newQuantTrendOutputRegistry()
	if err != nil {
		t.Fatal(err)
	}
	risk, err := newQuantRiskOutputRegistry()
	if err != nil {
		t.Fatal(err)
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	value, combined, err := buildQuantResearchWorkflowTemplate(sma, factor, trend, risk, policies)
	if err != nil {
		t.Fatal(err)
	}
	return value, combined
}

func TestQuantResearchOutputRegistryIsCombinedAndVersionUnique(t *testing.T) {
	_, combined := newQuantResearchTemplateFixture(t)
	if combined.Version() != quantResearchStageOutputRegistryVersion || combined.Version() != "1.6.0" {
		t.Fatalf("combined registry version = %q, want 1.6.0", combined.Version())
	}
	wantEntries := [][3]string{
		{stage.SMA20StageCode, stage.SMA20OutputType, stage.SMA20SchemaVersion},
		{stage.FactorStageCode, stage.FactorOutputType, stage.FactorSchemaVersion},
		{stage.TrendStageCode, stage.TrendOutputType, stage.TrendSchemaVersion},
		{stage.RiskStageCode, stage.RiskOutputType, stage.RiskSchemaVersion},
	}
	for _, d := range wantEntries {
		if _, ok := combined.Entry(d[0], d[1], d[2]); !ok {
			t.Fatalf("combined registry is missing entry %s/%s@%s", d[0], d[1], d[2])
		}
	}
	// The set must accept the combined registry together with every existing
	// single-Stage version; a duplicate version would abort Server startup.
	sma, _ := newQuantSMA20OutputRegistry()
	factor, _ := newQuantFactorOutputRegistry()
	trend, _ := newQuantTrendOutputRegistry()
	risk, _ := newQuantRiskOutputRegistry()
	if _, err := researchflowapplication.NewStageOutputRegistrySet(sma, factor, trend, risk, combined); err != nil {
		t.Fatalf("combined registry version must not collide with single-Stage registries: %v", err)
	}
}

func TestQuantResearchTemplateShapeAndPropagation(t *testing.T) {
	value, combined := newQuantResearchTemplateFixture(t)
	if value.Ref != quantResearchWorkflowTemplateRef || value.Version != quantResearchWorkflowTemplateVersion {
		t.Fatalf("unexpected template identity: %s@%s", value.Ref, value.Version)
	}
	if value.ParameterScope != template.ParameterScopePerStage {
		t.Fatalf("parameter scope = %q, want per_stage", value.ParameterScope)
	}
	if value.ParameterSchemaRef != quantResearchParameterSchemaRef {
		t.Fatalf("parameter schema ref = %q, want %q", value.ParameterSchemaRef, quantResearchParameterSchemaRef)
	}
	if value.OutputRegistryVersion != combined.Version() {
		t.Fatalf("template registry version %q != combined %q", value.OutputRegistryVersion, combined.Version())
	}
	if value.RequiresHardRiskGate {
		t.Fatal("quant.research must not declare a hard risk gate (D4)")
	}
	if len(value.Stages) != 4 {
		t.Fatalf("stage count = %d, want 4", len(value.Stages))
	}
	wantCodes := map[string]bool{
		stage.SMA20StageCode: false, stage.FactorStageCode: false,
		stage.TrendStageCode: false, stage.RiskStageCode: false,
	}
	propagation := quantResearchPropagation()
	for _, s := range value.Stages {
		if _, ok := wantCodes[s.Code]; !ok {
			t.Fatalf("unexpected Stage code %q (Signal must not be in v1)", s.Code)
		}
		wantCodes[s.Code] = true
		if !s.Required {
			t.Errorf("Stage %s must be required", s.Code)
		}
		if len(s.Dependencies) != 0 {
			t.Errorf("Stage %s declared dependencies %v, want none (D2)", s.Code, s.Dependencies)
		}
		if s.Propagation != propagation {
			t.Errorf("Stage %s propagation = %+v, want D5 policy %+v", s.Code, s.Propagation, propagation)
		}
		if s.Retry.MaxAttempts != 1 || s.Retry.AttemptTimeoutMS <= 0 {
			t.Errorf("Stage %s has unbounded or zero-timeout retry policy: %+v", s.Code, s.Retry)
		}
	}
	for code, seen := range wantCodes {
		if !seen {
			t.Errorf("missing required Stage %s", code)
		}
	}
	if propagation.OnDegraded != researchflowdomain.PropagateAccept ||
		propagation.OnSkipped != researchflowdomain.PropagateFail ||
		propagation.OnFailed != researchflowdomain.PropagateFail ||
		propagation.OnTimedOut != researchflowdomain.PropagateFail ||
		propagation.OnBlocked != researchflowdomain.PropagateFail {
		t.Fatalf("D5 policy violated: %+v", propagation)
	}
}
