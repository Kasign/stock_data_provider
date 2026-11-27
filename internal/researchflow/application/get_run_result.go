package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resultvalidation"
)

// ErrRunResultNotFound means the authorized Run has no persistable complete
// report reference. It must not be replaced by an empty-shell report.
var ErrRunResultNotFound = errors.New("Researchflow RunResult not found")

// RunResultReadRecord is the Storage-neutral persisted complete report.
type RunResultReadRecord struct {
	ResultRef     string
	RunRef        string
	SchemaVersion string
	Payload       json.RawMessage
}

// RunResultReader is the read-side port for Researchflow-owned RunResults.
type RunResultReader interface {
	GetRunResult(context.Context, string) (RunResultReadRecord, error)
}

// GetRunResultService exposes an authorized Application query without leaking
// persistence models or turning a domain result reference into a full report.
type GetRunResultService struct {
	identity   RequestIdentity
	authorizer RunAccessAuthorizer
	runs       RunReader
	results    RunResultReader
}

func NewGetRunResultService(identity RequestIdentity, authorizer RunAccessAuthorizer, runs RunReader, results RunResultReader) (*GetRunResultService, error) {
	if identity == nil || authorizer == nil || runs == nil || results == nil {
		return nil, errors.New("Researchflow GetRunResult dependencies are incomplete")
	}
	return &GetRunResultService{identity: identity, authorizer: authorizer, runs: runs, results: results}, nil
}

func (s *GetRunResultService) GetRunResult(ctx context.Context, runRef string) (RunResultReadRecord, error) {
	if s == nil || s.identity == nil || s.authorizer == nil || s.runs == nil || s.results == nil {
		return RunResultReadRecord{}, errors.New("Researchflow GetRunResult service is not configured")
	}
	if strings.TrimSpace(runRef) == "" || len(runRef) > 255 || strings.TrimSpace(runRef) != runRef {
		return RunResultReadRecord{}, fmt.Errorf("%w: Run reference is invalid", domain.ErrInvalidCommand)
	}
	callerRef, err := s.identity.AuthenticatedCallerRef(ctx)
	if err != nil || strings.TrimSpace(callerRef) == "" {
		return RunResultReadRecord{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	if err := s.authorizer.AuthorizeRunRead(ctx, callerRef, runRef); err != nil {
		if errors.Is(err, ErrRunAccessDenied) {
			return RunResultReadRecord{}, ErrRunAccessDenied
		}
		return RunResultReadRecord{}, fmt.Errorf("authorize Researchflow RunResult read: %w", err)
	}
	run, err := s.runs.GetRun(ctx, runRef)
	if err != nil {
		return RunResultReadRecord{}, fmt.Errorf("read Researchflow Run %q for RunResult: %w", runRef, err)
	}
	if run.Run.Ref != runRef || !reportBearingTerminal(run.Run.Status, run.Run.TerminationReason) ||
		run.Run.ResultRef == nil || strings.TrimSpace(*run.Run.ResultRef) == "" {
		return RunResultReadRecord{}, ErrRunResultNotFound
	}
	result, err := s.results.GetRunResult(ctx, runRef)
	if err != nil {
		return RunResultReadRecord{}, fmt.Errorf("read Researchflow RunResult: %w", err)
	}
	if result.ResultRef != *run.Run.ResultRef || result.RunRef != runRef ||
		result.SchemaVersion != resultvalidation.RunResultPayloadSchemaVersion ||
		len(result.Payload) == 0 || !json.Valid(result.Payload) {
		return RunResultReadRecord{}, errors.New("Researchflow RunResult reader returned an inconsistent identity")
	}
	var identity struct {
		SchemaVersion string `json:"schema_version"`
		RunRef        string `json:"run_ref"`
	}
	if err := json.Unmarshal(result.Payload, &identity); err != nil ||
		identity.SchemaVersion != result.SchemaVersion || identity.RunRef != runRef {
		return RunResultReadRecord{}, errors.New("Researchflow RunResult payload is not bound to the authorized Run")
	}
	result.Payload = append(json.RawMessage(nil), result.Payload...)
	return result, nil
}
