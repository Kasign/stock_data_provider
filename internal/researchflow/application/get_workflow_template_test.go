package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type workflowTemplateRegistryStub struct {
	value        template.WorkflowTemplate
	err          error
	calls        int
	ref, version string
}

func (s *workflowTemplateRegistryStub) Get(_ context.Context, ref, version string) (template.WorkflowTemplate, error) {
	s.calls++
	s.ref, s.version = ref, version
	return s.value, s.err
}

func validWorkflowTemplateForQuery() template.WorkflowTemplate {
	retry := template.RetryPolicy{
		Ref: "retry-v1", MaxAttempts: 2,
		RetryableReasonCodes: []string{"provider_unavailable"},
		Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
		AttemptTimeoutMS:     5000,
	}
	propagation := template.PropagationPolicy{
		OnDegraded: domain.PropagateAccept, OnSkipped: domain.PropagatePartial,
		OnFailed: domain.PropagateFail, OnTimedOut: domain.PropagateFail, OnBlocked: domain.PropagateFail,
	}
	return template.WorkflowTemplate{
		Ref: "stock_research", Version: "1", ParameterSchemaRef: "params-v1",
		OutputRegistryVersion: "outputs-v1", ResourcePolicyRef: "resource-v1",
		Stages: []template.StageDefinition{{
			Code: "market", Required: true,
			Applicability: template.ApplicabilityRule{RuleRef: "always", RuleVersion: "1"},
			Retry:         retry, Propagation: propagation,
		}},
	}
}

func TestGetWorkflowTemplateReturnsExactVersionAndFingerprint(t *testing.T) {
	tmpl := validWorkflowTemplateForQuery()
	registry := &workflowTemplateRegistryStub{value: tmpl}
	service, err := NewGetWorkflowTemplateService(registry)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.GetWorkflowTemplate(context.Background(), "stock_research", "1")
	if err != nil {
		t.Fatal(err)
	}
	if registry.calls != 1 || registry.ref != "stock_research" || registry.version != "1" {
		t.Fatalf("registry lookup was not exact: %#v", registry)
	}
	if got.Template.Fingerprint == "" || got.Fingerprint != got.Template.Fingerprint {
		t.Fatalf("template fingerprint missing: %#v", got)
	}
	if tmpl.Fingerprint != "" {
		t.Fatal("query mutated registry-owned template")
	}
}

func TestGetWorkflowTemplateRejectsInvalidIdentityBeforeRegistry(t *testing.T) {
	registry := &workflowTemplateRegistryStub{value: validWorkflowTemplateForQuery()}
	service, err := NewGetWorkflowTemplateService(registry)
	if err != nil {
		t.Fatal(err)
	}
	for _, pair := range [][2]string{{"", "1"}, {"stock_research", ""}, {" stock_research", "1"}, {"stock_research", "1 "}, {"stock_research", strings.Repeat("v", 65)}} {
		if _, err := service.GetWorkflowTemplate(context.Background(), pair[0], pair[1]); err == nil {
			t.Fatalf("invalid identity accepted: %#v", pair)
		}
	}
	if registry.calls != 0 {
		t.Fatal("invalid template identity reached Registry")
	}
}

func TestGetWorkflowTemplateRejectsRegistryIdentityAndFingerprintMismatch(t *testing.T) {
	tmpl := validWorkflowTemplateForQuery()
	registry := &workflowTemplateRegistryStub{value: tmpl}
	service, err := NewGetWorkflowTemplateService(registry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetWorkflowTemplate(context.Background(), "other", "1"); !errors.Is(err, ErrTemplateVersionMismatch) {
		t.Fatalf("identity mismatch error=%v", err)
	}
	fingerprint, err := template.Fingerprint(tmpl)
	if err != nil {
		t.Fatal(err)
	}
	tmpl.Fingerprint = fingerprint
	tmpl.Stages[0].Priority++
	registry.value = tmpl
	if _, err := service.GetWorkflowTemplate(context.Background(), tmpl.Ref, tmpl.Version); !errors.Is(err, ErrWorkflowTemplateFingerprintMismatch) {
		t.Fatalf("fingerprint mismatch error=%v", err)
	}
}

func TestGetWorkflowTemplateReturnsOwnedSlices(t *testing.T) {
	tmpl := validWorkflowTemplateForQuery()
	registry := &workflowTemplateRegistryStub{value: tmpl}
	service, err := NewGetWorkflowTemplateService(registry)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.GetWorkflowTemplate(context.Background(), tmpl.Ref, tmpl.Version)
	if err != nil {
		t.Fatal(err)
	}
	got.Template.Stages[0].Dependencies = append(got.Template.Stages[0].Dependencies, "mutated")
	got.Template.Stages[0].Retry.RetryableReasonCodes[0] = "mutated"
	if len(registry.value.Stages[0].Dependencies) != 0 ||
		registry.value.Stages[0].Retry.RetryableReasonCodes[0] != "provider_unavailable" {
		t.Fatal("returned template shares mutable Registry slices")
	}
}

func TestGetWorkflowTemplatePropagatesRegistryError(t *testing.T) {
	registryErr := errors.New("registry unavailable")
	service, err := NewGetWorkflowTemplateService(&workflowTemplateRegistryStub{err: registryErr})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetWorkflowTemplate(context.Background(), "stock_research", "1"); !errors.Is(err, registryErr) {
		t.Fatalf("GetWorkflowTemplate error=%v, want Registry error", err)
	}
}
