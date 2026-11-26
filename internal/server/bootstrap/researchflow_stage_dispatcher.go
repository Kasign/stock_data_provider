package bootstrap

import (
	"context"
	"fmt"

	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
)

type researchflowStageDispatcher struct {
	news        researchflowapplication.StageExecutor
	newsFeed    researchflowapplication.StageExecutor
	quant       researchflowapplication.StageExecutor
	quantFactor researchflowapplication.StageExecutor
}

func (dispatcher researchflowStageDispatcher) ExecuteStage(
	ctx context.Context,
	request researchflowapplication.StageExecutionRequest,
) (researchflowapplication.StageExecutionResult, error) {
	switch request.Stage.Code {
	case newsstage.SearchCandidatesStageCode:
		if dispatcher.news == nil {
			return researchflowapplication.StageExecutionResult{}, fmt.Errorf("News Stage Executor is not configured")
		}
		return dispatcher.news.ExecuteStage(ctx, request)
	case newsstage.CollectFeedsStageCode:
		if dispatcher.newsFeed == nil {
			return researchflowapplication.StageExecutionResult{}, fmt.Errorf("News Feed Stage Executor is not configured")
		}
		return dispatcher.newsFeed.ExecuteStage(ctx, request)
	case quantstage.SMA20StageCode:
		if dispatcher.quant == nil {
			return researchflowapplication.StageExecutionResult{}, fmt.Errorf("Quant SMA20 Stage Executor is not configured")
		}
		return dispatcher.quant.ExecuteStage(ctx, request)
	case quantstage.FactorStageCode:
		if dispatcher.quantFactor == nil {
			return researchflowapplication.StageExecutionResult{}, fmt.Errorf("Quant Factor Stage Executor is not configured")
		}
		return dispatcher.quantFactor.ExecuteStage(ctx, request)
	default:
		return researchflowapplication.StageExecutionResult{}, fmt.Errorf("unsupported Researchflow Stage code %q", request.Stage.Code)
	}
}

var _ researchflowapplication.StageExecutor = researchflowStageDispatcher{}
