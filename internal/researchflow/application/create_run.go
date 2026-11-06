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
	ErrUnauthenticated = errors.New("authenticated caller is required")
	ErrTemplateVersionMismatch = errors.New("template registry returned a different version")
	ErrIdempotencyConflict = domain.ErrIdempotencyConflict
)

type RequestIdentity interface {
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
	Key string
}

type CreateRunResult struct {
	Run domain.Run
	Stages []domain.Stage
	Replayed bool
}

type CreateRunRecord struct {
	Scope IdempotencyScope
	Fingerprint string
	Run domain.Run
	Stages []domain.Stage
	ResolvedTemplate template.ResolvedTemplate
	Parameters json.RawMessage
}

// RunRepository must enforce the idempotency unique key and persist the complete
// Run, Stage set, resolved-template snapshot, parameters and idempotency record
// in one transaction. It must recheck deadline_at against database UTC time at commit.
// A concurrent duplicate with the same fingerprint returns the original result
// with Replayed=true; a different fingerprint returns ErrIdempotencyConflict.
type RunRepository interface {
	FindIdempotentRun(context.Context, IdempotencyScope) (CreateRunResult, string, bool, error)
	CreateRunAtomic(context.Context, CreateRunRecord) (CreateRunResult, error)
}

type CreateRunService struct {
	identity RequestIdentity
	clock DatabaseClock
	ids IDGenerator
	templates TemplateRegistry
	schemas template.ParameterSchemaValidator
	applicability template.ApplicabilityEvaluator
	gates template.HardRiskGateRegistry
	runs RunRepository
}

func NewCreateRunService(
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
	if err != nil { return CreateRunResult{}, err }
	scope := IdempotencyScope{CallerRef:callerRef, Operation:"CreateRun", Key:command.IdempotencyKey}
	existing, existingFingerprint, found, err := s.runs.FindIdempotentRun(ctx, scope)
	if err != nil { return CreateRunResult{}, fmt.Errorf("lookup create run idempotency: %w", err) }
	if found {
		if err := domain.ValidateIdempotentReplay(existingFingerprint, fingerprint); err != nil { return CreateRunResult{}, err }
		existing.Replayed = true
		return existing, nil
	}
	if command.CorrelationID == "" {
		command.CorrelationID, err = s.ids.NewRef(ctx, "correlation")
		if err != nil { return CreateRunResult{}, fmt.Errorf("generate correlation id: %w", err) }
	}

	tmpl, err := s.templates.Get(ctx, command.TemplateRef, command.TemplateVersion)
	if err != nil { return CreateRunResult{}, fmt.Errorf("load workflow template %q version %q: %w", command.TemplateRef, command.TemplateVersion, err) }
	if tmpl.Ref != command.TemplateRef || tmpl.Version != command.TemplateVersion {
		return CreateRunResult{}, ErrTemplateVersionMismatch
	}
	now, err := s.clock.NowUTC(ctx)
	if err != nil { return CreateRunResult{}, fmt.Errorf("read database UTC clock: %w", err) }
	now = now.UTC()
	if err := domain.ValidateCreateRunCommand(command, now, tmpl.AllowEmptyInputs); err != nil { return CreateRunResult{}, err }

	resolved, err := template.Resolve(ctx, tmpl, command.Parameters, s.schemas, s.applicability, s.gates)
	if err != nil { return CreateRunResult{}, fmt.Errorf("resolve workflow template: %w", err) }
	runRef, err := s.ids.NewRef(ctx, "run")
	if err != nil { return CreateRunResult{}, fmt.Errorf("generate run ref: %w", err) }

	stageRefs := make(map[string]string, len(resolved.Stages))
	for _, stage := range resolved.Stages {
		ref, err := s.ids.NewRef(ctx, "stage")
		if err != nil { return CreateRunResult{}, fmt.Errorf("generate stage ref: %w", err) }
		stageRefs[stage.Definition.Code] = ref
	}
	command.InputSnapshotRefs = append([]string(nil), command.InputSnapshotRefs...)
	sort.Strings(command.InputSnapshotRefs)
	canonicalParameters, err := jcs.Transform(command.Parameters)
	if err != nil { return CreateRunResult{}, fmt.Errorf("%w: canonicalize parameters for snapshot: %v", domain.ErrInvalidCommand, err) }
	stages := make([]domain.Stage, 0, len(resolved.Stages))
	for _, resolvedStage := range resolved.Stages {
		definition := resolvedStage.Definition
		dependencies := make([]string, 0, len(resolvedStage.Dependencies))
		for _, code := range resolvedStage.Dependencies {
			ref, ok := stageRefs[code]
			if !ok { return CreateRunResult{}, fmt.Errorf("resolved dependency %q has no stage reference", code) }
			dependencies = append(dependencies, ref)
		}
		stages = append(stages, domain.Stage{
			Ref:stageRefs[definition.Code], RunRef:runRef, Code:definition.Code,
			Status:domain.StagePending, Version:1, Required:definition.Required,
			Dependencies:dependencies, RetryPolicyRef:definition.Retry.Ref,
			AttemptNo:0, ReasonCodes:[]string{}, CreatedAt:now,
		})
	}
	run := domain.Run{
		Ref:runRef, SchemaVersion:command.SchemaVersion, Status:domain.RunCreated, Version:1,
		SystemAsOf:command.SystemAsOf.UTC(), DeadlineAt:command.DeadlineAt.UTC(),
		TemplateRef:resolved.Ref, TemplateVersion:resolved.Version, TemplateFingerprint:resolved.Fingerprint,
		InputSnapshotRefs:append([]string{}, command.InputSnapshotRefs...),
		OriginatingRequestID:cloneString(command.OriginatingRequestID), CorrelationID:command.CorrelationID,
		CreatedAt:now,
	}
	record := CreateRunRecord{
		Scope:scope, Fingerprint:fingerprint, Run:run, Stages:stages,
		ResolvedTemplate:resolved, Parameters:append(json.RawMessage(nil), canonicalParameters...),
	}
	result, err := s.runs.CreateRunAtomic(ctx, record)
	if err != nil { return CreateRunResult{}, fmt.Errorf("persist create run atomically: %w", err) }
	return result, nil
}

func cloneString(value *string) *string {
	if value == nil { return nil }
	copy := *value
	return &copy
}
