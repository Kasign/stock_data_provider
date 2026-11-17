package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/state"
)

type CancellationFinalizationStore interface {
	LoadDeadlineFinalizationSnapshot(context.Context, string) (DeadlineFinalizationSnapshot, error)
	FinalizeCancellationRun(context.Context, CancellationFinalizationRecord) (bool, error)
}

type CancellationFinalizationRecord struct {
	RunRef               string
	ExpectedVersion      uint64
	ResultSummaryRef     string
	SummarySchemaVersion string
	OutboxRef            string
	TerminalStatus       domain.RunStatus
	TerminationReason    string
	Summary              domain.ResultSummary
}

type CancellationFinalizerConfig struct {
	SummarySchemaVersion string
}

type CancellationFinalizationResult struct {
	Run       domain.Run
	Finalized bool
}

// CancellationFinalizer aggregates a fully stopped user-cancelled Run and
// delegates the atomic ResultSummary + Outbox + Run transaction to Storage.
type CancellationFinalizer struct {
	store  CancellationFinalizationStore
	clock  DatabaseClock
	ids    IDGenerator
	config CancellationFinalizerConfig
}

func NewCancellationFinalizer(store CancellationFinalizationStore, clock DatabaseClock, ids IDGenerator, config CancellationFinalizerConfig) (*CancellationFinalizer, error) {
	if store == nil || clock == nil || ids == nil || strings.TrimSpace(config.SummarySchemaVersion) == "" {
		return nil, errors.New("Researchflow cancellation finalizer dependencies or configuration are invalid")
	}
	return &CancellationFinalizer{store: store, clock: clock, ids: ids, config: config}, nil
}

func (f *CancellationFinalizer) Finalize(ctx context.Context, runRef string) (CancellationFinalizationResult, error) {
	if f == nil || f.store == nil || f.clock == nil || f.ids == nil {
		return CancellationFinalizationResult{}, errors.New("Researchflow cancellation finalizer is not configured")
	}
	if strings.TrimSpace(runRef) == "" {
		return CancellationFinalizationResult{}, errors.New("Researchflow cancellation Run reference is required")
	}
	snapshot, err := f.store.LoadDeadlineFinalizationSnapshot(ctx, runRef)
	if err != nil {
		return CancellationFinalizationResult{}, fmt.Errorf("load Researchflow cancellation snapshot: %w", err)
	}
	if snapshot.Run.Ref != runRef || snapshot.Run.Version == 0 ||
		snapshot.Run.Status != domain.RunCancelling || snapshot.Run.TerminationReason != "user_cancelled" {
		return CancellationFinalizationResult{}, errors.New("Researchflow Run is not eligible for user-cancellation finalization")
	}
	now, err := f.clock.NowUTC(ctx)
	if err != nil {
		return CancellationFinalizationResult{}, fmt.Errorf("read database UTC time for cancellation finalization: %w", err)
	}
	run, err := state.FinalizeCancellation(snapshot.Run, snapshot.Stages, snapshot.Attempts,
		snapshot.PropagationPolicies, snapshot.HardRiskStageCodes, now.UTC())
	if err != nil {
		return CancellationFinalizationResult{}, fmt.Errorf("aggregate cancelled Researchflow Run: %w", err)
	}
	if run.Status != domain.RunCancelled || run.ResultSummary == nil {
		return CancellationFinalizationResult{}, errors.New("Researchflow cancellation aggregation produced an invalid terminal result")
	}
	summaryRef, err := f.ids.NewRef(ctx, "result_summary")
	if err != nil {
		return CancellationFinalizationResult{}, fmt.Errorf("generate Researchflow ResultSummary reference: %w", err)
	}
	outboxRef, err := f.ids.NewRef(ctx, "outbox")
	if err != nil {
		return CancellationFinalizationResult{}, fmt.Errorf("generate Researchflow Outbox reference: %w", err)
	}
	record := CancellationFinalizationRecord{
		RunRef: runRef, ExpectedVersion: snapshot.Run.Version,
		ResultSummaryRef: summaryRef, SummarySchemaVersion: f.config.SummarySchemaVersion,
		OutboxRef: outboxRef, TerminalStatus: run.Status,
		TerminationReason: snapshot.Run.TerminationReason, Summary: *run.ResultSummary,
	}
	finalized, err := f.store.FinalizeCancellationRun(ctx, record)
	if err != nil {
		return CancellationFinalizationResult{}, fmt.Errorf("persist Researchflow cancellation finalization: %w", err)
	}
	if finalized {
		run.ResultSummaryRef = &summaryRef
	}
	return CancellationFinalizationResult{Run: run, Finalized: finalized}, nil
}
