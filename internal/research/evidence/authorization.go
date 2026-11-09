package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type AuthorizationDecision string

const (
	AuthorizationAllowed AuthorizationDecision = "allowed"
	AuthorizationDenied AuthorizationDecision = "denied"
	AuthorizationConditional AuthorizationDecision = "conditional"
	AuthorizationUnknown AuthorizationDecision = "unknown"
)

type PurposeCode string

const (
	PurposeQuantAnalysis PurposeCode = "quant_analysis"
	PurposeResearchAnalysis PurposeCode = "research_analysis"
	PurposeRiskReview PurposeCode = "risk_review"
	PurposeUserDisplay PurposeCode = "user_display"
)

type PolicyVersion struct {
	PolicyType string `json:"policy_type"`
	PolicyRef string `json:"policy_ref"`
	PolicyVersion string `json:"policy_version"`
}

type AuthorizationPolicySnapshot struct {
	SnapshotRef string `json:"snapshot_ref"`
	Policies []PolicyVersion `json:"policies"`
}

// AuthorizationRef is an auditable decision snapshot, never a bearer token.
// Integrity validation does not replace current authorization at read time.
type AuthorizationRef struct {
	AuthorizationRef string `json:"authorization_ref"`
	PolicyOwner SourceOwner `json:"policy_owner"`
	PolicyVersion string `json:"policy_version"`
	Decision AuthorizationDecision `json:"decision"`
	PurposeCode PurposeCode `json:"purpose_code"`
	ContentLevel ContentLevel `json:"content_level"`
	SubjectRef string `json:"subject_ref"`
	ResourceRef string `json:"resource_ref"`
	DecidedAt time.Time `json:"decided_at"`
	ExpiresAt *time.Time `json:"expires_at"`
	Conditions []string `json:"conditions"`
	DecisionFingerprint string `json:"decision_fingerprint"`
}

func (a AuthorizationRef) Validate(snapshot AuthorizationPolicySnapshot) error {
	if err := requiredRef("authorization_ref", a.AuthorizationRef); err != nil { return err }
	if err := validateSourceOwner(a.PolicyOwner); err != nil { return fmt.Errorf("policy_owner: %w", err) }
	if err := requiredRef("policy_version", a.PolicyVersion); err != nil { return err }
	if err := validateAuthorizationDecision(a.Decision); err != nil { return err }
	if err := validatePurposeCode(a.PurposeCode); err != nil { return err }
	if err := validateContentLevel(a.ContentLevel); err != nil { return err }
	if err := requiredRef("subject_ref", a.SubjectRef); err != nil { return err }
	if err := requiredRef("resource_ref", a.ResourceRef); err != nil { return err }
	if err := validateRequiredUTC("decided_at", a.DecidedAt); err != nil { return err }
	if a.ExpiresAt != nil {
		if err := validateRequiredUTC("expires_at", *a.ExpiresAt); err != nil { return err }
	}
	if a.Conditions == nil { return fmt.Errorf("conditions must be a non-null array") }
	if err := validateSortedUniqueStrings("conditions", a.Conditions); err != nil { return err }
	switch a.Decision {
	case AuthorizationAllowed:
		if len(a.Conditions) != 0 { return fmt.Errorf("allowed decision must have no conditions") }
	case AuthorizationConditional:
		if len(a.Conditions) == 0 { return fmt.Errorf("conditional decision requires conditions") }
	}
	if snapshot.SnapshotRef == "" || snapshot.SnapshotRef != a.PolicyVersion {
		return fmt.Errorf("policy_version must equal policy_snapshot.snapshot_ref")
	}
	if len(snapshot.Policies) == 0 { return fmt.Errorf("policy_snapshot.policies must not be empty") }
	if err := validatePolicyVersions(snapshot.Policies); err != nil { return err }
	if len(a.DecisionFingerprint) != 64 || strings.ToLower(a.DecisionFingerprint) != a.DecisionFingerprint {
		return fmt.Errorf("decision_fingerprint must be a lowercase SHA-256 hex string")
	}
	if _, err := hex.DecodeString(a.DecisionFingerprint); err != nil {
		return fmt.Errorf("decision_fingerprint must be a lowercase SHA-256 hex string")
	}
	fingerprint, err := a.ComputeDecisionFingerprint(snapshot)
	if err != nil { return err }
	if fingerprint != a.DecisionFingerprint {
		return fmt.Errorf("decision_fingerprint does not match AuthorizationRef and PolicySnapshot")
	}
	return nil
}

