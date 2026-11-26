package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

var ErrAuthorizedSecurityTradingStatusFactsSnapshotUnavailable = errors.New("authorized trading-status facts Snapshot reader unavailable")

// AuthorizedSecurityTradingStatusFactsSnapshotReader 在 Repository 的同一 MariaDB 一致性事务中
// 读取授权、撤销、当前策略和 Payload；策略语义在 Application 层完成最终判定。
//
// 它与 close-series Reader 的差别不是"换个 payload"，而是数据类不同：
// 授权决策必须绑定交易事实数据类的策略，且必须与该 Snapshot 的用途一致。
type AuthorizedSecurityTradingStatusFactsSnapshotReader struct {
	repository app.DataAuthorizedTradingFactsRepository
	now        func() time.Time
}

func NewAuthorizedSecurityTradingStatusFactsSnapshotReader(
	repository app.DataAuthorizedTradingFactsRepository,
) (*AuthorizedSecurityTradingStatusFactsSnapshotReader, error) {
	if repository == nil {
		return nil, ErrAuthorizedSecurityTradingStatusFactsSnapshotUnavailable
	}
	return &AuthorizedSecurityTradingStatusFactsSnapshotReader{
		repository: repository, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Read 仅在授权、当前策略和 Snapshot 内容均通过校验后返回 Payload。
func (r *AuthorizedSecurityTradingStatusFactsSnapshotReader) Read(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.SecurityTradingStatusFactsSnapshot, error) {
	if r == nil || r.repository == nil || ctx == nil {
		return app.SecurityTradingStatusFactsSnapshot{}, ErrAuthorizedSecurityTradingStatusFactsSnapshotUnavailable
	}
	_, subjectRef, scopes, err := trustedDataPrincipal(ctx)
	if err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, err
	}
	decision, snapshot, current, err := r.repository.ReadDataAuthorizedSecurityTradingStatusFactsSnapshot(ctx, authorizationRef, snapshotRef)
	if err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, err
	}
	var ref dataAuthorizationRef
	if err := json.Unmarshal(decision.AuthorizationRef, &ref); err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, ErrDataAuthorizationInvalid
	}
	var policySnapshot authorizedDataPolicySnapshot
	if err := json.Unmarshal(decision.PolicySnapshot, &policySnapshot); err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, ErrDataAuthorizationInvalid
	}
	if ref.SchemaVersion != "1.0.0" || ref.AuthorizationRef != authorizationRef ||
		ref.PolicyOwner != "data" || ref.Decision != "allowed" || ref.ContentLevel != "full" ||
		ref.SubjectRef != subjectRef || ref.ResourceRef != snapshotRef || ref.PurposeCode != purpose ||
		len(ref.Conditions) != 0 || ref.PolicyVersion != current.Version.PolicyVersion ||
		current.Version.PolicyRef != authorization.SecurityStatusFactsPolicyRef ||
		policySnapshot.SchemaVersion != "1.0.0" || policySnapshot.SnapshotRef != ref.PolicyVersion ||
		len(policySnapshot.Policies) != 1 ||
		policySnapshot.Policies[0].PolicyType != authorization.DataAccessPolicyType ||
		policySnapshot.Policies[0].PolicyRef != authorization.SecurityStatusFactsPolicyRef ||
		policySnapshot.Policies[0].PolicyVersion != current.Version.PolicyVersion ||
		snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.PurposeCode != purpose {
		return app.SecurityTradingStatusFactsSnapshot{}, ErrDataAuthorizationDenied
	}
	now := r.now().UTC()
	if ref.ExpiresAt != nil {
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, *ref.ExpiresAt)
		if parseErr != nil || !now.Before(expiresAt) {
			return app.SecurityTradingStatusFactsSnapshot{}, ErrDataAuthorizationDenied
		}
	}
	if err := authorization.Evaluate(current.Policy, authorization.AccessRequest{
		SubjectRef: subjectRef, PurposeCode: purpose, Scopes: scopes, Now: now,
	}); err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, ErrDataAuthorizationDenied
	}
	return snapshot, nil
}
