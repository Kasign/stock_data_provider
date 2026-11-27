package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/quant/application"
)

// Risk 门禁 Stage 的适配器（执行清单 1.4 / QNT-010）。
//
// 与 SMA20/Factor/Trend 同一套路：Stage 先用可信身份创建 Data 授权快照，
// 再把引用交给 Quant Facade，避免 Facade 直接持有 Data 的连接与策略。
// 事实分三类：交易状态来自 PIT 本地数据，tradable_volume 来自真实盘口，
// price_limit 来自当日全量涨跌停池。Stage 只传授权引用，不读取 Provider。
var ErrInvalidRiskStageRequest = errors.New("invalid Quant Risk stage request")

type TradingFactsSnapshotCreator interface {
	CreateTradingFactsSnapshot(
		ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
	) (snapshotRef, authorizationRef string, err error)
}

// OrderBookSnapshotCreator 创建并授权日内盘口事实快照。
// 盘口没有 trade_date：它以 observed_at / system_as_of 表达日内 PIT 精度。
type OrderBookSnapshotCreator interface {
	CreateOrderBookFactsSnapshot(
		ctx context.Context, symbol string, systemAsOf time.Time, purpose string,
	) (snapshotRef, authorizationRef string, err error)
}

// LimitPoolSnapshotCreator 创建并授权当日涨跌停事实快照。
// 它绑定 trade_date，且只在收盘后才可能给出 none 结论。
type LimitPoolSnapshotCreator interface {
	CreateLimitPoolFactsSnapshot(
		ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
	) (snapshotRef, authorizationRef string, err error)
}

type RiskRequest struct {
	Symbol     string
	TradeDate  string
	SystemAsOf time.Time
}

type RiskAdapter struct {
	facts      TradingFactsSnapshotCreator
	orderBooks OrderBookSnapshotCreator
	limitPools LimitPoolSnapshotCreator
	service    *application.CreateRiskService
}

func NewRiskAdapter(
	facts TradingFactsSnapshotCreator,
	orderBooks OrderBookSnapshotCreator,
	limitPools LimitPoolSnapshotCreator,
	service *application.CreateRiskService,
) (*RiskAdapter, error) {
	if facts == nil || orderBooks == nil || limitPools == nil || service == nil {
		return nil, ErrInvalidRiskStageRequest
	}
	return &RiskAdapter{facts: facts, orderBooks: orderBooks, limitPools: limitPools, service: service}, nil
}

// Execute 创建交易事实授权快照并评估可交易性门禁。
// 决策为 reject 是正常返回，不是错误：调用方必须读 Decision.Status。
func (adapter *RiskAdapter) Execute(
	ctx context.Context, request RiskRequest,
) (RiskOutput, RiskQualitySummary, error) {
	if adapter == nil || adapter.facts == nil || adapter.orderBooks == nil || adapter.limitPools == nil ||
		adapter.service == nil || ctx == nil ||
		!quantSymbolPattern.MatchString(request.Symbol) ||
		!validISODate(request.TradeDate) ||
		request.SystemAsOf.IsZero() || request.SystemAsOf.Location() != time.UTC ||
		request.TradeDate > request.SystemAsOf.Format("2006-01-02") {
		return RiskOutput{}, RiskQualitySummary{}, ErrInvalidRiskStageRequest
	}
	if err := ctx.Err(); err != nil {
		return RiskOutput{}, RiskQualitySummary{}, err
	}
	snapshotRef, authorizationRef, err := adapter.facts.CreateTradingFactsSnapshot(
		ctx, request.Symbol, request.TradeDate, request.SystemAsOf, RiskPurposeCode,
	)
	if err != nil {
		return RiskOutput{}, RiskQualitySummary{}, fmt.Errorf("create authorized trading-status facts Snapshot: %w", err)
	}
	orderBookRef, orderBookAuthorizationRef, err := adapter.orderBooks.CreateOrderBookFactsSnapshot(
		ctx, request.Symbol, request.SystemAsOf, RiskPurposeCode,
	)
	if err != nil {
		return RiskOutput{}, RiskQualitySummary{}, fmt.Errorf("create authorized order-book facts Snapshot: %w", err)
	}
	limitPoolRef, limitPoolAuthorizationRef, err := adapter.limitPools.CreateLimitPoolFactsSnapshot(
		ctx, request.Symbol, request.TradeDate, request.SystemAsOf, RiskPurposeCode,
	)
	if err != nil {
		return RiskOutput{}, RiskQualitySummary{}, fmt.Errorf("create authorized limit-pool facts Snapshot: %w", err)
	}
	if strings.TrimSpace(snapshotRef) == "" || strings.TrimSpace(authorizationRef) == "" ||
		strings.TrimSpace(orderBookRef) == "" || strings.TrimSpace(orderBookAuthorizationRef) == "" ||
		strings.TrimSpace(limitPoolRef) == "" || strings.TrimSpace(limitPoolAuthorizationRef) == "" {
		return RiskOutput{}, RiskQualitySummary{}, ErrInvalidRiskStageRequest
	}
	parameters, err := json.Marshal(struct {
		Symbol    string `json:"symbol"`
		TradeDate string `json:"trade_date"`
	}{Symbol: request.Symbol, TradeDate: request.TradeDate})
	if err != nil {
		return RiskOutput{}, RiskQualitySummary{}, ErrInvalidRiskStageRequest
	}
	execution, err := adapter.service.Execute(ctx, application.CreateRiskRequest{
		SnapshotRef: snapshotRef, AuthorizationRef: authorizationRef,
		OrderBookSnapshotRef: orderBookRef, OrderBookAuthorizationRef: orderBookAuthorizationRef,
		LimitPoolSnapshotRef: limitPoolRef, LimitPoolAuthorizationRef: limitPoolAuthorizationRef,
		SystemAsOf: request.SystemAsOf, Parameters: parameters,
	})
	if err != nil {
		return RiskOutput{}, RiskQualitySummary{}, fmt.Errorf("produce authorized Risk decision: %w", err)
	}
	output := RiskOutput{
		SchemaVersion: RiskSchemaVersion,
		ResultID:      execution.Manifest.ResultID,
		ResultStatus:  execution.Manifest.ResultStatus,
		MissingFacts:  riskFactNames(execution.MissingFacts),
		BlockingFacts: append([]string{}, execution.BlockingFacts...),
		Decision:      execution.Decision,
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return RiskOutput{}, RiskQualitySummary{}, ErrInvalidRiskStageOutput
	}
	if err := ValidateRiskStageOutputJSON(payload); err != nil {
		return RiskOutput{}, RiskQualitySummary{}, err
	}
	quality := RiskQualitySummary{
		SchemaVersion:  RiskQualitySchemaVersion,
		RuleSetVersion: execution.RuleSetVersion,
		QualityStatus:  execution.Manifest.QualityStatus,
		DecisionStatus: string(execution.Decision.Status),
		ResultStatus:   execution.Manifest.ResultStatus,
		MissingFacts:   append([]string{}, output.MissingFacts...),
		BlockingFacts:  append([]string{}, output.BlockingFacts...),
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return RiskOutput{}, RiskQualitySummary{}, ErrInvalidRiskStageOutput
	}
	if err := ValidateRiskQualitySummaryJSON(qualityJSON); err != nil {
		return RiskOutput{}, RiskQualitySummary{}, err
	}
	return output, quality, nil
}
