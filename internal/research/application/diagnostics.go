package application

import (
	"context"
	"strings"
	"time"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	"example.com/stock-analysis-flow/internal/platform/fault"
	researcherrors "example.com/stock-analysis-flow/internal/research"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchdiagnostics "example.com/stock-analysis-flow/internal/research/diagnostics"
)

type DiagnosticsPort interface {
	BuildContextCheck(context.Context, ContextRef, time.Time, bool) (platformdiagnostics.Check, error)
	BuildEventCheck(context.Context, EventRef, time.Time, bool) (platformdiagnostics.Check, error)
	BuildClaimCheck(context.Context, ClaimRef, time.Time, bool) (platformdiagnostics.Check, error)
	BuildEvidenceCheck(context.Context, EvidenceRef, time.Time, bool) (platformdiagnostics.Check, error)
}

var _ DiagnosticsPort = (*Service)(nil)

// BuildContextCheck 从指定的不可变 Context 版本构造诊断项；执行调度由调用方交给 Platform Runner。
func (s *Service) BuildContextCheck(ctx context.Context, ref ContextRef, runAsOf time.Time, required bool) (platformdiagnostics.Check, error) {
	if s == nil || s.contexts == nil {
		return platformdiagnostics.Check{}, applicationFault(fault.ClassInternal, "research_context_repository_not_configured", "Research Context Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil || strings.TrimSpace(ref.ContextID) == "" || ref.ContextVersion < 1 {
		return platformdiagnostics.Check{}, validationFault("invalid_context_ref", "Context reference is invalid", nil)
	}
	if err := validateDiagnosticRunCutoff(runAsOf); err != nil {
		return platformdiagnostics.Check{}, err
	}
	value, err := s.GetContext(ctx, ref)
	if err != nil {
		return platformdiagnostics.Check{}, err
	}
	return researchdiagnostics.NewContextCheck("research.context.snapshot", "Research Context 快照", required, toDomainContext(value), runAsOf)
}

// BuildEventCheck 从指定的不可变 Event 版本构造诊断项。
func (s *Service) BuildEventCheck(ctx context.Context, ref EventRef, runAsOf time.Time, required bool) (platformdiagnostics.Check, error) {
	if s == nil || s.events == nil {
		return platformdiagnostics.Check{}, applicationFault(fault.ClassInternal, "research_event_repository_not_configured", "Research Event Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil || strings.TrimSpace(ref.EventID) == "" || ref.EventVersion < 1 {
		return platformdiagnostics.Check{}, validationFault("invalid_event_ref", "Event reference is invalid", nil)
	}
	if err := validateDiagnosticRunCutoff(runAsOf); err != nil {
		return platformdiagnostics.Check{}, err
	}
	value, err := s.events.GetVersion(ctx, ref.EventID, ref.EventVersion)
	if err != nil {
		return platformdiagnostics.Check{}, mapRepositoryError(err)
	}
	if value.EventID != ref.EventID || value.EventVersion != ref.EventVersion {
		return platformdiagnostics.Check{}, applicationFault(fault.ClassInternal, "stored_event_identity_mismatch", "Stored Research Event identity does not match the requested version", researcherrors.ErrCorruptRecord)
	}
	if err := value.Validate(); err != nil {
		return platformdiagnostics.Check{}, applicationFault(fault.ClassInternal, "stored_event_invalid", "Stored Research Event is invalid", err)
	}
	return researchdiagnostics.NewEventCheck("research.event.snapshot", "Research Event 快照", required, value, runAsOf)
}

// BuildClaimCheck 从指定的不可变 Claim 版本构造诊断项。
func (s *Service) BuildClaimCheck(ctx context.Context, ref ClaimRef, runAsOf time.Time, required bool) (platformdiagnostics.Check, error) {
	if s == nil || s.claims == nil {
		return platformdiagnostics.Check{}, applicationFault(fault.ClassInternal, "research_claim_repository_not_configured", "Research Claim Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil || strings.TrimSpace(ref.ClaimID) == "" || ref.ClaimVersion < 1 {
		return platformdiagnostics.Check{}, validationFault("invalid_claim_ref", "Claim reference is invalid", nil)
	}
	if err := validateDiagnosticRunCutoff(runAsOf); err != nil {
		return platformdiagnostics.Check{}, err
	}
	value, err := s.claims.GetVersion(ctx, ref.ClaimID, ref.ClaimVersion)
	if err != nil {
		return platformdiagnostics.Check{}, mapRepositoryError(err)
	}
	if value.ClaimID != ref.ClaimID || value.ClaimVersion != ref.ClaimVersion {
		return platformdiagnostics.Check{}, applicationFault(fault.ClassInternal, "stored_claim_identity_mismatch", "Stored Research Claim identity does not match the requested version", researcherrors.ErrCorruptRecord)
	}
	if err := value.Validate(); err != nil {
		return platformdiagnostics.Check{}, applicationFault(fault.ClassInternal, "stored_claim_invalid", "Stored Research Claim is invalid", err)
	}
	return researchdiagnostics.NewClaimCheck("research.claim.snapshot", "Research Claim 快照", required, value, runAsOf)
}

// BuildEvidenceCheck 先通过来源 Owner 当前授权复核通过后，再构造不含证据正文的诊断项。
func (s *Service) BuildEvidenceCheck(ctx context.Context, ref EvidenceRef, runAsOf time.Time, required bool) (platformdiagnostics.Check, error) {
	if s == nil || s.evidence == nil || s.authorizer == nil {
		return platformdiagnostics.Check{}, applicationFault(fault.ClassInternal, "research_evidence_dependencies_not_configured", "Research Evidence dependencies are not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil || strings.TrimSpace(ref.EvidenceID) == "" || strings.TrimSpace(ref.CallerRef) == "" {
		return platformdiagnostics.Check{}, validationFault("invalid_evidence_ref", "Evidence reference and trusted caller are required", nil)
	}
	if err := validateDiagnosticRunCutoff(runAsOf); err != nil {
		return platformdiagnostics.Check{}, err
	}

	record, err := s.GetEvidence(ctx, ref)
	if err != nil {
		return platformdiagnostics.Check{}, err
	}
	value := toDomainEvidence(record.EvidenceDraft)
	value.EvidenceID = record.EvidenceID
	value.ContentFingerprint = record.ContentFingerprint
	return researchdiagnostics.NewEvidenceCheck("research.evidence.snapshot", "Research Evidence 快照", required, value, runAsOf)
}

// toDomainContext 将 Application DTO 转换为独立的 Domain 快照，供领域诊断器校验。
func toDomainContext(value ResearchContext) researchcontext.Context {
	return researchcontext.Context{
		SchemaVersion:    value.SchemaVersion,
		ContextID:        value.ContextID,
		ContextVersion:   value.ContextVersion,
		ResearchQuestion: value.ResearchQuestion,
		Scope:            researchcontext.Scope(value.Scope),
		SubjectRefs:      cloneStrings(value.SubjectRefs),
		SystemAsOf:       value.SystemAsOf,
		SectionRefs:      toDomainSections(value.SectionRefs),
		QualitySummary:   toDomainQuality(value.QualitySummary),
		IncludedSections: cloneStrings(value.IncludedSections),
		OmittedSections:  toDomainOmitted(value.OmittedSections),
		BuilderVersion:   value.BuilderVersion,
		CreatedAt:        value.CreatedAt,
	}
}

// validateDiagnosticRunCutoff validates the authoritative UTC cutoff supplied by Researchflow CreateRun.
func validateDiagnosticRunCutoff(value time.Time) error {
	if value.IsZero() || value.Location() != time.UTC {
		return validationFault("invalid_run_system_as_of", "Run system_as_of is required and must use UTC", nil)
	}
	return nil
}
