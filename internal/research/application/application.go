// Package application exposes the stable Research Application Port.
// Domain entities and Repository implementations remain private to callers.
package application

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	researcherrors "example.com/stock-analysis-flow/internal/research"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
	"example.com/stock-analysis-flow/internal/platform/fault"
)

const (
	ContextIDPrefix = "ctx_"
	EvidenceIDPrefix = "ev_"
	EventIDPrefix = "evt_"
	ClaimIDPrefix = "clm_"
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


type EventSourceTime struct {
	EvidenceRef string `json:"evidence_ref"`
	TimeType string `json:"time_type"`
	Value time.Time `json:"value"`
	AssertedBy string `json:"asserted_by"`
	Confidence *float64 `json:"confidence"`
	ConfidenceMethod *string `json:"confidence_method"`
	ConfidenceMethodVersion *string `json:"confidence_method_version"`
	Notes *string `json:"notes"`
}
type ResearchEvent struct {
	EventID string `json:"event_id"`
	EventVersion int `json:"event_version"`
	EventType string `json:"event_type"`
	Title string `json:"title"`
	Status string `json:"status"`
	SubjectRefs []string `json:"subject_refs"`
	EvidenceRefs []string `json:"evidence_refs"`
	OccurredAt *time.Time `json:"occurred_at"`
	PublishedAt *time.Time `json:"published_at"`
	AvailableAt *time.Time `json:"available_at"`
	SystemObservedAt time.Time `json:"system_observed_at"`
	SystemAsOf time.Time `json:"system_as_of"`
	ImpactHypothesisRefs []string `json:"impact_hypothesis_refs"`
	InvalidationConditions []string `json:"invalidation_conditions"`
	Uncertainty []string `json:"uncertainty"`
	SupersedesEventVersion *int `json:"supersedes_event_version"`
	SourceTimes []EventSourceTime `json:"source_times"`
}
type CreateEventCommand struct {
	EventType string
	Title string
	Status string
	SubjectRefs []string
	EvidenceRefs []string
	OccurredAt *time.Time
	PublishedAt *time.Time
	AvailableAt *time.Time
	SystemObservedAt time.Time
	SystemAsOf time.Time
	ImpactHypothesisRefs []string
	InvalidationConditions []string
	Uncertainty []string
	SupersedesEventVersion *int
	SourceTimes []EventSourceTime
}
type EventRef struct { EventID string; EventVersion int }
type EventRepository interface {
	Append(context.Context, researchevent.ResearchEvent) error
	AppendWithLifecycle(context.Context, researchevent.ResearchEvent, *string, string) error
	GetVersion(context.Context, string, int) (researchevent.ResearchEvent, error)
}
type UpdateEventCommand struct {
	EventID string
	ExpectedVersion int
	Revision CreateEventCommand
	ReasonCode *string
}
type EventPort interface {
	CreateEvent(context.Context, CreateEventCommand) (ResearchEvent, error)
	GetEvent(context.Context, EventRef) (ResearchEvent, error)
	UpdateEvent(context.Context, UpdateEventCommand) (ResearchEvent, error)
}


type Claim struct {
	ClaimID string `json:"claim_id"`
	ClaimVersion int `json:"claim_version"`
	Statement string `json:"statement"`
	EpistemicType string `json:"epistemic_type"`
	ScopeRefs []string `json:"scope_refs"`
	SystemAsOf time.Time `json:"system_as_of"`
	SupportingEvidenceRefs []string `json:"supporting_evidence_refs"`
	ContradictingEvidenceRefs []string `json:"contradicting_evidence_refs"`
	RequiredDataRefs []string `json:"required_data_refs"`
	ValidationMethod *string `json:"validation_method"`
	ExpectedObservation *string `json:"expected_observation"`
	InvalidationConditions []string `json:"invalidation_conditions"`
	Status string `json:"status"`
	Limitations []string `json:"limitations"`
}
type CreateClaimCommand struct {
	Statement string
	EpistemicType string
	ScopeRefs []string
	SystemAsOf time.Time
	SupportingEvidenceRefs []string
	ContradictingEvidenceRefs []string
	RequiredDataRefs []string
	ValidationMethod *string
	ExpectedObservation *string
	InvalidationConditions []string
	Status string
	Limitations []string
}
type ClaimRef struct { ClaimID string; ClaimVersion int }
type ClaimRepository interface {
	Append(context.Context, researchclaim.Claim) error
	GetVersion(context.Context, string, int) (researchclaim.Claim, error)
}
type UpdateClaimCommand struct {
	ClaimID string
	ExpectedVersion int
	Revision CreateClaimCommand
}
type ClaimPort interface {
	CreateClaim(context.Context, CreateClaimCommand) (Claim, error)
	GetClaim(context.Context, ClaimRef) (Claim, error)
	UpdateClaim(context.Context, UpdateClaimCommand) (Claim, error)
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
	events EventRepository
	claims ClaimRepository
	clock Clock
	ids IDGenerator
}
var _ Port = (*Service)(nil)
var _ EventPort = (*Service)(nil)
var _ ClaimPort = (*Service)(nil)

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
	if err := s.authorizer.AuthorizeEvidence(ctx, command.CallerRef, purpose, cloneDomainEvidence(value)); err != nil {
		return EvidenceRecord{}, mapAuthorizationError(err)
	}
	if err := s.evidence.Append(ctx, cloneDomainEvidence(value)); err != nil { return EvidenceRecord{}, mapRepositoryError(err) }
	return fromDomainEvidence(value), nil
}

