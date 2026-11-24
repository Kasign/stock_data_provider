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

var (
	ErrStageOutputUnavailable = errors.New("Researchflow Stage Output is unavailable")
	ErrStageOutputIntegrity   = errors.New("Researchflow Stage Output integrity check failed")
	ErrInvalidStageOutputRead = errors.New("invalid Researchflow Stage Output read request")
)

// StageOutputExpectation pins the Stage and output contract expected by a trusted internal consumer.
type StageOutputExpectation struct {
	StageCode     string
	OutputType    string
	SchemaVersion string
	SchemaRef     string
}

// GetStageOutputForRunStage resolves a successful Stage's persisted payload through the Run aggregate.
// It never accepts a caller-supplied resource reference; the reference must come from the persisted Stage.
type GetStageOutputForRunStage struct {
	runs    RunReader
	outputs StageOutputResourceReader
}

func NewGetStageOutputForRunStage(runs RunReader, outputs StageOutputResourceReader) (*GetStageOutputForRunStage, error) {
	if runs == nil || outputs == nil {
		return nil, ErrInvalidStageOutputRead
	}
	return &GetStageOutputForRunStage{runs: runs, outputs: outputs}, nil
}

func (reader *GetStageOutputForRunStage) Read(
	ctx context.Context,
	runRef string,
	expectation StageOutputExpectation,
) (StageOutputResource, error) {
	if reader == nil || reader.runs == nil || reader.outputs == nil || ctx == nil ||
		!validStageOutputText(runRef) || !validStageOutputText(expectation.StageCode) ||
		!validStageOutputText(expectation.OutputType) || !validStageOutputText(expectation.SchemaVersion) ||
		!validStageOutputText(expectation.SchemaRef) {
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
	if selected == nil || selected.RunRef != runRef || !validStageOutputText(selected.Ref) ||
		selected.AttemptNo <= 0 {
		return StageOutputResource{}, ErrStageOutputUnavailable
	}
	if selected.Status != domain.StageSucceeded && selected.Status != domain.StageDegraded {
		return StageOutputResource{}, ErrStageOutputUnavailable
	}
	if selected.OutputRef == nil || !validStageOutputText(*selected.OutputRef) ||
		selected.QualitySummaryRef == nil || !validStageOutputText(*selected.QualitySummaryRef) {
		return StageOutputResource{}, ErrStageOutputUnavailable
	}
	resource, err := reader.outputs.GetStageOutputResource(ctx, *selected.OutputRef)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return StageOutputResource{}, ctxErr
		}
		return StageOutputResource{}, fmt.Errorf("%w: %v", ErrStageOutputUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return StageOutputResource{}, err
	}
	if resource.Ref != *selected.OutputRef || resource.RunRef != runRef ||
		resource.StageRef != selected.Ref || resource.AttemptRef == "" ||
		resource.Kind != "payload" || resource.StageCode != expectation.StageCode ||
		resource.TemplateVersion != aggregate.Run.TemplateVersion ||
		resource.RegistryVersion == "" || !validStageOutputFingerprint(resource.RegistryFingerprint) ||
		resource.OutputType != expectation.OutputType ||
		resource.SchemaVersion != expectation.SchemaVersion ||
		resource.SchemaRef != expectation.SchemaRef ||
		!validStageOutputCreatedAt(resource.CreatedAt) ||
		len(resource.Content) == 0 || !json.Valid(resource.Content) {
		return StageOutputResource{}, ErrStageOutputIntegrity
	}
	resource.Content = append([]byte(nil), resource.Content...)
	return resource, nil
}

func validStageOutputText(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

func validStageOutputFingerprint(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func validStageOutputCreatedAt(value string) bool {
	parsed, err := time.Parse("2006-01-02T15:04:05.000000000Z", value)
	return err == nil && parsed.Location() == time.UTC && parsed.Format("2006-01-02T15:04:05.000000000Z") == value
}
