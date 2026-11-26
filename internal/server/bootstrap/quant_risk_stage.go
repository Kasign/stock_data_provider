package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/risk"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"example.com/stock-analysis-flow/internal/researchflow/template"
	"example.com/stock-analysis-flow/internal/server/resources"
)

// Quant Risk Stage 的 Server 组合（执行清单 1.4 / QNT-010）。
//
// Output Registry 版本在 Registry Set 内必须唯一（Set 以 version 为键），
// 因此这里使用与 News(1.0.0/1.2.0)、SMA20(1.1.0)、Factor(1.3.0)、Trend(1.4.0) 不同的版本。
//
// 本轮刻意**不**声明 hard_risk 门禁类型：组合根的 HardRiskGateRegistry 仍为 nil，
// 且 hard_risk 会进入 Run 终态判定，而当前决策恒 reject，声明它会让所有含此 Stage 的
// Run 必然失败。门禁类型与 Run 生命周期的绑定属独立治理动作。
const (
	quantRiskStageOutputRegistryVersion = "1.5.0"
	quantRiskPayloadSchemaRef           = "doc/quant/schema/QNT-CTR-003-risk-stage-output-1.0.0.schema.json"
	quantRiskQualitySchemaRef           = "doc/quant/schema/QNT-CTR-003-risk-quality-summary-1.0.0.schema.json"
	quantRiskWorkflowTemplateRef        = "quant.risk"
	quantRiskWorkflowTemplateVersion    = "1.0.0"
	quantRiskRetryPolicyRef             = "quant.risk.retry@1.0.0"

	// quantRiskDecisionPurpose 是决策身份的引用用途前缀。
	quantRiskDecisionPurpose = "risk_decision"
)

type quantRiskStageComposition struct {
	adapter          *quantstage.RiskAdapter
	repository       *quantstorage.MySQLRepository
	executor         researchflowapplication.StageExecutor
	registry         *stageoutput.Registry
	parameterSchemas *parameterschema.Validator
	applicability    template.ApplicabilityEvaluator
}

func newQuantRiskStageComposition(
	dataModule *marketdatabootstrap.Module,
	shared *resources.Resources,
) (*quantRiskStageComposition, error) {
	if dataModule == nil || shared == nil || shared.DB == nil {
		return nil, errors.New("Quant Risk Stage requires the Data Module and shared MariaDB")
	}
	// 与其它 Quant Stage 同一门槛：Data 未配置任何行情 Provider 时 Quant 能力整体不装配。
	// 交易事实来自 Data 的 PIT 落库数据，不需要 Provider 白名单，但门槛保持一致可避免
	// 出现"只有风险门禁在线"的半装配状态。
	if len(quantDailyBarsProviderCodes(dataModule, quantstage.QuantProviderCode)) == 0 {
		return nil, nil
	}
	factsAdapter, err := newQuantTradingFactsAdapter(dataModule)
	if err != nil {
		return nil, fmt.Errorf("create Quant trading-status facts adapter: %w", err)
	}
	quantRepository, err := quantstorage.NewMySQLRepository(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("create Quant result Repository: %w", err)
	}
	service, err := quantapplication.NewCreateRiskService(
		factsAdapter, quantRiskDecisionIDs{generator: researchflowIDGenerator{}}, quantRepository,
		func() time.Time { return time.Now().UTC() },
	)
	if err != nil {
		return nil, fmt.Errorf("create Quant Risk Application Service: %w", err)
	}
	adapter, err := quantstage.NewRiskAdapter(factsAdapter, service)
	if err != nil {
		return nil, fmt.Errorf("create Quant Risk Stage Adapter: %w", err)
	}
	registry, err := newQuantRiskOutputRegistry()
	if err != nil {
		return nil, fmt.Errorf("create Quant Risk Output Registry: %w", err)
	}
	parameterSchemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		return nil, fmt.Errorf("create Quant Risk parameter Schema Validator: %w", err)
	}
	return &quantRiskStageComposition{
		adapter:          adapter,
		repository:       quantRepository,
		executor:         quantRiskStageExecutor{adapter: adapter},
		registry:         registry,
		parameterSchemas: parameterSchemas,
		applicability:    quantRiskApplicabilityEvaluator{},
	}, nil
}

// quantRiskDecisionIDs 把 Researchflow 的引用生成器适配为 Quant 的决策身份生成器。
// 决策身份必须外部生成：决策内容含评估时刻，不能与 payload 指纹互相推导。
type quantRiskDecisionIDs struct {
	generator researchflowapplication.IDGenerator
}

func (ids quantRiskDecisionIDs) NewDecisionID(ctx context.Context) (string, error) {
	if ids.generator == nil {
		return "", errors.New("Quant Risk decision identity generator is unavailable")
	}
	return ids.generator.NewRef(ctx, quantRiskDecisionPurpose)
}

