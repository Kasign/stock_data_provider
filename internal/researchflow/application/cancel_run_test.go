package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type cancelIdentityStub struct {
	caller string
	err    error
}

func (s cancelIdentityStub) AuthenticatedCallerRef(context.Context) (string, error) {
	return s.caller, s.err
}
func (cancelIdentityStub) OriginatingRequestID(context.Context) *string { return nil }
func (cancelIdentityStub) CorrelationID(context.Context) string         { return "" }

type cancelAuthorizerStub struct {
	caller, run string
	err         error
	calls       int
}

func (s *cancelAuthorizerStub) AuthorizeRunCancellation(_ context.Context, caller, run string) error {
	s.calls++
	s.caller, s.run = caller, run
	return s.err
}

type cancelReaderStub struct {
	results []CreateRunResult
	err     error
	calls   int
}

func (s *cancelReaderStub) GetRun(context.Context, string) (CreateRunResult, error) {
	s.calls++
	if s.err != nil {
		return CreateRunResult{}, s.err
	}
	if len(s.results) == 0 {
		return CreateRunResult{}, errors.New("no read result")
	}
	result := s.results[0]
	if len(s.results) > 1 {
		s.results = s.results[1:]
	}
	return result, nil
}

type cancelStoreStub struct {
	changed bool
	err     error
	runRef  string
	version uint64
	calls   int
}

func (s *cancelStoreStub) BeginRunCancellation(_ context.Context, ref string, version uint64) (bool, error) {
	s.calls++
	s.runRef, s.version = ref, version
	return s.changed, s.err
}

func cancellationReadResult(status domain.RunStatus, reason string, version uint64) CreateRunResult {
	result := validReadResult()
	result.Run.Status, result.Run.TerminationReason, result.Run.Version = status, reason, version
	return result
}

func newCancelService(t *testing.T, identity RequestIdentity, auth *cancelAuthorizerStub, reader *cancelReaderStub, store *cancelStoreStub) *CancelRunService {
	t.Helper()
	service, err := NewCancelRunService(identity, auth, reader, store)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func TestCancelRunPersistsIntentAndReturnsFreshSnapshot(t *testing.T) {
	reader := &cancelReaderStub{results: []CreateRunResult{
		cancellationReadResult(domain.RunRunning, "", 4),
		cancellationReadResult(domain.RunCancelling, "user_cancelled", 5),
	}}
	auth := &cancelAuthorizerStub{}
	store := &cancelStoreStub{changed: true}
	service := newCancelService(t, cancelIdentityStub{caller: "caller-1"}, auth, reader, store)
	got, err := service.CancelRun(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Started || got.Replayed || got.Run.Status != domain.RunCancelling || got.Run.Version != 5 {
		t.Fatalf("unexpected cancellation result: %#v", got)
	}
	if auth.caller != "caller-1" || auth.run != "run-1" || store.runRef != "run-1" || store.version != 4 {
		t.Fatalf("trusted identity or CAS arguments mismatch: auth=%#v store=%#v", auth, store)
	}
}

func TestCancelRunIsIdempotentWhenAlreadyCancelling(t *testing.T) {
	reader := &cancelReaderStub{results: []CreateRunResult{cancellationReadResult(domain.RunCancelling, "user_cancelled", 5)}}
	store := &cancelStoreStub{}
	service := newCancelService(t, cancelIdentityStub{caller: "caller-1"}, &cancelAuthorizerStub{}, reader, store)
	got, err := service.CancelRun(context.Background(), "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if !got.Replayed || got.Started || store.calls != 0 {
		t.Fatalf("existing cancellation must be replayed without a write: %#v", got)
	}
}

func TestCancelRunRejectsUnauthenticatedOrUnauthorizedCaller(t *testing.T) {
	for _, tc := range []struct {
		name     string
		identity RequestIdentity
		authErr  error
	}{
		{name: "missing caller", identity: cancelIdentityStub{}},
		{name: "identity error", identity: cancelIdentityStub{err: errors.New("identity unavailable")}},
		{name: "access denied", identity: cancelIdentityStub{caller: "caller-1"}, authErr: ErrRunAccessDenied},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := &cancelAuthorizerStub{err: tc.authErr}
			reader, store := &cancelReaderStub{}, &cancelStoreStub{}
			service := newCancelService(t, tc.identity, auth, reader, store)
			_, err := service.CancelRun(context.Background(), "run-1")
			if err == nil {
				t.Fatal("expected caller rejection")
			}
			if reader.calls != 0 || store.calls != 0 {
				t.Fatal("unauthenticated/unauthorized request reached Run storage")
			}
		})
	}
}

func TestCancelRunRejectsTerminalAndDeadlineRuns(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status domain.RunStatus
		reason string
	}{
		{name: "terminal", status: domain.RunSucceeded},
		{name: "deadline", status: domain.RunRunning, reason: "deadline_exceeded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := &cancelReaderStub{results: []CreateRunResult{cancellationReadResult(tc.status, tc.reason, 3)}}
			store := &cancelStoreStub{}
			service := newCancelService(t, cancelIdentityStub{caller: "caller-1"}, &cancelAuthorizerStub{}, reader, store)
			if _, err := service.CancelRun(context.Background(), "run-1"); !errors.Is(err, ErrRunCancellationDenied) {
				t.Fatalf("CancelRun error=%v, want ErrRunCancellationDenied", err)
			}
			if store.calls != 0 {
				t.Fatal("ineligible Run reached cancellation Storage")
			}
		})
	}
}

func TestCancelRunRejectsVersionRaceWhenCancellationDidNotWin(t *testing.T) {
	reader := &cancelReaderStub{results: []CreateRunResult{
		cancellationReadResult(domain.RunRunning, "", 4),
		cancellationReadResult(domain.RunRunning, "", 5),
	}}
	store := &cancelStoreStub{changed: false}
	service := newCancelService(t, cancelIdentityStub{caller: "caller-1"}, &cancelAuthorizerStub{}, reader, store)
	if _, err := service.CancelRun(context.Background(), "run-1"); !errors.Is(err, ErrRunVersionConflict) {
		t.Fatalf("CancelRun error=%v, want ErrRunVersionConflict", err)
	}
}

func TestCancelRunRejectsInvalidReferenceBeforeIdentity(t *testing.T) {
	identity := cancelIdentityStub{caller: "caller-1"}
	reader, store := &cancelReaderStub{}, &cancelStoreStub{}
	service := newCancelService(t, identity, &cancelAuthorizerStub{}, reader, store)
	for _, ref := range []string{"", " run-1", "run-1 "} {
		if _, err := service.CancelRun(context.Background(), ref); !errors.Is(err, domain.ErrInvalidCommand) {
			t.Fatalf("CancelRun(%q) error=%v", ref, err)
		}
	}
	if reader.calls != 0 || store.calls != 0 {
		t.Fatal("invalid Run ref reached storage")
	}
}
