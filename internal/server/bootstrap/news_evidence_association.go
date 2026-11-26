package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

// newsEvidenceStageAssociationSpec 是已登记 News Stage 的不可变输出契约。
// 与 quant_result_association.go 的 quantStageAssociationSpec 同构，但身份模型
// 完全不同：这里绑定的是 NewsID，不是内容寻址 result_id。
type newsEvidenceStageAssociationSpec struct {
	StageCode               string
	OutputType              string
	SchemaVersion           string
	PayloadSchemaRef        string
	QualitySummarySchemaRef string
	PayloadValidator        func(json.RawMessage) error
	QualitySummaryValidator func(json.RawMessage) error
	EvidenceIDs             func(json.RawMessage) ([]string, error)
}

func (spec newsEvidenceStageAssociationSpec) expectation() researchflowapplication.StageOutputExpectation {
	return researchflowapplication.StageOutputExpectation{
		StageCode:               spec.StageCode,
		OutputType:              spec.OutputType,
		SchemaVersion:           spec.SchemaVersion,
		SchemaRef:               spec.PayloadSchemaRef,
		QualitySummarySchemaRef: spec.QualitySummarySchemaRef,
		PayloadValidator: func(value json.RawMessage) error {
			return spec.PayloadValidator(value)
		},
		QualitySummaryValidator: func(value json.RawMessage) error {
			return spec.QualitySummaryValidator(value)
		},
	}
}

// newsEvidenceStageAssociationSpecs 是 Run↔Evidence 关联允许的 Stage 全集。
//
// 只登记 news.collect_feeds：news.search_candidates 在设计上只承载候选发现，
// 不承载 Evidence 身份（source_item_id 不是 news_id），因此"未登记即拒绝"是
// 正确的 fail-closed 语义。规格值全部取自生产 News Stage Output Registry
// （news_search_stage.go::newNewsStageOutputRegistry），不复制第二套常量。
var newsEvidenceStageAssociationSpecs = []newsEvidenceStageAssociationSpec{
	{
		StageCode:               newsstage.CollectFeedsStageCode,
		OutputType:              newsstage.CollectFeedsOutputType,
		SchemaVersion:           newsstage.CollectFeedsSchemaVersion,
		PayloadSchemaRef:        newsstage.CollectFeedsPayloadSchemaRef,
		QualitySummarySchemaRef: newsstage.CollectFeedsQualitySummarySchemaRef,
		PayloadValidator:        newsstage.ValidateFeedCollectionPayload,
		QualitySummaryValidator: newsstage.ValidateFeedCollectionQualitySummary,
		EvidenceIDs:             newsFeedAssociationEvidenceIDs,
	},
}

func newsEvidenceStageAssociationSpecFor(stageCode string) (newsEvidenceStageAssociationSpec, bool) {
	for _, spec := range newsEvidenceStageAssociationSpecs {
		if spec.StageCode == stageCode {
			return spec, true
		}
	}
	return newsEvidenceStageAssociationSpec{}, false
}

// newsEvidenceRunStageReader 读取 Run 的 Stage 清单：必须先知道 Run 实际包含
// 哪些 Stage，才能区分"该 Stage 未产出"与"已产出但证据不一致"。
type newsEvidenceRunStageReader interface {
	GetRun(context.Context, string) (researchflowapplication.CreateRunResult, error)
}

type newsEvidenceStageOutputReader interface {
	Read(context.Context, string, researchflowapplication.StageOutputExpectation) (researchflowapplication.StageOutputResource, error)
}

type newsEvidenceAssociationVerifier struct {
	runs    newsEvidenceRunStageReader
	outputs newsEvidenceStageOutputReader
}

var _ newsapp.RunEvidenceAssociationVerifier = newsEvidenceAssociationVerifier{}

