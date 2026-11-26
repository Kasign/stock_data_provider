package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

type quantStageOutputReader interface {
	Read(context.Context, string, researchflowapplication.StageOutputExpectation) (researchflowapplication.StageOutputResource, error)
}

// quantRunStageReader 读取 Run 的 Stage 清单。
// 关联校验必须先知道 Run 实际包含哪些 Stage，才能区分"该 Stage 未产出"与"已产出但不可读"，
// 而不是靠试错读取把真实故障当作"无关联"。
type quantRunStageReader interface {
	GetRun(context.Context, string) (researchflowapplication.CreateRunResult, error)
}

// quantStageAssociationSpec 是一个已支持 Quant Stage 的不可变输出契约。
//
// 新增 Quant Stage 时必须在此登记；未登记的 Stage 不会被关联校验接受，
// 保持"未注册即拒绝"的 fail-closed 语义。
type quantStageAssociationSpec struct {
	StageCode               string
	OutputType              string
	SchemaVersion           string
	PayloadSchemaRef        string
	QualitySummarySchemaRef string
	PayloadValidator        func([]byte) error
	QualitySummaryValidator func([]byte) error
	ResultID                func(json.RawMessage) (string, error)
}

func (spec quantStageAssociationSpec) expectation() researchflowapplication.StageOutputExpectation {
	return researchflowapplication.StageOutputExpectation{
		StageCode:               spec.StageCode,
		OutputType:              spec.OutputType,
		SchemaVersion:           spec.SchemaVersion,
		SchemaRef:               spec.PayloadSchemaRef,
		QualitySummarySchemaRef: spec.QualitySummarySchemaRef,
		PayloadValidator: func(value json.RawMessage) error {
			return spec.PayloadValidator([]byte(value))
		},
		QualitySummaryValidator: func(value json.RawMessage) error {
			return spec.QualitySummaryValidator([]byte(value))
		},
	}
}

// quantStageAssociationSpecs 是 Quant 结果授权读取允许的 Stage 全集。
var quantStageAssociationSpecs = []quantStageAssociationSpec{
	{
		StageCode:               quantstage.SMA20StageCode,
		OutputType:              quantstage.SMA20OutputType,
		SchemaVersion:           quantstage.SMA20SchemaVersion,
		PayloadSchemaRef:        quantSMA20PayloadSchemaRef,
		QualitySummarySchemaRef: quantSMA20QualitySchemaRef,
		PayloadValidator:        quantstage.ValidateSMA20OutputJSON,
		QualitySummaryValidator: quantstage.ValidateSMA20QualitySummaryJSON,
		ResultID:                quantSMA20AssociationResultID,
	},
	{
		StageCode:               quantstage.FactorStageCode,
		OutputType:              quantstage.FactorOutputType,
		SchemaVersion:           quantstage.FactorSchemaVersion,
		PayloadSchemaRef:        quantFactorPayloadSchemaRef,
		QualitySummarySchemaRef: quantFactorQualitySchemaRef,
		PayloadValidator:        quantstage.ValidateFactorStageOutputJSON,
		QualitySummaryValidator: quantstage.ValidateFactorQualitySummaryJSON,
		ResultID:                quantFactorAssociationResultID,
	},
	{
		StageCode:               quantstage.TrendStageCode,
		OutputType:              quantstage.TrendOutputType,
		SchemaVersion:           quantstage.TrendSchemaVersion,
		PayloadSchemaRef:        quantTrendPayloadSchemaRef,
		QualitySummarySchemaRef: quantTrendQualitySchemaRef,
		PayloadValidator:        quantstage.ValidateTrendStageOutputJSON,
		QualitySummaryValidator: quantstage.ValidateTrendQualitySummaryJSON,
		ResultID:                quantTrendAssociationResultID,
	},
}

func quantStageAssociationSpecFor(stageCode string) (quantStageAssociationSpec, bool) {
	for _, spec := range quantStageAssociationSpecs {
		if spec.StageCode == stageCode {
			return spec, true
		}
	}
	return quantStageAssociationSpec{}, false
}

