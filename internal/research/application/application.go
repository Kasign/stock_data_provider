// Package application exposes the stable Research Application Port.
// Domain entities and Repository implementations remain private to callers.
package application

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	researcherrors "example.com/stock-analysis-flow/internal/research"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
	"example.com/stock-analysis-flow/internal/platform/fault"
)

const (
	ContextIDPrefix = "ctx_"
	EvidenceIDPrefix = "ev_"
)

type Clock func() time.Time

type IDGenerator interface { NewID(prefix string) (string, error) }

// EvidenceAuthorizer must delegate current policy/revocation checks to the source owner.
type EvidenceAuthorizer interface {
	AuthorizeEvidence(context.Context, string, researchevidence.PurposeCode, researchevidence.Evidence) error
}

type ContextRepository interface {
	Append(context.Context, researchcontext.Context) error
	GetVersion(context.Context, string, int64) (researchcontext.Context, error)
}
type EvidenceRepository interface {
	Append(context.Context, researchevidence.Evidence) error
	Get(context.Context, string) (researchevidence.Evidence, error)
}

type CreateContextCommand struct {
	ResearchQuestion string
	Scope researchcontext.Scope
	SubjectRefs []string
	SystemAsOf time.Time
	SectionRefs []researchcontext.SectionRef
	QualitySummary researchcontext.QualitySummary
	IncludedSections []string
	OmittedSections []researchcontext.OmittedSection
	BuilderVersion string
}
type ContextRef struct { ContextID string; ContextVersion int64 }

type AddEvidenceCommand struct {
	CallerRef string
	PurposeCode researchevidence.PurposeCode
	// Draft 的 EvidenceID 与 ContentFingerprint 会被服务端忽略并重新生成。
	Draft researchevidence.Evidence
}
type EvidenceRef struct { EvidenceID string; CallerRef string; PurposeCode researchevidence.PurposeCode }

type Port interface {
	CreateContext(context.Context, CreateContextCommand) (researchcontext.Context, error)
	AddEvidence(context.Context, AddEvidenceCommand) (researchevidence.Evidence, error)
	GetContext(context.Context, ContextRef) (researchcontext.Context, error)
	GetEvidence(context.Context, EvidenceRef) (researchevidence.Evidence, error)
}

type Service struct {
	contexts ContextRepository
	evidence EvidenceRepository
	authorizer EvidenceAuthorizer
	clock Clock
	ids IDGenerator
}
var _ Port = (*Service)(nil)

func NewService(contexts ContextRepository, evidence EvidenceRepository, authorizer EvidenceAuthorizer, clock Clock, ids IDGenerator) (*Service, error) {
	if contexts == nil || evidence == nil || authorizer == nil {
		return nil, fault.New(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository)
	}
	if clock == nil { clock = func() time.Time { return time.Now().UTC() } }
	if ids == nil { ids = randomIDGenerator{} }
	return &Service{contexts: contexts, evidence: evidence, authorizer: authorizer, clock: clock, ids: ids}, nil
}

