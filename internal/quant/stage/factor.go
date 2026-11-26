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

// Factor Stage Adapter：把已授权 Data Close Series Snapshot 交给 Factor 生产 Facade，
// 再把已持久化的 Decimal Metric 投影为 QNT-CTR-003 Stage Output/Quality。
//
// 与 SMA20 的分工保持一致：Authorization 只由 Data 签发，result_id 只来自已落库
// Manifest；Adapter 不推导、不伪造二者，也不重复实现 Facade 的门禁。
var ErrInvalidFactorStageRequest = errors.New("invalid Quant Factor stage request")

// FactorRequest 是 Factor Stage Adapter 的输入。
//
// FactorCode 是 Stage 层参数，只用于封闭分派；它不会透传给 Application 的
// parameters —— CreateFactorService 要求 parameters 为严格空对象 {}，因为
// 因子参数本身由活动 Registry Entry 固定，允许外部传参等于允许绕过契约。
type FactorRequest struct {
	FactorCode           string
	Symbol               string
	StartDate            string
	EndDate              string
	Limit                int
	SystemAsOf           time.Time
	AllowedProviderCodes []string
}

type FactorAdapter struct {
	snapshots DailyBarsCloseSeriesSnapshotCreator
	service   *application.CreateFactorService
}

func NewFactorAdapter(
	snapshots DailyBarsCloseSeriesSnapshotCreator,
	service *application.CreateFactorService,
) (*FactorAdapter, error) {
	if snapshots == nil || service == nil {
		return nil, ErrInvalidFactorStageRequest
	}
	return &FactorAdapter{snapshots: snapshots, service: service}, nil
}

// Execute 在同一可信 Run SystemAsOf 下创建 Data Snapshot，再调用 Factor 生产 Facade。
// 每个因子的最小样本数按封闭分派表校验，避免向 Data 发起必然不足样本的请求；
// 实际样本充分性仍由 Facade 依据 Registry Entry 判定并可能降级为 insufficient_data。
func (adapter *FactorAdapter) Execute(
	ctx context.Context,
	request FactorRequest,
) (FactorOutput, FactorQualitySummary, error) {
	minimumBars, knownCode := factorStageMinimumBarsByCode[request.FactorCode]
	if adapter == nil || adapter.snapshots == nil || adapter.service == nil || ctx == nil ||
		!knownCode ||
		!quantSymbolPattern.MatchString(request.Symbol) ||
		!validISODate(request.StartDate) || !validISODate(request.EndDate) ||
		request.StartDate > request.EndDate ||
		request.Limit < minimumBars || request.Limit > 1000 ||
		request.SystemAsOf.IsZero() || request.SystemAsOf.Location() != time.UTC ||
		!sortedUniqueContains(request.AllowedProviderCodes, QuantProviderCode) {
		return FactorOutput{}, FactorQualitySummary{}, ErrInvalidFactorStageRequest
	}
	if err := ctx.Err(); err != nil {
		return FactorOutput{}, FactorQualitySummary{}, err
	}
	refs, err := adapter.snapshots.CreateDailyBarsCloseSeriesSnapshot(ctx, SnapshotCreateRequest{
		Symbol: request.Symbol, StartDate: request.StartDate, EndDate: request.EndDate,
		Limit: request.Limit, SystemAsOf: request.SystemAsOf,
		AllowedProviderCodes: append([]string(nil), request.AllowedProviderCodes...),
	})
	if err != nil {
		return FactorOutput{}, FactorQualitySummary{}, fmt.Errorf("create authorized Data Snapshot for Factor: %w", err)
	}
	if strings.TrimSpace(refs.SnapshotRef) == "" || strings.TrimSpace(refs.AuthorizationRef) == "" {
		return FactorOutput{}, FactorQualitySummary{}, ErrInvalidFactorStageRequest
	}
	execution, err := adapter.service.Execute(ctx, application.CreateFactorRequest{
		FactorCode:       request.FactorCode,
		SnapshotRef:      refs.SnapshotRef,
		AuthorizationRef: refs.AuthorizationRef,
		SystemAsOf:       request.SystemAsOf,
		// Facade 的 parameters 契约是严格空对象：因子参数由 Registry Entry 固定。
		Parameters:           json.RawMessage("{}"),
		AllowedProviderCodes: request.AllowedProviderCodes,
	})
	if err != nil {
		return FactorOutput{}, FactorQualitySummary{}, fmt.Errorf("produce authorized Factor Quant Result: %w", err)
	}
	value := json.RawMessage("null")
	if execution.Metric.Value != nil {
		encoded, encodeErr := json.Marshal(*execution.Metric.Value)
		if encodeErr != nil {
			return FactorOutput{}, FactorQualitySummary{}, ErrInvalidFactorStageOutput
		}
		value = encoded
	}
	output := FactorOutput{
		SchemaVersion:      FactorSchemaVersion,
		ResultID:           execution.Manifest.ResultID,
		SnapshotRef:        execution.SnapshotRef,
		MetricCode:         execution.Metric.MetricCode,
		Value:              value,
		Unit:               execution.Metric.Unit,
		Period:             execution.Metric.Period,
		MetricStatus:       string(execution.Metric.MetricStatus),
		SampleCount:        int64(execution.Metric.SampleCount),
		TradeDate:          execution.Manifest.TradeDate,
		SystemAsOf:         execution.Manifest.SystemAsOf,
		QualityStatus:      execution.Manifest.QualityStatus,
		ResultStatus:       execution.Manifest.ResultStatus,
		CalculationVersion: execution.Metric.CalculationVersion,
	}
	quality := FactorQualitySummary{
		SchemaVersion:         FactorQualitySchemaVersion,
		MetricCode:            execution.Metric.MetricCode,
		QualityStatus:         execution.Manifest.QualityStatus,
		CalendarQualityStatus: execution.Coverage.CalendarQualityStatus,
		ExpectedTradingDays:   execution.Coverage.ExpectedTradingDays,
		ObservedBars:          execution.ObservedBars,
		MinimumRequiredBars:   execution.Coverage.MinimumRequiredBars,
		CoverageSufficient:    execution.Coverage.CoverageSufficient,
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return FactorOutput{}, FactorQualitySummary{}, ErrInvalidFactorStageOutput
	}
	if err := ValidateFactorStageOutputJSON(payload); err != nil {
		return FactorOutput{}, FactorQualitySummary{}, err
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return FactorOutput{}, FactorQualitySummary{}, ErrInvalidFactorStageOutput
	}
	if err := ValidateFactorQualitySummaryJSON(qualityJSON); err != nil {
		return FactorOutput{}, FactorQualitySummary{}, err
	}
	return output, quality, nil
}
