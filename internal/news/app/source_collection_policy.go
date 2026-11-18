package app

import (
	"context"
	"errors"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var ErrSourceCollectionPolicyUnavailable = errors.New("news: current source collection policy unavailable")

// CurrentSourceCollectionPolicyResolver 读取并验证与目录快照版本完全一致的当前 Source/Endpoint 策略。
// EndpointPolicyVersionID 为空时，Storage 必须确认数据库中不存在 Endpoint 当前策略指针。
type CurrentSourceCollectionPolicyResolver interface {
	ResolveCurrentSourceCollectionPolicies(
		context.Context,
		string, string,
		string, string,
	) (domain.SourcePolicy, *domain.EndpointPolicy, error)
}
