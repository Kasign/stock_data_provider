package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resultvalidation"
	"example.com/stock-analysis-flow/internal/researchflow/state"
)

type RunFinalizationSnapshotStore interface {
	LoadRunFinalizationSnapshot(context.Context, string) (DeadlineFinalizationSnapshot, error)
	FinalizeRunResult(context.Context, RunFinalizationRecord) (bool, error)
}

type RunFinalizationRecord struct {
	RunRef               string
	ExpectedVersion      uint64
	ResultSummaryRef     string
	SummarySchemaVersion string
	OutboxRef            string
	TerminalStatus       domain.RunStatus
	TerminationReason    string
	Summary              domain.ResultSummary
	ResultRef            string
	ResultSchemaVersion  string
	ResultPayload        json.RawMessage
}

// RunResultProjectionRequest carries only immutable Run identity to the
// projector. The projector must obtain protected source content through its
// Owner Facades; it never receives caller-controlled Run identity.
type RunResultProjectionRequest struct {
	RunRef          string
	SystemAsOf      time.Time
	TemplateRef     string
	TemplateVersion string
}

// RunResultProjection is an already validated, persistable complete report.
// An empty projection means no report is available; it must never become an
// empty-shell RunResult.
type RunResultProjection struct {
	ResultRef     string
	SchemaVersion string
	Payload       json.RawMessage
}

// RunResultProjector is the sole producer of a complete report projection.
type RunResultProjector interface {
	ProjectRunResult(context.Context, RunResultProjectionRequest) (*RunResultProjection, error)
}

type RunFinalizerConfig struct {
	SummarySchemaVersion string
}

type RunFinalizationResult struct {
	Run       domain.Run
	Finalized bool
}

// RunFinalizer aggregates a completed Stage set and delegates the atomic
// ResultSummary + Outbox + Run transaction to Storage.
type RunFinalizer struct {
	store     RunFinalizationSnapshotStore
	clock     DatabaseClock
	ids       IDGenerator
	projector RunResultProjector
	config    RunFinalizerConfig
}

func NewRunFinalizer(store RunFinalizationSnapshotStore, clock DatabaseClock, ids IDGenerator, projector RunResultProjector, config RunFinalizerConfig) (*RunFinalizer, error) {
	if store == nil || clock == nil || ids == nil || strings.TrimSpace(config.SummarySchemaVersion) == "" {
		return nil, errors.New("Researchflow run finalizer dependencies or configuration are invalid")
	}
	return &RunFinalizer{store: store, clock: clock, ids: ids, projector: projector, config: config}, nil
}