// quantResultAssociationVerifier 把 Researchflow 持久化的 Run/Stage/Attempt 关联
// 适配为 Quant 的窄结果关联端口。
type quantResultAssociationVerifier struct {
	runs    quantRunStageReader
	outputs quantStageOutputReader
}

var _ quantapplication.ResultAssociationVerifier = quantResultAssociationVerifier{}

func (verifier quantResultAssociationVerifier) VerifyResultAssociation(
	ctx context.Context,
	runRef, resultID string,
) error {
	if verifier.runs == nil || verifier.outputs == nil || ctx == nil ||
		!validQuantAssociationRef(runRef) || !validQuantAssociationRef(resultID) {
		return quantapplication.ErrAuthorizationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	run, err := verifier.runs.GetRun(ctx, runRef)
	if err != nil {
		return fmt.Errorf("read Researchflow Run for Quant result association: %w", err)
	}
	if run.Run.Ref != runRef || len(run.Stages) == 0 {
		return researchflowapplication.ErrStageOutputIntegrity
	}
	matched, err := verifier.matchSupportedStages(ctx, runRef, resultID, run.Stages)
	if err != nil {
		return err
	}
	if !matched {
		return quantapplication.ErrResultAssociationNotFound
	}
	return nil
}

// matchSupportedStages 只对规格表中登记且已成功产出的 Quant Stage 执行严格读取，
// 返回是否存在 result_id 精确相等的 Stage Output。
func (verifier quantResultAssociationVerifier) matchSupportedStages(
	ctx context.Context,
	runRef, resultID string,
	stages []researchflowdomain.Stage,
) (bool, error) {
	observed := make(map[string]*researchflowdomain.Stage, len(quantStageAssociationSpecs))
	for index := range stages {
		stage := &stages[index]
		if _, supported := quantStageAssociationSpecFor(stage.Code); !supported {
			continue
		}
		if _, duplicate := observed[stage.Code]; duplicate {
			// 同一 Run 内同码 Stage 不唯一时无法证明唯一关联，按完整性失败拒绝。
			return false, researchflowapplication.ErrStageOutputIntegrity
		}
		if stage.RunRef != runRef || !validQuantAssociationRef(stage.Ref) {
			return false, researchflowapplication.ErrStageOutputIntegrity
		}
		observed[stage.Code] = stage
	}
	matched := false
	for _, spec := range quantStageAssociationSpecs {
		stage, present := observed[spec.StageCode]
		if !present {
			continue
		}
		// 未成功产出的 Stage 不可能承载 result_id：这是正常的"无关联"，不是数据损坏。
		if stage.Status != researchflowdomain.StageSucceeded && stage.Status != researchflowdomain.StageDegraded {
			continue
		}
		// 已成功却缺少输出资源引用属于持久化不一致，必须 fail-closed，不能当作无关联。
		if stage.OutputRef == nil || stage.QualitySummaryRef == nil {
			return false, researchflowapplication.ErrStageOutputIntegrity
		}
		resource, readErr := verifier.outputs.Read(ctx, runRef, spec.expectation())
		if readErr != nil {
			// 已确认该 Stage 成功产出，此处的读取失败是真实故障，不得降级为"无关联"。
			return false, fmt.Errorf("read %s Stage Output association: %w", spec.StageCode, readErr)
		}
		observedID, decodeErr := spec.ResultID(resource.Content)
		if decodeErr != nil {
			return false, fmt.Errorf("decode validated %s Stage Output: %w", spec.StageCode, decodeErr)
		}
		if observedID == resultID {
			matched = true
		}
	}
	return matched, nil
}

func quantSMA20AssociationResultID(raw json.RawMessage) (string, error) {
	var output quantstage.SMA20Output
	if err := json.Unmarshal(raw, &output); err != nil {
		return "", err
	}
	return output.ResultID, nil
}

func quantFactorAssociationResultID(raw json.RawMessage) (string, error) {
	var output quantstage.FactorOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return "", err
	}
	return output.ResultID, nil
}

func quantTrendAssociationResultID(raw json.RawMessage) (string, error) {
	var output quantstage.TrendOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return "", err
	}
	return output.ResultID, nil
}

func validQuantAssociationRef(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}