// ValidateEvidenceAuthorizationBinding checks the snapshot against trusted
// caller/resource values supplied by the Owner adapter. It cannot detect revocation.
func ValidateEvidenceAuthorizationBinding(
	e Evidence,
	a AuthorizationRef,
	snapshot AuthorizationPolicySnapshot,
	expectedSubjectRef, expectedResourceRef string,
	expectedPurpose PurposeCode,
) error {
	if e.AuthorizationRef != a.AuthorizationRef {
		return fmt.Errorf("evidence authorization_ref does not match supplied AuthorizationRef")
	}
	if err := a.Validate(snapshot); err != nil { return fmt.Errorf("authorization snapshot: %w", err) }
	if a.Decision != AuthorizationAllowed { return fmt.Errorf("authorization decision %q is not readable", a.Decision) }
	if a.PolicyOwner != e.SourceOwner { return fmt.Errorf("authorization policy_owner does not match evidence source_owner") }
	if a.SubjectRef != expectedSubjectRef { return fmt.Errorf("authorization subject_ref does not match trusted caller") }
	if a.ResourceRef != expectedResourceRef { return fmt.Errorf("authorization resource_ref does not match requested resource") }
	if a.PurposeCode != expectedPurpose { return fmt.Errorf("authorization purpose_code does not match requested purpose") }
	if a.ContentLevel != e.ContentLevel { return fmt.Errorf("evidence content_level does not match authorization content_level") }
	return nil
}

// ValidateEvidenceAuthorizationBindingAt adds the contract's fixed-expiry check
// to the structural binding check. now must come from a trusted caller clock.
// This does not re-evaluate the Owner's current policy or protect the
// policy-check/content-read interval; the Owner adapter must still do both.
func ValidateEvidenceAuthorizationBindingAt(
	e Evidence,
	a AuthorizationRef,
	snapshot AuthorizationPolicySnapshot,
	expectedSubjectRef, expectedResourceRef string,
	expectedPurpose PurposeCode,
	now time.Time,
) error {
	if err := ValidateEvidenceAuthorizationBinding(
		e, a, snapshot, expectedSubjectRef, expectedResourceRef, expectedPurpose,
	); err != nil {
		return err
	}
	if err := validateRequiredUTC("authorization_check_time", now); err != nil {
		return err
	}
	if a.ExpiresAt != nil && !now.Before(*a.ExpiresAt) {
		return fmt.Errorf("authorization_ref is expired")
	}
	return nil
}

func (a AuthorizationRef) ComputeDecisionFingerprint(snapshot AuthorizationPolicySnapshot) (string, error) {
	policies := make([]any, 0, len(snapshot.Policies))
	for _, policy := range snapshot.Policies {
		policies = append(policies, map[string]any{
			"policy_type": policy.PolicyType,
			"policy_ref": policy.PolicyRef,
			"policy_version": policy.PolicyVersion,
		})
	}
	fields := map[string]any{
		"conditions": a.Conditions,
		"content_level": string(a.ContentLevel),
		"decided_at": a.DecidedAt.UTC().Format(time.RFC3339Nano),
		"decision": string(a.Decision),
		"evaluated_policy_versions": policies,
		"expires_at": nullableTime(a.ExpiresAt),
		"policy_owner": string(a.PolicyOwner),
		"policy_version": a.PolicyVersion,
		"purpose_code": string(a.PurposeCode),
		"resource_ref": a.ResourceRef,
		"subject_ref": a.SubjectRef,
	}
	canonical, err := canonicalObject(fields)
	if err != nil { return "", err }
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), nil
}

func validateAuthorizationDecision(value AuthorizationDecision) error {
	switch value {
	case AuthorizationAllowed, AuthorizationDenied, AuthorizationConditional, AuthorizationUnknown: return nil
	default: return fmt.Errorf("decision is invalid")
	}
}

func validatePurposeCode(value PurposeCode) error {
	switch value {
	case PurposeQuantAnalysis, PurposeResearchAnalysis, PurposeRiskReview, PurposeUserDisplay: return nil
	default: return fmt.Errorf("purpose_code is invalid")
	}
}

func requiredRef(name, value string) error {
	if value == "" || value != strings.TrimSpace(value) { return fmt.Errorf("%s is required without surrounding whitespace", name) }
	return nil
}

func validateSortedUniqueStrings(name string, values []string) error {
	if err := validateUniqueStrings(name, values); err != nil { return err }
	for i := 1; i < len(values); i++ {
		if values[i-1] >= values[i] { return fmt.Errorf("%s must be sorted in ascending order without duplicates", name) }
	}
	return nil
}

func validatePolicyVersions(values []PolicyVersion) error {
	seen := make(map[PolicyVersion]struct{}, len(values))
	var previous PolicyVersion
	for i, value := range values {
		if err := requiredRef("policy_type", value.PolicyType); err != nil { return err }
		if err := requiredRef("policy_ref", value.PolicyRef); err != nil { return err }
		if err := requiredRef("policy_version", value.PolicyVersion); err != nil { return err }
		if _, exists := seen[value]; exists { return fmt.Errorf("policy_snapshot contains duplicate policy tuple") }
		seen[value] = struct{}{}
		if i > 0 && (previous.PolicyType > value.PolicyType ||
			(previous.PolicyType == value.PolicyType && previous.PolicyRef > value.PolicyRef) ||
			(previous.PolicyType == value.PolicyType && previous.PolicyRef == value.PolicyRef && previous.PolicyVersion >= value.PolicyVersion)) {
			return fmt.Errorf("policy_snapshot.policies must be sorted by policy_type, policy_ref, policy_version")
		}
		previous = value
	}
	return nil
}
