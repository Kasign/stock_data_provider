package claim

import (
	"strings"
	"testing"
	"time"
)

func claimTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil { t.Fatal(err) }
	return parsed
}

func validHypothesis(t *testing.T) Claim {
	t.Helper()
	method := "breakout_follow_through"
	expected := "close remains above the prior resistance for three sessions"
	return Claim{
		ClaimID: "claim:research:0001",
		ClaimVersion: 1,
		Statement: "The stock may sustain its breakout",
		EpistemicType: EpistemicHypothesis,
		ScopeRefs: []string{"600000.SH"},
		SystemAsOf: claimTime(t, "2026-11-06T05:00:00Z"),
		SupportingEvidenceRefs: []string{"evidence:research:0001"},
		ContradictingEvidenceRefs: []string{"evidence:research:0002"},
		RequiredDataRefs: []string{"daily_bars:600000.SH"},
		ValidationMethod: &method,
		ExpectedObservation: &expected,
		InvalidationConditions: []string{"close falls below breakout level"},
		Status: StatusTesting,
		Limitations: []string{"single-security hypothesis"},
	}
}

func TestHypothesisValidate(t *testing.T) {
	value := validHypothesis(t)
	if err := value.Validate(); err != nil { t.Fatalf("Validate() error = %v", err) }
}

func TestReportedFactDoesNotRequireHypothesisFields(t *testing.T) {
	value := validHypothesis(t)
	value.EpistemicType = EpistemicReportedFact
	value.ValidationMethod = nil
	value.ExpectedObservation = nil
	value.InvalidationConditions = nil
	value.Status = StatusProposed
	if err := value.Validate(); err != nil { t.Fatalf("reported fact rejected: %v", err) }
}

func TestClaimRejectsInvalidContractValues(t *testing.T) {
	tests := []struct { name string; mutate func(*Claim); want string }{
		{"zero version", func(c *Claim) { c.ClaimVersion = 0 }, "claim_version"},
		{"blank statement", func(c *Claim) { c.Statement = " " }, "statement"},
		{"unknown epistemic type", func(c *Claim) { c.EpistemicType = "fact" }, "epistemic_type"},
		{"unknown status", func(c *Claim) { c.Status = "accepted" }, "status"},
		{"nil scope refs", func(c *Claim) { c.ScopeRefs = nil }, "scope_refs"},
		{"duplicate supporting evidence", func(c *Claim) { c.SupportingEvidenceRefs = []string{"e1", "e1"} }, "duplicate"},
		{"same evidence both stances", func(c *Claim) { c.ContradictingEvidenceRefs = []string{"evidence:research:0001"} }, "both supporting and contradicting"},
		{"missing validation method", func(c *Claim) { c.ValidationMethod = nil }, "validation_method"},
		{"missing expected observation", func(c *Claim) { c.ExpectedObservation = nil }, "expected_observation"},
		{"padded validation method", func(c *Claim) { value := " breakout_follow_through "; c.ValidationMethod = &value }, "surrounding whitespace"},
		{"padded expected observation", func(c *Claim) { value := " close remains above resistance "; c.ExpectedObservation = &value }, "surrounding whitespace"},
		{"empty invalidation conditions", func(c *Claim) { c.InvalidationConditions = []string{} }, "non-empty array"},
		{"non UTC cutoff", func(c *Claim) { c.SystemAsOf = time.Date(2026, 11, 6, 13, 0, 0, 0, time.FixedZone("CST", 8*60*60)) }, "must be UTC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			value := validHypothesis(t)
			tt.mutate(&value)
			if err := value.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestClaimVersionMustBeAppendOnly(t *testing.T) {
	previous := validHypothesis(t)
	next := previous
	next.ClaimVersion = 2
	next.SystemAsOf = previous.SystemAsOf.Add(time.Minute)
	if err := next.ValidateNextVersion(previous); err != nil { t.Fatalf("contiguous version rejected: %v", err) }

	next.ClaimVersion = 3
	if err := next.ValidateNextVersion(previous); err == nil || !strings.Contains(err.Error(), "increment by exactly one") {
		t.Fatalf("version gap should fail, got %v", err)
	}
	next = previous
	next.ClaimVersion = 2
	next.ClaimID = "claim:research:other"
	if err := next.ValidateNextVersion(previous); err == nil || !strings.Contains(err.Error(), "claim_id cannot change") {
		t.Fatalf("changed ID should fail, got %v", err)
	}
}


func TestClaimValidateNextVersionRejectsIntegerOverflow(t *testing.T) {
	previous := validHypothesis(t)
	previous.ClaimVersion = int(^uint(0) >> 1)
	next := previous
	next.ClaimVersion = 1
	if err := next.ValidateNextVersion(previous); err == nil || !strings.Contains(err.Error(), "integer limit") {
		t.Fatalf("ValidateNextVersion() error = %v, want integer limit rejection", err)
	}
}


func TestClaimStatusTransitionRules(t *testing.T) {
	allowed := []struct {
		from ClaimStatus
		to   ClaimStatus
	}{
		{StatusProposed, StatusTesting},
		{StatusTesting, StatusSupported},
		{StatusTesting, StatusContradicted},
		{StatusTesting, StatusInvalidated},
		{StatusTesting, StatusUnknown},
	}
	for _, pair := range allowed {
		if !CanTransition(pair.from, pair.to) {
			t.Errorf("CanTransition(%s, %s) = false, want true", pair.from, pair.to)
		}
	}

	blocked := []struct {
		from ClaimStatus
		to   ClaimStatus
	}{
		{StatusProposed, StatusSupported},
		{StatusTesting, StatusProposed},
		{StatusSupported, StatusTesting},
		{StatusContradicted, StatusTesting},
		{StatusInvalidated, StatusTesting},
		{StatusUnknown, StatusTesting},
	}
	for _, pair := range blocked {
		if CanTransition(pair.from, pair.to) {
			t.Errorf("CanTransition(%s, %s) = true, want false", pair.from, pair.to)
		}
	}
}

func TestClaimValidateNextVersionEnforcesStatusTransitions(t *testing.T) {
	tests := []struct {
		name      string
		from      ClaimStatus
		to        ClaimStatus
		wantError bool
	}{
		{"same status revision", StatusTesting, StatusTesting, false},
		{"proposed to testing", StatusProposed, StatusTesting, false},
		{"testing to supported", StatusTesting, StatusSupported, false},
		{"testing to unknown", StatusTesting, StatusUnknown, false},
		{"proposed skips testing", StatusProposed, StatusSupported, true},
		{"testing returns to proposed", StatusTesting, StatusProposed, true},
		{"supported is terminal", StatusSupported, StatusTesting, true},
		{"contradicted is terminal", StatusContradicted, StatusTesting, true},
		{"invalidated is terminal", StatusInvalidated, StatusTesting, true},
		{"unknown has no declared recovery", StatusUnknown, StatusTesting, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			previous := validHypothesis(t)
			previous.Status = tt.from
			next := previous
			next.ClaimVersion++
			next.SystemAsOf = previous.SystemAsOf.Add(time.Minute)
			next.Status = tt.to
			err := next.ValidateNextVersion(previous)
			if tt.wantError && (err == nil || !strings.Contains(err.Error(), "status transition")) {
				t.Fatalf("ValidateNextVersion() error = %v, want status transition rejection", err)
			}
			if !tt.wantError && err != nil {
				t.Fatalf("ValidateNextVersion() error = %v, want success", err)
			}
		})
	}
}