func (s *Service) CreateContext(ctx context.Context, command CreateContextCommand) (researchcontext.Context, error) {
	if s == nil { return researchcontext.Context{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil { return researchcontext.Context{}, validationFault("request_context_required", "request context is required", nil) }
	id, err := s.ids.NewID(ContextIDPrefix)
	if err != nil { return researchcontext.Context{}, applicationFault(fault.ClassInternal, "research_id_generation_failed", "Research identifier generation failed", err) }
	now := s.clock().UTC()
	if now.IsZero() { return researchcontext.Context{}, applicationFault(fault.ClassInternal, "research_clock_invalid", "Research application clock is invalid", nil) }
	value := researchcontext.Context{
		SchemaVersion: researchcontext.SchemaVersion, ContextID: id, ContextVersion: 1,
		ResearchQuestion: command.ResearchQuestion, Scope: command.Scope,
		SubjectRefs: cloneStrings(command.SubjectRefs), SystemAsOf: command.SystemAsOf,
		SectionRefs: cloneSectionRefs(command.SectionRefs), QualitySummary: cloneQualitySummary(command.QualitySummary),
		IncludedSections: cloneStrings(command.IncludedSections), OmittedSections: cloneOmittedSections(command.OmittedSections),
		BuilderVersion: command.BuilderVersion, CreatedAt: now,
	}
	if err := value.Validate(); err != nil { return researchcontext.Context{}, validationFault("invalid_research_context", "Research Context is invalid", err) }
	if err := s.contexts.Append(ctx, value); err != nil { return researchcontext.Context{}, mapRepositoryError(err) }
	return value, nil
}

func (s *Service) AddEvidence(ctx context.Context, command AddEvidenceCommand) (researchevidence.Evidence, error) {
	if s == nil { return researchevidence.Evidence{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil { return researchevidence.Evidence{}, validationFault("request_context_required", "request context is required", nil) }
	if strings.TrimSpace(command.CallerRef) == "" { return researchevidence.Evidence{}, validationFault("caller_ref_required", "trusted caller reference is required", nil) }
	purpose := command.PurposeCode
	if purpose == "" { purpose = researchevidence.PurposeResearchAnalysis }
	value := command.Draft
	value.EvidenceID = ""
	value.ContentFingerprint = ""
	value.DerivedFromRefs = cloneStrings(value.DerivedFromRefs)
	value.SubjectRefs = cloneStrings(value.SubjectRefs)
	value.Limitations = cloneStrings(value.Limitations)
	id, err := s.ids.NewID(EvidenceIDPrefix)
	if err != nil { return researchevidence.Evidence{}, applicationFault(fault.ClassInternal, "research_id_generation_failed", "Research identifier generation failed", err) }
	value.EvidenceID = id
	fingerprint, err := value.ComputeContentFingerprint()
	if err != nil { return researchevidence.Evidence{}, validationFault("invalid_evidence", "Research Evidence is invalid", err) }
	value.ContentFingerprint = fingerprint
	if err := value.Validate(); err != nil { return researchevidence.Evidence{}, validationFault("invalid_evidence", "Research Evidence is invalid", err) }
	if err := s.authorizer.AuthorizeEvidence(ctx, command.CallerRef, purpose, value); err != nil {
		return researchevidence.Evidence{}, applicationFault(fault.ClassForbidden, "evidence_authorization_rejected", "Evidence is not authorized for this Research operation", err)
	}
	if err := s.evidence.Append(ctx, value); err != nil { return researchevidence.Evidence{}, mapRepositoryError(err) }
	return value, nil
}

func (s *Service) GetContext(ctx context.Context, ref ContextRef) (researchcontext.Context, error) {
	if s == nil { return researchcontext.Context{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || strings.TrimSpace(ref.ContextID) == "" || ref.ContextVersion < 1 {
		return researchcontext.Context{}, validationFault("invalid_context_ref", "Context reference is invalid", nil)
	}
	value, err := s.contexts.GetVersion(ctx, ref.ContextID, ref.ContextVersion)
	if err != nil { return researchcontext.Context{}, mapRepositoryError(err) }
	if err := value.Validate(); err != nil { return researchcontext.Context{}, applicationFault(fault.ClassInternal, "stored_context_invalid", "Stored Research Context is invalid", err) }
	return value, nil
}

func (s *Service) GetEvidence(ctx context.Context, ref EvidenceRef) (researchevidence.Evidence, error) {
	if s == nil { return researchevidence.Evidence{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || strings.TrimSpace(ref.EvidenceID) == "" || strings.TrimSpace(ref.CallerRef) == "" {
		return researchevidence.Evidence{}, validationFault("invalid_evidence_ref", "Evidence reference and trusted caller are required", nil)
	}
	value, err := s.evidence.Get(ctx, ref.EvidenceID)
	if err != nil { return researchevidence.Evidence{}, mapRepositoryError(err) }
	if err := value.Validate(); err != nil { return researchevidence.Evidence{}, applicationFault(fault.ClassInternal, "stored_evidence_invalid", "Stored Research Evidence is invalid", err) }
	purpose := ref.PurposeCode
	if purpose == "" { purpose = researchevidence.PurposeResearchAnalysis }
	if err := s.authorizer.AuthorizeEvidence(ctx, ref.CallerRef, purpose, value); err != nil {
		return researchevidence.Evidence{}, applicationFault(fault.ClassForbidden, "evidence_authorization_rejected", "Evidence is not currently authorized for this Research operation", err)
	}
	return value, nil
}

func mapRepositoryError(err error) error {
	switch {
	case errors.Is(err, researcherrors.ErrNotFound):
		return applicationFault(fault.ClassNotFound, "research_record_not_found", "Research record was not found", err)
	case errors.Is(err, researcherrors.ErrConflict):
		return applicationFault(fault.ClassConflict, "research_record_conflict", "Research record conflicts with an existing version", err)
	case errors.Is(err, researcherrors.ErrCorruptRecord):
		return applicationFault(fault.ClassInternal, "research_record_corrupt", "Stored Research record failed integrity validation", err)
	case errors.Is(err, context.Canceled):
		return applicationFault(fault.ClassTimeout, "research_request_canceled", "Research request was canceled", err)
	case errors.Is(err, context.DeadlineExceeded):
		return applicationFault(fault.ClassTimeout, "research_request_timeout", "Research request deadline exceeded", err)
	default:
		return applicationFault(fault.ClassInternal, "research_operation_failed", "Research operation failed", err)
	}
}
func validationFault(code, message string, cause error) error { return fault.New(fault.ClassValidation, code, message, cause) }
func applicationFault(class fault.Class, code, message string, cause error) error { return fault.New(class, code, message, cause) }

type randomIDGenerator struct{}
func (randomIDGenerator) NewID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil { return "", fmt.Errorf("generate opaque Research ID: %w", err) }
	return prefix + hex.EncodeToString(raw[:]), nil
}
func cloneQualitySummary(value researchcontext.QualitySummary) researchcontext.QualitySummary {
	value.ReasonCodes = cloneStrings(value.ReasonCodes)
	return value
}
func cloneStrings(values []string) []string {
	if values == nil { return []string{} }
	return append([]string(nil), values...)
}
func cloneSectionRefs(values []researchcontext.SectionRef) []researchcontext.SectionRef {
	out := append([]researchcontext.SectionRef(nil), values...)
	for i := range out {
		out[i].SourceRefs = cloneStrings(values[i].SourceRefs)
		out[i].EvidenceRefs = cloneStrings(values[i].EvidenceRefs)
	}
	if out == nil { return []researchcontext.SectionRef{} }
	return out
}
func cloneOmittedSections(values []researchcontext.OmittedSection) []researchcontext.OmittedSection {
	out := append([]researchcontext.OmittedSection(nil), values...)
	for i := range out { out[i].SourceRefs = cloneStrings(values[i].SourceRefs) }
	if out == nil { return []researchcontext.OmittedSection{} }
	return out
}
