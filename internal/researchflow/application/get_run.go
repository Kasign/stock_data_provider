package application

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

// RunReader is the read-side port for retrieving the persisted execution aggregate.
type RunReader interface {
	GetRun(context.Context, string) (CreateRunResult, error)
}

// GetRunService exposes a stable Application query without leaking Storage models.
type GetRunService struct {
	reader RunReader
}

func NewGetRunService(reader RunReader) (*GetRunService, error) {
	if reader == nil {
		return nil, errors.New("Researchflow Run reader is required")
	}
	return &GetRunService{reader: reader}, nil
}

func (s *GetRunService) GetRun(ctx context.Context, runRef string) (CreateRunResult, error) {
	if s == nil || s.reader == nil {
		return CreateRunResult{}, errors.New("Researchflow GetRun service is not configured")
	}
	if strings.TrimSpace(runRef) == "" || len(runRef) > 255 || strings.TrimSpace(runRef) != runRef {
		return CreateRunResult{}, fmt.Errorf("%w: Run reference is invalid", domain.ErrInvalidCommand)
	}
	result, err := s.reader.GetRun(ctx, runRef)
	if err != nil {
		return CreateRunResult{}, fmt.Errorf("read Researchflow Run %q: %w", runRef, err)
	}
	if result.Run.Ref != runRef || result.Run.Version == 0 || result.Run.Status == "" {
		return CreateRunResult{}, errors.New("Researchflow Run reader returned an inconsistent aggregate identity")
	}
	seenRefs := make(map[string]struct{}, len(result.Stages))
	seenCodes := make(map[string]struct{}, len(result.Stages))
	for _, stage := range result.Stages {
		if stage.Ref == "" || stage.RunRef != runRef || stage.Code == "" || stage.Version == 0 {
			return CreateRunResult{}, errors.New("Researchflow Run reader returned an invalid Stage identity")
		}
		if _, exists := seenRefs[stage.Ref]; exists {
			return CreateRunResult{}, fmt.Errorf("Researchflow Run reader returned duplicate Stage ref %q", stage.Ref)
		}
		if _, exists := seenCodes[stage.Code]; exists {
			return CreateRunResult{}, fmt.Errorf("Researchflow Run reader returned duplicate Stage code %q", stage.Code)
		}
		seenRefs[stage.Ref] = struct{}{}
		seenCodes[stage.Code] = struct{}{}
	}
	if len(result.Stages) == 0 {
		return CreateRunResult{}, errors.New("Researchflow Run reader returned an aggregate without Stages")
	}
	if result.Run.Status.Terminal() {
		if result.Run.ResultSummary == nil || result.Run.ResultSummaryRef == nil || strings.TrimSpace(*result.Run.ResultSummaryRef) == "" {
			return CreateRunResult{}, errors.New("terminal Researchflow Run is missing its ResultSummary")
		}
	} else if result.Run.ResultSummary != nil || result.Run.ResultSummaryRef != nil {
		return CreateRunResult{}, errors.New("non-terminal Researchflow Run must not expose a ResultSummary")
	}
	if result.Run.InputSnapshotRefs == nil || result.Run.ExcludedStages == nil {
		return CreateRunResult{}, errors.New("Researchflow Run reader returned incomplete snapshot collections")
	}
	result.Stages = append([]domain.Stage(nil), result.Stages...)
	return result, nil
}
