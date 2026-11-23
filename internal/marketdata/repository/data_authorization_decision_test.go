package repository

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateDataAuthorizationDecisionRejectsMissingAndNonAllowedDecisions(t *testing.T) {
	cases := []DataAuthorizationDecision{
		{},
		{AuthorizationRef: json.RawMessage(`{"authorization_ref":"auth_1","policy_owner":"data","decision":"denied"}`), PolicySnapshot: json.RawMessage(`{}`)},
		{AuthorizationRef: json.RawMessage(`{"authorization_ref":"auth_1","policy_owner":"data","policy_version":"policy-snapshot:data:v1","decision":"allowed","purpose_code":"user_display","content_level":"full","subject_ref":"principal:owner","resource_ref":"data_snapshot:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","decided_at":"2026-11-23T00:00:00Z","expires_at":null,"conditions":[],"decision_fingerprint":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","schema_version":"1.0.0"}`), PolicySnapshot: json.RawMessage(`{"snapshot_ref":"policy-snapshot:data:v1","policies":[{"policy_type":"data_access","policy_ref":"dailybars-v2-access","policy_version":"1.0.0"}],"schema_version":"1.0.0"}`)},
	}
	for i, value := range cases {
		if _, _, _, _, _, _, err := validateDataAuthorizationDecision(value); !errors.Is(err, ErrDataAuthorizationDecisionInvalid) {
			t.Fatalf("case %d: expected fail-closed invalid decision, got %v", i, err)
		}
	}
}
