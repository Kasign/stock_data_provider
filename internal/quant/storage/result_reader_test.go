package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	platformauthorization "example.com/stock-analysis-flow/internal/platform/authorization"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/result"
)

type authorizedPolicyGuardStub struct {
	state quantapplication.ResultAccessPolicyState
	err error
	calls int
}

func (stub *authorizedPolicyGuardStub) WithCurrentResultAccessPolicy(
	_ context.Context,
	read func(quantapplication.ResultAccessPolicyState) error,
) error {
	stub.calls++
	if stub.err != nil {
		return stub.err
	}
	return read(stub.state)
}

type authorizedRecordReaderStub struct {
	record result.Record
	err error
	calls int
}

func (stub *authorizedRecordReaderStub) GetRecord(_ context.Context, _ string) (result.Record, error) {
	stub.calls++
	return stub.record, stub.err
}

type resultAssociationVerifierStub struct {
	err error
	calls int
	runRef string
	resultID string
}

func (stub *resultAssociationVerifierStub) VerifyResultAssociation(_ context.Context, runRef, resultID string) error {
	stub.calls++
	stub.runRef, stub.resultID = runRef, resultID
	return stub.err
}

func authorizedResultFixture(t *testing.T) (result.Record, quantapplication.PolicySnapshot, quantapplication.ReadRequest) {
	t.Helper()
	payload := json.RawMessage(`{"decision_id":"risk-decision-001","status":"unknown","rule_set_version":"1.0.0","evaluated_at":"2026-11-25T00:00:00Z","system_as_of":"2026-11-24T23:59:00Z","reasons":[{"code":"INPUT_QUALITY_UNKNOWN","severity":"block","status":"unknown","safe_message":"输入质量无法确认","evidence_refs":["evidence:quality:001"]}],"input_refs":["quant_result:001"]}`)
	now := time.Date(2026, 11, 25, 1, 0, 0, 0, time.UTC)
	stored, err := manifest.BuildWithPayload(manifest.Input{
		ResultType: "risk", Scope: "stock", SecurityID: "600000.SH", Period: "1d",
		QualityStatus: "unknown", ResultStatus: "insufficient_data", SchemaVersion: "1.0.0",
		PayloadSchemaVersion: "1.0.0", AlgorithmVersion: "1.0.0",
		ParameterFingerprint: "sha256:parameters", SystemAsOf: now.Add(-time.Hour),
		InputSnapshotRefs: []string{"snapshot:001"},
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	policy := quantapplication.PolicySnapshot{
		SchemaVersion: "1.0.0", SnapshotRef: "policy-snapshot:quant:result-access:1",
		Policies: []quantapplication.PolicyVersion{{
			PolicyType: "quant_result_access", PolicyRef: "quant-result-access", PolicyVersion: "1",
		}},
	}
	request := quantapplication.ReadRequest{
		RunRef: "run:researchflow:001", SubjectRef: "service:researchflow",
		ResourceRef: stored.ResultID, PurposeCode: "quant_analysis", ContentLevel: "full", At: now,
	}
	return result.Record{Manifest: stored, Payload: payload}, policy, request
}

func TestMySQLAuthorizedResultReaderReturnsCurrentRunBoundAuthorization(t *testing.T) {
	record, policy, request := authorizedResultFixture(t)
	guard := &authorizedPolicyGuardStub{state: quantapplication.ResultAccessPolicyState{Version: 1, Enabled: true, Snapshot: &policy}}
	records := &authorizedRecordReaderStub{record: record}
	association := &resultAssociationVerifierStub{}
	reader, err := NewMySQLAuthorizedResultReader(guard, records, association)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.ReadAuthorized(context.Background(), request)
	if err != nil {
		t.Fatalf("ReadAuthorized() error = %v", err)
	}
	if got.Record.Manifest.ResultID != request.ResourceRef ||
		got.AuthorizationRef.SubjectRef != request.SubjectRef ||
		got.AuthorizationRef.ResourceRef != request.ResourceRef ||
		got.AuthorizationRef.PolicyVersion != policy.SnapshotRef ||
		got.AuthorizationRef.DecidedAt != request.At.Format(time.RFC3339Nano) ||
		!got.AuthorizationRef.ExpiresAtPresent || got.AuthorizationRef.ExpiresAt != nil ||
		guard.calls != 1 || association.calls != 1 || records.calls != 1 ||
		association.runRef != request.RunRef || association.resultID != request.ResourceRef {
		t.Fatalf("authorized read did not bind the current policy, Run and result: %+v", got)
	}
	refJSON, err := json.Marshal(got.AuthorizationRef)
	if err != nil {
		t.Fatal(err)
	}
	policyJSON, err := json.Marshal(got.PolicySnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := platformauthorization.VerifyFingerprint(refJSON, policyJSON); err != nil {
		t.Fatalf("generated authorization fingerprint is invalid: %v", err)
	}
}

func TestMySQLAuthorizedResultReaderRejectsDisabledPolicyAndMissingRunAssociation(t *testing.T) {
	record, policy, request := authorizedResultFixture(t)
	for _, test := range []struct {
		name string
		state quantapplication.ResultAccessPolicyState
		associationErr error
	}{
		{name: "disabled", state: quantapplication.ResultAccessPolicyState{Version: 1, Enabled: false, Snapshot: &policy}},
		{name: "missing association", state: quantapplication.ResultAccessPolicyState{Version: 1, Enabled: true, Snapshot: &policy}, associationErr: quantapplication.ErrResultAssociationNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			guard := &authorizedPolicyGuardStub{state: test.state}
			records := &authorizedRecordReaderStub{record: record}
			association := &resultAssociationVerifierStub{err: test.associationErr}
			reader, err := NewMySQLAuthorizedResultReader(guard, records, association)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.ReadAuthorized(context.Background(), request); !errors.Is(err, quantapplication.ErrAccessDenied) {
				t.Fatalf("ReadAuthorized() error = %v, want access denied", err)
			}
			if records.calls != 0 {
				t.Fatalf("Quant record was read after access denial: %d", records.calls)
			}
		})
	}
}

func TestMySQLAuthorizedResultReaderFailsClosedForInvalidAssociationAndRecord(t *testing.T) {
	record, policy, request := authorizedResultFixture(t)
	for _, test := range []struct {
		name string
		associationErr error
		mutateRecord bool
		want error
	}{
		{name: "association backend unavailable", associationErr: errors.New("Researchflow storage unavailable"), want: quantapplication.ErrAuthorizationUnavailable},
		{name: "corrupt stored record", mutateRecord: true, want: quantapplication.ErrInvalidStoredRecord},
	} {
		t.Run(test.name, func(t *testing.T) {
			guard := &authorizedPolicyGuardStub{state: quantapplication.ResultAccessPolicyState{Version: 1, Enabled: true, Snapshot: &policy}}
			if test.mutateRecord {
				record.Payload = json.RawMessage(`{"tampered":true}`)
			}
			records := &authorizedRecordReaderStub{record: record}
			association := &resultAssociationVerifierStub{err: test.associationErr}
			reader, err := NewMySQLAuthorizedResultReader(guard, records, association)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.ReadAuthorized(context.Background(), request); !errors.Is(err, test.want) {
				t.Fatalf("ReadAuthorized() error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestMySQLAuthorizedResultReaderRejectsInvalidRequestBeforePolicyRead(t *testing.T) {
	record, policy, request := authorizedResultFixture(t)
	guard := &authorizedPolicyGuardStub{state: quantapplication.ResultAccessPolicyState{Version: 1, Enabled: true, Snapshot: &policy}}
	reader, err := NewMySQLAuthorizedResultReader(guard, &authorizedRecordReaderStub{record: record}, &resultAssociationVerifierStub{})
	if err != nil {
		t.Fatal(err)
	}
	request.SubjectRef = "owner"
	if _, err := reader.ReadAuthorized(context.Background(), request); !errors.Is(err, quantapplication.ErrInvalidGetResultRequest) {
		t.Fatalf("invalid principal request error = %v", err)
	}
	if guard.calls != 0 {
		t.Fatalf("policy was read for an invalid request: %d", guard.calls)
	}
}
