package app

import (
	"context"
	"errors"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrInvalidAuthorizationSnapshot = errors.New("news.authorization.snapshot_invalid")
	ErrAuthorizationSnapshotConflict = errors.New("news.authorization.snapshot_conflict")
	ErrAuthorizationSnapshotNotFound = errors.New("news.authorization.snapshot_not_found")
)

// AuthorizationSnapshotRepository is an append-only store for immutable
// NEWS-CTR-002 decisions. It is not a policy evaluator or current-access gate.
type AuthorizationSnapshotRepository interface {
	AppendAuthorizationSnapshot(context.Context, domain.NewsAuthorizationBinding) error
	GetAuthorizationSnapshot(context.Context, string) (domain.NewsAuthorizationBinding, error)
}

type RecordAuthorizationSnapshot struct {
	repository AuthorizationSnapshotRepository
}

func NewRecordAuthorizationSnapshot(repository AuthorizationSnapshotRepository) (*RecordAuthorizationSnapshot, error) {
	if repository == nil {
		return nil, ErrInvalidAuthorizationSnapshot
	}
	return &RecordAuthorizationSnapshot{repository: repository}, nil
}

func (s *RecordAuthorizationSnapshot) Execute(
	ctx context.Context,
	binding domain.NewsAuthorizationBinding,
) error {
	if s == nil || s.repository == nil || ctx == nil ||
		domain.ValidateNewsAuthorizationBinding(binding) != nil {
		return ErrInvalidAuthorizationSnapshot
	}
	return s.repository.AppendAuthorizationSnapshot(ctx, binding)
}
