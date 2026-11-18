package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// FinalizableRunCandidateReader lists Runs whose persisted Stage set is terminal
// and whose Attempts no longer hold execution leases. It is a recovery port:
// finalization itself re-loads and validates the authoritative snapshot.
type FinalizableRunCandidateReader interface {
	ListFinalizableRunRefs(context.Context, string, int) ([]string, error)
}

type RunFinalizationCandidatePage struct {
	RunRefs    []string
	NextCursor string
	HasMore    bool
}

type RunFinalizationCandidateScanner struct {
	reader    FinalizableRunCandidateReader
	batchSize int
}

func NewRunFinalizationCandidateScanner(reader FinalizableRunCandidateReader, batchSize int) (*RunFinalizationCandidateScanner, error) {
	if reader == nil || batchSize < 1 || batchSize > 1000 {
		return nil, errors.New("Researchflow finalization candidate scanner dependencies or batch size are invalid")
	}
	return &RunFinalizationCandidateScanner{reader: reader, batchSize: batchSize}, nil
}

// ListNextPage uses RunRef keyset pagination. A failed candidate remains
// discoverable on the next full scan; a successful finalization removes it.
func (s *RunFinalizationCandidateScanner) ListNextPage(ctx context.Context, afterRunRef string) (RunFinalizationCandidatePage, error) {
	if s == nil || s.reader == nil {
		return RunFinalizationCandidatePage{}, errors.New("Researchflow finalization candidate scanner is not configured")
	}
	if ctx == nil || strings.TrimSpace(afterRunRef) != afterRunRef || len(afterRunRef) > 255 {
		return RunFinalizationCandidatePage{}, errors.New("Researchflow finalization candidate cursor is invalid")
	}
	refs, err := s.reader.ListFinalizableRunRefs(ctx, afterRunRef, s.batchSize)
	if err != nil {
		return RunFinalizationCandidatePage{}, fmt.Errorf("list finalizable Researchflow Runs: %w", err)
	}
	if len(refs) > s.batchSize {
		return RunFinalizationCandidatePage{}, errors.New("Researchflow finalization candidate reader exceeded the requested page size")
	}
	page := RunFinalizationCandidatePage{
		RunRefs: append([]string{}, refs...),
		HasMore: len(refs) == s.batchSize,
	}
	previous := afterRunRef
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" || ref != strings.TrimSpace(ref) || ref <= previous {
			return RunFinalizationCandidatePage{}, errors.New("Researchflow finalization candidate reader returned an invalid or unordered Run reference")
		}
		previous = ref
	}
	if len(refs) > 0 {
		page.NextCursor = refs[len(refs)-1]
	}
	return page, nil
}
