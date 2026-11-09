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

// Public DTOs deliberately use Application-owned scalar/enumeration fields.
// No Domain Entity or Repository type crosses this boundary.
type ContextSectionRef struct {
	SectionID string `json:"section_id"`
	SectionVersion string `json:"section_version"`
	SourceOwner string `json:"source_owner"`
	SourceRefs []string `json:"source_refs"`
	EvidenceRefs []string `json:"evidence_refs"`
	QualityStatus string `json:"quality_status"`
	Required bool `json:"required"`
	Included bool `json:"included"`
	AssembledAt time.Time `json:"assembled_at"`
	SystemAsOf time.Time `json:"system_as_of"`
}
type ContextOmittedSection struct {
	SectionID string `json:"section_id"`
	SourceOwner string `json:"source_owner"`
	ReasonCode string `json:"reason_code"`
	Required bool `json:"required"`
	SourceRefs []string `json:"source_refs"`
	SafeMessage string `json:"safe_message"`
	Retryable bool `json:"retryable"`
}
type ContextQualitySummary struct {
	Status string `json:"status"`
	MissingRequiredCount int `json:"missing_required_count"`
	MissingOptionalCount int `json:"missing_optional_count"`
	ConflictCount int `json:"conflict_count"`
	StaleCount int `json:"stale_count"`
	ReasonCodes []string `json:"reason_codes"`
}
type ResearchContext struct {
	SchemaVersion string `json:"schema_version"`
	ContextID string `json:"context_id"`
	ContextVersion int64 `json:"context_version"`
	ResearchQuestion string `json:"research_question"`
	Scope string `json:"scope"`
	SubjectRefs []string `json:"subject_refs"`
	SystemAsOf time.Time `json:"system_as_of"`
	SectionRefs []ContextSectionRef `json:"section_refs"`
	QualitySummary ContextQualitySummary `json:"quality_summary"`
	IncludedSections []string `json:"included_sections"`
	OmittedSections []ContextOmittedSection `json:"omitted_sections"`
	BuilderVersion string `json:"builder_version"`
	CreatedAt time.Time `json:"created_at"`
}
type EvidenceDraft struct {
	EvidenceType string `json:"evidence_type"`
	SourceOwner string `json:"source_owner"`
	SourceRef string `json:"source_ref"`
	SourceVersion *string `json:"source_version"`
	SourceLocator *string `json:"source_locator"`
	DerivedFromRefs []string `json:"derived_from_refs"`
	ClaimRef *string `json:"claim_ref"`
	SubjectRefs []string `json:"subject_refs"`
	PublishedAt *time.Time `json:"published_at"`
	AvailableAt *time.Time `json:"available_at"`
	SystemObservedAt time.Time `json:"system_observed_at"`
	ValidTimeStart *time.Time `json:"valid_time_start"`
	ValidTimeEnd *time.Time `json:"valid_time_end"`
	SystemAsOf time.Time `json:"system_as_of"`
	Stance string `json:"stance"`
	Confidence *float64 `json:"confidence"`
	ConfidenceMethod *string `json:"confidence_method"`
	ConfidenceMethodVersion *string `json:"confidence_method_version"`
	QualityStatus string `json:"quality_status"`
	ContentLevel string `json:"content_level"`
	AuthorizationRef string `json:"authorization_ref"`
	Limitations []string `json:"limitations"`
}
type EvidenceRecord struct {
	EvidenceDraft
	EvidenceID string `json:"evidence_id"`
	ContentFingerprint string `json:"content_fingerprint"`
}

type CreateContextCommand struct {
	ResearchQuestion string
	Scope string
	SubjectRefs []string
	SystemAsOf time.Time
	SectionRefs []ContextSectionRef
	QualitySummary ContextQualitySummary
	IncludedSections []string
	OmittedSections []ContextOmittedSection
	BuilderVersion string
}
type ContextRef struct { ContextID string; ContextVersion int64 }
type AddEvidenceCommand struct {
	CallerRef string
	PurposeCode string
	// EvidenceID 与 ContentFingerprint 不接受调用方指定，由 Research 生成。
	Draft EvidenceDraft
}
type EvidenceRef struct { EvidenceID string; CallerRef string; PurposeCode string }

