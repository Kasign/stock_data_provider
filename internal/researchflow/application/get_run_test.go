package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type getRunIdentityStub struct {
	caller string
	err    error
}

func (s getRunIdentityStub) AuthenticatedCallerRef(context.Context) (string, error) {
	return s.caller, s.err
}
func (getRunIdentityStub) OriginatingRequestID(context.Context) *string { return nil }
func (getRunIdentityStub) CorrelationID(context.Context) string { return "" }

type getRunAuthorizerStub struct {
	caller, run string
	err error
	calls int
}

func (s *getRunAuthorizerStub) AuthorizeRunRead(_ context.Context, caller, run string) error {
	s.calls++
	s.caller, s.run = caller, run
	return s.err
}
func (*getRunAuthorizerStub) AuthorizeRunCancellation(context.Context, string, string) error { return nil }

func newGetRunService(t *testing.T, reader RunReader, authorizer *getRunAuthorizerStub) *GetRunService {
	t.Helper()
	service, err := NewGetRunService(getRunIdentityStub{caller: "owner"}, authorizer, reader)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

type runReaderStub struct {
	result CreateRunResult
	err    error
	called string
}

func (s *runReaderStub) GetRun(_ context.Context, ref string) (CreateRunResult, error) {
	s.called = ref
	return s.result, s.err
}

func validReadResult() CreateRunResult {
	return CreateRunResult{
		Run: domain.Run{
			Ref: "run-1", Version: 3, Status: domain.RunRunning,
			InputSnapshotRefs: []string{"snapshot-1"}, ExcludedStages: []domain.ExcludedStage{},
		},
		Stages: []domain.Stage{{Ref: "stage-1", RunRef: "run-1", Code: "context", Status: domain.StageReady, Version: 2}},
	}
}

func TestGetRunReturnsValidatedAggregate(t *testing.T) {
	reader := &runReaderStub{result: validReadResult()}
	authorizer := &getRunAuthorizerStub{}
	service, err := NewGetRunService(getRunIdentityStub{caller: "owner"}, authorizer, reader)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.GetRun(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Run.Ref != "run-1" || len(got.Stages) != 1 || reader.called != "run-1" ||
		authorizer.caller != "owner" || authorizer.run != "run-1" || authorizer.calls != 1 {
		t.Fatalf("unexpected GetRun result: %#v", got)
	}
}

func TestGetRunRejectsUnauthenticatedOrUnauthorizedCallerBeforeStorage(t *testing.T) {
	for _, tc := range []struct {
		name string
		identity RequestIdentity
		authErr error
	}{
		{name: "missing caller", identity: getRunIdentityStub{}},
		{name: "identity error", identity: getRunIdentityStub{err: errors.New("identity unavailable")}},
		{name: "access denied", identity: getRunIdentityStub{caller: "owner"}, authErr: ErrRunAccessDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &runReaderStub{result: validReadResult()}
			authorizer := &getRunAuthorizerStub{err: tc.authErr}
			service, err := NewGetRunService(tc.identity, authorizer, reader)
			if err != nil { t.Fatal(err) }
			_, err = service.GetRun(context.Background(), "run-1")
			if err == nil {
				t.Fatal("expected caller rejection")
			}
			if reader.called != "" {
				t.Fatal("unauthenticated/unauthorized request reached Run reader")
			}
		})
	}
}

func TestGetRunRejectsInvalidReferenceBeforeStorage(t *testing.T) {
	reader := &runReaderStub{result: validReadResult()}
	service := newGetRunService(t, reader, &getRunAuthorizerStub{})
	for _, ref := range []string{"", " run-1", "run-1 "} {
		if _, err := service.GetRun(context.Background(), ref); !errors.Is(err, domain.ErrInvalidCommand) {
			t.Fatalf("GetRun(%q) error = %v, want ErrInvalidCommand", ref, err)
		}
	}
	if reader.called != "" {
		t.Fatal("invalid Run reference reached Storage")
	}
}

func TestGetRunRejectsCrossRunAndDuplicateStages(t *testing.T) {
	cases := []struct {
		name   string
		stages []domain.Stage
	}{
		{name: "cross run", stages: []domain.Stage{{Ref: "stage-1", RunRef: "other", Code: "context", Version: 1}}},
		{name: "duplicate ref", stages: []domain.Stage{
			{Ref: "stage-1", RunRef: "run-1", Code: "context", Version: 1},
			{Ref: "stage-1", RunRef: "run-1", Code: "quant", Version: 1},
		}},
		{name: "duplicate code", stages: []domain.Stage{
			{Ref: "stage-1", RunRef: "run-1", Code: "context", Version: 1},
			{Ref: "stage-2", RunRef: "run-1", Code: "context", Version: 1},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validReadResult()
			result.Stages = tc.stages
			service := newGetRunService(t, &runReaderStub{result: result}, &getRunAuthorizerStub{})
			if _, err := service.GetRun(context.Background(), "run-1"); err == nil {
				t.Fatal("expected inconsistent Stage set to be rejected")
			}
		})
	}
}

func TestGetRunRequiresResultSummaryForTerminalRun(t *testing.T) {
	result := validReadResult()
	result.Run.Status = domain.RunSucceeded
	service := newGetRunService(t, &runReaderStub{result: result}, &getRunAuthorizerStub{})
	if _, err := service.GetRun(context.Background(), "run-1"); err == nil {
		t.Fatal("terminal Run without ResultSummary must be rejected")
	}
}

func TestGetRunPropagatesStorageError(t *testing.T) {
	want := errors.New("storage unavailable")
	service := newGetRunService(t, &runReaderStub{err: want}, &getRunAuthorizerStub{})
	if _, err := service.GetRun(context.Background(), "run-1"); !errors.Is(err, want) {
		t.Fatalf("GetRun error = %v, want wrapped storage error", err)
	}
}
