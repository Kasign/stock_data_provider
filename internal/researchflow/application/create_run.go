package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gowebpki/jcs"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var (
	ErrUnauthenticated         = errors.New("authenticated caller is required")
	ErrUnauthenticated = errors.New("authenticated caller is required")
	ErrIdempotencyConflict     = domain.ErrIdempotencyConflict
	ErrTemplateVersionMismatch = errors.New("template registry returned a different version")
	ErrIdempotencyConflict = domain.ErrIdempotencyConflict
)
	AuthenticatedCallerRef(context.Context) (string, error)
	OriginatingRequestID(context.Context) *string
	CorrelationID(context.Context) string
}

type DatabaseClock interface {
	NowUTC(context.Context) (time.Time, error)
}

type IDGenerator interface {
	NewRef(context.Context, string) (string, error)
}

type TemplateRegistry interface {
	Get(context.Context, string, string) (template.WorkflowTemplate, error)
}

type IdempotencyScope struct {
	CallerRef string
	Operation string
	Key       string
	Key string
}

	Run      domain.Run
	Stages   []domain.Stage
type CreateRunResult struct {
	Run domain.Run
	Stages []domain.Stage
	Replayed bool
	Scope            IdempotencyScope
	Fingerprint      string
	Run              domain.Run
	Stages           []domain.Stage
}
	Parameters       json.RawMessage

