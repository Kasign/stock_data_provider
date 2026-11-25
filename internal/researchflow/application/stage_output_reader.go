package application

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// StageOutputResource is the immutable, schema-qualified output persisted by Researchflow Storage.
type StageOutputResource struct {
	Ref                 string
	RunRef              string
	StageRef            string
	AttemptRef          string
	Kind                string
	StageCode           string
	TemplateVersion     string
	RegistryVersion     string
	RegistryFingerprint string
	OutputType          string
	SchemaVersion       string
	SchemaRef           string
	Content             []byte
	CreatedAt           string
}

// StageOutputResourceReader is the Researchflow-owned read port for immutable output resources.
type StageOutputResourceReader interface {
	GetStageOutputResource(ctx context.Context, ref string) (StageOutputResource, error)
}

// AttemptReader reads an immutable execution attempt by its opaque reference.
type AttemptReader interface {
	GetAttempt(ctx context.Context, attemptRef string) (domain.Attempt, error)
}

var (
	ErrStageOutputUnavailable = errors.New("Researchflow Stage Output is unavailable")
	ErrStageOutputIntegrity   = errors.New("Researchflow Stage Output integrity check failed")
	ErrInvalidStageOutputRead = errors.New("invalid Researchflow Stage Output read request")
)

// StageOutputExpectation pins the Stage and both payload/quality output schemas expected by a trusted consumer.
type StageOutputExpectation struct {
	StageCode               string
	OutputType              string
	SchemaVersion           string
	SchemaRef               string
	QualitySummarySchemaRef string
	PayloadValidator        func(json.RawMessage) error
	QualitySummaryValidator func(json.RawMessage) error
}

// GetStageOutputForRunStage resolves a successful Stage's persisted payload through the Run, Attempt and resource records.
// It never accepts a caller-supplied resource reference; both references must come from the persisted Stage.
type GetStageOutputForRunStage struct {
	runs     RunReader
	attempts AttemptReader
	outputs  StageOutputResourceReader
}

func NewGetStageOutputForRunStage(
	runs RunReader,
	attempts AttemptReader,
	outputs StageOutputResourceReader,
) (*GetStageOutputForRunStage, error) {
	if runs == nil || attempts == nil || outputs == nil {
		return nil, ErrInvalidStageOutputRead
	}
	return &GetStageOutputForRunStage{runs: runs, attempts: attempts, outputs: outputs}, nil
}

