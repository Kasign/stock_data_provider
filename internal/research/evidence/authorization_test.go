package evidence

import (
	"strings"
	"testing"
	"time"
)

const researchAuthorizationFingerprint = "059366735753b82b018143b2f300bc9bc6d2763416af7316271dccd9682f1199"

func researchAuthorizationGolden(t *testing.T) (AuthorizationRef, AuthorizationPolicySnapshot) {
	t.Helper()
	decidedAt, err := time.Parse(time.RFC3339Nano, "2026-11-06T05:03:00Z")
	if err != nil { t.Fatal(err) }
	snapshot := AuthorizationPolicySnapshot{
		SnapshotRef: "policy-snapshot:research-evidence-access:v1",
		Policies: []PolicyVersion{{PolicyType: "research_evidence_access", PolicyRef: "research-evidence-access", PolicyVersion: "1.0.0"}},
	}
	ref := AuthorizationRef{
		AuthorizationRef: "auth_research_001",
		PolicyOwner: SourceResearch,
		PolicyVersion: snapshot.SnapshotRef,
		Decision: AuthorizationAllowed,
		PurposeCode: PurposeResearchAnalysis,
		ContentLevel: ContentSummary,
		SubjectRef: "service:researchflow",
		ResourceRef: "research_evidence:evidence_001",
		DecidedAt: decidedAt,
		Conditions: []string{},
		DecisionFingerprint: researchAuthorizationFingerprint,
	}
	return ref, snapshot
}

func TestAuthorizationFingerprintMatchesGOVCTR006ResearchGolden(t *testing.T) {
	ref, snapshot := researchAuthorizationGolden(t)
	fingerprint, err := ref.ComputeDecisionFingerprint(snapshot)
	if err != nil { t.Fatalf("ComputeDecisionFingerprint() error = %v", err) }
	if fingerprint != researchAuthorizationFingerprint { t.Fatalf("fingerprint = %s, want %s", fingerprint, researchAuthorizationFingerprint) }
	if err := ref.Validate(snapshot); err != nil { t.Fatalf("Validate() error = %v", err) }
}