// newQuantRiskOutputRegistry 构建 quant.risk 的唯一 Output Registry。
// 组合根与契约测试共用本函数，避免测试复制 Entry 而掩盖真实注册漂移。
func newQuantRiskOutputRegistry() (*stageoutput.Registry, error) {
	return stageoutput.NewRegistry(quantRiskStageOutputRegistryVersion, []stageoutput.Entry{{
		StageCode:               quantstage.RiskStageCode,
		OutputType:              quantstage.RiskOutputType,
		SchemaVersion:           quantstage.RiskSchemaVersion,
		PayloadSchemaRef:        quantRiskPayloadSchemaRef,
		QualitySummarySchemaRef: quantRiskQualitySchemaRef,
		PayloadValidator:        stageoutput.ValidatorFunc(func(value json.RawMessage) error { return quantstage.ValidateRiskStageOutputJSON(value) }),
		QualitySummaryValidator: stageoutput.ValidatorFunc(func(value json.RawMessage) error { return quantstage.ValidateRiskQualitySummaryJSON(value) }),
	}})
}

func buildQuantRiskWorkflowTemplate(
	stage *quantRiskStageComposition,
	policies *resourcepolicy.Registry,
) (template.WorkflowTemplate, error) {
	if stage == nil || stage.parameterSchemas == nil || stage.applicability == nil || stage.registry == nil || policies == nil {
		return template.WorkflowTemplate{}, errors.New("Quant Risk template prerequisites are incomplete")
	}
	if stage.registry.Version() != quantRiskStageOutputRegistryVersion {
		return template.WorkflowTemplate{}, fmt.Errorf("Quant Risk Output Registry version mismatch: %q", stage.registry.Version())
	}
	// 交易事实来自 Data PIT，不消费 Provider；这里仍解析同一份资源策略，
	// 只是为了让模板的 ResourcePolicyRef 可解析，执行期不再校验 Provider 清单。
	if _, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{quantstage.QuantProviderCode}); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("resolve Quant Data Provider ResourcePolicy: %w", err)
	}
	parameters := json.RawMessage(`{"symbol":"600519.SH","trade_date":"2026-11-25"}`)
	schemaSnapshot, err := stage.parameterSchemas.Validate(context.Background(), quantRiskParameterSchemaRef, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate Quant Risk parameter Schema: %w", err)
	}
	if schemaSnapshot.Ref != quantRiskParameterSchemaRef ||
		schemaSnapshot.Version != quantRiskParameterSchemaVersion || len(schemaSnapshot.Fingerprint) != 64 {
		return template.WorkflowTemplate{}, errors.New("Quant Risk parameter Schema returned an inconsistent immutable snapshot")
	}
	applicable, reason, err := stage.applicability.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: quantRiskApplicabilityRuleRef, RuleVersion: quantRiskApplicabilityVersion,
	}, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("evaluate Quant Risk applicability rule: %w", err)
	}
	if !applicable || reason != "" {
		return template.WorkflowTemplate{}, errors.New("Quant Risk applicability preflight did not accept the validated parameters")
	}
	entry, ok := stage.registry.Entry(quantstage.RiskStageCode, quantstage.RiskOutputType, quantstage.RiskSchemaVersion)
	if !ok || entry.PayloadSchemaRef != quantRiskPayloadSchemaRef ||
		entry.QualitySummarySchemaRef != quantRiskQualitySchemaRef ||
		entry.PayloadValidator == nil || entry.QualitySummaryValidator == nil {
		return template.WorkflowTemplate{}, errors.New("Quant Risk Output Registry entry does not match its Stage contract")
	}
	value := template.WorkflowTemplate{
		Ref:                   quantRiskWorkflowTemplateRef,
		Version:               quantRiskWorkflowTemplateVersion,
		RequiresHardRiskGate:  false,
		AllowEmptyInputs:      true,
		ParameterSchemaRef:    quantRiskParameterSchemaRef,
		OutputRegistryVersion: stage.registry.Version(),
		ResourcePolicyRef:     resourcepolicy.QuantDailyBarsProvidersRef,
		Stages: []template.StageDefinition{{
			Code:         quantstage.RiskStageCode,
			Required:     true,
			Dependencies: []string{},
			Priority:     0,
			Applicability: template.ApplicabilityRule{
				RuleRef:     quantRiskApplicabilityRuleRef,
				RuleVersion: quantRiskApplicabilityVersion,
			},
			Retry: template.RetryPolicy{
				Ref:                  quantRiskRetryPolicyRef,
				MaxAttempts:          1,
				RetryableReasonCodes: []string{},
				Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
				AttemptTimeoutMS:     60000,
			},
			// 拒绝不是 Stage 失败：评估完成即成功，结论由 payload 的 status 承载。
			Propagation: template.PropagationPolicy{
				OnDegraded: researchflowdomain.PropagateAccept,
				OnSkipped:  researchflowdomain.PropagateFail,
				OnFailed:   researchflowdomain.PropagateFail,
				OnTimedOut: researchflowdomain.PropagateFail,
				OnBlocked:  researchflowdomain.PropagateFail,
			},
		}},
	}
	if err := template.Validate(value, nil); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate frozen Quant Risk workflow template: %w", err)
	}
	return value, nil
}

