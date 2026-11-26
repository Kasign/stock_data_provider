package stage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

// Trend Stage Adapter：把已授权 Data Close Series Snapshot 交给 Trend 生产 Facade，
// 再把已持久化的 TrendResult 投影为 QNT-CTR-003 Stage Output/Quality。
//
// 与 SMA20/Factor 的分工保持一致：Authorization 只由 Data 签发，result_id 只来自
// 已落库 Manifest；Adapter 不推导方向、不伪造质量，也不重复实现 Facade 的门禁。
var ErrInvalidTrendStageRequest = errors.New("invalid Quant Trend stage request")

// TrendRequest 是 Trend Stage Adapter 的输入。参数只有行情窗口：
// 方向判定的周期集合由 D1 决议固定为 5/20/60，不允许调用方覆盖。
type TrendRequest struct {
	Symbol               string
	StartDate            string
	EndDate              string
	Limit                int
	SystemAsOf           time.Time
	AllowedProviderCodes []string
}

type TrendAdapter struct {
	snapshots DailyBarsCloseSeriesSnapshotCreator
	service   *application.CreateTrendService
}

func NewTrendAdapter(
	snapshots DailyBarsCloseSeriesSnapshotCreator,
	service *application.CreateTrendService,
) (*TrendAdapter, error) {
	if snapshots == nil || service == nil {
		return nil, ErrInvalidTrendStageRequest
	}
	return &TrendAdapter{snapshots: snapshots, service: service}, nil
}

// Execute 在同一可信 Run SystemAsOf 下创建 Data Snapshot，再调用 Trend 生产 Facade。
// 61 根下限来自 D1 的最长窗口（momentum_60d）；样本实际充分性仍由证据 Metric 判定。
func (adapter *TrendAdapter) Execute(
	ctx context.Context,
	request TrendRequest,
) (TrendOutput, TrendQualitySummary, error) {
	if adapter == nil || adapter.snapshots == nil || adapter.service == nil || ctx == nil ||
		!quantSymbolPattern.MatchString(request.Symbol) ||
		!validISODate(request.StartDate) || !validISODate(request.EndDate) ||
		request.StartDate > request.EndDate ||
		request.Limit < trend.CloseTrendMinimumBars || request.Limit > 1000 ||
		request.SystemAsOf.IsZero() || request.SystemAsOf.Location() != time.UTC ||
		!sortedUniqueContains(request.AllowedProviderCodes, QuantProviderCode) {
		return TrendOutput{}, TrendQualitySummary{}, ErrInvalidTrendStageRequest
	}
	if err := ctx.Err(); err != nil {
		return TrendOutput{}, TrendQualitySummary{}, err
	}
	refs, err := adapter.snapshots.CreateDailyBarsCloseSeriesSnapshot(ctx, SnapshotCreateRequest{
		Symbol: request.Symbol, StartDate: request.StartDate, EndDate: request.EndDate,
		Limit: request.Limit, SystemAsOf: request.SystemAsOf,
		AllowedProviderCodes: append([]string(nil), request.AllowedProviderCodes...),
	})
	if err != nil {
		return TrendOutput{}, TrendQualitySummary{}, fmt.Errorf("create authorized Data Snapshot for Trend: %w", err)
	}
	if strings.TrimSpace(refs.SnapshotRef) == "" || strings.TrimSpace(refs.AuthorizationRef) == "" {
		return TrendOutput{}, TrendQualitySummary{}, ErrInvalidTrendStageRequest
	}
	execution, err := adapter.service.Execute(ctx, application.CreateTrendRequest{
		SnapshotRef:      refs.SnapshotRef,
		AuthorizationRef: refs.AuthorizationRef,
		SystemAsOf:       request.SystemAsOf,
		// Facade 的 parameters 契约是严格空对象：周期集合由决议固定。
		Parameters:           json.RawMessage("{}"),
		AllowedProviderCodes: request.AllowedProviderCodes,
	})
	if err != nil {
		return TrendOutput{}, TrendQualitySummary{}, fmt.Errorf("produce authorized Trend Quant Result: %w", err)
	}
	output := TrendOutput{
		SchemaVersion: TrendSchemaVersion,
		ResultID:      execution.Manifest.ResultID,
		SnapshotRef:   execution.SnapshotRef,
		ResultStatus:  execution.Manifest.ResultStatus,
		Result:        execution.Result,
	}
	quality := TrendQualitySummary{
		SchemaVersion:         TrendQualitySchemaVersion,
		TrendCode:             execution.Result.TrendCode,
		QualityStatus:         execution.InputQualityStatus,
		ResultQualityStatus:   string(execution.Result.QualityStatus),
		CalendarQualityStatus: execution.Coverage.CalendarQualityStatus,
		ExpectedTradingDays:   execution.Coverage.ExpectedTradingDays,
		ObservedBars:          execution.ObservedBars,
		MinimumRequiredBars:   execution.Coverage.MinimumRequiredBars,
		CoverageSufficient:    execution.Coverage.CoverageSufficient,
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return TrendOutput{}, TrendQualitySummary{}, ErrInvalidTrendStageOutput
	}
	if err := ValidateTrendStageOutputJSON(payload); err != nil {
		return TrendOutput{}, TrendQualitySummary{}, err
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return TrendOutput{}, TrendQualitySummary{}, ErrInvalidTrendStageOutput
	}
	if err := ValidateTrendQualitySummaryJSON(qualityJSON); err != nil {
		return TrendOutput{}, TrendQualitySummary{}, err
	}
	return output, quality, nil
}
