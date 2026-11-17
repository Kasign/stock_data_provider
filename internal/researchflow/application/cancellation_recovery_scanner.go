package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type ExpiredCancellationAttemptCandidate struct {
	RunRef             string
	StageRef           string
	AttemptRef         string
	ExpectedStageVersion uint64
}

type CancellationAttemptRecoveryStore interface {
	ListExpiredCancellationAttemptCandidates(context.Context, string, string, int) ([]ExpiredCancellationAttemptCandidate, error)
	FenceExpiredCancellationAttempt(context.Context, string, string, string, uint64) (bool, error)
}

type CancellationRecoveryPage struct {
	Candidates   []ExpiredCancellationAttemptCandidate
	NextRunRef   string
	NextStageRef string
	HasMore      bool
}

type CancellationRecoveryResult struct {
	Scanned   int
	Fenced    int
	Conflicts int
}

type CancellationRecoveryScanner struct {
	store     CancellationAttemptRecoveryStore
	batchSize int
}

func NewCancellationRecoveryScanner(store CancellationAttemptRecoveryStore, batchSize int) (*CancellationRecoveryScanner, error) {
	if store == nil || batchSize < 1 || batchSize > 1000 {
		return nil, errors.New("Researchflow cancellation recovery dependencies or batch size are invalid")
	}
	return &CancellationRecoveryScanner{store: store, batchSize: batchSize}, nil
}

func (s *CancellationRecoveryScanner) ListNextPage(ctx context.Context, afterRunRef, afterStageRef string) (CancellationRecoveryPage, error) {
	if s == nil || s.store == nil {
		return CancellationRecoveryPage{}, errors.New("Researchflow cancellation recovery scanner is not configured")
	}
	if ctx == nil || strings.TrimSpace(afterRunRef) != afterRunRef ||
		strings.TrimSpace(afterStageRef) != afterStageRef ||
		len(afterRunRef) > 255 || len(afterStageRef) > 255 ||
		(afterRunRef == "" && afterStageRef != "") {
		return CancellationRecoveryPage{}, errors.New("Researchflow cancellation recovery cursor is invalid")
	}
	items, err := s.store.ListExpiredCancellationAttemptCandidates(ctx, afterRunRef, afterStageRef, s.batchSize)
	if err != nil {
		return CancellationRecoveryPage{}, fmt.Errorf("list expired cancellation Attempts: %w", err)
	}
	if len(items) > s.batchSize {
		return CancellationRecoveryPage{}, errors.New("Researchflow cancellation recovery reader exceeded the requested page size")
	}
	page := CancellationRecoveryPage{
		Candidates: append([]ExpiredCancellationAttemptCandidate{}, items...),
		HasMore: len(items) == s.batchSize,
	}
	previousRun, previousStage := afterRunRef, afterStageRef
	for _, item := range items {
		if strings.TrimSpace(item.RunRef) == "" || item.RunRef != strings.TrimSpace(item.RunRef) ||
			strings.TrimSpace(item.StageRef) == "" || item.StageRef != strings.TrimSpace(item.StageRef) ||
			strings.TrimSpace(item.AttemptRef) == "" || item.AttemptRef != strings.TrimSpace(item.AttemptRef) ||
			item.ExpectedStageVersion == 0 ||
			(item.RunRef < previousRun) || (item.RunRef == previousRun && item.StageRef <= previousStage) {
			return CancellationRecoveryPage{}, errors.New("Researchflow cancellation recovery reader returned an invalid or unordered page")
		}
		previousRun, previousStage = item.RunRef, item.StageRef
	}
	if len(items) > 0 {
		page.NextRunRef = items[len(items)-1].RunRef
		page.NextStageRef = items[len(items)-1].StageRef
	}
	return page, nil
}

func (s *CancellationRecoveryScanner) RecoverPage(ctx context.Context, page CancellationRecoveryPage) (CancellationRecoveryResult, error) {
	if s == nil || s.store == nil || ctx == nil {
		return CancellationRecoveryResult{}, errors.New("Researchflow cancellation recovery dependencies and context are required")
	}
	result := CancellationRecoveryResult{Scanned: len(page.Candidates)}
	for _, candidate := range page.Candidates {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		fenced, err := s.store.FenceExpiredCancellationAttempt(ctx, candidate.RunRef,
			candidate.StageRef, candidate.AttemptRef, candidate.ExpectedStageVersion)
		if err != nil {
			return result, fmt.Errorf("fence expired cancellation Attempt %q: %w", candidate.AttemptRef, err)
		}
		if fenced {
			result.Fenced++
		} else {
			result.Conflicts++
		}
	}
	return result, nil
}
