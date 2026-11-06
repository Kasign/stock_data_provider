package claim

import "time"

type EpistemicType string

const (
	EpistemicReportedFact EpistemicType = "reported_fact"
	EpistemicInference    EpistemicType = "inference"
	EpistemicHypothesis   EpistemicType = "hypothesis"
)

type ClaimStatus string

const (
	StatusProposed     ClaimStatus = "proposed"
	StatusTesting      ClaimStatus = "testing"
	StatusSupported    ClaimStatus = "supported"
	StatusContradicted ClaimStatus = "contradicted"
	StatusInvalidated  ClaimStatus = "invalidated"
	StatusUnknown      ClaimStatus = "unknown"
)

// Claim is one immutable version of a verifiable research statement.
// Hypothesis-specific fields are required only when EpistemicType is hypothesis.
type Claim struct {
	ClaimID                 string        `json:"claim_id"`
	ClaimVersion            int           `json:"claim_version"`
	Statement               string        `json:"statement"`
	EpistemicType           EpistemicType `json:"epistemic_type"`
	ScopeRefs               []string      `json:"scope_refs"`
	SystemAsOf              time.Time     `json:"system_as_of"`
	SupportingEvidenceRefs  []string      `json:"supporting_evidence_refs"`
	ContradictingEvidenceRefs []string    `json:"contradicting_evidence_refs"`
	RequiredDataRefs        []string      `json:"required_data_refs"`
	ValidationMethod        *string       `json:"validation_method,omitempty"`
	ExpectedObservation     *string       `json:"expected_observation,omitempty"`
	InvalidationConditions  []string      `json:"invalidation_conditions,omitempty"`
	Status                  ClaimStatus   `json:"status"`
	Limitations             []string      `json:"limitations"`
}
