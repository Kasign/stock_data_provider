package domain

import (
	"testing"
	"time"
)

func TestEvaluateNewsAuthorizationAgainstCurrentPolicies(t *testing.T) {
	binding := loadNewsAuthorizationGolden(t)
	binding.AuthorizationRef.Decision = AuthorizationDecisionAllowed
	binding.AuthorizationRef.Conditions = []string{}
	decidedAt, ok := parseUTCDateTime(binding.AuthorizationRef.DecidedAt)
	if !ok {
		t.Fatal("golden decided_at is invalid")
	}
	now := decidedAt.Add(time.Minute)
	current := CurrentAuthorizationPolicySet{
		EvaluatedAt: now,
		Policies: []CurrentAuthorizationPolicy{
			{PolicyType: "endpoint", PolicyRef: "sec-edgar-filings", PolicyVersion: "1.0.0", Status: AuthorizationApproved, Allows: true},
			{PolicyType: "provider_output", PolicyRef: "tavily-output", PolicyVersion: "3.0.0", Status: AuthorizationApproved, Allows: true},
			{PolicyType: "source", PolicyRef: "sec-edgar", PolicyVersion: "2.1.0", Status: AuthorizationApproved, Allows: true},
		},
	}
	if decision := EvaluateNewsAuthorizationAgainstCurrentPolicies(binding, current, now); !decision.Allowed {
		t.Fatalf("matching current policies rejected: %+v", decision)
	}

	changed := current
	changed.Policies = append([]CurrentAuthorizationPolicy(nil), current.Policies...)
	changed.Policies[0].PolicyVersion = "1.1.0"
	if decision := EvaluateNewsAuthorizationAgainstCurrentPolicies(binding, changed, now); decision.Allowed ||
		decision.ReasonCode != "news.authorization.policy_version_changed" {
		t.Fatalf("changed policy version was not rejected: %+v", decision)
	}

	revoked := current
	revoked.Policies = append([]CurrentAuthorizationPolicy(nil), current.Policies...)
	revoked.Policies[0].Status = AuthorizationRevoked
	if decision := EvaluateNewsAuthorizationAgainstCurrentPolicies(binding, revoked, now); decision.Allowed ||
		decision.ReasonCode != "news.authorization.policy_revoked" {
		t.Fatalf("revoked policy was not rejected: %+v", decision)
	}

	denied := current
	denied.Policies = append([]CurrentAuthorizationPolicy(nil), current.Policies...)
	denied.Policies[0].Allows = false
	if decision := EvaluateNewsAuthorizationAgainstCurrentPolicies(binding, denied, now); decision.Allowed ||
		decision.ReasonCode != "news.authorization.current_policy_denied" {
		t.Fatalf("current scope denial was not rejected: %+v", decision)
	}
}

func TestEvaluateNewsAuthorizationAgainstCurrentPoliciesRejectsIncompleteSnapshot(t *testing.T) {
	binding := loadNewsAuthorizationGolden(t)
	binding.AuthorizationRef.Decision = AuthorizationDecisionAllowed
	binding.AuthorizationRef.Conditions = []string{}
	decidedAt, _ := parseUTCDateTime(binding.AuthorizationRef.DecidedAt)
	now := decidedAt.Add(time.Minute)
	current := CurrentAuthorizationPolicySet{EvaluatedAt: now}
	decision := EvaluateNewsAuthorizationAgainstCurrentPolicies(binding, current, now)
	if decision.Allowed || decision.ReasonCode != "news.authorization.current_policy_unavailable" {
		t.Fatalf("incomplete current policy snapshot was not rejected: %+v", decision)
	}
}
