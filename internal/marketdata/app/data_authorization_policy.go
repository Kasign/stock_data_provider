package app

import (
	"context"
	"encoding/json"
	"errors"

	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

var (
	ErrDataAuthorizationPolicyVersionConflict = errors.New("market-data authorization policy version conflict")
	ErrDataAuthorizationPolicyNotPublished    = errors.New("market-data authorization policy not published")
)

// DataAuthorizationPolicyRepository 仅供 Data Owner Application 使用；
// 策略版本不可变，当前指针按 expectedCurrentVersion 做原子推进。
type DataAuthorizationPolicyRepository interface {
	PublishDataAuthorizationPolicyVersion(context.Context, string, authorization.PolicyVersion) error
	ResolveCurrentDataAuthorizationPolicy(context.Context) (authorization.CurrentPolicy, error)
}

// DataAuthorizationDecision 是 Data Owner 授权决策的不可变持久化对象。
type DataAuthorizationDecision struct {
	AuthorizationRef json.RawMessage
	PolicySnapshot   json.RawMessage
}

// DataAuthorizationDecisionStore 持久化授权决策，并在读取时检查撤销状态。
type DataAuthorizationDecisionStore interface {
	AppendDataAuthorizationDecision(context.Context, DataAuthorizationDecision) error
	GetDataAuthorizationDecision(context.Context, string) (DataAuthorizationDecision, error)
}
