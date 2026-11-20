package storage

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

var _ app.CurrentSourceCollectionPolicyResolver = (*MySQLRepository)(nil)

// ResolveCurrentSourceCollectionPolicies 在单个只读事务快照中读取当前指针和不可变 payload。
// 不持有事务跨越 HTTP 请求；版本不一致、Endpoint 意外出现策略或载荷校验失败均拒绝。
func (r *MySQLRepository) ResolveCurrentSourceCollectionPolicies(
	ctx context.Context,
	sourceRef, expectedSourceVersion, endpointRef, expectedEndpointVersion string,
) (domain.SourcePolicy, *domain.EndpointPolicy, error) {
	if r == nil || r.db == nil || ctx == nil ||
		sourceRef == "" || expectedSourceVersion == "" || endpointRef == "" {
		return domain.SourcePolicy{}, nil, app.ErrSourceCollectionPolicyUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.SourcePolicy{}, nil, err
	}
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return domain.SourcePolicy{}, nil, errors.Join(app.ErrSourceCollectionPolicyUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	sourceVersion, err := readCurrentSourcePolicyVersion(ctx, tx, domain.NewsPolicyTypeSource, sourceRef)
	if err != nil || sourceVersion != expectedSourceVersion {
		return domain.SourcePolicy{}, nil, app.ErrSourceCollectionPolicyUnavailable
	}
	sourcePayload, err := readSourcePolicyPayload(ctx, tx, domain.NewsPolicyTypeSource, sourceRef, sourceVersion)
	if err != nil {
		return domain.SourcePolicy{}, nil, err
	}
	var sourcePolicy domain.SourcePolicy
	if decodeCurrentPolicy(sourcePayload, &sourcePolicy) != nil {
		return domain.SourcePolicy{}, nil, app.ErrSourceCollectionPolicyUnavailable
	}

	endpointVersion, endpointErr := readCurrentSourcePolicyVersion(ctx, tx, domain.NewsPolicyTypeEndpoint, endpointRef)
	if expectedEndpointVersion == "" {
		if !errors.Is(endpointErr, sql.ErrNoRows) {
			return domain.SourcePolicy{}, nil, app.ErrSourceCollectionPolicyUnavailable
		}
	} else {
		if endpointErr != nil || endpointVersion != expectedEndpointVersion {
			return domain.SourcePolicy{}, nil, app.ErrSourceCollectionPolicyUnavailable
		}
		endpointPayload, payloadErr := readSourcePolicyPayload(ctx, tx, domain.NewsPolicyTypeEndpoint, endpointRef, endpointVersion)
		if payloadErr != nil {
			return domain.SourcePolicy{}, nil, payloadErr
		}
		var policy domain.EndpointPolicy
		if decodeCurrentPolicy(endpointPayload, &policy) != nil {
			return domain.SourcePolicy{}, nil, app.ErrSourceCollectionPolicyUnavailable
		}
		if err := tx.Commit(); err != nil {
			return domain.SourcePolicy{}, nil, errors.Join(app.ErrSourceCollectionPolicyUnavailable, err)
		}
		return sourcePolicy, &policy, nil
	}
	if err := tx.Commit(); err != nil {
		return domain.SourcePolicy{}, nil, errors.Join(app.ErrSourceCollectionPolicyUnavailable, err)
	}
	return sourcePolicy, nil, nil
}

func readCurrentSourcePolicyVersion(ctx context.Context, tx *sql.Tx, policyType, policyRef string) (string, error) {
	var version string
	err := tx.QueryRowContext(ctx, `SELECT policy_version FROM news_current_policy_versions
		WHERE policy_type = ? AND policy_ref = ?`, policyType, policyRef).Scan(&version)
	return version, err
}

func readSourcePolicyPayload(ctx context.Context, tx *sql.Tx, policyType, policyRef, version string) ([]byte, error) {
	var storedType, storedRef, storedVersion, storedHash string
	var payload []byte
	var createdAt time.Time
	err := tx.QueryRowContext(ctx, `SELECT policy_type, policy_ref, policy_version,
		payload_json, payload_hash, created_at FROM news_policy_versions
		WHERE policy_type = ? AND policy_ref = ? AND policy_version = ?`,
		policyType, policyRef, version,
	).Scan(&storedType, &storedRef, &storedVersion, &payload, &storedHash, &createdAt)
	if err != nil {
		return nil, errors.Join(app.ErrSourceCollectionPolicyUnavailable, err)
	}
	value := domain.NewsPolicyVersion{
		PolicyType: storedType, PolicyRef: storedRef, PolicyVersion: storedVersion,
		Payload: append([]byte(nil), payload...), PayloadHash: storedHash, CreatedAt: createdAt.UTC(),
	}
	if value.PolicyType != policyType || value.PolicyRef != policyRef ||
		value.PolicyVersion != version || domain.ValidateNewsPolicyVersion(value) != nil {
		return nil, app.ErrSourceCollectionPolicyUnavailable
	}
	sum := sha256.Sum256(value.Payload)
	actualHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(actualHash), []byte(value.PayloadHash)) != 1 {
		return nil, app.ErrSourceCollectionPolicyUnavailable
	}
	// ValidateNewsPolicyVersion 已校验严格 JSON 形状；此处复制 payload 供领域对象解码。
	if !json.Valid(value.Payload) {
		return nil, app.ErrSourceCollectionPolicyUnavailable
	}
	return value.Payload, nil
}
