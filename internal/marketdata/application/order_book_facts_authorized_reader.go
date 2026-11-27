package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

var ErrAuthorizedOrderBookFactsSnapshotUnavailable = errors.New("authorized order-book facts Snapshot reader unavailable")

// AuthorizedOrderBookFactsSnapshotReader 在 Repository 的同一 MariaDB 一致性事务中
// 读取授权、撤销、当前策略和 Payload；只有盘口数据类策略与用途都匹配才返回快照。
type AuthorizedOrderBookFactsSnapshotReader struct {
	repository app.DataAuthorizedOrderBookRepository
	now        func() time.Time
}

func NewAuthorizedOrderBookFactsSnapshotReader(
	repository app.DataAuthorizedOrderBookRepository,
) (*AuthorizedOrderBookFactsSnapshotReader, error) {
	if repository == nil {
		return nil, ErrAuthorizedOrderBookFactsSnapshotUnavailable
	}
	return &AuthorizedOrderBookFactsSnapshotReader{
		repository: repository, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Read 读取并复核授权后返回 Payload；任何字段漂移都 fail-closed。
func (r *AuthorizedOrderBookFactsSnapshotReader) Read(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.OrderBookFactsSnapshot, error) {
	if r == nil || r.repository == nil || ctx == nil {
		return app.OrderBookFactsSnapshot{}, ErrAuthorizedOrderBookFactsSnapshotUnavailable
	}
	_, subjectRef, scopes, err := trustedDataPrincipal(ctx)
	if err != nil {
		return app.OrderBookFactsSnapshot{}, err
	}
	decision, snapshot, current, err := r.repository.ReadDataAuthorizedOrderBookFactsSnapshot(ctx, authorizationRef, snapshotRef)
	if err != nil {
		return app.OrderBookFactsSnapshot{}, err
	}
	var ref dataAuthorizationRef
	if err := json.Unmarshal(decision.AuthorizationRef, &ref); err != nil {
		return app.OrderBookFactsSnapshot{}, ErrDataAuthorizationInvalid
	}
	var policySnapshot authorizedDataPolicySnapshot
	if err := json.Unmarshal(decision.PolicySnapshot, &policySnapshot); err != nil {
		return app.OrderBookFactsSnapshot{}, ErrDataAuthorizationInvalid
	}
	if ref.SchemaVersion != "1.0.0" || ref.AuthorizationRef != authorizationRef ||
		ref.PolicyOwner != "data" || ref.Decision != "allowed" || ref.ContentLevel != "full" ||
		ref.SubjectRef != subjectRef || ref.ResourceRef != snapshotRef || ref.PurposeCode != purpose ||
		len(ref.Conditions) != 0 || ref.PolicyVersion != current.Version.PolicyVersion ||
		current.Version.PolicyRef != authorization.OrderBookFactsPolicyRef ||
		policySnapshot.SchemaVersion != "1.0.0" || policySnapshot.SnapshotRef != ref.PolicyVersion ||
		len(policySnapshot.Policies) != 1 ||
		policySnapshot.Policies[0].PolicyType != authorization.DataAccessPolicyType ||
		policySnapshot.Policies[0].PolicyRef != authorization.OrderBookFactsPolicyRef ||
		policySnapshot.Policies[0].PolicyVersion != current.Version.PolicyVersion ||
		snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.PurposeCode != purpose {
		return app.OrderBookFactsSnapshot{}, ErrDataAuthorizationDenied
	}
	now := r.now().UTC()
	if ref.ExpiresAt != nil {
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, *ref.ExpiresAt)
		if parseErr != nil || !now.Before(expiresAt) {
			return app.OrderBookFactsSnapshot{}, ErrDataAuthorizationDenied
		}
	}
	if err := authorization.Evaluate(current.Policy, authorization.AccessRequest{
		SubjectRef: subjectRef, PurposeCode: purpose, Scopes: scopes, Now: now,
	}); err != nil {
		return app.OrderBookFactsSnapshot{}, ErrDataAuthorizationDenied
	}
	if err := snapshot.Verify(); err != nil {
		return app.OrderBookFactsSnapshot{}, fmt.Errorf("Data returned an invalid order-book facts Snapshot: %w", err)
	}
	return snapshot, nil
}
