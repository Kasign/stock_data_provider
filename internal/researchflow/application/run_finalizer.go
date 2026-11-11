package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
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
	store  RunFinalizationSnapshotStore
	clock  DatabaseClock
	ids    IDGenerator
	config RunFinalizerConfig
}

func NewRunFinalizer(store RunFinalizationSnapshotStore, clock DatabaseClock, ids IDGenerator, config RunFinalizerConfig) (*RunFinalizer, error) {
	if store == nil || clock == nil || ids == nil || strings.TrimSpace(config.SummarySchemaVersion) == "" {
		return nil, errors.New("Researchflow run finalizer dependencies or configuration are invalid")
	}
	return &RunFinalizer{store: store, clock: clock, ids: ids, config: config}, nil
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
	finalized, err := f.store.FinalizeRunResult(ctx, record)
	if err != nil {
		return RunFinalizationResult{}, fmt.Errorf("persist Researchflow Run finalization: %w", err)
	}
	if finalized {
		run.ResultSummaryRef = &summaryRef
	}
	return RunFinalizationResult{Run: run, Finalized: finalized}, nil
}