func (s *Service) GetContext(ctx context.Context, ref ContextRef) (ResearchContext, error) {
	if s == nil { return ResearchContext{}, applicationFault(fault.ClassInternal, "research_application_not_configured", "Research Application is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || strings.TrimSpace(ref.ContextID) == "" || ref.ContextVersion < 1 {
		return ResearchContext{}, validationFault("invalid_context_ref", "Context reference is invalid", nil)
	}
	value, err := s.contexts.GetVersion(ctx, ref.ContextID, ref.ContextVersion)
	if err != nil { return ResearchContext{}, mapRepositoryError(err) }
	if value.ContextID != ref.ContextID || value.ContextVersion != ref.ContextVersion {
		return ResearchContext{}, applicationFault(fault.ClassInternal, "stored_context_identity_mismatch", "Stored Research Context identity does not match the requested version", researcherrors.ErrCorruptRecord)
	}
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
	if value.EvidenceID != ref.EvidenceID {
		return EvidenceRecord{}, applicationFault(fault.ClassInternal, "stored_evidence_identity_mismatch", "Stored Research Evidence identity does not match the requested record", researcherrors.ErrCorruptRecord)
	}
	if err := value.Validate(); err != nil { return EvidenceRecord{}, applicationFault(fault.ClassInternal, "stored_evidence_invalid", "Stored Research Evidence is invalid", err) }
	purpose := researchevidence.PurposeCode(ref.PurposeCode)
	if purpose == "" { purpose = researchevidence.PurposeResearchAnalysis }
	if err := s.authorizer.AuthorizeEvidence(ctx, ref.CallerRef, purpose, cloneDomainEvidence(value)); err != nil {
		return EvidenceRecord{}, mapAuthorizationError(err)
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
	return researchevidence.Evidence{
		EvidenceType: researchevidence.EvidenceType(v.EvidenceType), SourceOwner: researchevidence.SourceOwner(v.SourceOwner),
		SourceRef: v.SourceRef, SourceVersion: cloneString(v.SourceVersion), SourceLocator: cloneString(v.SourceLocator),
		DerivedFromRefs: cloneStrings(v.DerivedFromRefs), ClaimRef: cloneString(v.ClaimRef), SubjectRefs: cloneStrings(v.SubjectRefs),
		PublishedAt: cloneTime(v.PublishedAt), AvailableAt: cloneTime(v.AvailableAt),
		SystemObservedAt: v.SystemObservedAt, ValidTimeStart: cloneTime(v.ValidTimeStart), ValidTimeEnd: cloneTime(v.ValidTimeEnd),
		SystemAsOf: v.SystemAsOf, Stance: researchevidence.Stance(v.Stance), Confidence: cloneFloat(v.Confidence),
		ConfidenceMethod: cloneString(v.ConfidenceMethod), ConfidenceMethodVersion: cloneString(v.ConfidenceMethodVersion),
		QualityStatus: researchevidence.QualityStatus(v.QualityStatus), ContentLevel: researchevidence.ContentLevel(v.ContentLevel),
		AuthorizationRef: v.AuthorizationRef, Limitations: cloneStrings(v.Limitations),
	}
}

func fromDomainEvidence(v researchevidence.Evidence) EvidenceRecord {
	return EvidenceRecord{EvidenceDraft: EvidenceDraft{
		EvidenceType: string(v.EvidenceType), SourceOwner: string(v.SourceOwner), SourceRef: v.SourceRef,
		SourceVersion: cloneString(v.SourceVersion), SourceLocator: cloneString(v.SourceLocator),
		DerivedFromRefs: cloneStrings(v.DerivedFromRefs), ClaimRef: cloneString(v.ClaimRef),
		SubjectRefs: cloneStrings(v.SubjectRefs), PublishedAt: cloneTime(v.PublishedAt), AvailableAt: cloneTime(v.AvailableAt),
		SystemObservedAt: v.SystemObservedAt, ValidTimeStart: cloneTime(v.ValidTimeStart), ValidTimeEnd: cloneTime(v.ValidTimeEnd),
		SystemAsOf: v.SystemAsOf, Stance: string(v.Stance), Confidence: cloneFloat(v.Confidence),
		ConfidenceMethod: cloneString(v.ConfidenceMethod), ConfidenceMethodVersion: cloneString(v.ConfidenceMethodVersion),
		QualityStatus: string(v.QualityStatus), ContentLevel: string(v.ContentLevel),
		AuthorizationRef: v.AuthorizationRef, Limitations: cloneStrings(v.Limitations),
	}, EvidenceID: v.EvidenceID, ContentFingerprint: v.ContentFingerprint}
}

func cloneDomainEvidence(v researchevidence.Evidence) researchevidence.Evidence {
	v.SourceVersion = cloneString(v.SourceVersion)
	v.SourceLocator = cloneString(v.SourceLocator)
	v.DerivedFromRefs = cloneStrings(v.DerivedFromRefs)
	v.ClaimRef = cloneString(v.ClaimRef)
	v.SubjectRefs = cloneStrings(v.SubjectRefs)
	v.PublishedAt = cloneTime(v.PublishedAt)
	v.AvailableAt = cloneTime(v.AvailableAt)
	v.ValidTimeStart = cloneTime(v.ValidTimeStart)
	v.ValidTimeEnd = cloneTime(v.ValidTimeEnd)
	v.Confidence = cloneFloat(v.Confidence)
	v.ConfidenceMethod = cloneString(v.ConfidenceMethod)
	v.ConfidenceMethodVersion = cloneString(v.ConfidenceMethodVersion)
	v.Limitations = cloneStrings(v.Limitations)
	return v
}


func NewServiceWithEvents(contexts ContextRepository, evidence EvidenceRepository, events EventRepository, authorizer EvidenceAuthorizer, clock Clock, ids IDGenerator) (*Service, error) {
	if events == nil {
		return nil, fault.New(fault.ClassInternal, "research_event_repository_not_configured", "Research Event Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	service, err := NewService(contexts, evidence, authorizer, clock, ids)
	if err != nil { return nil, err }
	service.events = events
	return service, nil
}

func (s *Service) CreateEvent(ctx context.Context, command CreateEventCommand) (ResearchEvent, error) {
	if s == nil || s.events == nil {
		return ResearchEvent{}, applicationFault(fault.ClassInternal, "research_event_repository_not_configured", "Research Event Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil { return ResearchEvent{}, validationFault("request_context_required", "request context is required", nil) }
	id, err := s.ids.NewID(EventIDPrefix)
	if err != nil { return ResearchEvent{}, applicationFault(fault.ClassInternal, "research_id_generation_failed", "Research identifier generation failed", err) }
	value := researchevent.ResearchEvent{
		EventID:id, EventVersion:1, EventType:command.EventType, Title:command.Title,
		Status:researchevent.EventStatus(command.Status), SubjectRefs:cloneStrings(command.SubjectRefs),
		EvidenceRefs:cloneStrings(command.EvidenceRefs), OccurredAt:cloneTime(command.OccurredAt),
		PublishedAt:cloneTime(command.PublishedAt), AvailableAt:cloneTime(command.AvailableAt),
		SystemObservedAt:command.SystemObservedAt, SystemAsOf:command.SystemAsOf,
		ImpactHypothesisRefs:cloneStrings(command.ImpactHypothesisRefs),
		InvalidationConditions:cloneStrings(command.InvalidationConditions),
		Uncertainty:cloneStrings(command.Uncertainty), SupersedesEventVersion:cloneInt(command.SupersedesEventVersion),
		SourceTimes:toDomainEventTimes(command.SourceTimes),
	}
	if err := value.Validate(); err != nil { return ResearchEvent{}, validationFault("invalid_research_event", "Research Event is invalid", err) }
	correlationID, err := correlationIDForPublication(ctx, s.ids)
	if err != nil { return ResearchEvent{}, applicationFault(fault.ClassInternal, "research_correlation_id_generation_failed", "Research correlation identifier generation failed", err) }
	if err := s.events.AppendWithLifecycle(ctx, value, nil, correlationID); err != nil { return ResearchEvent{}, mapRepositoryError(err) }
	return fromDomainEvent(value), nil
}

func (s *Service) UpdateEvent(ctx context.Context, command UpdateEventCommand) (ResearchEvent, error) {
	if s == nil || s.events == nil {
		return ResearchEvent{}, applicationFault(fault.ClassInternal, "research_event_repository_not_configured", "Research Event Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil || strings.TrimSpace(command.EventID) == "" || command.ExpectedVersion < 1 {
		return ResearchEvent{}, validationFault("invalid_event_update_command", "Event update command is invalid", nil)
	}
	previous, err := s.events.GetVersion(ctx, command.EventID, command.ExpectedVersion)
	if err != nil {
		if errors.Is(err, researcherrors.ErrNotFound) {
			return ResearchEvent{}, applicationFault(fault.ClassConflict, "research_event_version_conflict", "Research Event expected version is no longer current", researcherrors.ErrConflict)
		}
		return ResearchEvent{}, mapRepositoryError(err)
	}
	if previous.EventID != command.EventID || previous.EventVersion != command.ExpectedVersion {
		return ResearchEvent{}, applicationFault(fault.ClassInternal, "stored_event_identity_mismatch", "Stored Research Event identity does not match the requested version", researcherrors.ErrCorruptRecord)
	}
	if err := previous.Validate(); err != nil {
		return ResearchEvent{}, applicationFault(fault.ClassInternal, "stored_event_invalid", "Stored Research Event is invalid", err)
	}
	if previous.EventVersion == int(^uint(0)>>1) {
		return ResearchEvent{}, validationFault("event_version_limit_reached", "Research Event version limit has been reached", nil)
	}
	next := researchevent.ResearchEvent{
		EventID: previous.EventID, EventVersion: previous.EventVersion + 1,
		EventType: command.Revision.EventType, Title: command.Revision.Title,
		Status: researchevent.EventStatus(command.Revision.Status),
		SubjectRefs: cloneStrings(command.Revision.SubjectRefs),
		EvidenceRefs: cloneStrings(command.Revision.EvidenceRefs),
		OccurredAt: cloneTime(command.Revision.OccurredAt),
		PublishedAt: cloneTime(command.Revision.PublishedAt),
		AvailableAt: cloneTime(command.Revision.AvailableAt),
		SystemObservedAt: command.Revision.SystemObservedAt,
		SystemAsOf: command.Revision.SystemAsOf,
		ImpactHypothesisRefs: cloneStrings(command.Revision.ImpactHypothesisRefs),
		InvalidationConditions: cloneStrings(command.Revision.InvalidationConditions),
		Uncertainty: cloneStrings(command.Revision.Uncertainty),
		SupersedesEventVersion: cloneInt(command.Revision.SupersedesEventVersion),
		SourceTimes: toDomainEventTimes(command.Revision.SourceTimes),
	}
	if err := next.ValidateNextVersion(previous); err != nil {
		return ResearchEvent{}, validationFault("invalid_research_event_transition", "Research Event revision is invalid", err)
	}
	if command.ReasonCode != nil && (strings.TrimSpace(*command.ReasonCode) == "" || *command.ReasonCode != strings.TrimSpace(*command.ReasonCode)) {
		return ResearchEvent{}, validationFault("invalid_event_reason_code", "Research Event reason code must be null or a non-empty trimmed value", nil)
	}
	if next.Status != previous.Status {
		correlationID, err := correlationIDForPublication(ctx, s.ids)
		if err != nil {
			return ResearchEvent{}, applicationFault(fault.ClassInternal, "research_correlation_id_generation_failed", "Research correlation identifier generation failed", err)
		}
		if err := s.events.AppendWithLifecycle(ctx, next, cloneString(command.ReasonCode), correlationID); err != nil {
			return ResearchEvent{}, mapRepositoryError(err)
		}
	} else if err := s.events.Append(ctx, next); err != nil {
		return ResearchEvent{}, mapRepositoryError(err)
	}
	return fromDomainEvent(next), nil
}

func (s *Service) GetEvent(ctx context.Context, ref EventRef) (ResearchEvent, error) {
	if s == nil || s.events == nil {
		return ResearchEvent{}, applicationFault(fault.ClassInternal, "research_event_repository_not_configured", "Research Event Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil || strings.TrimSpace(ref.EventID) == "" || ref.EventVersion < 1 {
		return ResearchEvent{}, validationFault("invalid_event_ref", "Event reference is invalid", nil)
	}
	value, err := s.events.GetVersion(ctx, ref.EventID, ref.EventVersion)
	if err != nil { return ResearchEvent{}, mapRepositoryError(err) }
	if value.EventID != ref.EventID || value.EventVersion != ref.EventVersion {
		return ResearchEvent{}, applicationFault(fault.ClassInternal, "stored_event_identity_mismatch", "Stored Research Event identity does not match the requested version", researcherrors.ErrCorruptRecord)
	}
	if err := value.Validate(); err != nil { return ResearchEvent{}, applicationFault(fault.ClassInternal, "stored_event_invalid", "Stored Research Event is invalid", err) }
	return fromDomainEvent(value), nil
}

func toDomainEventTimes(values []EventSourceTime) []researchevent.EventSourceTime {
	out:=make([]researchevent.EventSourceTime,len(values))
	for i,v:=range values {
		out[i]=researchevent.EventSourceTime{EvidenceRef:v.EvidenceRef,TimeType:researchevent.EventTimeType(v.TimeType),Value:v.Value,AssertedBy:v.AssertedBy,Confidence:cloneFloat(v.Confidence),ConfidenceMethod:cloneString(v.ConfidenceMethod),ConfidenceMethodVersion:cloneString(v.ConfidenceMethodVersion),Notes:cloneString(v.Notes)}
	}
	return out
}
func fromDomainEvent(value researchevent.ResearchEvent) ResearchEvent {
	out:=ResearchEvent{
		EventID:value.EventID,EventVersion:value.EventVersion,EventType:value.EventType,Title:value.Title,Status:string(value.Status),
		SubjectRefs:cloneStrings(value.SubjectRefs),EvidenceRefs:cloneStrings(value.EvidenceRefs),
		OccurredAt:cloneTime(value.OccurredAt),PublishedAt:cloneTime(value.PublishedAt),AvailableAt:cloneTime(value.AvailableAt),
		SystemObservedAt:value.SystemObservedAt,SystemAsOf:value.SystemAsOf,
		ImpactHypothesisRefs:cloneStrings(value.ImpactHypothesisRefs),
		InvalidationConditions:cloneStrings(value.InvalidationConditions),Uncertainty:cloneStrings(value.Uncertainty),
		SupersedesEventVersion:cloneInt(value.SupersedesEventVersion),SourceTimes:make([]EventSourceTime,len(value.SourceTimes)),
	}
	for i,v:=range value.SourceTimes { out.SourceTimes[i]=EventSourceTime{EvidenceRef:v.EvidenceRef,TimeType:string(v.TimeType),Value:v.Value,AssertedBy:v.AssertedBy,Confidence:cloneFloat(v.Confidence),ConfidenceMethod:cloneString(v.ConfidenceMethod),ConfidenceMethodVersion:cloneString(v.ConfidenceMethodVersion),Notes:cloneString(v.Notes)} }
	return out
}
func cloneTime(value *time.Time) *time.Time { if value==nil{return nil}; copy:=*value; return &copy }
func cloneFloat(value *float64) *float64 { if value==nil{return nil}; copy:=*value; return &copy }
func cloneString(value *string) *string { if value==nil{return nil}; copy:=*value; return &copy }
func cloneInt(value *int) *int { if value==nil{return nil}; copy:=*value; return &copy }


func NewServiceWithClaims(contexts ContextRepository, evidence EvidenceRepository, claims ClaimRepository, authorizer EvidenceAuthorizer, clock Clock, ids IDGenerator) (*Service, error) {
	if claims == nil {
		return nil, fault.New(fault.ClassInternal, "research_claim_repository_not_configured", "Research Claim Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	service, err := NewService(contexts, evidence, authorizer, clock, ids)
	if err != nil { return nil, err }
	service.claims = claims
	return service, nil
}

func (s *Service) CreateClaim(ctx context.Context, command CreateClaimCommand) (Claim, error) {
	if s == nil || s.claims == nil {
		return Claim{}, applicationFault(fault.ClassInternal, "research_claim_repository_not_configured", "Research Claim Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil { return Claim{}, validationFault("request_context_required", "request context is required", nil) }
	id, err := s.ids.NewID(ClaimIDPrefix)
	if err != nil { return Claim{}, applicationFault(fault.ClassInternal, "research_id_generation_failed", "Research identifier generation failed", err) }
	value := researchclaim.Claim{
		ClaimID:id, ClaimVersion:1, Statement:command.Statement,
		EpistemicType:researchclaim.EpistemicType(command.EpistemicType),
		ScopeRefs:cloneStrings(command.ScopeRefs), SystemAsOf:command.SystemAsOf,
		SupportingEvidenceRefs:cloneStrings(command.SupportingEvidenceRefs),
		ContradictingEvidenceRefs:cloneStrings(command.ContradictingEvidenceRefs),
		RequiredDataRefs:cloneStrings(command.RequiredDataRefs),
		ValidationMethod:cloneString(command.ValidationMethod), ExpectedObservation:cloneString(command.ExpectedObservation),
		InvalidationConditions:cloneStrings(command.InvalidationConditions),
		Status:researchclaim.ClaimStatus(command.Status), Limitations:cloneStrings(command.Limitations),
	}
	if err := value.Validate(); err != nil { return Claim{}, validationFault("invalid_research_claim", "Research Claim is invalid", err) }
	if err := s.claims.Append(ctx, value); err != nil { return Claim{}, mapRepositoryError(err) }
	return fromDomainClaim(value), nil
}

// UpdateClaim 基于调用方指定的前版追加同一 Claim 的新版本。
func (s *Service) UpdateClaim(ctx context.Context, command UpdateClaimCommand) (Claim, error) {
	if s == nil || s.claims == nil {
		return Claim{}, applicationFault(fault.ClassInternal, "research_claim_repository_not_configured", "Research Claim Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil || strings.TrimSpace(command.ClaimID) == "" || command.ExpectedVersion < 1 {
		return Claim{}, validationFault("invalid_claim_update_ref", "Claim ID and expected version are required", nil)
	}

	previous, err := s.claims.GetVersion(ctx, command.ClaimID, command.ExpectedVersion)
	if err != nil {
		return Claim{}, mapRepositoryError(err)
	}
	if previous.ClaimID != command.ClaimID || previous.ClaimVersion != command.ExpectedVersion {
		return Claim{}, applicationFault(fault.ClassInternal, "stored_claim_identity_mismatch", "Stored Research Claim identity does not match the requested version", researcherrors.ErrCorruptRecord)
	}
	if previous.ClaimVersion == int(^uint(0)>>1) {
		return Claim{}, applicationFault(fault.ClassConflict, "research_claim_version_exhausted", "Research Claim version cannot be incremented", researcherrors.ErrConflict)
	}

	revision := command.Revision
	next := researchclaim.Claim{
		ClaimID: previous.ClaimID,
		ClaimVersion: previous.ClaimVersion + 1,
		Statement: revision.Statement,
		EpistemicType: researchclaim.EpistemicType(revision.EpistemicType),
		ScopeRefs: cloneStrings(revision.ScopeRefs),
		SystemAsOf: revision.SystemAsOf,
		SupportingEvidenceRefs: cloneStrings(revision.SupportingEvidenceRefs),
		ContradictingEvidenceRefs: cloneStrings(revision.ContradictingEvidenceRefs),
		RequiredDataRefs: cloneStrings(revision.RequiredDataRefs),
		ValidationMethod: cloneString(revision.ValidationMethod),
		ExpectedObservation: cloneString(revision.ExpectedObservation),
		InvalidationConditions: cloneStrings(revision.InvalidationConditions),
		Status: researchclaim.ClaimStatus(revision.Status),
		Limitations: cloneStrings(revision.Limitations),
	}
	if err := next.ValidateNextVersion(previous); err != nil {
		return Claim{}, validationFault("invalid_research_claim_revision", "Research Claim revision is invalid", err)
	}
	if err := s.claims.Append(ctx, next); err != nil {
		return Claim{}, mapRepositoryError(err)
	}
	return fromDomainClaim(next), nil
}

func (s *Service) GetClaim(ctx context.Context, ref ClaimRef) (Claim, error) {
	if s == nil || s.claims == nil {
		return Claim{}, applicationFault(fault.ClassInternal, "research_claim_repository_not_configured", "Research Claim Repository is not configured", researcherrors.ErrInvalidRepository)
	}
	if ctx == nil || strings.TrimSpace(ref.ClaimID) == "" || ref.ClaimVersion < 1 {
		return Claim{}, validationFault("invalid_claim_ref", "Claim reference is invalid", nil)
	}
	value, err := s.claims.GetVersion(ctx, ref.ClaimID, ref.ClaimVersion)
	if err != nil { return Claim{}, mapRepositoryError(err) }
	if value.ClaimID != ref.ClaimID || value.ClaimVersion != ref.ClaimVersion {
		return Claim{}, applicationFault(fault.ClassInternal, "stored_claim_identity_mismatch", "Stored Research Claim identity does not match the requested version", researcherrors.ErrCorruptRecord)
	}
	if err := value.Validate(); err != nil { return Claim{}, applicationFault(fault.ClassInternal, "stored_claim_invalid", "Stored Research Claim is invalid", err) }
	return fromDomainClaim(value), nil
}

func fromDomainClaim(value researchclaim.Claim) Claim {
	return Claim{
		ClaimID:value.ClaimID,ClaimVersion:value.ClaimVersion,Statement:value.Statement,
		EpistemicType:string(value.EpistemicType),ScopeRefs:cloneStrings(value.ScopeRefs),SystemAsOf:value.SystemAsOf,
		SupportingEvidenceRefs:cloneStrings(value.SupportingEvidenceRefs),
		ContradictingEvidenceRefs:cloneStrings(value.ContradictingEvidenceRefs),
		RequiredDataRefs:cloneStrings(value.RequiredDataRefs),
		ValidationMethod:cloneString(value.ValidationMethod),ExpectedObservation:cloneString(value.ExpectedObservation),
		InvalidationConditions:cloneStrings(value.InvalidationConditions),Status:string(value.Status),Limitations:cloneStrings(value.Limitations),
	}
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


// NotificationEventEnvelope is the immutable payload Research sends to the
// Notification Application Port. Delivery metadata is deliberately excluded.
type NotificationEventEnvelope struct {
	EventID string `json:"event_id"`
	EventType string `json:"event_type"`
	SchemaVersion string `json:"schema_version"`
	SourceModule string `json:"source_module"`
	OccurredAt string `json:"occurred_at"`
	PublishedAt string `json:"published_at"`
	SubjectRef string `json:"subject_ref"`
	IdempotencyKey string `json:"idempotency_key"`
	CorrelationID string `json:"correlation_id"`
	PayloadFingerprint string `json:"payload_fingerprint"`
	Payload json.RawMessage `json:"payload"`
	LeaseToken string `json:"-"`
}

type OutboxDeliveryRepository interface {
	ClaimOutbox(context.Context, string, int, time.Time) ([]NotificationEventEnvelope, error)
	MarkOutboxPublished(context.Context, string, string, string, time.Time) error
	ScheduleOutboxRetry(context.Context, string, string, string, time.Time, string, time.Time) error
	RejectOutbox(context.Context, string, string, string, string, time.Time) error
}

type OutboxDeliveryPort interface {
	Claim(context.Context, string, int, time.Duration) ([]NotificationEventEnvelope, error)
	Confirm(context.Context, string, string, string) error
	Retry(context.Context, string, string, string, time.Duration, string) error
	Reject(context.Context, string, string, string, string) error
}

type OutboxService struct {
	repository OutboxDeliveryRepository
	clock Clock
}

var _ OutboxDeliveryPort = (*OutboxService)(nil)

func NewOutboxService(repository OutboxDeliveryRepository, clock Clock) (*OutboxService, error) {
	if repository == nil { return nil, applicationFault(fault.ClassInternal, "research_outbox_repository_not_configured", "Research Outbox Repository is not configured", researcherrors.ErrInvalidRepository) }
	if clock == nil { clock = func() time.Time { return time.Now().UTC() } }
	return &OutboxService{repository: repository, clock: clock}, nil
}

func (s *OutboxService) Claim(ctx context.Context, workerID string, limit int, leaseDuration time.Duration) ([]NotificationEventEnvelope, error) {
	if s == nil || s.repository == nil { return nil, applicationFault(fault.ClassInternal, "research_outbox_repository_not_configured", "Research Outbox Repository is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || !validCorrelationID(workerID) || limit < 1 || limit > 100 || leaseDuration < time.Second || leaseDuration > 5*time.Minute {
		return nil, validationFault("invalid_research_outbox_claim", "Research Outbox claim parameters are invalid", nil)
	}
	now := s.clock().UTC()
	if now.IsZero() { return nil, applicationFault(fault.ClassInternal, "research_clock_invalid", "Research application clock is invalid", nil) }
	values, err := s.repository.ClaimOutbox(ctx, workerID, limit, now.Add(leaseDuration))
	if err != nil { return nil, mapRepositoryError(err) }
	return values, nil
}

func (s *OutboxService) Confirm(ctx context.Context, eventID, workerID, leaseToken string) error {
	if s == nil || s.repository == nil { return applicationFault(fault.ClassInternal, "research_outbox_repository_not_configured", "Research Outbox Repository is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || strings.TrimSpace(eventID) == "" || !validCorrelationID(workerID) || !validLeaseToken(leaseToken) {
		return validationFault("invalid_research_outbox_confirmation", "Research Outbox confirmation is invalid", nil)
	}
	now := s.clock().UTC()
	if now.IsZero() { return applicationFault(fault.ClassInternal, "research_clock_invalid", "Research application clock is invalid", nil) }
	if err := s.repository.MarkOutboxPublished(ctx, eventID, workerID, leaseToken, now); err != nil { return mapRepositoryError(err) }
	return nil
}

func (s *OutboxService) Retry(ctx context.Context, eventID, workerID, leaseToken string, delay time.Duration, reasonCode string) error {
	if s == nil || s.repository == nil { return applicationFault(fault.ClassInternal, "research_outbox_repository_not_configured", "Research Outbox Repository is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || strings.TrimSpace(eventID) == "" || !validCorrelationID(workerID) || !validLeaseToken(leaseToken) ||
		delay <= 0 || delay > 24*time.Hour || !validCorrelationID(reasonCode) {
		return validationFault("invalid_research_outbox_retry", "Research Outbox retry parameters are invalid", nil)
	}
	now := s.clock().UTC()
	if now.IsZero() { return applicationFault(fault.ClassInternal, "research_clock_invalid", "Research application clock is invalid", nil) }
	if err := s.repository.ScheduleOutboxRetry(ctx, eventID, workerID, leaseToken, now.Add(delay), reasonCode, now); err != nil { return mapRepositoryError(err) }
	return nil
}

func (s *OutboxService) Reject(ctx context.Context, eventID, workerID, leaseToken, reasonCode string) error {
	if s == nil || s.repository == nil { return applicationFault(fault.ClassInternal, "research_outbox_repository_not_configured", "Research Outbox Repository is not configured", researcherrors.ErrInvalidRepository) }
	if ctx == nil || strings.TrimSpace(eventID) == "" || !validCorrelationID(workerID) || !validLeaseToken(leaseToken) || !validCorrelationID(reasonCode) {
		return validationFault("invalid_research_outbox_rejection", "Research Outbox rejection is invalid", nil)
	}
	now := s.clock().UTC()
	if now.IsZero() { return applicationFault(fault.ClassInternal, "research_clock_invalid", "Research application clock is invalid", nil) }
	if err := s.repository.RejectOutbox(ctx, eventID, workerID, leaseToken, reasonCode, now); err != nil { return mapRepositoryError(err) }
	return nil
}

func validLeaseToken(value string) bool {
	if len(value) != 32 { return false }
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) { return false }
	}
	return true
}
