package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type DeadlineCandidate struct {
	RunRef  string
	Version uint64
}

type DeadlineExpirationStore interface {
	ListExpiredRunCandidates(context.Context, int) ([]DeadlineCandidate, error)
	ExpireRunDeadline(context.Context, string, uint64) (bool, uint64, error)
}

type DeadlineScanConfig struct {
	BatchSize int
}

type DeadlineScanResult struct {
	Scanned   int
	Expired   int
	Conflicts int
}

// DeadlineScanner fences expired Runs in bounded batches. Run aggregation and
// FinalizeRun remain a separate Application step so summary/outbox are atomic.
type DeadlineScanner struct {
	store  DeadlineExpirationStore
	config DeadlineScanConfig
}

func NewDeadlineScanner(store DeadlineExpirationStore, config DeadlineScanConfig) (*DeadlineScanner, error) {
	if store == nil || config.BatchSize <= 0 || config.BatchSize > 1000 {
		return nil, errors.New("Researchflow deadline scanner dependencies or configuration are invalid")
	}
	return &DeadlineScanner{store: store, config: config}, nil
}

func (s *DeadlineScanner) ScanBatch(ctx context.Context) (DeadlineScanResult, error) {
	if s == nil || s.store == nil {
		return DeadlineScanResult{}, errors.New("Researchflow deadline scanner is not configured")
	}
	candidates, err := s.store.ListExpiredRunCandidates(ctx, s.config.BatchSize)
	if err != nil {
		return DeadlineScanResult{}, fmt.Errorf("list expired Researchflow Runs: %w", err)
	}
	result := DeadlineScanResult{Scanned: len(candidates)}
	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.RunRef) == "" {
			return result, errors.New("Researchflow deadline candidate has an empty Run reference")
		}
		if _, exists := seen[candidate.RunRef]; exists {
			return result, fmt.Errorf("duplicate Researchflow deadline candidate %q", candidate.RunRef)
		}
		seen[candidate.RunRef] = struct{}{}
		changed, _, expireErr := s.store.ExpireRunDeadline(ctx, candidate.RunRef, candidate.Version)
		if expireErr != nil {
			return result, fmt.Errorf("expire Researchflow Run %q: %w", candidate.RunRef, expireErr)
		}
		if changed {
			result.Expired++
		} else {
			result.Conflicts++
		}
	}
	return result, nil
}
