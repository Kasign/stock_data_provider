package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type runResultReaderStub struct {
	result RunResultReadRecord
	err    error
	called string
}

func (s *runResultReaderStub) GetRunResult(_ context.Context, runRef string) (RunResultReadRecord, error) {
	s.called = runRef
	return s.result, s.err
}

func runResultStringPointer(value string) *string { return &value }

func terminalRunWithResultRef() CreateRunResult {
	resultRef := "result-1"
	return CreateRunResult{
		Run: domain.Run{
			Ref: "run-1", Version: 3, Status: domain.RunSucceeded,
			ResultSummaryRef: runResultStringPointer("summary-1"), ResultRef: &resultRef,
			InputSnapshotRefs: []string{"snapshot-1"}, ExcludedStages: []domain.ExcludedStage{},
		},
		Stages: []domain.Stage{{Ref: "stage-1", RunRef: "run-1", Code: "context", Status: domain.StageSucceeded, Version: 2}},
	}
}

func validRunResultReadRecord() RunResultReadRecord {
	return RunResultReadRecord{
		ResultRef:     "result-1",
		RunRef:        "run-1",
		SchemaVersion: "1.0.0",
		Payload:       json.RawMessage(`{"schema_version":"1.0.0","run_ref":"run-1"}`),
	}
}

func TestGetRunResultReturnsAuthorizedPersistedReport(t *testing.T) {
	runs := &runReaderStub{result: terminalRunWithResultRef()}
	results := &runResultReaderStub{result: validRunResultReadRecord()}
	authorizer := &getRunAuthorizerStub{}
	service, err := NewGetRunResultService(getRunIdentityStub{caller: "owner"}, authorizer, runs, results)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.GetRunResult(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultRef != "result-1" || got.RunRef != "run-1" || authorizer.calls != 1 || results.called != "run-1" {
		t.Fatalf("unexpected GetRunResult output: %+v authorizer=%+v reader=%s", got, authorizer, results.called)
	}
}

func TestGetRunResultRejectsUnauthorizedCallerBeforeStorage(t *testing.T) {
	runs := &runReaderStub{result: terminalRunWithResultRef()}
	results := &runResultReaderStub{result: validRunResultReadRecord()}
	service, err := NewGetRunResultService(getRunIdentityStub{caller: "owner"}, &getRunAuthorizerStub{err: ErrRunAccessDenied}, runs, results)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetRunResult(context.Background(), "run-1"); !errors.Is(err, ErrRunAccessDenied) {
		t.Fatalf("GetRunResult error = %v, want ErrRunAccessDenied", err)
	}
	if runs.called != "" || results.called != "" {
		t.Fatal("unauthorized request reached storage")
	}
}

func TestGetRunResultRejectsRunWithoutPersistedResultRef(t *testing.T) {
	terminalWithoutRef := terminalRunWithResultRef()
	terminalWithoutRef.Run.ResultRef = nil
	runs := &runReaderStub{result: terminalWithoutRef}
	results := &runResultReaderStub{result: validRunResultReadRecord()}
	service := newGetRunResultServiceForTest(t, runs, results)
	if _, err := service.GetRunResult(context.Background(), "run-1"); err == nil {
		t.Fatal("terminal Run without result_ref was accepted")
	}
	if results.called != "" {
		t.Fatal("Run without result_ref reached RunResult reader")
	}
}

func TestGetRunResultRejectsMismatchedPersistedIdentity(t *testing.T) {
	for name, mutate := range map[string]func(*RunResultReadRecord){
		"wrong result ref": func(value *RunResultReadRecord) { value.ResultRef = "other-result" },
		"wrong run ref":    func(value *RunResultReadRecord) { value.RunRef = "other-run" },
		"wrong schema":     func(value *RunResultReadRecord) { value.SchemaVersion = "9.9.9" },
		"unbound payload": func(value *RunResultReadRecord) {
			value.Payload = json.RawMessage(`{"schema_version":"1.0.0","run_ref":"other-run"}`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			runs := &runReaderStub{result: terminalRunWithResultRef()}
			record := validRunResultReadRecord()
			mutate(&record)
			results := &runResultReaderStub{result: record}
			service := newGetRunResultServiceForTest(t, runs, results)
			if _, err := service.GetRunResult(context.Background(), "run-1"); err == nil {
				t.Fatalf("mismatched RunResult identity %q was accepted", name)
			}
		})
	}
}

func newGetRunResultServiceForTest(t *testing.T, runs RunReader, results RunResultReader) *GetRunResultService {
	t.Helper()
	service, err := NewGetRunResultService(getRunIdentityStub{caller: "owner"}, &getRunAuthorizerStub{}, runs, results)
	if err != nil {
		t.Fatal(err)
	}
	return service
}
