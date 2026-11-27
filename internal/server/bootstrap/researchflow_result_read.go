package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	quantresult "example.com/stock-analysis-flow/internal/quant/result"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

// quantResultReadUseCase 是阶段 2.3 结果读取的窄端口。
// 生产实现是 2.2 已装配的 researchflowQuantResultReader：它在 Read 内部注入
// 进程内可信 Worker 身份，HTTP 输入无法构造该身份，也无法绕过关联与当前授权校验。
type quantResultReadUseCase interface {
	Read(ctx context.Context, runRef, resultID string) (quantresult.Envelope, error)
}

var _ quantResultReadUseCase = (*researchflowQuantResultReader)(nil)

// researchflowRunReadPort 是已授权 Run 读取的窄端口。生产实现是
// *researchflowapplication.GetRunService：它在返回 Stage 集合前已经完成
// Owner 身份解析与资源级读取授权，因此结果读取可以安全地以其返回的
// Stage 集合作为唯一发现来源（D5：禁止用 HTTP 输入直查 Repository）。
type researchflowRunReadPort interface {
	GetRun(ctx context.Context, runRef string) (researchflowapplication.CreateRunResult, error)
}

// researchflowRunResultReadService 是阶段 2.3 的 Server 读用例：
// 先经 GetRun 完成资源级授权，再按已授权 Stage 集合读取领域结果。
// 两个依赖都是窄端口，便于在单测中验证"授权失败不得触达结果读取"。
type researchflowRunResultReadService struct {
	runs   researchflowRunReadPort
	reader *researchflowComposition
}

func (s *researchflowRunResultReadService) ReadRunResults(
	ctx context.Context,
	runRef string,
) ([]quantresult.Envelope, error) {
	if s == nil || s.runs == nil || s.reader == nil || ctx == nil {
		return nil, quantresult.ErrUnsupportedPayload
	}
	// 资源级授权必须先于任何结果读取；GetRun 失败时直接上抛，不降级。
	aggregate, err := s.runs.GetRun(ctx, runRef)
	if err != nil {
		return nil, err
	}
	if aggregate.Run.Ref != runRef {
		return nil, researchflowapplication.ErrStageOutputIntegrity
	}
	return s.reader.readQuantResultsForStages(ctx, runRef, aggregate.Stages)
}

// researchflowResultReadItem 是 Server 对外公开的受限结果投影。
// 只暴露 Envelope 的元数据与被允许的强类型 Payload；不含 StageOutputResource.Content、
// domain.Run 内部字段、Attempt 明细与 RiskDecision 原始结构（D6）。
type researchflowResultReadItem struct {
	SchemaVersion        string          `json:"schema_version"`
	ResultID             string          `json:"result_id"`
	ResultType           string          `json:"result_type"`
	Scope                string          `json:"scope"`
	SecurityID           string          `json:"security_id,omitempty"`
	SystemAsOf           string          `json:"system_as_of"`
	TradeDate            string          `json:"trade_date,omitempty"`
	QualityStatus        string          `json:"quality_status"`
	ResultStatus         string          `json:"result_status"`
	AuthorizationRef     string          `json:"authorization_ref"`
	PayloadSchemaVersion string          `json:"payload_schema_version"`
	Payload              json.RawMessage `json:"payload,omitempty"`
}

// projectQuantResultForRead 把已授权 Envelope 投影为受限公开 DTO。
// risk 结果的 Payload 承载 RiskDecision 原始结构，按 D6 明确不对外暴露；
// 其余已注册 Payload 是经 QNT-CTR-004 校验的强类型结果，可完整投影。
// 未登记的 result_type 一律拒绝，防止新增类型绕过投影审查。
func projectQuantResultForRead(envelope quantresult.Envelope) (researchflowResultReadItem, error) {
	switch envelope.ResultType {
	case "indicator", "factor", "trend":
	case "risk":
	default:
		return researchflowResultReadItem{}, fmt.Errorf("%w: result_type %q is not allowed for public read",
			quantresult.ErrUnsupportedPayload, envelope.ResultType)
	}
	item := researchflowResultReadItem{
		SchemaVersion:        envelope.SchemaVersion,
		ResultID:             envelope.ResultID,
		ResultType:           envelope.ResultType,
		Scope:                envelope.Scope,
		SecurityID:           envelope.SecurityID,
		SystemAsOf:           envelope.SystemAsOf,
		TradeDate:            envelope.TradeDate,
		QualityStatus:        envelope.QualityStatus,
		ResultStatus:         envelope.ResultStatus,
		AuthorizationRef:     envelope.AuthorizationRef,
		PayloadSchemaVersion: envelope.PayloadSchemaVersion,
	}
	if envelope.ResultType != "risk" {
		payload, err := json.Marshal(envelope.Payload)
		if err != nil {
			return researchflowResultReadItem{}, err
		}
		item.Payload = json.RawMessage(payload)
	}
	return item, nil
}

// readQuantResultsForStages 从已通过 GetRun 资源级授权的 Run Stage 集合中读取
// 已成功产出的 Quant 结果。result_id 只从持久化 Stage Output 的已校验 payload 解析，
// 不接受 HTTP 输入；读取必须经 quantResultRead 的关联与当前授权校验。
//
// fail-closed 语义与 2.2 关联校验器一致：
//   - 未登记的 Stage 跳过；
//   - 非成功产出的 Stage 跳过（正常无关联）；
//   - 已成功产出但缺引用、跨 Run、读失败、解析失败一律上抛，不降级为无关联。
func (composition *researchflowComposition) readQuantResultsForStages(
	ctx context.Context,
	runRef string,
	stages []researchflowdomain.Stage,
) ([]quantresult.Envelope, error) {
	if composition == nil || composition.stageOutputReader == nil || composition.quantResultRead == nil || ctx == nil {
		return nil, quantresult.ErrUnsupportedPayload
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	envelopes := make([]quantresult.Envelope, 0, len(stages))
	for index := range stages {
		stage := &stages[index]
		spec, registered := quantStageAssociationSpecFor(stage.Code)
		if !registered {
			continue
		}
		if stage.RunRef != runRef || !validQuantAssociationRef(stage.Ref) {
			return nil, researchflowapplication.ErrStageOutputIntegrity
		}
		if stage.Status != researchflowdomain.StageSucceeded && stage.Status != researchflowdomain.StageDegraded {
			continue
		}
		if stage.OutputRef == nil || stage.QualitySummaryRef == nil {
			return nil, researchflowapplication.ErrStageOutputIntegrity
		}
		resource, readErr := composition.stageOutputReader.Read(ctx, runRef, spec.expectation())
		if readErr != nil {
			return nil, fmt.Errorf("read %s Stage Output for result discovery: %w", stage.Code, readErr)
		}
		resultID, decodeErr := spec.ResultID(resource.Content)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode validated %s Stage Output result_id: %w", stage.Code, decodeErr)
		}
		if !validQuantAssociationRef(resultID) {
			return nil, researchflowapplication.ErrStageOutputIntegrity
		}
		envelope, resultErr := composition.quantResultRead.Read(ctx, runRef, resultID)
		if resultErr != nil {
			return nil, fmt.Errorf("authorized read %s result %s: %w", stage.Code, resultID, resultErr)
		}
		envelopes = append(envelopes, envelope)
	}
	return envelopes, nil
}
