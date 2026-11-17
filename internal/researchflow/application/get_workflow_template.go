package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var ErrWorkflowTemplateFingerprintMismatch = errors.New("Researchflow workflow template fingerprint mismatch")

type GetWorkflowTemplateResult struct {
	Template    template.WorkflowTemplate
	Fingerprint string
}

// GetWorkflowTemplateService exposes exact immutable template versions.
// It deliberately has no "latest" lookup or fallback behavior.
type GetWorkflowTemplateService struct {
	registry TemplateRegistry
}

func NewGetWorkflowTemplateService(registry TemplateRegistry) (*GetWorkflowTemplateService, error) {
	if registry == nil {
		return nil, errors.New("Researchflow Template Registry is required")
	}
	return &GetWorkflowTemplateService{registry: registry}, nil
}

func (s *GetWorkflowTemplateService) GetWorkflowTemplate(ctx context.Context, ref, version string) (GetWorkflowTemplateResult, error) {
	if s == nil || s.registry == nil {
		return GetWorkflowTemplateResult{}, errors.New("Researchflow GetWorkflowTemplate service is not configured")
	}
	if !validTemplateIdentity(ref) || !validTemplateVersion(version) {
		return GetWorkflowTemplateResult{}, fmt.Errorf("%w: exact template ref and version are required", template.ErrInvalidTemplate)
	}
	tmpl, err := s.registry.Get(ctx, ref, version)
	if err != nil {
		return GetWorkflowTemplateResult{}, fmt.Errorf("load Researchflow workflow template %q version %q: %w", ref, version, err)
	}
	if tmpl.Ref != ref || tmpl.Version != version {
		return GetWorkflowTemplateResult{}, ErrTemplateVersionMismatch
	}
	fingerprint, err := template.Fingerprint(tmpl)
	if err != nil {
		return GetWorkflowTemplateResult{}, fmt.Errorf("fingerprint Researchflow workflow template: %w", err)
	}
	if tmpl.Fingerprint != "" && tmpl.Fingerprint != fingerprint {
		return GetWorkflowTemplateResult{}, ErrWorkflowTemplateFingerprintMismatch
	}
	tmpl = cloneWorkflowTemplate(tmpl)
	tmpl.Fingerprint = fingerprint
	return GetWorkflowTemplateResult{Template: tmpl, Fingerprint: fingerprint}, nil
}

func validTemplateVersion(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && len(value) <= 64
}

func validTemplateIdentity(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && len(value) <= 255
}

func cloneWorkflowTemplate(source template.WorkflowTemplate) template.WorkflowTemplate {
	clone := source
	clone.Stages = append([]template.StageDefinition(nil), source.Stages...)
	for i := range clone.Stages {
		clone.Stages[i].Dependencies = append([]string(nil), source.Stages[i].Dependencies...)
		clone.Stages[i].Retry.RetryableReasonCodes = append([]string(nil), source.Stages[i].Retry.RetryableReasonCodes...)
	}
	return clone
}