type ContextRepository interface {
	Append(context.Context, researchcontext.Context) error
	GetVersion(context.Context, string, int64) (researchcontext.Context, error)
}
type EvidenceRepository interface {
	Append(context.Context, researchevidence.Evidence) error
	Get(context.Context, string) (researchevidence.Evidence, error)
}
type EvidenceAuthorizer interface {
	AuthorizeEvidence(context.Context, string, researchevidence.PurposeCode, researchevidence.Evidence) error
}
type Port interface {
	CreateContext(context.Context, CreateContextCommand) (ResearchContext, error)
	AddEvidence(context.Context, AddEvidenceCommand) (EvidenceRecord, error)
	GetContext(context.Context, ContextRef) (ResearchContext, error)
	GetEvidence(context.Context, EvidenceRef) (EvidenceRecord, error)
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

func (s *Service) CreateContext(ctx context.Context, command CreateContextCommand) (ResearchContext, error) {
	if s == nil { return ResearchContext{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil { return ResearchContext{}, validationFault("request_context_required", "request context is required", nil) }
	id, err := s.ids.NewID(ContextIDPrefix)
	if err != nil { return ResearchContext{}, applicationFault(fault.ClassInternal, "research_id_generation_failed", "Research identifier generation failed", err) }
	now := s.clock().UTC()
	if now.IsZero() { return ResearchContext{}, applicationFault(fault.ClassInternal, "research_clock_invalid", "Research application clock is invalid", nil) }
	value := researchcontext.Context{
		SchemaVersion: researchcontext.SchemaVersion, ContextID: id, ContextVersion: 1,
		ResearchQuestion: command.ResearchQuestion, Scope: researchcontext.Scope(command.Scope),
		SubjectRefs: cloneStrings(command.SubjectRefs), SystemAsOf: command.SystemAsOf,
		SectionRefs: toDomainSections(command.SectionRefs), QualitySummary: toDomainQuality(command.QualitySummary),
		IncludedSections: cloneStrings(command.IncludedSections), OmittedSections: toDomainOmitted(command.OmittedSections),
		BuilderVersion: command.BuilderVersion, CreatedAt: now,
	}
	if err := value.Validate(); err != nil { return ResearchContext{}, validationFault("invalid_research_context", "Research Context is invalid", err) }
	if err := s.contexts.Append(ctx, value); err != nil { return ResearchContext{}, mapRepositoryError(err) }
	return fromDomainContext(value), nil
}

func (s *Service) AddEvidence(ctx context.Context, command AddEvidenceCommand) (EvidenceRecord, error) {
	if s == nil { return EvidenceRecord{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil { return EvidenceRecord{}, validationFault("request_context_required", "request context is required", nil) }
	if strings.TrimSpace(command.CallerRef) == "" { return EvidenceRecord{}, validationFault("caller_ref_required", "trusted caller reference is required", nil) }
	purpose := researchevidence.PurposeCode(command.PurposeCode)
	if purpose == "" { purpose = researchevidence.PurposeResearchAnalysis }
	value := toDomainEvidence(command.Draft)
	id, err := s.ids.NewID(EvidenceIDPrefix)
	if err != nil { return EvidenceRecord{}, applicationFault(fault.ClassInternal, "research_id_generation_failed", "Research identifier generation failed", err) }
	value.EvidenceID = id
	fingerprint, err := value.ComputeContentFingerprint()
	if err != nil { return EvidenceRecord{}, validationFault("invalid_evidence", "Research Evidence is invalid", err) }
	value.ContentFingerprint = fingerprint
	if err := value.Validate(); err != nil { return EvidenceRecord{}, validationFault("invalid_evidence", "Research Evidence is invalid", err) }
	if err := s.authorizer.AuthorizeEvidence(ctx, command.CallerRef, purpose, value); err != nil {
		return EvidenceRecord{}, applicationFault(fault.ClassForbidden, "evidence_authorization_rejected", "Evidence is not authorized for this Research operation", err)
	}
	if err := s.evidence.Append(ctx, value); err != nil { return EvidenceRecord{}, mapRepositoryError(err) }
	return fromDomainEvidence(value), nil
}

func (s *Service) GetContext(ctx context.Context, ref ContextRef) (ResearchContext, error) {
	if s == nil { return ResearchContext{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || strings.TrimSpace(ref.ContextID) == "" || ref.ContextVersion < 1 {
		return ResearchContext{}, validationFault("invalid_context_ref", "Context reference is invalid", nil)
	}
	value, err := s.contexts.GetVersion(ctx, ref.ContextID, ref.ContextVersion)
	if err != nil { return ResearchContext{}, mapRepositoryError(err) }
	if err := value.Validate(); err != nil { return ResearchContext{}, applicationFault(fault.ClassInternal, "stored_context_invalid", "Stored Research Context is invalid", err) }
	return fromDomainContext(value), nil
}

func (s *Service) GetEvidence(ctx context.Context, ref EvidenceRef) (EvidenceRecord, error) {
	if s == nil { return EvidenceRecord{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || strings.TrimSpace(ref.EvidenceID) == "" || strings.TrimSpace(ref.CallerRef) == "" {
		return EvidenceRecord{}, validationFault("invalid_evidence_ref", "Evidence reference and trusted caller are required", nil)
	}
	value, err := s.evidence.Get(ctx, ref.EvidenceID)
	if err != nil { return EvidenceRecord{}, mapRepositoryError(err) }
	if err := value.Validate(); err != nil { return EvidenceRecord{}, applicationFault(fault.ClassInternal, "stored_evidence_invalid", "Stored Research Evidence is invalid", err) }
	purpose := researchevidence.PurposeCode(ref.PurposeCode)
	if purpose == "" { purpose = researchevidence.PurposeResearchAnalysis }
	if err := s.authorizer.AuthorizeEvidence(ctx, ref.CallerRef, purpose, value); err != nil {
		return EvidenceRecord{}, applicationFault(fault.ClassForbidden, "evidence_authorization_rejected", "Evidence is not currently authorized for this Research operation", err)
	}
	return fromDomainEvidence(value), nil
}

func fromDomainContext(value researchcontext.Context) ResearchContext {
	out := ResearchContext{
		SchemaVersion:value.SchemaVersion,ContextID:value.ContextID,ContextVersion:value.ContextVersion,
		ResearchQuestion:value.ResearchQuestion,Scope:string(value.Scope),SubjectRefs:cloneStrings(value.SubjectRefs),
		SystemAsOf:value.SystemAsOf,QualitySummary:ContextQualitySummary{
			Status:string(value.QualitySummary.Status),MissingRequiredCount:value.QualitySummary.MissingRequiredCount,
			MissingOptionalCount:value.QualitySummary.MissingOptionalCount,ConflictCount:value.QualitySummary.ConflictCount,
			StaleCount:value.QualitySummary.StaleCount,ReasonCodes:cloneStrings(value.QualitySummary.ReasonCodes),
		},IncludedSections:cloneStrings(value.IncludedSections),BuilderVersion:value.BuilderVersion,CreatedAt:value.CreatedAt,
	}
	out.SectionRefs=make([]ContextSectionRef,len(value.SectionRefs))
	for i,v:=range value.SectionRefs { out.SectionRefs[i]=ContextSectionRef{SectionID:v.SectionID,SectionVersion:v.SectionVersion,SourceOwner:string(v.SourceOwner),SourceRefs:cloneStrings(v.SourceRefs),EvidenceRefs:cloneStrings(v.EvidenceRefs),QualityStatus:string(v.QualityStatus),Required:v.Required,Included:v.Included,AssembledAt:v.AssembledAt,SystemAsOf:v.SystemAsOf} }
	out.OmittedSections=make([]ContextOmittedSection,len(value.OmittedSections))
	for i,v:=range value.OmittedSections { out.OmittedSections[i]=ContextOmittedSection{SectionID:v.SectionID,SourceOwner:string(v.SourceOwner),ReasonCode:string(v.ReasonCode),Required:v.Required,SourceRefs:cloneStrings(v.SourceRefs),SafeMessage:v.SafeMessage,Retryable:v.Retryable} }
	return out
}
func toDomainSections(values []ContextSectionRef) []researchcontext.SectionRef {
	out:=make([]researchcontext.SectionRef,len(values))
	for i,v:=range values { out[i]=researchcontext.SectionRef{SectionID:v.SectionID,SectionVersion:v.SectionVersion,SourceOwner:researchcontext.SourceOwner(v.SourceOwner),SourceRefs:cloneStrings(v.SourceRefs),EvidenceRefs:cloneStrings(v.EvidenceRefs),QualityStatus:researchcontext.QualityStatus(v.QualityStatus),Required:v.Required,Included:v.Included,AssembledAt:v.AssembledAt,SystemAsOf:v.SystemAsOf} }
	return out
}
func toDomainOmitted(values []ContextOmittedSection) []researchcontext.OmittedSection {
	out:=make([]researchcontext.OmittedSection,len(values))
	for i,v:=range values { out[i]=researchcontext.OmittedSection{SectionID:v.SectionID,SourceOwner:researchcontext.SourceOwner(v.SourceOwner),ReasonCode:researchcontext.OmissionReason(v.ReasonCode),Required:v.Required,SourceRefs:cloneStrings(v.SourceRefs),SafeMessage:v.SafeMessage,Retryable:v.Retryable} }
	return out
}
func toDomainQuality(v ContextQualitySummary) researchcontext.QualitySummary {
	return researchcontext.QualitySummary{Status:researchcontext.QualityStatus(v.Status),MissingRequiredCount:v.MissingRequiredCount,MissingOptionalCount:v.MissingOptionalCount,ConflictCount:v.ConflictCount,StaleCount:v.StaleCount,ReasonCodes:cloneStrings(v.ReasonCodes)}
}
func toDomainEvidence(v EvidenceDraft) researchevidence.Evidence {
	return researchevidence.Evidence{EvidenceType:researchevidence.EvidenceType(v.EvidenceType),SourceOwner:researchevidence.SourceOwner(v.SourceOwner),SourceRef:v.SourceRef,SourceVersion:v.SourceVersion,SourceLocator:v.SourceLocator,DerivedFromRefs:cloneStrings(v.DerivedFromRefs),ClaimRef:v.ClaimRef,SubjectRefs:cloneStrings(v.SubjectRefs),PublishedAt:v.PublishedAt,AvailableAt:v.AvailableAt,SystemObservedAt:v.SystemObservedAt,ValidTimeStart:v.ValidTimeStart,ValidTimeEnd:v.ValidTimeEnd,SystemAsOf:v.SystemAsOf,Stance:researchevidence.Stance(v.Stance),Confidence:v.Confidence,ConfidenceMethod:v.ConfidenceMethod,ConfidenceMethodVersion:v.ConfidenceMethodVersion,QualityStatus:researchevidence.QualityStatus(v.QualityStatus),ContentLevel:researchevidence.ContentLevel(v.ContentLevel),AuthorizationRef:v.AuthorizationRef,Limitations:cloneStrings(v.Limitations)}
}
func fromDomainEvidence(v researchevidence.Evidence) EvidenceRecord {
	return EvidenceRecord{EvidenceDraft:EvidenceDraft{EvidenceType:string(v.EvidenceType),SourceOwner:string(v.SourceOwner),SourceRef:v.SourceRef,SourceVersion:v.SourceVersion,SourceLocator:v.SourceLocator,DerivedFromRefs:cloneStrings(v.DerivedFromRefs),ClaimRef:v.ClaimRef,SubjectRefs:cloneStrings(v.SubjectRefs),PublishedAt:v.PublishedAt,AvailableAt:v.AvailableAt,SystemObservedAt:v.SystemObservedAt,ValidTimeStart:v.ValidTimeStart,ValidTimeEnd:v.ValidTimeEnd,SystemAsOf:v.SystemAsOf,Stance:string(v.Stance),Confidence:v.Confidence,ConfidenceMethod:v.ConfidenceMethod,ConfidenceMethodVersion:v.ConfidenceMethodVersion,QualityStatus:string(v.QualityStatus),ContentLevel:string(v.ContentLevel),AuthorizationRef:v.AuthorizationRef,Limitations:cloneStrings(v.Limitations)},EvidenceID:v.EvidenceID,ContentFingerprint:v.ContentFingerprint}
}

func mapRepositoryError(err error) error {
	switch {
	case errors.Is(err, researcherrors.ErrNotFound): return applicationFault(fault.ClassNotFound, "research_record_not_found", "Research record was not found", err)
	case errors.Is(err, researcherrors.ErrConflict): return applicationFault(fault.ClassConflict, "research_record_conflict", "Research record conflicts with an existing version", err)
	case errors.Is(err, researcherrors.ErrCorruptRecord): return applicationFault(fault.ClassInternal, "research_record_corrupt", "Stored Research record failed integrity validation", err)
	case errors.Is(err, context.Canceled): return applicationFault(fault.ClassTimeout, "research_request_canceled", "Research request was canceled", err)
	case errors.Is(err, context.DeadlineExceeded): return applicationFault(fault.ClassTimeout, "research_request_timeout", "Research request deadline exceeded", err)
	default: return applicationFault(fault.ClassInternal, "research_operation_failed", "Research operation failed", err)
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
func cloneStrings(values []string) []string {
	if values == nil { return []string{} }
	return append([]string(nil), values...)
}
