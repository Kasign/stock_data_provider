package app

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

var (
	ErrDataAuthorizationPolicyVersionConflict = errors.New("market-data authorization policy version conflict")
	ErrDataAuthorizationPolicyNotPublished    = errors.New("market-data authorization policy not published")
)

// DataAuthorizationPolicyRepository 仅供 Data Owner Application 使用；
// 策略版本不可变，当前指针按 expectedCurrentVersion 做原子推进。
type DataAuthorizationPolicyPublicationAudit struct {
	ActorRef   string
	RequestID  string
	OccurredAt time.Time
}

type DataAuthorizationPolicyRepository interface {
	PublishDataAuthorizationPolicyVersion(context.Context, string, authorization.PolicyVersion, DataAuthorizationPolicyPublicationAudit) error
	ResolveCurrentDataAuthorizationPolicy(context.Context) (authorization.CurrentPolicy, error)
}

// DataAuthorizationPolicyRefResolver 按数据类解析当前策略。
//
// 它与 DataAuthorizationPolicyRepository 分开定义：既有实现只需实现日线数据类，
// 未实现本接口的仓储在签发其他数据类授权时必须失败（fail-closed），而不是回落到日线策略。
type DataAuthorizationPolicyRefResolver interface {
	ResolveCurrentDataAuthorizationPolicyByRef(context.Context, string) (authorization.CurrentPolicy, error)
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
