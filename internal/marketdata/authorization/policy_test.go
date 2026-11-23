package authorization

import (
	"errors"
	"testing"
	"time"
)

func TestEvaluateRequiresExplicitSubjectPurposeAndScope(t *testing.T) {
	now := time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)
	policy := DataAccessPolicy{
		SchemaVersion: DataAccessPolicySchemaVersion,
		Status: PolicyApproved,
		AllowedSubjects: []string{"principal:service:researchflow"},
		AllowedPurposes: []string{"research_analysis"},
		RequiredScopes: []string{"data:read"},
		EffectiveAt: now.Add(-time.Hour),
	}
	request := AccessRequest{SubjectRef:"principal:service:researchflow",PurposeCode:"research_analysis",Scopes:[]string{"data:read"},Now:now}
	if err := Evaluate(policy,request); err != nil { t.Fatalf("Evaluate() error = %v",err) }
	request.SubjectRef = "principal:service:other"
	if !errors.Is(Evaluate(policy,request),ErrPolicyDenied) { t.Fatal("different subject must be denied") }
	request.SubjectRef = "principal:service:researchflow"
	request.Scopes = nil
	if !errors.Is(Evaluate(policy,request),ErrPolicyInvalid) { t.Fatal("missing trusted scopes must be rejected") }
	request.Scopes = []string{"data:read"}
	request.Now = *policy.ExpiresAt
	if !errors.Is(Evaluate(policy,request),ErrPolicyDenied) { t.Fatal("now == expires_at must be denied") }
}

func TestValidatePolicyRejectsEmptyAllowListsAndUnsortedValues(t *testing.T) {
	now := time.Now().UTC()
	policy := DataAccessPolicy{SchemaVersion:DataAccessPolicySchemaVersion,Status:PolicyApproved,
		AllowedSubjects:[]string{"principal:owner"},AllowedPurposes:[]string{"research_analysis"},
		RequiredScopes:[]string{"data:read"},EffectiveAt:now}
	if err := ValidatePolicy(policy); err != nil { t.Fatal(err) }
	policy.AllowedSubjects = nil
	if !errors.Is(ValidatePolicy(policy),ErrPolicyInvalid) { t.Fatal("empty subjects must not create an implicit allow-all policy") }
	policy.AllowedSubjects = []string{"principal:owner"}
	policy.AllowedPurposes = []string{"risk_review","research_analysis"}
	if !errors.Is(ValidatePolicy(policy),ErrPolicyInvalid) { t.Fatal("purpose codes must be sorted") }
}
