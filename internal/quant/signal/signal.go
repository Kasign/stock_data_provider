package signal

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// QNT-CTR-006 Signal 最小切片（2026-11-26 决议）。
//
// 背景：RiskDecision 的评估对象是"某个待执行的候选交易"，而旧链把它叫
// CandidateSignal。旧结构的 EntryRule/ExitRule/InvalidationRule 是自然语言，
// 不适合作为跨模块机器契约（doc/quant/07 第 89 行），因此新契约只保留
// 可机读的身份、方向、时点、证据与规则版本，不承载交易规则文本与价格：
//   - 价格由 Risk/Planner 另行读取授权 Snapshot，避免在 Signal 里重复表达并绑定复权口径；
//   - direction 与 status 是两个独立枚举，不互相推导，也不由 TrendResult 自动投射。
//
// 本包只做结构契约与校验，不做策略评估、不做 I/O、不落库、不装配 Stage。

// ContractVersion 是 Signal 契约的版本，必须与 Schema 文件名一致。
const ContractVersion = "1.0.0"

var ErrInvalidSignal = errors.New("invalid quant signal")

// Direction 是信号方向。v1 只有 long（建立多头候选）与 close（平掉多头候选），
// 不引入 short：A 股融券能力与规则未进入本契约。
type Direction string

const (
	DirectionLong  Direction = "long"
	DirectionClose Direction = "close"
)

// Status 是信号生命周期状态。candidate 表示仍待评估，invalidated 表示已失效，
// 但仍必须保留原信号以便审计"为何未进入 Risk"。
type Status string

const (
	StatusCandidate   Status = "candidate"
	StatusInvalidated Status = "invalidated"
)

// StrategyRef 标识产出该信号的策略与版本，供审计与跨 Run 复现。
type StrategyRef struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// Signal 是 QNT-CTR-006 Signal 1.0.0 的强类型表示。
// 证据必须是已授权的 Quant 结果引用（例如 TrendResult/Factor 的 result_id），
// 由上游保证可解析；本 DTO 只校验引用形状，不校验存在性（存在性属授权读取链路）。
type Signal struct {
	SignalID string `json:"signal_id"`
	Symbol   string `json:"symbol"`
	// Direction 与 Status 独立取值：契约不允许由方向推出状态或反之。
	Direction    Direction   `json:"direction"`
	Status       Status      `json:"status"`
	StrategyRef  StrategyRef `json:"strategy_ref"`
	SystemAsOf   string      `json:"system_as_of"`
	TradeDate    string      `json:"trade_date"`
	EvidenceRefs []string    `json:"evidence_refs"`
	// StrategyRuleSetVersion 是产出该信号的策略规则集版本；与 RiskDecision 的
	// rule_set_version 属不同语义层，禁止互相填充或直接转换。
	StrategyRuleSetVersion string `json:"strategy_rule_set_version"`
}

// Validate 校验结构、枚举、时间规范与证据非空。
// 交叉业务约束（例如信号是否已被后续数据推翻）不在此处判断，避免把策略语义塞进 DTO。
func (s Signal) Validate() error {
	if !canonicalIdentity(s.SignalID) || !canonicalIdentity(s.Symbol) ||
		!validDirection(s.Direction) || !validStatus(s.Status) ||
		!canonicalIdentity(s.StrategyRef.ID) || !canonicalIdentity(s.StrategyRef.Version) ||
		!canonicalUTCTimestamp(s.SystemAsOf) || !canonicalTradeDate(s.TradeDate) ||
		!canonicalIdentity(s.StrategyRuleSetVersion) {
		return ErrInvalidSignal
	}
	if len(s.EvidenceRefs) == 0 {
		return ErrInvalidSignal
	}
	seen := make(map[string]struct{}, len(s.EvidenceRefs))
	for _, ref := range s.EvidenceRefs {
		if !canonicalIdentity(ref) {
			return ErrInvalidSignal
		}
		if _, exists := seen[ref]; exists {
			return ErrInvalidSignal
		}
		seen[ref] = struct{}{}
	}
	return nil
}

// NewSignal 构造并校验 Signal；所有切片都会被复制，避免调用方后续修改已构造结果。
func NewSignal(candidate Signal) (Signal, error) {
	candidate.EvidenceRefs = append([]string{}, candidate.EvidenceRefs...)
	if candidate.EvidenceRefs == nil {
		candidate.EvidenceRefs = []string{}
	}
	if err := candidate.Validate(); err != nil {
		return Signal{}, err
	}
	return candidate, nil
}

func canonicalIdentity(value string) bool {
	return value != "" && utf8.ValidString(value) && strings.TrimSpace(value) == value
}

func canonicalUTCTimestamp(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || !strings.HasSuffix(value, "Z") {
		return false
	}
	return parsed.UTC().Format(time.RFC3339Nano) == value
}

func canonicalTradeDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return false
	}
	return parsed.Format("2006-01-02") == value
}

func validDirection(value Direction) bool {
	switch value {
	case DirectionLong, DirectionClose:
		return true
	default:
		return false
	}
}

func validStatus(value Status) bool {
	switch value {
	case StatusCandidate, StatusInvalidated:
		return true
	default:
		return false
	}
}
