// Package evidence defines Research-owned evidence semantics.
// Source ownership, content access and authorization decisions remain with source modules.
package evidence

import "time"

type EvidenceType string

const (
	TypeFact         EvidenceType = "fact"
	TypeReportedFact EvidenceType = "reported_fact"
	TypeInference    EvidenceType = "inference"
	TypeHypothesis   EvidenceType = "hypothesis"
	TypeUnknown      EvidenceType = "unknown"
)

type SourceOwner string

const (
	SourceData     SourceOwner = "data"
	SourceNews     SourceOwner = "news"
	SourceQuant    SourceOwner = "quant"
	SourceResearch SourceOwner = "research"
)

type Stance string

const (
	StanceSupports    Stance = "supports"
	StanceContradicts Stance = "contradicts"
	StanceNeutral     Stance = "neutral"
)

type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

type ContentLevel string

const (
	ContentMetadata ContentLevel = "metadata"
	ContentSummary  ContentLevel = "summary"
	ContentSnippet  ContentLevel = "snippet"
	ContentFull     ContentLevel = "full"
)

// Evidence mirrors RSC-CTR-001. Optional nullable fields are emitted as null
// when absent so the serialized DTO keeps a stable, explicit shape.
type Evidence struct {
	EvidenceID              string        `json:"evidence_id"`
	EvidenceType            EvidenceType  `json:"evidence_type"`
	SourceOwner             SourceOwner   `json:"source_owner"`
	SourceRef               string        `json:"source_ref"`
	SourceVersion           *string       `json:"source_version"`
	SourceLocator           *string       `json:"source_locator"`
	DerivedFromRefs         []string      `json:"derived_from_refs"`
	ClaimRef                *string       `json:"claim_ref"`
	SubjectRefs             []string      `json:"subject_refs"`
	PublishedAt             *time.Time    `json:"published_at"`
	AvailableAt             *time.Time    `json:"available_at"`
	SystemObservedAt        time.Time     `json:"system_observed_at"`
	ValidTimeStart          *time.Time    `json:"valid_time_start"`
	ValidTimeEnd            *time.Time    `json:"valid_time_end"`
	SystemAsOf              time.Time     `json:"system_as_of"`
	Stance                  Stance        `json:"stance"`
	Confidence              *float64      `json:"confidence"`
	ConfidenceMethod        *string       `json:"confidence_method"`
	ConfidenceMethodVersion *string       `json:"confidence_method_version"`
	QualityStatus           QualityStatus `json:"quality_status"`
	ContentLevel            ContentLevel  `json:"content_level"`
	AuthorizationRef        string        `json:"authorization_ref"`
	Limitations             []string      `json:"limitations"`
	ContentFingerprint      string        `json:"content_fingerprint"`
}