type quantRiskStageParameters struct {
	Symbol    string `json:"symbol"`
	TradeDate string `json:"trade_date"`
}

type quantRiskStageAdapter interface {
	Execute(context.Context, quantstage.RiskRequest) (quantstage.RiskOutput, quantstage.RiskQualitySummary, error)
}

type quantRiskStageExecutor struct {
	adapter quantRiskStageAdapter
}

func (executor quantRiskStageExecutor) ExecuteStage(
	ctx context.Context,
	request researchflowapplication.StageExecutionRequest,
) (researchflowapplication.StageExecutionResult, error) {
	if executor.adapter == nil || ctx == nil || request.Stage.Code != quantstage.RiskStageCode {
		return researchflowapplication.StageExecutionResult{}, errors.New("Quant Risk Stage Executor is unavailable")
	}
	var parameters quantRiskStageParameters
	decoder := json.NewDecoder(bytes.NewReader(request.Parameters))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil || ensureQuantJSONEOF(decoder) != nil {
		return failedQuantRiskStage("quant_risk_invalid_parameters"), nil
	}
	// 参数 Schema 在创建 Run 时已校验过，但执行期仍要自己把关：
	// 空 symbol/trade_date 会让 Data 用空值去查 PIT，属于必须前置拦掉的错配。
	if parameters.Symbol == "" || parameters.TradeDate == "" {
		return failedQuantRiskStage("quant_risk_invalid_parameters"), nil
	}
	output, quality, err := executor.adapter.Execute(ctx, quantstage.RiskRequest{
		Symbol: parameters.Symbol, TradeDate: parameters.TradeDate, SystemAsOf: request.SystemAsOf,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptTimedOut,
				StageStatus:   researchflowdomain.StageTimedOut,
				ReasonCodes:   []string{"quant_risk_deadline_exceeded"},
			}, nil
		case errors.Is(err, context.Canceled):
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptCancelled,
				StageStatus:   researchflowdomain.StageCancelled,
				ReasonCodes:   []string{"quant_risk_cancelled"},
			}, nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationDenied):
			return failedQuantRiskStage("data_authorization_denied"), nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationUnavailable):
			return failedQuantRiskStage("data_authorization_unavailable"), nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationInvalid):
			return failedQuantRiskStage("data_authorization_reference_invalid"), nil
		case errors.Is(err, quantstage.ErrInvalidRiskStageRequest),
			errors.Is(err, quantapplication.ErrInvalidCreateRiskRequest):
			return failedQuantRiskStage("quant_risk_invalid_parameters"), nil
		case errors.Is(err, quantapplication.ErrInvalidRiskSnapshot),
			errors.Is(err, quantapplication.ErrInvalidTradingFactsSnapshot):
			return failedQuantRiskStage("quant_risk_invalid_facts_snapshot"), nil
		default:
			return failedQuantRiskStage("quant_risk_execution_failed"), nil
		}
	}
	// 规则执行失败（unknown）不是"评估完成"：不产出可读的 Stage Output，
	// 否则下游会把 failed 的决策当成一次正常评估结果。
	if output.Status != risk.DecisionPass && output.Status != risk.DecisionReject {
		return failedQuantRiskStage("quant_risk_decision_unknown"), nil
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return failedQuantRiskStage("quant_risk_output_encoding_failed"), nil
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return failedQuantRiskStage("quant_risk_quality_encoding_failed"), nil
	}
	providerResult := &stageoutput.ProviderResult{
		OutputType:     quantstage.RiskOutputType,
		SchemaVersion:  quantstage.RiskSchemaVersion,
		Payload:        payload,
		QualitySummary: qualityJSON,
		DiagnosticRefs: []string{},
	}
	if quality.QualityStatus == "degraded" {
		return researchflowapplication.StageExecutionResult{
			AttemptStatus:  researchflowdomain.AttemptDegraded,
			StageStatus:    researchflowdomain.StageDegraded,
			ProviderResult: providerResult,
			ReasonCodes:    []string{"quant_risk_degraded_facts"},
		}, nil
	}
	return researchflowapplication.StageExecutionResult{
		AttemptStatus:  researchflowdomain.AttemptSucceeded,
		StageStatus:    researchflowdomain.StageSucceeded,
		ProviderResult: providerResult,
	}, nil
}

func failedQuantRiskStage(reasonCode string) researchflowapplication.StageExecutionResult {
	return researchflowapplication.StageExecutionResult{
		AttemptStatus: researchflowdomain.AttemptFailed,
		StageStatus:   researchflowdomain.StageFailed,
		ReasonCodes:   []string{reasonCode},
	}
}

var _ researchflowapplication.StageExecutor = quantRiskStageExecutor{}
