package claim

import (
	"encoding/json"
	"fmt"
	"time"
)

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
	ClaimID                   string        `json:"claim_id"`
	ClaimVersion              int           `json:"claim_version"`
	SupersedesVersion         *int          `json:"supersedes_version,omitempty"`
	Statement                 string        `json:"statement"`
	EpistemicType             EpistemicType `json:"epistemic_type"`
	ScopeRefs                 []string      `json:"scope_refs"`
	SystemAsOf                time.Time     `json:"system_as_of"`
	SupportingEvidenceRefs    []string      `json:"supporting_evidence_refs"`
	ContradictingEvidenceRefs []string      `json:"contradicting_evidence_refs"`
	RequiredDataRefs          []string      `json:"required_data_refs"`
	ValidationMethod          *string       `json:"validation_method,omitempty"`
	ExpectedObservation       *string       `json:"expected_observation,omitempty"`
	InvalidationConditions    []string      `json:"invalidation_conditions,omitempty"`
	Status                    ClaimStatus   `json:"status"`
	Limitations               []string      `json:"limitations"`
}

// UnmarshalJSON enforces the frozen Claim/Hypothesis DTO field set.
func (c *Claim) UnmarshalJSON(data []byte) error {
	if c == nil {
		return fmt.Errorf("Claim target is nil")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	allowed := map[string]struct{}{
		"claim_id": {}, "claim_version": {}, "supersedes_version": {}, "statement": {},
		"epistemic_type": {}, "scope_refs": {}, "system_as_of": {},
		"supporting_evidence_refs": {}, "contradicting_evidence_refs": {}, "required_data_refs": {},
		"validation_method": {}, "expected_observation": {}, "invalidation_conditions": {},
		"status": {}, "limitations": {},
	}
	for field := range fields {
		if _, ok := allowed[field]; !ok {
			return fmt.Errorf("unknown field %q", field)
		}
	}
	for _, field := range []string{"claim_id", "claim_version", "statement", "epistemic_type", "scope_refs", "system_as_of", "supporting_evidence_refs", "contradicting_evidence_refs", "required_data_refs", "status", "limitations"} {
		if _, ok := fields[field]; !ok {
			return fmt.Errorf("required field %q is missing", field)
		}
	}
	type claimAlias Claim
	var value claimAlias
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*c = Claim(value)
	return nil
}