func (reader *GetStageOutputForRunStage) Read(
	ctx context.Context,
	runRef string,
	expectation StageOutputExpectation,
) (StageOutputResource, error) {
	if reader == nil || reader.runs == nil || reader.attempts == nil || reader.outputs == nil || ctx == nil ||
		!validStageOutputText(runRef) || !validStageOutputText(expectation.StageCode) ||
		!validStageOutputText(expectation.OutputType) || !validStageOutputText(expectation.SchemaVersion) ||
		!validStageOutputText(expectation.SchemaRef) || !validStageOutputText(expectation.QualitySummarySchemaRef) ||
		expectation.PayloadValidator == nil || expectation.QualitySummaryValidator == nil {
		return StageOutputResource{}, ErrInvalidStageOutputRead
	}
	if err := ctx.Err(); err != nil {
		return StageOutputResource{}, err
	}
	aggregate, err := reader.runs.GetRun(ctx, runRef)
	if err != nil {
		return StageOutputResource{}, fmt.Errorf("read Researchflow Run for Stage Output association: %w", err)
	}
	if aggregate.Run.Ref != runRef || !validStageOutputText(aggregate.Run.TemplateVersion) || len(aggregate.Stages) == 0 {
		return StageOutputResource{}, ErrStageOutputIntegrity
	}
	var selected *domain.Stage
	for index := range aggregate.Stages {
		stage := &aggregate.Stages[index]
		if stage.Code != expectation.StageCode {
			continue
		}
		if selected != nil {
			return StageOutputResource{}, ErrStageOutputIntegrity
		}
		selected = stage
	}
	if selected == nil || selected.RunRef != runRef || !validStageOutputText(selected.Ref) || selected.AttemptNo <= 0 {
		return StageOutputResource{}, ErrStageOutputUnavailable
	}
	if selected.Status != domain.StageSucceeded && selected.Status != domain.StageDegraded {
		return StageOutputResource{}, ErrStageOutputUnavailable
	}
	if selected.OutputRef == nil || !validStageOutputText(*selected.OutputRef) ||
		selected.QualitySummaryRef == nil || !validStageOutputText(*selected.QualitySummaryRef) {
		return StageOutputResource{}, ErrStageOutputUnavailable
	}
	payload, err := reader.outputs.GetStageOutputResource(ctx, *selected.OutputRef)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return StageOutputResource{}, ctxErr
		}
		return StageOutputResource{}, fmt.Errorf("%w: %v", ErrStageOutputUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return StageOutputResource{}, err
	}
	if !validStageOutputResource(payload, *selected.OutputRef, "payload", expectation.SchemaRef,
		runRef, aggregate.Run.TemplateVersion, selected.Ref, expectation) ||
		expectation.PayloadValidator(json.RawMessage(payload.Content)) != nil {
		return StageOutputResource{}, ErrStageOutputIntegrity
	}
	attempt, err := reader.attempts.GetAttempt(ctx, payload.AttemptRef)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return StageOutputResource{}, ctxErr
		}
		return StageOutputResource{}, fmt.Errorf("%w: linked Attempt is unavailable: %v", ErrStageOutputIntegrity, err)
	}
	expectedAttemptStatus := domain.AttemptSucceeded
	if selected.Status == domain.StageDegraded {
		expectedAttemptStatus = domain.AttemptDegraded
	}
	if attempt.Ref != payload.AttemptRef || attempt.StageRef != selected.Ref ||
		attempt.Number != selected.AttemptNo || attempt.FencingToken == 0 ||
		attempt.Status != expectedAttemptStatus || attempt.OutputRef == nil || *attempt.OutputRef != payload.Ref {
		return StageOutputResource{}, ErrStageOutputIntegrity
	}
	quality, err := reader.outputs.GetStageOutputResource(ctx, *selected.QualitySummaryRef)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return StageOutputResource{}, ctxErr
		}
		return StageOutputResource{}, fmt.Errorf("%w: linked quality summary is unavailable: %v", ErrStageOutputIntegrity, err)
	}
	if err := ctx.Err(); err != nil {
		return StageOutputResource{}, err
	}
	if !validStageOutputResource(quality, *selected.QualitySummaryRef, "quality_summary",
		expectation.QualitySummarySchemaRef, runRef, aggregate.Run.TemplateVersion, selected.Ref, expectation) ||
		quality.AttemptRef != attempt.Ref || quality.RegistryVersion != payload.RegistryVersion ||
		quality.RegistryFingerprint != payload.RegistryFingerprint || quality.CreatedAt != payload.CreatedAt ||
		expectation.QualitySummaryValidator(json.RawMessage(quality.Content)) != nil {
		return StageOutputResource{}, ErrStageOutputIntegrity
	}
	payload.Content = append([]byte(nil), payload.Content...)
	return payload, nil
}

func validStageOutputResource(
	resource StageOutputResource,
	expectedRef, kind, schemaRef, runRef, templateVersion, stageRef string,
	expectation StageOutputExpectation,
) bool {
	return resource.Ref == expectedRef && resource.RunRef == runRef &&
		resource.StageRef == stageRef && validStageOutputText(resource.AttemptRef) &&
		resource.Kind == kind && resource.StageCode == expectation.StageCode &&
		resource.TemplateVersion == templateVersion &&
		validStageOutputText(resource.RegistryVersion) && validStageOutputFingerprint(resource.RegistryFingerprint) &&
		resource.OutputType == expectation.OutputType &&
		resource.SchemaVersion == expectation.SchemaVersion && resource.SchemaRef == schemaRef &&
		validStageOutputCreatedAt(resource.CreatedAt) &&
		len(resource.Content) > 0 && json.Valid(resource.Content)
}

func validStageOutputText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

func validStageOutputFingerprint(value string) bool {
	const prefix = "sha256:"
	digest := strings.TrimPrefix(value, prefix)
	if digest == value || len(digest) != 64 || strings.ToLower(digest) != digest {
		return false
	}
	decoded, err := hex.DecodeString(digest)
	return err == nil && len(decoded) == 32
}

func validStageOutputCreatedAt(value string) bool {
	parsed, err := time.Parse("2006-01-02T15:04:05.000000000Z", value)
	return err == nil && parsed.Location() == time.UTC && parsed.Format("2006-01-02T15:04:05.000000000Z") == value
}
