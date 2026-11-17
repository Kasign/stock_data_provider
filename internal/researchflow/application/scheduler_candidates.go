package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// SchedulableRunCandidateReader is the Application read port for durable Run recovery.
// The cursor is the last RunRef from the previous page; implementations must return
// strictly increasing RunRefs and must not expose Storage models.
type SchedulableRunCandidateReader interface {
	ListSchedulableRunRefs(context.Context, string, int) ([]string, error)
}

type SchedulerCandidatePage struct {
	RunRefs    []string
	NextCursor string
	HasMore    bool
}

type SchedulerCandidateScanner struct {
	reader    SchedulableRunCandidateReader
	batchSize int
}

func NewSchedulerCandidateScanner(reader SchedulableRunCandidateReader, batchSize int) (*SchedulerCandidateScanner, error) {
	if reader == nil || batchSize < 1 || batchSize > 1000 {
		return nil, errors.New("Researchflow scheduler candidate scanner dependencies or batch size are invalid")
	}
	return &SchedulerCandidateScanner{reader: reader, batchSize: batchSize}, nil
}

// ListNextPage uses keyset pagination so long-lived early Runs cannot starve later
// candidates and new Runs are picked up on the next scan cycle.
func (s *SchedulerCandidateScanner) ListNextPage(ctx context.Context, afterRunRef string) (SchedulerCandidatePage, error) {
	if s == nil || s.reader == nil {
		return SchedulerCandidatePage{}, errors.New("Researchflow scheduler candidate scanner is not configured")
	}
	if ctx == nil || strings.TrimSpace(afterRunRef) != afterRunRef || len(afterRunRef) > 255 {
		return SchedulerCandidatePage{}, errors.New("Researchflow scheduler candidate cursor is invalid")
	}
	refs, err := s.reader.ListSchedulableRunRefs(ctx, afterRunRef, s.batchSize)
	if err != nil {
		return SchedulerCandidatePage{}, fmt.Errorf("list schedulable Researchflow Runs: %w", err)
	}
	if len(refs) > s.batchSize {
		return SchedulerCandidatePage{}, errors.New("Researchflow scheduler candidate reader exceeded the requested page size")
	}
	page := SchedulerCandidatePage{RunRefs: append([]string{}, refs...), HasMore: len(refs) == s.batchSize}
	previous := afterRunRef
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" || ref != strings.TrimSpace(ref) || ref <= previous {
			return SchedulerCandidatePage{}, errors.New("Researchflow scheduler candidate reader returned an invalid or unordered Run reference")
		}
		previous = ref
	}
	if len(refs) > 0 {
		page.NextCursor = refs[len(refs)-1]
	}
	return page, nil
}
