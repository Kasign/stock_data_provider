package application

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

var ErrAuthorizedDailyBarsCloseSeriesSnapshotUnavailable = errors.New("authorized daily-bars close-series Snapshot reader unavailable")

type authorizedDataPolicySnapshot struct {
	SnapshotRef   string                        `json:"snapshot_ref"`
	Policies      []authorizedDataPolicyVersion `json:"policies"`
	SchemaVersion string                        `json:"schema_version"`
}

type authorizedDataPolicyVersion struct {
	PolicyType    string `json:"policy_type"`
	PolicyRef     string `json:"policy_ref"`
	PolicyVersion string `json:"policy_version"`
}

// AuthorizedDailyBarsCloseSeriesSnapshotReader 在 Repository 的同一 MariaDB 一致性事务中
// 读取授权、撤销、当前策略和 Payload；策略语义在 Application 层完成最终判定。
type AuthorizedDailyBarsCloseSeriesSnapshotReader struct {
	repository app.DataAuthorizedSnapshotRepository
	now        func() time.Time
}

func NewAuthorizedDailyBarsCloseSeriesSnapshotReader(
	repository app.DataAuthorizedSnapshotRepository,
) (*AuthorizedDailyBarsCloseSeriesSnapshotReader, error) {
	if repository == nil {
		return nil, ErrAuthorizedDailyBarsCloseSeriesSnapshotUnavailable
	}
	return &AuthorizedDailyBarsCloseSeriesSnapshotReader{
		repository: repository, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

// Read 仅在授权、当前策略和 Snapshot 内容均通过校验后返回 Payload。
func (r *AuthorizedDailyBarsCloseSeriesSnapshotReader) Read(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.DailyBarsCloseSeriesSnapshot, error) {
	if r == nil || r.repository == nil || ctx == nil {
		return app.DailyBarsCloseSeriesSnapshot{}, ErrAuthorizedDailyBarsCloseSeriesSnapshotUnavailable
	}
	_, subjectRef, scopes, err := trustedDataPrincipal(ctx)
	if err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, err
	}
	decision, snapshot, current, err := r.repository.ReadDataAuthorizedDailyBarsCloseSeriesSnapshot(ctx, authorizationRef, snapshotRef)
	if err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, err
	}
	var ref dataAuthorizationRef
	if err := json.Unmarshal(decision.AuthorizationRef, &ref); err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, ErrDataAuthorizationInvalid
	}
	var policySnapshot authorizedDataPolicySnapshot
	if err := json.Unmarshal(decision.PolicySnapshot, &policySnapshot); err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, ErrDataAuthorizationInvalid
	}
	if ref.SchemaVersion != "1.0.0" || ref.AuthorizationRef != authorizationRef ||
		ref.PolicyOwner != "data" || ref.Decision != "allowed" || ref.ContentLevel != "full" ||
		ref.SubjectRef != subjectRef || ref.ResourceRef != snapshotRef || ref.PurposeCode != purpose ||
		len(ref.Conditions) != 0 || ref.PolicyVersion != current.Version.PolicyVersion ||
		policySnapshot.SchemaVersion != "1.0.0" || policySnapshot.SnapshotRef != ref.PolicyVersion ||
		len(policySnapshot.Policies) != 1 ||
		policySnapshot.Policies[0].PolicyType != authorization.DataAccessPolicyType ||
		policySnapshot.Policies[0].PolicyRef != authorization.DailyBarsV2PolicyRef ||
		policySnapshot.Policies[0].PolicyVersion != current.Version.PolicyVersion ||
		snapshot.Manifest.SnapshotRef != snapshotRef {
		return app.DailyBarsCloseSeriesSnapshot{}, ErrDataAuthorizationDenied
	}
	now := r.now().UTC()
	if ref.ExpiresAt != nil {
		expiresAt, parseErr := time.Parse(time.RFC3339Nano, *ref.ExpiresAt)
		if parseErr != nil || !now.Before(expiresAt) {
			return app.DailyBarsCloseSeriesSnapshot{}, ErrDataAuthorizationDenied
		}
	}
	if err := authorization.Evaluate(current.Policy, authorization.AccessRequest{
		SubjectRef: subjectRef, PurposeCode: purpose, Scopes: scopes, Now: now,
	}); err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, ErrDataAuthorizationDenied
	}
	return snapshot, nil
}
