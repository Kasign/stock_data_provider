// Package context defines the Research-owned semantic Context model.
// Run assembly, section selection and token-budget truncation belong to Researchflow.
package context

import "time"

type Scope string

const (
	ScopeMarket   Scope = "market"
	ScopeIndustry Scope = "industry"
	ScopeStock    Scope = "stock"
	ScopeEvent    Scope = "event"
)

type SourceOwner string

const (
	SourceOwnerData     SourceOwner = "data"
	SourceOwnerQuant    SourceOwner = "quant"
	SourceOwnerResearch SourceOwner = "research"
	SourceOwnerNews     SourceOwner = "news"
)

type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

type OmissionReason string

const (
	OmissionNotRequested    OmissionReason = "not_requested"
	OmissionBudgetExceeded  OmissionReason = "budget_exceeded"
	OmissionUnavailable     OmissionReason = "unavailable"
	OmissionUnauthorized    OmissionReason = "unauthorized"
	OmissionQualityRejected OmissionReason = "quality_rejected"
	OmissionFailed          OmissionReason = "failed"
	OmissionDuplicate       OmissionReason = "duplicate"
)

type SectionRef struct {
	SectionID      string        `json:"section_id"`
	SectionVersion string        `json:"section_version"`
	SourceOwner    SourceOwner   `json:"source_owner"`
	SourceRefs     []string      `json:"source_refs"`
	EvidenceRefs   []string      `json:"evidence_refs"`
	QualityStatus  QualityStatus `json:"quality_status"`
	Required       bool          `json:"required"`
	Included       bool          `json:"included"`
	AssembledAt    time.Time     `json:"assembled_at"`
	SystemAsOf     time.Time     `json:"system_as_of"`
}

type OmittedSection struct {
	SectionID   string         `json:"section_id"`
	SourceOwner SourceOwner    `json:"source_owner"`
	ReasonCode  OmissionReason `json:"reason_code"`
	Required    bool           `json:"required"`
	SourceRefs  []string       `json:"source_refs"`
	SafeMessage string         `json:"safe_message"`
	Retryable   bool           `json:"retryable"`
}

type QualitySummary struct {
	Status               QualityStatus `json:"status"`
	MissingRequiredCount int           `json:"missing_required_count"`
	MissingOptionalCount int           `json:"missing_optional_count"`
	ConflictCount        int           `json:"conflict_count"`
	StaleCount           int           `json:"stale_count"`
	ReasonCodes          []string      `json:"reason_codes"`
}

type Context struct {
	SchemaVersion    string           `json:"schema_version"`
	ContextID        string           `json:"context_id"`
	ContextVersion   int64            `json:"context_version"`
	ResearchQuestion string           `json:"research_question"`
	Scope            Scope            `json:"scope"`
	SubjectRefs      []string         `json:"subject_refs"`
	SystemAsOf       time.Time        `json:"system_as_of"`
	SectionRefs      []SectionRef     `json:"section_refs"`
	QualitySummary   QualitySummary   `json:"quality_summary"`
	IncludedSections []string         `json:"included_sections"`
	OmittedSections  []OmittedSection `json:"omitted_sections"`
	BuilderVersion   string           `json:"builder_version"`
	CreatedAt        time.Time        `json:"created_at"`
}