func (verifier newsEvidenceAssociationVerifier) VerifyEvidenceAssociation(
	ctx context.Context,
	runRef string,
	request newsapp.GetEvidenceRequest,
) error {
	if verifier.runs == nil || verifier.outputs == nil || ctx == nil ||
		!validNewsAssociationRef(runRef) || !validNewsAssociationRef(request.NewsID) {
		return newsapp.ErrEvidenceAssociationUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	run, err := verifier.runs.GetRun(ctx, runRef)
	if err != nil {
		return fmt.Errorf("read Researchflow Run for News evidence association: %w", err)
	}
	if run.Run.Ref != runRef || len(run.Stages) == 0 {
		return researchflowapplication.ErrStageOutputIntegrity
	}
	matched, err := verifier.matchSupportedStages(ctx, runRef, request.NewsID, run.Stages)
	if err != nil {
		return err
	}
	if !matched {
		return newsapp.ErrEvidenceAssociationNotFound
	}
	return nil
}

// matchSupportedStages 与 quantResultAssociationVerifier 逐行同构：只对规格表中
// 已登记且成功产出的 Stage 做严格读取，"已产出但不可读/不自洽"一律上抛，
// 绝不降级为"无关联"。
func (verifier newsEvidenceAssociationVerifier) matchSupportedStages(
	ctx context.Context,
	runRef, newsID string,
	stages []researchflowdomain.Stage,
) (bool, error) {
	observed := make(map[string]*researchflowdomain.Stage, len(newsEvidenceStageAssociationSpecs))
	for index := range stages {
		stage := &stages[index]
		if _, supported := newsEvidenceStageAssociationSpecFor(stage.Code); !supported {
			continue
		}
		if _, duplicate := observed[stage.Code]; duplicate {
			return false, researchflowapplication.ErrStageOutputIntegrity
		}
		if stage.RunRef != runRef || !validNewsAssociationRef(stage.Ref) {
			return false, researchflowapplication.ErrStageOutputIntegrity
		}
		observed[stage.Code] = stage
	}
	matched := false
	for _, spec := range newsEvidenceStageAssociationSpecs {
		stage, present := observed[spec.StageCode]
		if !present {
			continue
		}
		if stage.Status != researchflowdomain.StageSucceeded && stage.Status != researchflowdomain.StageDegraded {
			continue
		}
		if stage.OutputRef == nil || stage.QualitySummaryRef == nil {
			return false, researchflowapplication.ErrStageOutputIntegrity
		}
		resource, readErr := verifier.outputs.Read(ctx, runRef, spec.expectation())
		if readErr != nil {
			return false, fmt.Errorf("read %s Stage Output association: %w", spec.StageCode, readErr)
		}
		ids, decodeErr := spec.EvidenceIDs(resource.Content)
		if decodeErr != nil {
			return false, fmt.Errorf("decode validated %s Stage Output: %w", spec.StageCode, decodeErr)
		}
		for _, id := range ids {
			if id == newsID {
				matched = true
			}
		}
	}
	return matched, nil
}

// newsFeedAssociationEvidenceIDs 从已通过生产 Validator 校验的 feed payload 中
// 提取 NewsID。只有 status=="success" 的 source 才允许携带 news_ids
// （ValidateFeedCollectionPayload 强制其它状态不得携带）。
//
// feed payload 不携带 version_id，且 GetEvidenceRequest 没有 SourceRef 字段，
// 因此关联只绑定 NewsID；PIT 版本、授权快照与当前策略复核仍由 GetEvidence.Execute
// 严格负责，本 verifier 不放宽任何一项。
func newsFeedAssociationEvidenceIDs(raw json.RawMessage) ([]string, error) {
	var output newsstage.FeedCollectionOutput
	if err := json.Unmarshal(raw, &output); err != nil {
		return nil, err
	}
	ids := make([]string, 0)
	for _, source := range output.Sources {
		if source.Status != "success" {
			continue
		}
		ids = append(ids, source.NewsIDs...)
	}
	return ids, nil
}

func validNewsAssociationRef(value string) bool {
	return value != "" && strings.TrimSpace(value) == value
}

// evidenceReadUseCase 是 bootstrap 侧对 News 授权读取用例的窄端口；
// *newsapp.GetEvidence 天然实现它，News 内核不需要任何改动。
type evidenceReadUseCase interface {
	Execute(ctx context.Context, request newsapp.GetEvidenceRequest) (newsapp.ResearchEvidenceRecord, error)
}

// researchflowNewsEvidenceReader 与 researchflowQuantResultReader 同构：
// 先证明 Run↔Evidence 关联，再委托既有 GetEvidence.Execute 完成 PIT / 当前授权
// / 撤销竞态读取。它不重做授权、不访问 Repository、不改写返回的 DTO
// （RelationStatus 保持 News 内核的 not_provided，关联证明在 wrapper 之外）。
type researchflowNewsEvidenceReader struct {
	verifier newsapp.RunEvidenceAssociationVerifier
	read     evidenceReadUseCase
}

func (reader *researchflowNewsEvidenceReader) Read(
	ctx context.Context,
	runRef string,
	request newsapp.GetEvidenceRequest,
) (newsapp.ResearchEvidenceRecord, error) {
	if reader == nil || reader.verifier == nil || reader.read == nil || ctx == nil {
		return newsapp.ResearchEvidenceRecord{}, newsapp.ErrEvidenceAssociationUnavailable
	}
	if err := reader.verifier.VerifyEvidenceAssociation(ctx, runRef, request); err != nil {
		return newsapp.ResearchEvidenceRecord{}, err
	}
	return reader.read.Execute(ctx, request)
}