func (f *RunFinalizer) Finalize(ctx context.Context, runRef string) (RunFinalizationResult, error) {
	if f == nil || f.store == nil || f.clock == nil || f.ids == nil {
		return RunFinalizationResult{}, errors.New("Researchflow run finalizer is not configured")
	}
	if strings.TrimSpace(runRef) == "" {
		return RunFinalizationResult{}, errors.New("Researchflow Run reference is required")
	}
	snapshot, err := f.store.LoadRunFinalizationSnapshot(ctx, runRef)
	if err != nil {
		return RunFinalizationResult{}, fmt.Errorf("load Researchflow Run finalization snapshot: %w", err)
	}
	if snapshot.Run.Ref != runRef || snapshot.Run.Version == 0 ||
		snapshot.Run.Status != domain.RunRunning || snapshot.Run.TerminationReason != "" {
		return RunFinalizationResult{}, errors.New("Researchflow Run is not eligible for normal finalization")
	}
	now, err := f.clock.NowUTC(ctx)
	if err != nil {
		return RunFinalizationResult{}, fmt.Errorf("read database UTC time for Run finalization: %w", err)
	}
	run, err := state.AggregateRun(snapshot.Run, snapshot.Stages, snapshot.Attempts,
		snapshot.PropagationPolicies, snapshot.HardRiskStageCodes, snapshot.RequiresHardRiskGate, now.UTC())
	if err != nil {
		return RunFinalizationResult{}, fmt.Errorf("aggregate Researchflow Run: %w", err)
	}
	if run.ResultSummary == nil || !run.Status.Terminal() {
		return RunFinalizationResult{}, errors.New("Researchflow aggregation did not produce a terminal ResultSummary")
	}
	summaryRef, err := f.ids.NewRef(ctx, "result_summary")
	if err != nil {
		return RunFinalizationResult{}, fmt.Errorf("generate Researchflow ResultSummary reference: %w", err)
	}
	outboxRef, err := f.ids.NewRef(ctx, "outbox")
	if err != nil {
		return RunFinalizationResult{}, fmt.Errorf("generate Researchflow Outbox reference: %w", err)
	}
	record := RunFinalizationRecord{
		RunRef: runRef, ExpectedVersion: snapshot.Run.Version,
		ResultSummaryRef: summaryRef, SummarySchemaVersion: f.config.SummarySchemaVersion,
		OutboxRef: outboxRef, TerminalStatus: run.Status, Summary: *run.ResultSummary,
	}
	if f.projector != nil && reportBearingTerminal(run.Status, run.TerminationReason) {
		projection, err := f.projector.ProjectRunResult(ctx, RunResultProjectionRequest{
			RunRef: runRef, SystemAsOf: snapshot.Run.SystemAsOf,
			TemplateRef: snapshot.Run.TemplateRef, TemplateVersion: snapshot.Run.TemplateVersion,
		})
		if err != nil {
			return RunFinalizationResult{}, fmt.Errorf("project Researchflow RunResult: %w", err)
		}
		if projection != nil {
			if err := validateRunResultProjection(projection, runRef); err != nil {
				return RunFinalizationResult{}, err
			}
			resultRef, err := f.ids.NewRef(ctx, "run_result")
			if err != nil {
				return RunFinalizationResult{}, fmt.Errorf("generate Researchflow RunResult reference: %w", err)
			}
			record.ResultRef = resultRef
			record.ResultSchemaVersion = projection.SchemaVersion
			record.ResultPayload = append(json.RawMessage(nil), projection.Payload...)
		}
	}
	finalized, err := f.store.FinalizeRunResult(ctx, record)
	if err != nil {
		return RunFinalizationResult{}, fmt.Errorf("persist Researchflow Run finalization: %w", err)
	}
	if finalized {
		run.ResultSummaryRef = &summaryRef
		if record.ResultRef != "" {
			value := record.ResultRef
			run.ResultRef = &value
		}
	}
	return RunFinalizationResult{Run: run, Finalized: finalized}, nil
}

func reportBearingTerminal(status domain.RunStatus, terminationReason string) bool {
	return terminationReason == "" &&
		(status == domain.RunSucceeded || status == domain.RunPartial || status == domain.RunFailed)
}

func validateRunResultProjection(projection *RunResultProjection, trustedRunRef string) error {
	if projection == nil {
		return errors.New("Researchflow RunResult projector returned no projection")
	}
	if strings.TrimSpace(projection.ResultRef) == "" || projection.ResultRef != strings.TrimSpace(projection.ResultRef) ||
		projection.SchemaVersion != resultvalidation.RunResultPayloadSchemaVersion ||
		len(projection.Payload) == 0 || !json.Valid(projection.Payload) {
		return errors.New("Researchflow RunResult projection is incomplete or invalid")
	}
	var identity struct {
		SchemaVersion string `json:"schema_version"`
		RunRef        string `json:"run_ref"`
	}
	if err := json.Unmarshal(projection.Payload, &identity); err != nil ||
		identity.SchemaVersion != projection.SchemaVersion || identity.RunRef != trustedRunRef {
		return errors.New("Researchflow RunResult payload is not bound to the finalizing Run")
	}
	return nil
}