type CreateRunRecord struct {
	Scope IdempotencyScope
// Run, Stage set, resolved-template snapshot, parameters and idempotency record
// in one transaction. It must recheck deadline_at against database UTC time at commit.
// A concurrent duplicate with the same fingerprint returns the original result
// with Replayed=true; a different fingerprint returns ErrIdempotencyConflict.
// Terminal finalization must allocate result_summary_ref and persist the ResultSummary
// resource and terminal Run atomically; terminal Run DTOs without that ref are invalid.
type RunRepository interface {
	FindIdempotentRun(context.Context, IdempotencyScope) (CreateRunResult, string, bool, error)
	CreateRunAtomic(context.Context, CreateRunRecord) (CreateRunResult, error)
}

type CreateRunService struct {
	identity      RequestIdentity
	clock         DatabaseClock
	ids           IDGenerator
	templates     TemplateRegistry
	schemas       template.ParameterSchemaValidator
	identity RequestIdentity
	gates         template.HardRiskGateRegistry
	runs          RunRepository
	clock DatabaseClock
	ids IDGenerator
	templates TemplateRegistry
	identity RequestIdentity,
	clock DatabaseClock,
	ids IDGenerator,
	templates TemplateRegistry,
	schemas template.ParameterSchemaValidator,
	applicability template.ApplicabilityEvaluator,
	gates template.HardRiskGateRegistry,
	runs RunRepository,
) (*CreateRunService, error) {
	if identity == nil || clock == nil || ids == nil || templates == nil || schemas == nil ||
		applicability == nil || runs == nil {
		return nil, errors.New("create run service dependencies are incomplete")
	}
	return &CreateRunService{
		identity: identity, clock: clock, ids: ids, templates: templates,
		schemas: schemas, applicability: applicability, gates: gates, runs: runs,
	}, nil
}

func (s *CreateRunService) CreateRun(ctx context.Context, command domain.CreateRunCommand) (CreateRunResult, error) {
	callerRef, err := s.identity.AuthenticatedCallerRef(ctx)
	if err != nil || strings.TrimSpace(callerRef) == "" {
		return CreateRunResult{}, fmt.Errorf("%w: %v", ErrUnauthenticated, err)
	}
	command.OriginatingRequestID = cloneString(s.identity.OriginatingRequestID(ctx))
	command.CorrelationID = strings.TrimSpace(s.identity.CorrelationID(ctx))
	if strings.TrimSpace(command.IdempotencyKey) == "" || len(command.IdempotencyKey) > 128 ||
		strings.TrimSpace(command.SchemaVersion) == "" || strings.TrimSpace(command.TemplateRef) == "" ||
		strings.TrimSpace(command.TemplateVersion) == "" {
		return CreateRunResult{}, fmt.Errorf("%w: schema, idempotency key and exact template identity are required", domain.ErrInvalidCommand)
	}
	fingerprint, err := domain.RequestFingerprint(callerRef, command)
	if err != nil {
		return CreateRunResult{}, err
	}
	scope := IdempotencyScope{CallerRef: callerRef, Operation: "CreateRun", Key: command.IdempotencyKey}
	if err != nil { return CreateRunResult{}, err }
	if err != nil {
		return CreateRunResult{}, fmt.Errorf("lookup create run idempotency: %w", err)
	}
	scope := IdempotencyScope{CallerRef:callerRef, Operation:"CreateRun", Key:command.IdempotencyKey}
		if err := domain.ValidateIdempotentReplay(existingFingerprint, fingerprint); err != nil {
			return CreateRunResult{}, err
		}
	existing, existingFingerprint, found, err := s.runs.FindIdempotentRun(ctx, scope)
	if err != nil { return CreateRunResult{}, fmt.Errorf("lookup create run idempotency: %w", err) }
	if found {
		if err := domain.ValidateIdempotentReplay(existingFingerprint, fingerprint); err != nil { return CreateRunResult{}, err }
		existing.Replayed = true
		if err != nil {
			return CreateRunResult{}, fmt.Errorf("generate correlation id: %w", err)
		}
		return existing, nil
	}
	if command.CorrelationID == "" {
	if err != nil {
		return CreateRunResult{}, fmt.Errorf("load workflow template %q version %q: %w", command.TemplateRef, command.TemplateVersion, err)
	}
		command.CorrelationID, err = s.ids.NewRef(ctx, "correlation")
		if err != nil { return CreateRunResult{}, fmt.Errorf("generate correlation id: %w", err) }
	}

	if err != nil {
		return CreateRunResult{}, fmt.Errorf("read database UTC clock: %w", err)
	}
	tmpl, err := s.templates.Get(ctx, command.TemplateRef, command.TemplateVersion)
	if err := domain.ValidateCreateRunCommand(command, now, tmpl.AllowEmptyInputs); err != nil {
		return CreateRunResult{}, err
	}
	if err != nil { return CreateRunResult{}, fmt.Errorf("load workflow template %q version %q: %w", command.TemplateRef, command.TemplateVersion, err) }
	if tmpl.Ref != command.TemplateRef || tmpl.Version != command.TemplateVersion {
	if err != nil {
		return CreateRunResult{}, fmt.Errorf("resolve workflow template: %w", err)
	}
		return CreateRunResult{}, ErrTemplateVersionMismatch
	if err != nil {
		return CreateRunResult{}, fmt.Errorf("generate run ref: %w", err)
	}
	}
	now, err := s.clock.NowUTC(ctx)
	if err != nil { return CreateRunResult{}, fmt.Errorf("read database UTC clock: %w", err) }
	now = now.UTC()
		if err != nil {
			return CreateRunResult{}, fmt.Errorf("generate stage ref: %w", err)
		}
	if err := domain.ValidateCreateRunCommand(command, now, tmpl.AllowEmptyInputs); err != nil { return CreateRunResult{}, err }

	resolved, err := template.Resolve(ctx, tmpl, command.Parameters, s.schemas, s.applicability, s.gates)
	if err != nil { return CreateRunResult{}, fmt.Errorf("resolve workflow template: %w", err) }
	runRef, err := s.ids.NewRef(ctx, "run")
	if err != nil {
		return CreateRunResult{}, fmt.Errorf("%w: canonicalize parameters for snapshot: %v", domain.ErrInvalidCommand, err)
	}
	if err != nil { return CreateRunResult{}, fmt.Errorf("generate run ref: %w", err) }

	stageRefs := make(map[string]string, len(resolved.Stages))
		dependencies := make([]string, 0, len(resolvedStage.Dependencies))
		for _, code := range resolvedStage.Dependencies {
			ref, ok := stageRefs[code]
			if !ok {
				return CreateRunResult{}, fmt.Errorf("resolved dependency %q has no stage reference", code)
			}
			if !ok { return CreateRunResult{}, fmt.Errorf("resolved dependency %q has no stage reference", code) }
			dependencies = append(dependencies, ref)
		}
		if initialStatus == "" {
			initialStatus = domain.StagePending
		}
		initialStatus := resolvedStage.InitialStatus
		if initialStatus == "" { initialStatus = domain.StagePending }
		var finishedAt *time.Time
			finishedAt = &finished
		}
		reasonCodes := append([]string(nil), resolvedStage.ReasonCodes...)
		if reasonCodes == nil {
			reasonCodes = []string{}
		}
		if reasonCodes == nil { reasonCodes = []string{} }
			Ref: stageRefs[definition.Code], RunRef: runRef, Code: definition.Code,
			Status: initialStatus, Version: 1, Required: definition.Required,
			Dependencies: dependencies, RetryPolicyRef: definition.Retry.Ref,
			AttemptNo: 0, ReasonCodes: reasonCodes, CreatedAt: now, FinishedAt: finishedAt,
		stages = append(stages, domain.Stage{
			Ref:stageRefs[definition.Code], RunRef:runRef, Code:definition.Code,
			Status:initialStatus, Version:1, Required:definition.Required,
		Ref: runRef, SchemaVersion: command.SchemaVersion, Status: domain.RunCreated, Version: 1,
		SystemAsOf: command.SystemAsOf.UTC(), DeadlineAt: command.DeadlineAt.UTC(),
		TemplateRef: resolved.Ref, TemplateVersion: resolved.Version, TemplateFingerprint: resolved.Fingerprint,
		ExcludedStages:       cloneExcludedStages(resolved.Excluded),
		InputSnapshotRefs:    append([]string{}, command.InputSnapshotRefs...),
		OriginatingRequestID: cloneString(command.OriginatingRequestID), CorrelationID: command.CorrelationID,
		CreatedAt: now,
			Dependencies:dependencies, RetryPolicyRef:definition.Retry.Ref,
			AttemptNo:0, ReasonCodes:reasonCodes, CreatedAt:now, FinishedAt:finishedAt,
		Scope: scope, Fingerprint: fingerprint, Run: run, Stages: stages,
		ResolvedTemplate: resolved, Parameters: append(json.RawMessage(nil), canonicalParameters...),
		})
	}
	if err != nil {
		return CreateRunResult{}, fmt.Errorf("persist create run atomically: %w", err)
	}
	run := domain.Run{
		Ref:runRef, SchemaVersion:command.SchemaVersion, Status:domain.RunCreated, Version:1,
		SystemAsOf:command.SystemAsOf.UTC(), DeadlineAt:command.DeadlineAt.UTC(),
		TemplateRef:resolved.Ref, TemplateVersion:resolved.Version, TemplateFingerprint:resolved.Fingerprint,
	if value == nil {
		return nil
	}
		ExcludedStages:cloneExcludedStages(resolved.Excluded),
		InputSnapshotRefs:append([]string{}, command.InputSnapshotRefs...),
		OriginatingRequestID:cloneString(command.OriginatingRequestID), CorrelationID:command.CorrelationID,
		CreatedAt:now,
	}
	if values == nil {
		return []domain.ExcludedStage{}
	}
	record := CreateRunRecord{
		Scope:scope, Fingerprint:fingerprint, Run:run, Stages:stages,
