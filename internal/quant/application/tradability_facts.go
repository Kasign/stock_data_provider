package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/quant/risk"
)

// 交易状态事实的消费边界（执行清单 1.4 前置②③之后的物理读取链路）。
//
// Quant 只消费 Data 授权快照里的事实，不重新解释事实、不推导涨跌停与可成交量：
// tradable_volume 来自独立的真实盘口授权快照；price_limit 仍无合格事实来源，
// 因此当前只能继续列为缺失。任何没有授权快照支撑的"补值"都是猜测。

const (
	// TradingFactsPayloadKind 必须与 Data 侧 payload kind 完全一致，用于防止串读。
	TradingFactsPayloadKind = "security_trading_status_facts"
	// TradingFactsPayloadSchemaVersion 是当前唯一支持的 Payload 版本。
	TradingFactsPayloadSchemaVersion = "1.0.0"
	// TradingFactsPurposeCode 是风险复核场景读取交易事实的用途编码。
	TradingFactsPurposeCode = "risk_review"
)

// 事实键的封闭枚举，顺序即契约（与 Data Payload 一致）。
const (
	tradingFactKeyRiskWarning = "risk_warning_status"
	tradingFactKeySuspension  = "suspension_status"
	tradingFactKeyDelisting   = "delisting_status"
	tradingFactKeyListingDate = "listing_date"
	tradingFactKeyTradingDay  = "is_trading_day"
)

// 事实可用性枚举：没有"部分可用"。
const (
	tradingFactAvailable   = "available"
	tradingFactUnavailable = "unavailable"
)

// 已知不可交易的取值。这些值属于"事实已核验且结论为不可交易"，
// 与被判定为缺失的事实不同，不能混为一谈，也不得升级为 Verified。
const (
	tradingFactRiskWarningApplied = "risk_warning"
	tradingFactSuspended          = "suspended"
	tradingFactIntradaySuspended  = "intraday_suspension"
	tradingFactDelistingRisk      = "delisting_risk"
	tradingFactDelistingPeriod    = "delisting_period"
)

var ErrInvalidTradingFactsSnapshot = errors.New("invalid trading-status facts snapshot")

// TradingFact 是 Data 授权快照中的单项事实，原样保留可用性与原因。
type TradingFact struct {
	Key               string
	Availability      string
	Value             string
	UnavailableReason string
}

// TradingFactsSnapshot 是 Quant 消费的可追溯交易事实证据。
// AuthorizationRef 与 SnapshotRef 一起构成证据引用：只有前者能证明这份事实是按什么授权读到的。
type TradingFactsSnapshot struct {
	SnapshotRef           string
	AuthorizationRef      string
	ManifestSchemaVersion string
	PayloadSchemaVersion  string
	PayloadKind           string
	CallerRef             string
	PurposeCode           string
	SystemAsOf            string
	Symbol                string
	TradeDate             string
	QualityStatus         string
	Facts                 []TradingFact
}

var tradingFactOrder = []string{
	tradingFactKeyRiskWarning,
	tradingFactKeySuspension,
	tradingFactKeyDelisting,
	tradingFactKeyListingDate,
	tradingFactKeyTradingDay,
}

// Validate 校验身份、顺序与可用性一致性。Quant 不信任传输层：
// 即使 Data 侧已经校验过，这里仍按契约重新判定一次，避免 Adapter 适配错误变成静默事实。
func (snapshot TradingFactsSnapshot) Validate() error {
	if !nonBlankTradingFacts(snapshot.SnapshotRef) || !nonBlankTradingFacts(snapshot.AuthorizationRef) ||
		!nonBlankTradingFacts(snapshot.Symbol) || !nonBlankTradingFacts(snapshot.TradeDate) ||
		!nonBlankTradingFacts(snapshot.CallerRef) || !nonBlankTradingFacts(snapshot.SystemAsOf) {
		return fmt.Errorf("%w: identity or evidence references are missing", ErrInvalidTradingFactsSnapshot)
	}
	if snapshot.PayloadKind != TradingFactsPayloadKind ||
		snapshot.PayloadSchemaVersion != TradingFactsPayloadSchemaVersion ||
		!nonBlankTradingFacts(snapshot.ManifestSchemaVersion) {
		return fmt.Errorf("%w: payload kind or schema version mismatch", ErrInvalidTradingFactsSnapshot)
	}
	if len(snapshot.Facts) != len(tradingFactOrder) {
		return fmt.Errorf("%w: expected %d facts, got %d", ErrInvalidTradingFactsSnapshot, len(tradingFactOrder), len(snapshot.Facts))
	}
	for index, expected := range tradingFactOrder {
		fact := snapshot.Facts[index]
		if fact.Key != expected {
			return fmt.Errorf("%w: fact %d key = %q, want %q", ErrInvalidTradingFactsSnapshot, index, fact.Key, expected)
		}
		switch fact.Availability {
		case tradingFactAvailable:
			if !nonBlankTradingFacts(fact.Value) || fact.UnavailableReason != "" {
				return fmt.Errorf("%w: available fact %q must carry value and no reason", ErrInvalidTradingFactsSnapshot, fact.Key)
			}
		case tradingFactUnavailable:
			if fact.Value != "" || !nonBlankTradingFacts(fact.UnavailableReason) {
				return fmt.Errorf("%w: unavailable fact %q must carry reason and no value", ErrInvalidTradingFactsSnapshot, fact.Key)
			}
		default:
			return fmt.Errorf("%w: fact %q has unknown availability %q", ErrInvalidTradingFactsSnapshot, fact.Key, fact.Availability)
		}
	}
	if snapshot.QualityStatus != "valid" && snapshot.QualityStatus != "degraded" {
		return fmt.Errorf("%w: quality_status %q is not admissible", ErrInvalidTradingFactsSnapshot, snapshot.QualityStatus)
	}
	return nil
}

