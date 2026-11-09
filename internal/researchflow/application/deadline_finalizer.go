package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/state"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type DeadlineFinalizationSnapshot struct {
	Run                  domain.Run
	Stages               []domain.Stage
	Attempts             []domain.Attempt
	PropagationPolicies  map[string]template.PropagationPolicy
	HardRiskStageCodes   map[string]bool
	RequiresHardRiskGate bool
}

type DeadlineFinalizationRecord struct {
	RunRef               string
	ExpectedVersion      uint64
	ResultSummaryRef     string
	SummarySchemaVersion string
	OutboxRef            string
	TerminalStatus       domain.RunStatus
	TerminationReason    string
	Summary              domain.ResultSummary
}

type DeadlineFinalizationStore interface {
	LoadDeadlineFinalizationSnapshot(context.Context, string) (DeadlineFinalizationSnapshot, error)
	FinalizeDeadlineRun(context.Context, DeadlineFinalizationRecord) (bool, error)
}

type DeadlineFinalizerConfig struct {
	SummarySchemaVersion string
}

type DeadlineFinalizationResult struct {
	Run       domain.Run
	Finalized bool
}

// DeadlineFinalizer derives the terminal result after Storage has fenced expired
// Attempts. The Store adapter must map FinalizeDeadlineRun to the atomic
// ResultSummary + Outbox + Run transaction.
type DeadlineFinalizer struct {
	store  DeadlineFinalizationStore
	clock  DatabaseClock
	ids    IDGenerator
	config DeadlineFinalizerConfig
}

func NewDeadlineFinalizer(store DeadlineFinalizationStore, clock DatabaseClock, ids IDGenerator, config DeadlineFinalizerConfig) (*DeadlineFinalizer, error) {
	if store == nil || clock == nil || ids == nil || strings.TrimSpace(config.SummarySchemaVersion) == "" {
		return nil, errors.New("Researchflow deadline finalizer dependencies or configuration are invalid")
	}
	return &DeadlineFinalizer{store: store, clock: clock, ids: ids, config: config}, nil
}

func (f *DeadlineFinalizer) Finalize(ctx context.Context, runRef string) (DeadlineFinalizationResult, error) {
	if f == nil || f.store == nil || f.clock == nil || f.ids == nil {
		return DeadlineFinalizationResult{}, errors.New("Researchflow deadline finalizer is not configured")
	}
	if strings.TrimSpace(runRef) == "" {
		return DeadlineFinalizationResult{}, errors.New("Researchflow deadline finalizer Run reference is required")
	}
	snapshot, err := f.store.LoadDeadlineFinalizationSnapshot(ctx, runRef)
	if err != nil {
		return DeadlineFinalizationResult{}, fmt.Errorf("load expired Researchflow Run snapshot: %w", err)
	}
	if snapshot.Run.Ref != runRef || snapshot.Run.Version == 0 ||
		snapshot.Run.Status != domain.RunRunning || snapshot.Run.TerminationReason != "deadline_exceeded" {
		return DeadlineFinalizationResult{}, errors.New("Researchflow Run is not in the persisted deadline-finalization state")
	}
	now, err := f.clock.NowUTC(ctx)
	if err != nil {
		return DeadlineFinalizationResult{}, fmt.Errorf("read database UTC time for deadline finalization: %w", err)
	}
	run, err := state.FinalizeDeadline(snapshot.Run, snapshot.Stages, snapshot.Attempts,
		snapshot.PropagationPolicies, snapshot.HardRiskStageCodes, snapshot.RequiresHardRiskGate, now.UTC())
	if err != nil {
		return DeadlineFinalizationResult{}, fmt.Errorf("aggregate expired Researchflow Run: %w", err)
	}
	if run.ResultSummary == nil || !run.Status.Terminal() ||
		(run.Status != domain.RunFailed && run.Status != domain.RunPartial) {
		return DeadlineFinalizationResult{}, errors.New("Researchflow deadline aggregation produced an invalid terminal result")
	}
	summaryRef, err := f.ids.NewRef(ctx, "result_summary")
	if err != nil {
		return DeadlineFinalizationResult{}, fmt.Errorf("generate Researchflow ResultSummary reference: %w", err)
	}
	outboxRef, err := f.ids.NewRef(ctx, "outbox")
	if err != nil {
		return DeadlineFinalizationResult{}, fmt.Errorf("generate Researchflow Outbox reference: %w", err)
	}
	record := DeadlineFinalizationRecord{
		RunRef: runRef, ExpectedVersion: snapshot.Run.Version,
		ResultSummaryRef: summaryRef, SummarySchemaVersion: f.config.SummarySchemaVersion,
		OutboxRef: outboxRef, TerminalStatus: run.Status,
		TerminationReason: snapshot.Run.TerminationReason, Summary: *run.ResultSummary,
	}
	finalized, err := f.store.FinalizeDeadlineRun(ctx, record)
	if err != nil {
		return DeadlineFinalizationResult{}, fmt.Errorf("persist Researchflow deadline finalization: %w", err)
	}
	if finalized {
		run.ResultSummaryRef = &summaryRef
	}
	return DeadlineFinalizationResult{Run: run, Finalized: finalized}, nil
}

// Keep time imported in this file's public contract tests deterministic.
var _ = time.Time{}
