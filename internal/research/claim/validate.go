package claim

import (
	"fmt"
	"strings"
	"time"
)

// Validate checks one Claim/Hypothesis version without resolving external refs.
func (c Claim) Validate() error {
	if err := required("claim_id", c.ClaimID); err != nil { return err }
	if c.ClaimVersion < 1 { return fmt.Errorf("claim_version must be >= 1") }
	if err := required("statement", c.Statement); err != nil { return err }
	if err := validateEpistemicType(c.EpistemicType); err != nil { return err }
	if err := validateStatus(c.Status); err != nil { return err }
	if err := utc("system_as_of", c.SystemAsOf); err != nil { return err }

	for _, item := range []struct { name string; values []string }{
		{"scope_refs", c.ScopeRefs},
		{"supporting_evidence_refs", c.SupportingEvidenceRefs},
		{"contradicting_evidence_refs", c.ContradictingEvidenceRefs},
		{"required_data_refs", c.RequiredDataRefs},
		{"limitations", c.Limitations},
	} {
		if item.values == nil { return fmt.Errorf("%s must be a non-null array", item.name) }
		if err := uniqueStrings(item.name, item.values); err != nil { return err }
	}
	if err := rejectEvidenceStanceOverlap(c.SupportingEvidenceRefs, c.ContradictingEvidenceRefs); err != nil {
		return err
	}

	if c.EpistemicType == EpistemicHypothesis {
		if c.ValidationMethod == nil {
			return fmt.Errorf("validation_method is required for hypothesis")
		}
		if err := required("validation_method", *c.ValidationMethod); err != nil {
			return err
		}
		if c.ExpectedObservation == nil {
			return fmt.Errorf("expected_observation is required for hypothesis")
		}
		if err := required("expected_observation", *c.ExpectedObservation); err != nil {
			return err
		}
		if c.InvalidationConditions == nil || len(c.InvalidationConditions) == 0 {
			return fmt.Errorf("invalidation_conditions must be a non-empty array for hypothesis")
		}
		if err := uniqueStrings("invalidation_conditions", c.InvalidationConditions); err != nil { return err }
	} else if c.InvalidationConditions != nil {
		if err := uniqueStrings("invalidation_conditions", c.InvalidationConditions); err != nil { return err }
	}
	return nil
}

// ValidateNextVersion checks immutable ID, contiguous version and monotonic
// knowledge cutoff. The current DTO has no supersedes_version field, so explicit
// cross-version supersession linkage remains a contract gap.
func (next Claim) ValidateNextVersion(previous Claim) error {
	if err := previous.Validate(); err != nil { return fmt.Errorf("previous claim: %w", err) }
	if err := next.Validate(); err != nil { return fmt.Errorf("next claim: %w", err) }
	if next.ClaimID != previous.ClaimID { return fmt.Errorf("claim_id cannot change across versions") }
	if previous.ClaimVersion == int(^uint(0)>>1) {
		return fmt.Errorf("claim_version reached the supported integer limit")
	}
	if next.ClaimVersion != previous.ClaimVersion+1 { return fmt.Errorf("claim_version must increment by exactly one") }
	if next.SystemAsOf.Before(previous.SystemAsOf) { return fmt.Errorf("system_as_of cannot move backwards across claim versions") }
	return nil
}

func rejectEvidenceStanceOverlap(supporting, contradicting []string) error {
	set := make(map[string]struct{}, len(supporting))
	for _, ref := range supporting { set[ref] = struct{}{} }
	for _, ref := range contradicting {
		if _, exists := set[ref]; exists {
			return fmt.Errorf("evidence_ref %q cannot be both supporting and contradicting", ref)
		}
	}
	return nil
}

func validateEpistemicType(value EpistemicType) error {
	switch value {
	case EpistemicReportedFact, EpistemicInference, EpistemicHypothesis:
		return nil
	default:
		return fmt.Errorf("epistemic_type is invalid")
	}
}

func validateStatus(value ClaimStatus) error {
	switch value {
	case StatusProposed, StatusTesting, StatusSupported, StatusContradicted, StatusInvalidated, StatusUnknown:
		return nil
	default:
		return fmt.Errorf("status is invalid")
	}
}

func required(name, value string) error {
	if value == "" || value != strings.TrimSpace(value) {
		return fmt.Errorf("%s is required without surrounding whitespace", name)
	}
	return nil
}

func uniqueStrings(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if err := required(name+" item", value); err != nil { return err }
		if _, exists := seen[value]; exists { return fmt.Errorf("%s contains duplicate value %q", name, value) }
		seen[value] = struct{}{}
	}
	return nil
}

func utc(name string, value time.Time) error {
	if value.IsZero() { return fmt.Errorf("%s is required", name) }
	_, offset := value.Zone()
	if offset != 0 || value.Location() != time.UTC { return fmt.Errorf("%s must be UTC", name) }
	return nil
}
