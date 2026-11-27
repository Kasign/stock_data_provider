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

var ErrAuthorizedLimitPoolFactsSnapshotUnavailable = errors.New("authorized limit-pool facts Snapshot reader unavailable")

// AuthorizedLimitPoolFactsSnapshotReader 在 Repository 的同一 MariaDB 一致性事务中
// 读取授权、撤销、当前策略和 Payload；只有涨跌停数据类策略与用途都匹配才返回快照。
type AuthorizedLimitPoolFactsSnapshotReader struct {
	repository app.DataAuthorizedLimitPoolRepository
	now        func() time.Time
}

func NewAuthorizedLimitPoolFactsSnapshotReader(
	repository app.DataAuthorizedLimitPoolRepository,
) (*AuthorizedLimitPoolFactsSnapshotReader, error) {
	if repository == nil {
		return nil, ErrAuthorizedLimitPoolFactsSnapshotUnavailable
	}
	return &AuthorizedLimitPoolFactsSnapshotReader{
		repository: repository, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Read 读取并复核授权后返回 Payload；任何字段漂移都 fail-closed。
func (r *AuthorizedLimitPoolFactsSnapshotReader) Read(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.LimitPoolFactsSnapshot, error) {
	if r == nil || r.repository == nil || ctx == nil {
		return app.LimitPoolFactsSnapshot{}, ErrAuthorizedLimitPoolFactsSnapshotUnavailable
	}
	_, subjectRef, scopes, err := trustedDataPrincipal(ctx)
	if err != nil {
		return app.LimitPoolFactsSnapshot{}, err
	}
	decision, snapshot, current, err := r.repository.ReadDataAuthorizedLimitPoolFactsSnapshot(ctx, authorizationRef, snapshotRef)
	if err != nil {
		return app.LimitPoolFactsSnapshot{}, err
	}
	var ref dataAuthorizationRef
	if err := json.Unmarshal(decision.AuthorizationRef, &ref); err != nil {
		return app.LimitPoolFactsSnapshot{}, ErrDataAuthorizationInvalid
	}
	var policySnapshot authorizedDataPolicySnapshot
	if err := json.Unmarshal(decision.PolicySnapshot, &policySnapshot); err != nil {
		return app.LimitPoolFactsSnapshot{}, ErrDataAuthorizationInvalid
	}
	if ref.SchemaVersion != "1.0.0" || ref.AuthorizationRef != authorizationRef ||
		ref.PolicyOwner != "data" || ref.Decision != "allowed" || ref.ContentLevel != "full" ||
		ref.SubjectRef != subjectRef || ref.ResourceRef != snapshotRef || ref.PurposeCode != purpose ||
		len(ref.Conditions) != 0 || ref.PolicyVersion != current.Version.PolicyVersion ||
		current.Version.PolicyRef != authorization.LimitPoolFactsPolicyRef ||
		policySnapshot.SchemaVersion != "1.0.0" || policySnapshot.SnapshotRef != ref.PolicyVersion ||
		len(policySnapshot.Policies) != 1 ||
		policySnapshot.Policies[0].PolicyType != authorization.DataAccessPolicyType ||
		policySnapshot.Policies[0].PolicyRef != authorization.LimitPoolFactsPolicyRef ||
		policySnapshot.Policies[0].PolicyVersion != current.Version.PolicyVersion ||
		snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.PurposeCode != purpose {
		return app.LimitPoolFactsSnapshot{}, ErrDataAuthorizationDenied
	}
	now := r.now().UTC()
	if ref.ExpiresAt != nil {
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, *ref.ExpiresAt)
		if parseErr != nil || !now.Before(expiresAt) {
			return app.LimitPoolFactsSnapshot{}, ErrDataAuthorizationDenied
		}
	}
	if err := authorization.Evaluate(current.Policy, authorization.AccessRequest{
		SubjectRef: subjectRef, PurposeCode: purpose, Scopes: scopes, Now: now,
	}); err != nil {
		return app.LimitPoolFactsSnapshot{}, ErrDataAuthorizationDenied
	}
	if err := snapshot.Verify(); err != nil {
		return app.LimitPoolFactsSnapshot{}, fmt.Errorf("Data returned an invalid limit-pool facts Snapshot: %w", err)
	}
	return snapshot, nil
}
