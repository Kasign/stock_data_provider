package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var ErrIdentityHistoryInvalid = errors.New("news.identity.history_invalid")

// IdentityHistoryRepository reads the credential state that was known at a
// system knowledge time. It is separate from the current-state ingest transaction.
type IdentityHistoryRepository interface {
	ListIdentityCredentialsAt(context.Context, string, []domain.IdentityCredential, time.Time) ([]domain.IdentityCredential, error)
}

type ResolveIdentityAtRequest struct {
	SourceID    string
	Credentials []domain.IdentityCredential
	SystemAsOf  time.Time
}

// ResolveIdentityAt reconstructs same-source identity matching using only
// credential states observed no later than SystemAsOf.
type ResolveIdentityAt struct {
	repository IdentityHistoryRepository
	now        func() time.Time
}

func NewResolveIdentityAt(repository IdentityHistoryRepository) (*ResolveIdentityAt, error) {
	if repository == nil {
		return nil, ErrInvalidRequest
	}
	return &ResolveIdentityAt{repository: repository, now: time.Now}, nil
}

func (u *ResolveIdentityAt) Execute(ctx context.Context, request ResolveIdentityAtRequest) (domain.IdentityResolution, error) {
	if u == nil || u.repository == nil || u.now == nil || ctx == nil {
		return domain.IdentityResolution{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return domain.IdentityResolution{}, err
	}
	request.SourceID = strings.TrimSpace(request.SourceID)
	now := u.now().UTC()
	if request.SourceID == "" || len(request.Credentials) == 0 || len(request.Credentials) > 10 ||
		request.SystemAsOf.IsZero() || !validEvidenceUTC(request.SystemAsOf) ||
		request.SystemAsOf.After(now) ||
		domain.ResolveIdentity(request.SourceID, request.Credentials, nil).Status == domain.IdentityConflict {
		return domain.IdentityResolution{}, ErrInvalidRequest
	}
	existing, err := u.repository.ListIdentityCredentialsAt(ctx, request.SourceID, request.Credentials, request.SystemAsOf.UTC())
	if err != nil {
		return domain.IdentityResolution{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.IdentityResolution{}, err
	}
	for _, credential := range existing {
		if err := ctx.Err(); err != nil {
			return domain.IdentityResolution{}, err
		}
		if !validIdentityHistoryCredential(request.SourceID, request.Credentials, credential, request.SystemAsOf) {
			return domain.IdentityResolution{}, ErrIdentityHistoryInvalid
		}
	}
	resolution := domain.ResolveIdentity(request.SourceID, request.Credentials, existing)
	if resolution.Status == domain.IdentityConflict && resolution.Reason == "news.identity.candidate_invalid" {
		return domain.IdentityResolution{}, ErrInvalidRequest
	}
	return resolution, nil
}

func validIdentityHistoryCredential(
	sourceID string,
	candidates []domain.IdentityCredential,
	credential domain.IdentityCredential,
	systemAsOf time.Time,
) bool {
	if credential.SourceID != sourceID || credential.NewsID == "" ||
		credential.ObservedAt.IsZero() || !validEvidenceUTC(credential.ObservedAt) ||
		credential.ObservedAt.After(systemAsOf) {
		return false
	}
	switch credential.Status {
	case domain.IdentifierActive, domain.IdentifierHistorical, domain.IdentifierInvalid:
	default:
		return false
	}
	for _, candidate := range candidates {
		if credential.Type == candidate.Type &&
			credential.Value == candidate.Value &&
			credential.Hash == candidate.Hash {
			return true
		}
	}
	return false
}