// TradingFactsReader 读取交易事实授权快照。生产实现由 Server 组合根注入。
type TradingFactsReader interface {
	ReadTradingFactsSnapshot(ctx context.Context, snapshotRef, authorizationRef, purpose string) (TradingFactsSnapshot, error)
}

// TradabilityAssessment 是事实到门禁输入的转换结果。
//
// BlockingFacts 单独列出"已核验且结论为不可交易"的事实项：门禁规则集只能表达
// 已核验/缺失两种状态，把已知不可交易当成"已核验"会让门禁在事实补齐后直接放行。
type TradabilityAssessment struct {
	Facts         risk.TradabilityFacts
	BlockingFacts []string
}

// AssessTradabilityFacts 把两份 Data 授权事实转换为 tradability 门禁输入。
//
// 映射规则（不新增任何业务阈值）：
//   - risk_warning_status → risk_warning；suspension_status → suspension；
//     这两项 Data 能提供，可用即可核验，取值命中已知不可交易时记入 BlockingFacts。
//   - tradable_volume → 只有合法且完成绑定的 order_book 授权快照能核验；
//     该事实是真实买盘五档挂单量之和，Quant 不推导、不估算。
//   - price_limit → 当前仍无合格事实来源，必须继续列入缺失；
//     因此 Verified 仍为 false，门禁仍 reject，但不能把 tradable_volume 伪称缺失。
//   - delisting_status / is_trading_day 不属于门禁四项，但已知的不可交易取值
//     仍记入 BlockingFacts，避免"事实越多、结论越松"。
func AssessTradabilityFacts(evidence TradabilityEvidence) (TradabilityAssessment, error) {
	if err := evidence.Validate(); err != nil {
		return TradabilityAssessment{}, err
	}
	snapshot := evidence.TradingFacts
	byKey := make(map[string]TradingFact, len(snapshot.Facts))
	for _, fact := range snapshot.Facts {
		byKey[fact.Key] = fact
	}
	missing := make([]risk.TradabilityFact, 0, 4)
	blocking := make([]string, 0, 4)

	assess := func(key string, gateFact risk.TradabilityFact, blockingValues ...string) {
		fact, present := byKey[key]
		if !present || fact.Availability != tradingFactAvailable {
			missing = append(missing, gateFact)
			return
		}
		for _, value := range blockingValues {
			if fact.Value == value {
				blocking = append(blocking, key)
				return
			}
		}
	}

	assess(tradingFactKeyRiskWarning, risk.TradabilityFactRiskWarning, tradingFactRiskWarningApplied)
	assess(tradingFactKeySuspension, risk.TradabilityFactSuspension, tradingFactSuspended, tradingFactIntradaySuspended)

	// 唯一仍无合格来源的门禁事实；不得把已核验的真实盘口事实也写成缺失。
	missing = append(missing, risk.TradabilityFactPriceLimit)

	if fact, present := byKey[tradingFactKeyDelisting]; present && fact.Availability == tradingFactAvailable {
		if fact.Value == tradingFactDelistingRisk || fact.Value == tradingFactDelistingPeriod {
			blocking = append(blocking, tradingFactKeyDelisting)
		}
	}
	if fact, present := byKey[tradingFactKeyTradingDay]; present && fact.Availability == tradingFactAvailable {
		if fact.Value == "false" {
			blocking = append(blocking, tradingFactKeyTradingDay)
		}
	}

	return TradabilityAssessment{
		Facts: risk.TradabilityFacts{
			Verified:     len(missing) == 0 && len(blocking) == 0,
			EvidenceRefs: evidence.evidenceRefs(),
			MissingFacts: missing,
		},
		BlockingFacts: blocking,
	}, nil
}

// EvaluateTradabilityDecision 用冻结的 tradability 规则集给出正式 RiskDecision。
// 事实缺失或被判定为已知不可交易，都只能得到 reject；不存在评分或模型覆盖入口。
func EvaluateTradabilityDecision(
	ctx context.Context, decisionID string, evaluatedAt time.Time, evidence TradabilityEvidence,
) (risk.Decision, error) {
	if evaluatedAt.IsZero() || evaluatedAt.Location() != time.UTC {
		return risk.Decision{}, errors.New("tradability decision evaluated_at must be a UTC time")
	}
	assessment, err := AssessTradabilityFacts(evidence)
	if err != nil {
		return risk.Decision{}, err
	}
	return risk.EvaluateTradabilityGate(ctx, risk.TradabilityGateInput{
		DecisionID:  decisionID,
		EvaluatedAt: evaluatedAt.Format(time.RFC3339Nano),
		SystemAsOf:  evidence.TradingFacts.SystemAsOf,
		InputRefs:   append([]string{}, assessment.Facts.EvidenceRefs...),
		Facts:       assessment.Facts,
	})
}

func nonBlankTradingFacts(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value
}