func TestAuthorizationValidateRejectsPolicyAndDecisionInconsistency(t *testing.T) {
	tests := []struct {
		name string
		mutate func(*AuthorizationRef, *AuthorizationPolicySnapshot)
		want string
	}{
		{"policy snapshot mismatch", func(a *AuthorizationRef, s *AuthorizationPolicySnapshot) { s.SnapshotRef = "another-snapshot" }, "policy_version"},
		{"empty policy list", func(a *AuthorizationRef, s *AuthorizationPolicySnapshot) { s.Policies = []PolicyVersion{} }, "must not be empty"},
		{"duplicate policy tuple", func(a *AuthorizationRef, s *AuthorizationPolicySnapshot) { s.Policies = append(s.Policies, s.Policies[0]) }, "duplicate policy tuple"},
		{"unsorted policy tuple", func(a *AuthorizationRef, s *AuthorizationPolicySnapshot) {
			s.Policies = []PolicyVersion{
				{PolicyType: "z_policy", PolicyRef: "policy", PolicyVersion: "1"},
				{PolicyType: "a_policy", PolicyRef: "policy", PolicyVersion: "1"},
			}
			a.DecisionFingerprint, _ = a.ComputeDecisionFingerprint(*s)
		}, "must be sorted by policy_type"},
		{"allowed with conditions", func(a *AuthorizationRef, s *AuthorizationPolicySnapshot) { a.Conditions = []string{"attribution_required"} }, "allowed decision"},
		{"conditional without conditions", func(a *AuthorizationRef, s *AuthorizationPolicySnapshot) { a.Decision = AuthorizationConditional }, "requires conditions"},
		{"unsorted conditions", func(a *AuthorizationRef, s *AuthorizationPolicySnapshot) { a.Conditions = []string{"z_condition", "a_condition"} }, "sorted in ascending"},
		{"fingerprint tampered", func(a *AuthorizationRef, s *AuthorizationPolicySnapshot) { a.ResourceRef = "research_evidence:other" }, "decision_fingerprint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, snapshot := researchAuthorizationGolden(t)
			tt.mutate(&ref, &snapshot)
			if err := ref.Validate(snapshot); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidateEvidenceAuthorizationBindingUsesTrustedScope(t *testing.T) {
	ref, snapshot := researchAuthorizationGolden(t)
	e := Evidence{EvidenceID: "evidence_001", SourceOwner: SourceResearch, ContentLevel: ContentSummary, AuthorizationRef: "auth_research_001"}
	if err := ValidateEvidenceAuthorizationBinding(e, ref, snapshot, "service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis); err != nil {
		t.Fatalf("valid binding rejected: %v", err)
	}
	tests := []struct {
		name string
		subject string
		resource string
		purpose PurposeCode
		content ContentLevel
		owner SourceOwner
		want string
	}{
		{"untrusted subject mismatch", "service:other", "research_evidence:evidence_001", PurposeResearchAnalysis, ContentSummary, SourceResearch, "trusted caller"},
		{"resource mismatch", "service:researchflow", "research_evidence:other", PurposeResearchAnalysis, ContentSummary, SourceResearch, "requested resource"},
		{"purpose mismatch", "service:researchflow", "research_evidence:evidence_001", PurposeRiskReview, ContentSummary, SourceResearch, "requested purpose"},
		{"content mismatch", "service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, ContentFull, SourceResearch, "content_level"},
		{"owner mismatch", "service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, ContentSummary, SourceNews, "source_owner"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := e
			candidate.SourceOwner = tt.owner
			candidate.ContentLevel = tt.content
			err := ValidateEvidenceAuthorizationBinding(candidate, ref, snapshot, tt.subject, tt.resource, tt.purpose)
			if err == nil || !strings.Contains(err.Error(), tt.want) { t.Fatalf("binding error = %v, want containing %q", err, tt.want) }
		})
	}
}

func TestValidateEvidenceAuthorizationBindingRejectsDifferentAuthorizationRef(t *testing.T) {
	ref, snapshot := researchAuthorizationGolden(t)
	e := Evidence{SourceOwner: SourceResearch, ContentLevel: ContentSummary, AuthorizationRef: "auth_other"}
	err := ValidateEvidenceAuthorizationBinding(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis)
	if err == nil || !strings.Contains(err.Error(), "does not match supplied") {
		t.Fatalf("expected mismatched authorization_ref to be rejected, got %v", err)
	}
}

func TestValidateEvidenceAuthorizationBindingRejectsNonAllowedDecision(t *testing.T) {
	for _, decision := range []AuthorizationDecision{AuthorizationDenied, AuthorizationConditional, AuthorizationUnknown} {
		t.Run(string(decision), func(t *testing.T) {
			ref, snapshot := researchAuthorizationGolden(t)
			ref.Decision = decision
			if decision == AuthorizationConditional { ref.Conditions = []string{"attribution_required"} }
			ref.DecisionFingerprint, _ = ref.ComputeDecisionFingerprint(snapshot)
			err := ValidateEvidenceAuthorizationBinding(Evidence{SourceOwner: SourceResearch, ContentLevel: ContentSummary}, ref, snapshot,
				"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis)
			if err == nil || !strings.Contains(err.Error(), "not readable") { t.Fatalf("expected non-readable decision to be rejected, got %v", err) }
		})
	}
}


func TestValidateEvidenceAuthorizationBindingAtEnforcesExpiryBoundary(t *testing.T) {
	now := time.Date(2026, 11, 6, 5, 10, 0, 0, time.UTC)
	expiresAt := now.Add(time.Minute)
	ref, snapshot := researchAuthorizationGolden(t)
	ref.ExpiresAt = &expiresAt
	ref.DecisionFingerprint, _ = ref.ComputeDecisionFingerprint(snapshot)
	e := Evidence{SourceOwner: SourceResearch, ContentLevel: ContentSummary, AuthorizationRef: ref.AuthorizationRef}

	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, ref.DecidedAt.Add(-time.Nanosecond)); err == nil || !strings.Contains(err.Error(), "not yet effective") {
		t.Fatalf("authorization before decision time must be rejected, got %v", err)
	}
	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, ref.DecidedAt); err != nil {
		t.Fatalf("authorization at decision time should be effective: %v", err)
	}
	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, now); err != nil {
		t.Fatalf("unexpired authorization rejected: %v", err)
	}
	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, expiresAt.Add(-time.Nanosecond)); err != nil {
		t.Fatalf("authorization immediately before expiry rejected: %v", err)
	}
	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, expiresAt); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("authorization at expiry must be rejected, got %v", err)
	}
	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, expiresAt.Add(time.Nanosecond)); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("authorization after expiry must be rejected, got %v", err)
	}
	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, time.Time{}); err == nil || !strings.Contains(err.Error(), "authorization_check_time") {
		t.Fatalf("zero check time must be rejected, got %v", err)
	}
	nonUTCNow := now.In(time.FixedZone("non-UTC", 0))
	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, nonUTCNow); err == nil || !strings.Contains(err.Error(), "authorization_check_time") {
		t.Fatalf("zero-offset non-UTC check time must be rejected, got %v", err)
	}
}


func TestValidateEvidenceAuthorizationBindingAtAllowsNoFixedExpiry(t *testing.T) {
	now := time.Date(2026, 11, 6, 5, 10, 0, 0, time.UTC)
	ref, snapshot := researchAuthorizationGolden(t)
	e := Evidence{SourceOwner: SourceResearch, ContentLevel: ContentSummary, AuthorizationRef: ref.AuthorizationRef}
	if err := ValidateEvidenceAuthorizationBindingAt(e, ref, snapshot,
		"service:researchflow", "research_evidence:evidence_001", PurposeResearchAnalysis, now); err != nil {
		t.Fatalf("authorization with expires_at=null rejected: %v", err)
	}
}


func TestAuthorizationValidateAcceptsPolicyTupleFieldsContainingSeparator(t *testing.T) {
	ref, snapshot := researchAuthorizationGolden(t)
	snapshot.Policies = []PolicyVersion{
		{PolicyType: "a", PolicyRef: "b\x00c", PolicyVersion: "d"},
		{PolicyType: "a\x00b", PolicyRef: "c", PolicyVersion: "d"},
	}
	var err error
	ref.DecisionFingerprint, err = ref.ComputeDecisionFingerprint(snapshot)
	if err != nil {
		t.Fatalf("ComputeDecisionFingerprint() error = %v", err)
	}
	if err := ref.Validate(snapshot); err != nil {
		t.Fatalf("distinct sorted policy tuples containing separator bytes were rejected: %v", err)
	}
}
