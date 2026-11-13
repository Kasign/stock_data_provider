package storage

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

var _ app.CurrentIngestPolicyIdentityTransaction = (*mysqlNewsTransaction)(nil)

// LockCurrentIngestPolicies 在 Observation 身份事务中按固定顺序锁定并读取当前策略组合。
// 策略撤销/替换必须更新相同指针行，因此不能越过本次授权与 Observation 写入。
func (t *mysqlNewsTransaction) LockCurrentIngestPolicies(
	ctx context.Context,
	sourceRef, sourceVersion, endpointRef, endpointVersion, providerRef, providerVersion string,
) (domain.SourcePolicy, *domain.EndpointPolicy, *domain.ProviderOutputPolicy, error) {
	if t == nil || t.tx == nil || ctx == nil || sourceRef == "" || sourceVersion == "" || endpointRef == "" ||
		(providerRef == "") != (providerVersion == "") {
		return domain.SourcePolicy{}, nil, nil, app.ErrNewsCurrentPolicyUnavailable
	}

	expected := make([]domain.AuthorizationPolicyVersion, 0, 3)
	if endpointVersion != "" {
		expected = append(expected, domain.AuthorizationPolicyVersion{
			PolicyType: domain.NewsPolicyTypeEndpoint, PolicyRef: endpointRef, PolicyVersion: endpointVersion,
		})
	} else {
		// Endpoint 当前没有策略时也必须确认该指针不存在；否则旧目录快照不能绕过新策略。
		var currentVersion string
		err := t.tx.QueryRowContext(ctx, "SELECT policy_version FROM news_current_policy_versions WHERE policy_type = ? AND policy_ref = ? FOR UPDATE",
			domain.NewsPolicyTypeEndpoint, endpointRef,
		).Scan(&currentVersion)
		if err == nil {
			return domain.SourcePolicy{}, nil, nil, app.ErrNewsCurrentPolicyUnavailable
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return domain.SourcePolicy{}, nil, nil, errors.Join(app.ErrNewsCurrentPolicyUnavailable, err)
		}
	}
	if providerRef != "" {
		expected = append(expected, domain.AuthorizationPolicyVersion{
			PolicyType: domain.NewsPolicyTypeProviderOutput, PolicyRef: providerRef, PolicyVersion: providerVersion,
		})
	}
	expected = append(expected, domain.AuthorizationPolicyVersion{
		PolicyType: domain.NewsPolicyTypeSource, PolicyRef: sourceRef, PolicyVersion: sourceVersion,
	})
	sort.Slice(expected, func(i, j int) bool {
		if expected[i].PolicyType != expected[j].PolicyType {
			return expected[i].PolicyType < expected[j].PolicyType
		}
		return expected[i].PolicyRef < expected[j].PolicyRef
	})

	loaded, err := loadCurrentNewsPolicyVersions(ctx, t.tx, domain.NewsAuthorizationBinding{
		PolicySnapshot: domain.AuthorizationPolicySnapshot{Policies: expected},
	}, true)
	if err != nil {
		return domain.SourcePolicy{}, nil, nil, err
	}
	sourceVersionValue, ok := loaded[domain.NewsPolicyTypeSource+"\x00"+sourceRef]
	if !ok {
		return domain.SourcePolicy{}, nil, nil, app.ErrNewsCurrentPolicyUnavailable
	}
	var sourcePolicy domain.SourcePolicy
	if decodeCurrentPolicy(sourceVersionValue.Payload, &sourcePolicy) != nil {
		return domain.SourcePolicy{}, nil, nil, app.ErrNewsCurrentPolicyUnavailable
	}

	var endpointPolicy *domain.EndpointPolicy
	if endpointVersion != "" {
		endpointVersionValue, ok := loaded[domain.NewsPolicyTypeEndpoint+"\x00"+endpointRef]
		if !ok {
			return domain.SourcePolicy{}, nil, nil, app.ErrNewsCurrentPolicyUnavailable
		}
		endpointPolicy = new(domain.EndpointPolicy)
		if decodeCurrentPolicy(endpointVersionValue.Payload, endpointPolicy) != nil {
			return domain.SourcePolicy{}, nil, nil, app.ErrNewsCurrentPolicyUnavailable
		}
	}

	var providerPolicy *domain.ProviderOutputPolicy
	if providerRef != "" {
		providerVersionValue, ok := loaded[domain.NewsPolicyTypeProviderOutput+"\x00"+providerRef]
		if !ok {
			return domain.SourcePolicy{}, nil, nil, app.ErrNewsCurrentPolicyUnavailable
		}
		providerPolicy = new(domain.ProviderOutputPolicy)
		if decodeCurrentPolicy(providerVersionValue.Payload, providerPolicy) != nil {
			return domain.SourcePolicy{}, nil, nil, app.ErrNewsCurrentPolicyUnavailable
		}
	}
	return sourcePolicy, endpointPolicy, providerPolicy, nil
}
