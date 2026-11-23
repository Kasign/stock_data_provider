package app

import (
	"context"

	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

// DataAuthorizationPolicyRepository 仅供 Data Owner Application 使用；
// 策略版本不可变，当前指针在同一事务中切换。
type DataAuthorizationPolicyRepository interface {
	PublishDataAuthorizationPolicyVersion(context.Context, authorization.PolicyVersion) error
	ResolveCurrentDataAuthorizationPolicy(context.Context) (authorization.CurrentPolicy, error)
}
