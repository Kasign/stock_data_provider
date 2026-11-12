package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/gowebpki/jcs"

	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/result"
)

type fixedPrincipal struct {
	principal Principal
	err       error
}

func (p fixedPrincipal) Resolve(context.Context) (Principal, error) { return p.principal, p.err }

type fixedReader struct {
	value   AuthorizedRecord
	err     error
	request ReadRequest
	calls   int
}

func (r *fixedReader) ReadAuthorized(_ context.Context, request ReadRequest) (AuthorizedRecord, error) {
	r.calls++
	r.request = request
	return r.value, r.err
}

func getResultFixture(t *testing.T, now time.Time) AuthorizedRecord {
	t.Helper()
	payload := json.RawMessage(`{"decision_id":"risk-decision-001","status":"unknown","rule_set_version":"1.0.0","evaluated_at":"2026-11-09T01:00:00Z","system_as_of":"2026-11-09T00:59:00Z","reasons":[{"code":"INPUT_QUALITY_UNKNOWN","severity":"block","status":"unknown","safe_message":"输入质量无法确认","evidence_refs":["evidence:quality:001"]}],"input_refs":["quant_result:001"]}`)
	stored, err := manifest.BuildWithPayload(manifest.Input{
		ResultType: "risk", Scope: "stock", SecurityID: "600000.SH", Period: "1d",
		QualityStatus: "unknown", ResultStatus: "insufficient_data", SchemaVersion: "1.0.0",
		PayloadSchemaVersion: "1.0.0", AlgorithmVersion: "1.0.0",
		ParameterFingerprint: "sha256:parameters", SystemAsOf: now.Add(-time.Minute),
		InputSnapshotRefs: []string{"snapshot:001"},
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	record := result.Record{Manifest: stored, Payload: payload}
	policy := PolicySnapshot{SchemaVersion: "1.0.0", SnapshotRef: "policy-snapshot:quant:1", Policies: []PolicyVersion{
		{PolicyType: "quant_result_access", PolicyRef: "quant-result-access", PolicyVersion: "1.0.0"},
	}}
	ref := AuthorizationRef{
		SchemaVersion: "1.0.0", AuthorizationRef: "authz:quant:001", PolicyOwner: "quant",
		PolicyVersion: policy.SnapshotRef, Decision: "allowed", PurposeCode: quantPurposeCode,
		ContentLevel: quantContentLevel, SubjectRef: "service:researchflow", ResourceRef: stored.ResultID,
		DecidedAt: now.Add(-time.Minute).Format(time.RFC3339Nano), Conditions: []string{},
	}
	input := authorizationFingerprintInput{
		SchemaVersion: ref.SchemaVersion, PolicyOwner: ref.PolicyOwner, PolicyVersion: ref.PolicyVersion,
		Decision: ref.Decision, PurposeCode: ref.PurposeCode, ContentLevel: ref.ContentLevel,
		SubjectRef: ref.SubjectRef, ResourceRef: ref.ResourceRef, DecidedAt: ref.DecidedAt,
		ExpiresAt: ref.ExpiresAt, Conditions: ref.Conditions, EvaluatedPolicyVersions: policy.Policies,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		t.Fatal(err)
	}
	ref.DecisionFingerprint, err = manifest.FingerprintPayload(canonical)
	if err != nil {
		t.Fatal(err)
	}
	return AuthorizedRecord{Record: record, AuthorizationRef: ref, PolicySnapshot: policy}
}

func TestGetResultReturnsValidatedAuthorizedEnvelope(t *testing.T) {
	now := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	reader := &fixedReader{value: getResultFixture(t, now)}
	usecase, err := NewGetResult(reader, fixedPrincipal{principal: Principal{SubjectRef: "service:researchflow"}}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	got, err := usecase.Execute(context.Background(), reader.value.Record.Manifest.ResultID)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.ResultID != reader.value.Record.Manifest.ResultID || got.AuthorizationRef != reader.value.AuthorizationRef.AuthorizationRef || got.ResultType != "risk" {
		t.Fatalf("unexpected envelope: %+v", got)
	}
	if reader.calls != 1 || reader.request.SubjectRef != "service:researchflow" ||
		reader.request.ResourceRef != got.ResultID || reader.request.PurposeCode != quantPurposeCode ||
		reader.request.ContentLevel != quantContentLevel || !reader.request.At.Equal(now) {
		t.Fatalf("authorization read request mismatch: %+v", reader.request)
	}
}

func TestGetResultFailsClosedForInvalidAuthorization(t *testing.T) {
	now := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		mutate func(*AuthorizedRecord)
	}{
		{"subject mismatch", func(v *AuthorizedRecord) { v.AuthorizationRef.SubjectRef = "service:other" }},
		{"resource mismatch", func(v *AuthorizedRecord) { v.AuthorizationRef.ResourceRef = "result:other" }},
		{"wrong owner", func(v *AuthorizedRecord) { v.AuthorizationRef.PolicyOwner = "research" }},
		{"denied", func(v *AuthorizedRecord) { v.AuthorizationRef.Decision = "denied" }},
		{"policy snapshot mismatch", func(v *AuthorizedRecord) { v.PolicySnapshot.SnapshotRef = "policy-snapshot:other" }},
		{"empty policy snapshot", func(v *AuthorizedRecord) { v.PolicySnapshot.Policies = []PolicyVersion{} }},
		{"expired", func(v *AuthorizedRecord) {
			expired := now.Format(time.RFC3339Nano)
			v.AuthorizationRef.ExpiresAt = &expired
		}},
		{"bad fingerprint", func(v *AuthorizedRecord) { v.AuthorizationRef.DecisionFingerprint = "invalid" }},
		{"conditional", func(v *AuthorizedRecord) {
			v.AuthorizationRef.Decision = "conditional"
			v.AuthorizationRef.Conditions = []string{"needs_review"}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			value := getResultFixture(t, now)
			tc.mutate(&value)
			reader := &fixedReader{value: value}
			usecase, _ := NewGetResult(reader, fixedPrincipal{principal: Principal{SubjectRef: "service:researchflow"}}, func() time.Time { return now })
			got, err := usecase.Execute(context.Background(), value.Record.Manifest.ResultID)
			if !errors.Is(err, ErrInvalidAuthorization) || got.ResultID != "" || got.Payload != nil {
				t.Fatalf("expected fail-closed authorization error, got=%+v err=%v", got, err)
			}
		})
	}
}

func TestGetResultRejectsTamperedRecord(t *testing.T) {
	now := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	value := getResultFixture(t, now)
	value.Record.Payload = []byte(`{"unexpected":true}`)
	reader := &fixedReader{value: value}
	usecase, _ := NewGetResult(reader, fixedPrincipal{principal: Principal{SubjectRef: "service:researchflow"}}, func() time.Time { return now })
	got, err := usecase.Execute(context.Background(), value.Record.Manifest.ResultID)
	if !errors.Is(err, ErrInvalidStoredRecord) || got.ResultID != "" {
		t.Fatalf("tampered record should not escape: %+v %v", got, err)
	}
}

func TestGetResultRejectsInvalidRequestAndDoesNotCallReader(t *testing.T) {
	now := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	reader := &fixedReader{value: getResultFixture(t, now)}
	usecase, _ := NewGetResult(reader, fixedPrincipal{principal: Principal{SubjectRef: "service:researchflow"}}, func() time.Time { return now })
	for _, id := range []string{"", "result-1", "quant_result:sha256:ABC"} {
		if got, err := usecase.Execute(context.Background(), id); !errors.Is(err, ErrInvalidGetResultRequest) || got.ResultID != "" {
			t.Fatalf("invalid result id accepted: %q", id)
		}
	}
	if reader.calls != 0 {
		t.Fatalf("reader called for invalid id: %d", reader.calls)
	}
}

func TestGetResultRejectsReaderFailureAndNonUTCClock(t *testing.T) {
	now := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	reader := &fixedReader{value: getResultFixture(t, now), err: errors.New("policy service unavailable")}
	usecase, _ := NewGetResult(reader, fixedPrincipal{principal: Principal{SubjectRef: "service:researchflow"}}, func() time.Time { return now })
	got, err := usecase.Execute(context.Background(), reader.value.Record.Manifest.ResultID)
	if !errors.Is(err, ErrResultUnavailable) || got.ResultID != "" {
		t.Fatalf("reader failure leaked result: %+v %v", got, err)
	}
	reader.err = nil
	usecase, _ = NewGetResult(reader, fixedPrincipal{principal: Principal{SubjectRef: "service:researchflow"}}, func() time.Time {
		return now.In(time.FixedZone("UTC+8", 8*60*60))
	})
	got, err = usecase.Execute(context.Background(), reader.value.Record.Manifest.ResultID)
	if !errors.Is(err, ErrResultUnavailable) || got.ResultID != "" {
		t.Fatalf("non-UTC trusted clock accepted: %+v %v", got, err)
	}
}
