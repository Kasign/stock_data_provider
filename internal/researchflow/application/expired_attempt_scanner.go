package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ExpiredAttemptCandidate identifies a running Stage whose latest Worker Lease
// has expired. It is a recovery hint only; Worker.Execute/ClaimAttempt rechecks
// the Lease and fencing state under the authoritative database transaction.
type ExpiredAttemptCandidate struct {
	RunRef     string
	StageRef   string
	AttemptRef string
}

// ExpiredAttemptCandidateReader is the Application read port for abandoned
// Worker execution recovery.
type ExpiredAttemptCandidateReader interface {
	ListExpiredAttemptCandidates(context.Context, string, string, int) ([]ExpiredAttemptCandidate, error)
}

type ExpiredAttemptCandidatePage struct {
	Candidates []ExpiredAttemptCandidate
	NextRunRef string
	NextStageRef string
	HasMore bool
}

type ExpiredAttemptScanner struct {
	reader ExpiredAttemptCandidateReader
	batchSize int
}

func NewExpiredAttemptScanner(reader ExpiredAttemptCandidateReader, batchSize int) (*ExpiredAttemptScanner, error) {
	if reader == nil || batchSize < 1 || batchSize > 1000 {
		return nil, errors.New("Researchflow expired Attempt scanner dependencies or batch size are invalid")
	}
	return &ExpiredAttemptScanner{reader: reader, batchSize: batchSize}, nil
}

// ListNextPage uses a composite (RunRef, StageRef) keyset cursor. The returned
// candidates must be passed to Worker.Execute with a fresh Attempt reference;
// ClaimAttempt decides atomically whether the expired Lease is still reclaimable.
func (s *ExpiredAttemptScanner) ListNextPage(ctx context.Context, afterRunRef, afterStageRef string) (ExpiredAttemptCandidatePage, error) {
	if s == nil || s.reader == nil {
		return ExpiredAttemptCandidatePage{}, errors.New("Researchflow expired Attempt scanner is not configured")
	}
	if ctx == nil || strings.TrimSpace(afterRunRef) != afterRunRef || strings.TrimSpace(afterStageRef) != afterStageRef ||
		len(afterRunRef) > 255 || len(afterStageRef) > 255 || (afterRunRef == "" && afterStageRef != "") {
		return ExpiredAttemptCandidatePage{}, errors.New("Researchflow expired Attempt scanner cursor is invalid")
	}
	items, err := s.reader.ListExpiredAttemptCandidates(ctx, afterRunRef, afterStageRef, s.batchSize)
	if err != nil {
		return ExpiredAttemptCandidatePage{}, fmt.Errorf("list expired Researchflow Attempts: %w", err)
	}
	if len(items) > s.batchSize {
		return ExpiredAttemptCandidatePage{}, errors.New("Researchflow expired Attempt reader exceeded the requested page size")
	}
	page := ExpiredAttemptCandidatePage{
		Candidates: append([]ExpiredAttemptCandidate{}, items...),
		HasMore: len(items) == s.batchSize,
	}
	previousRun, previousStage := afterRunRef, afterStageRef
	for _, item := range items {
		if strings.TrimSpace(item.RunRef) == "" || item.RunRef != strings.TrimSpace(item.RunRef) ||
			strings.TrimSpace(item.StageRef) == "" || item.StageRef != strings.TrimSpace(item.StageRef) ||
			strings.TrimSpace(item.AttemptRef) == "" || item.AttemptRef != strings.TrimSpace(item.AttemptRef) ||
			(item.RunRef < previousRun) || (item.RunRef == previousRun && item.StageRef <= previousStage) {
			return ExpiredAttemptCandidatePage{}, errors.New("Researchflow expired Attempt reader returned an invalid or unordered page")
		}
		previousRun, previousStage = item.RunRef, item.StageRef
	}
	if len(items) > 0 {
		page.NextRunRef = items[len(items)-1].RunRef
		page.NextStageRef = items[len(items)-1].StageRef
	}
	return page, nil
}
