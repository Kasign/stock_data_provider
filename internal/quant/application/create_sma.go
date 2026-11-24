package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
	"example.com/stock-analysis-flow/internal/quant/indicator"
	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
)

const (
	smaMetricCode          = "sma"
	smaMetricSchemaVersion = "1.0.0"
	smaPurposeCode         = "quant_analysis"
	smaSampleCountUnit     = "bar"
	smaSampleCountRule     = "window_bars"
	smaAlgorithmVersion    = "sma20-decimal-exact-v1"
	smaUnit                = "CNY/share"
	smaWindow              = 20
	smaMaximumDigits       = 128
	smaSourceRef           = "eastmoney"
	smaProviderAPIVersion  = "eastmoney-stock-api-v1"
)

var (
	ErrInvalidCreateSMARequest = errors.New("invalid Quant SMA request")
	ErrInvalidSMASnapshot      = errors.New("invalid Quant SMA input snapshot")
	ErrSMASnapshotQuality      = errors.New("Quant SMA input snapshot does not satisfy the registered quality policy")
	ErrUnsupportedSMAEntry     = errors.New("active SMA registry entry is not supported by the exact-decimal facade")
)

type CloseSeriesBar struct {
	TradeDate     string
	Close         string
	QualityStatus string
}

type CloseSeriesSource struct {
	SourceRef                string
	SourceVersion            *string
	ProviderInterfaceVersion string
}

type CloseSeriesCoverageEvidence struct {
	CalendarQualityStatus string
	ExpectedTradingDays   int
	MinimumRequiredBars   int
	CoverageSufficient    bool
}

// CloseSeriesSnapshot 是 Quant 消费 Data 已授权 Snapshot 的 Application DTO。
// 来源、Provider 版本、PIT 和复权证据由 Data Authorized Reader 在返回前完整校验。
type CloseSeriesSnapshot struct {
	SnapshotRef           string
	ManifestSchemaVersion string
	PayloadSchemaVersion  string
	PayloadKind           string
	CallerRef             string
	PurposeCode           string
	Sources               []CloseSeriesSource
	Symbol                string
	StartDate             string
	EndDate               string
	AdjustmentBasis       string
	SystemAsOf            string
	Interval              string
	TradingCalendarRef    string
	QualityStatus         string
	Coverage              CloseSeriesCoverageEvidence
	Bars                  []CloseSeriesBar
}

// CloseSeriesSnapshotReader 必须由 Server 适配 Data Authorized Snapshot Reader 实现。
// 授权、撤销状态、当前策略与 Payload 必须继续由 Data Owner Application 原子校验。
type CloseSeriesSnapshotReader interface {
	ReadAuthorizedCloseSeriesSnapshot(ctx context.Context, snapshotRef, authorizationRef, purposeCode string) (CloseSeriesSnapshot, error)
}

type CreateSMARequest struct {
	SnapshotRef          string
	AuthorizationRef     string
	SystemAsOf           time.Time
	Parameters           json.RawMessage
	AllowedProviderCodes []string
}

// SMAExecutionResult 是一次 SMA20 生产计算的持久化结果和输入质量摘要。
type SMAExecutionResult struct {
	Manifest     manifest.Manifest
	Metric       metric.Metric
	SnapshotRef  string
	Coverage     CloseSeriesCoverageEvidence
	ObservedBars int
}

// CreateSMAService 将 Data 的精确 Close Snapshot 转换为不可变 Quant Indicator Result。
type CreateSMAService struct {
	snapshots CloseSeriesSnapshotReader
	registry  *registry.Registry
	persister *resultPersister
	now       func() time.Time
}

// NewCreateSMAService 创建精确 SMA Facade；不注入默认 Registry Entry 或默认数据授权。
func NewCreateSMAService(
	snapshots CloseSeriesSnapshotReader,
	quantRegistry *registry.Registry,
	repository result.Repository,
	now func() time.Time,
) (*CreateSMAService, error) {
	if snapshots == nil || quantRegistry == nil || repository == nil || now == nil {
		return nil, ErrInvalidCreateSMARequest
	}
	persister, err := newResultPersister(repository)
	if err != nil {
		return nil, err
	}
	return &CreateSMAService{snapshots: snapshots, registry: quantRegistry, persister: persister, now: now}, nil
}

// Execute 读取已授权 Snapshot，按活动 Registry Entry 校验参数与精度，精确计算并持久化结果。
// Facade 不创建 Snapshot、不签发 Data 授权、不绕过 Registry，也不建立 Run 关联。
func (service *CreateSMAService) Execute(ctx context.Context, request CreateSMARequest) (SMAExecutionResult, error) {
	if service == nil || service.snapshots == nil || service.registry == nil || service.persister == nil ||
		ctx == nil || strings.TrimSpace(request.SnapshotRef) == "" ||
		strings.TrimSpace(request.AuthorizationRef) == "" || request.SystemAsOf.IsZero() ||
		request.SystemAsOf.Location() != time.UTC || len(request.Parameters) == 0 || !json.Valid(request.Parameters) ||
		!containsSortedUnique(request.AllowedProviderCodes, smaSourceRef) {
		return SMAExecutionResult{}, ErrInvalidCreateSMARequest
	}
	if err := ctx.Err(); err != nil {
		return SMAExecutionResult{}, err
	}
	entry, err := service.registry.LookupActive(smaMetricCode, smaMetricSchemaVersion)
	if err != nil {
		return SMAExecutionResult{}, fmt.Errorf("lookup active SMA registry entry: %w", err)
	}
	if err := validateExactSMRegistryEntry(entry); err != nil {
		return SMAExecutionResult{}, err
	}
	if err := service.registry.ValidateParameters(entry.Code, entry.SchemaVersion, entry.AlgorithmVersion, request.Parameters); err != nil {
		return SMAExecutionResult{}, fmt.Errorf("%w: parameters: %v", ErrInvalidCreateSMARequest, err)
	}
	var parameters struct {
		Window int `json:"window"`
	}
	decoder := json.NewDecoder(bytes.NewReader(request.Parameters))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil || parameters.Window != smaWindow {
		return SMAExecutionResult{}, ErrInvalidCreateSMARequest
	}

	parameterFingerprint, err := manifest.FingerprintPayload(request.Parameters)
	if err != nil {
		return SMAExecutionResult{}, fmt.Errorf("%w: parameter fingerprint: %v", ErrInvalidCreateSMARequest, err)
	}
	snapshot, err := service.snapshots.ReadAuthorizedCloseSeriesSnapshot(ctx, request.SnapshotRef, request.AuthorizationRef, smaPurposeCode)
	if err != nil {
		return SMAExecutionResult{}, fmt.Errorf("read authorized Data Snapshot: %w", err)
	}
	trustedNow := service.now()
	systemAsOf, err := validateExactSMASnapshot(snapshot, request, entry, trustedNow)
	if err != nil {
		return SMAExecutionResult{}, err
	}

	closes := make([]string, len(snapshot.Bars))
	for index := range snapshot.Bars {
		closes[index] = snapshot.Bars[index].Close
	}
	tradeDate := snapshot.Bars[len(snapshot.Bars)-1].TradeDate
	calculationContext := domain.CalculationContext{
		SystemAsOf: systemAsOf, TradeDate: tradeDate,
		InputSnapshotRefs: []string{snapshot.SnapshotRef},
		AdjustmentBasis:   domain.AdjustmentBasisNone,
		QualityPolicy:     domain.QualityPolicyStrict,
	}
	calculated, err := indicator.DecimalSMAWithContext(calculationContext, trustedNow, closes, indicator.SMAParams{Window: parameters.Window})
	if err != nil {
		return SMAExecutionResult{}, fmt.Errorf("calculate exact SMA: %w", err)
	}

	metricValue := json.RawMessage("null")
	metricStatus := metric.StatusInsufficientData
	resultStatus := "insufficient_data"
	if calculated.Status == indicator.StatusAvailable {
		metricValue, err = json.Marshal(calculated.Value)
		if err != nil {
			return SMAExecutionResult{}, fmt.Errorf("encode exact SMA value: %w", err)
		}
		metricStatus = metric.StatusAvailable
		resultStatus = "completed"
	}
	sampleCount := calculated.SampleCount
	item := metric.Metric{
		MetricCode: smaMetricCode, Value: metricValue, Unit: entry.Unit,
		Period: fmt.Sprintf("%dd", parameters.Window), SystemAsOf: snapshot.SystemAsOf,
		QualityStatus: metric.QualityValid, MetricStatus: metricStatus,
		CalculationVersion: entry.AlgorithmVersion, SampleCount: &sampleCount,
		LowerBound: json.RawMessage("null"), UpperBound: json.RawMessage("null"),
	}
	metricJSON, err := json.Marshal(item)
	if err != nil {
		return SMAExecutionResult{}, fmt.Errorf("encode SMA QuantMetric: %w", err)
	}
	if err := service.registry.ValidateMetric(entry.Code, entry.SchemaVersion, entry.AlgorithmVersion, metricJSON); err != nil {
		return SMAExecutionResult{}, fmt.Errorf("validate SMA QuantMetric against active Registry Entry: %w", err)
	}
	payload, err := result.NewIndicatorPayload(smaMetricCode, []metric.Metric{item})
	if err != nil {
		return SMAExecutionResult{}, err
	}

	input := manifest.Input{
		ResultType: "indicator", Scope: "stock", SecurityID: snapshot.Symbol,
		TradeDate: tradeDate, Period: fmt.Sprintf("%dd", parameters.Window),
		QualityStatus: snapshot.QualityStatus, ResultStatus: resultStatus,
		SchemaVersion: result.EnvelopeSchemaVersion, PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion: entry.AlgorithmVersion, ParameterFingerprint: parameterFingerprint,
		SystemAsOf: systemAsOf, InputSnapshotRefs: []string{snapshot.SnapshotRef},
	}
	persisted, err := service.persister.persist(ctx, input, payload)
	if err != nil {
		return SMAExecutionResult{}, err
	}
	return SMAExecutionResult{
		Manifest: persisted,
		Metric: item,
		SnapshotRef: snapshot.SnapshotRef,
		Coverage: snapshot.Coverage,
		ObservedBars: len(snapshot.Bars),
	}, nil
}

func validateExactSMRegistryEntry(entry registry.Entry) error {
	if entry.Code != smaMetricCode || entry.SchemaVersion != smaMetricSchemaVersion ||
		entry.ValueType != registry.ValueTypeDecimalString || entry.Unit != smaUnit ||
		entry.AlgorithmVersion != smaAlgorithmVersion || entry.Precision.MaxIntegerDigits != smaMaximumDigits ||
		entry.Precision.MaxFractionDigits != smaMaximumDigits || entry.Precision.RoundingMode != "exact" ||
		!bytes.Equal(bytes.TrimSpace(entry.Precision.Tolerance), []byte("\"0\"")) ||
		!entry.SampleCount.RequiredInMetric || entry.SampleCount.Unit != smaSampleCountUnit ||
		entry.SampleCount.CountingRule != smaSampleCountRule || len(entry.InputRequirements) != 1 {
		return ErrUnsupportedSMAEntry
	}
	requirement := entry.InputRequirements[0]
	if len(requirement.Fields) != 1 || requirement.Fields[0] != "close" ||
		requirement.Interval != "1d" || requirement.Adjustment != "none" ||
		!requirement.PIT || !requirement.RequiresSnapshot || !containsString(requirement.Quality, "valid") ||
		!containsString(entry.QualityPolicy.QualityStates, "valid") ||
		!containsString(entry.QualityPolicy.UnavailableConditions, "insufficient_data") {
		return ErrUnsupportedSMAEntry
	}
	return nil
}

func validateExactSMASnapshot(snapshot CloseSeriesSnapshot, request CreateSMARequest, entry registry.Entry, trustedNow time.Time) (time.Time, error) {
	if snapshot.SnapshotRef != request.SnapshotRef ||
		snapshot.ManifestSchemaVersion != "2.0.0" || snapshot.PayloadSchemaVersion != "1.0.0" ||
		snapshot.PayloadKind != "daily_bars_close_series" || strings.TrimSpace(snapshot.CallerRef) == "" ||
		snapshot.PurposeCode != smaPurposeCode || !validSMAProviderSources(snapshot.Sources, request.AllowedProviderCodes) || strings.TrimSpace(snapshot.Symbol) == "" ||
		!validSMAISODate(snapshot.StartDate) || !validSMAISODate(snapshot.EndDate) || snapshot.StartDate > snapshot.EndDate ||
		snapshot.AdjustmentBasis != "none" || snapshot.Interval != "1d" ||
		strings.TrimSpace(snapshot.TradingCalendarRef) == "" || snapshot.QualityStatus != "valid" ||
		snapshot.Coverage.CalendarQualityStatus != "valid" ||
		snapshot.Coverage.ExpectedTradingDays <= 0 || snapshot.Coverage.MinimumRequiredBars <= 0 ||
		!snapshot.Coverage.CoverageSufficient ||
		len(snapshot.Bars) < snapshot.Coverage.MinimumRequiredBars ||
		len(snapshot.Bars) > snapshot.Coverage.ExpectedTradingDays {
		return time.Time{}, ErrInvalidSMASnapshot
	}
	coverage := new(big.Rat).SetFrac(big.NewInt(int64(len(snapshot.Bars))), big.NewInt(int64(snapshot.Coverage.ExpectedTradingDays)))
	minimumCoverage := new(big.Rat).SetFloat64(entry.QualityPolicy.MinimumCoverage)
	if minimumCoverage == nil || coverage.Cmp(minimumCoverage) < 0 {
		return time.Time{}, ErrSMASnapshotQuality
	}
	var previousDate string
	for index, bar := range snapshot.Bars {
		if !validSMAISODate(bar.TradeDate) || bar.TradeDate < snapshot.StartDate ||
			bar.TradeDate > snapshot.EndDate || (index > 0 && bar.TradeDate <= previousDate) ||
			bar.QualityStatus != "valid" || strings.TrimSpace(bar.Close) == "" {
			return time.Time{}, ErrInvalidSMASnapshot
		}
		previousDate = bar.TradeDate
	}
	if len(snapshot.Bars) == 0 {
		return time.Time{}, ErrInvalidSMASnapshot
	}
	systemAsOf, err := parseCanonicalSMAUTC(snapshot.SystemAsOf)
	if err != nil || !systemAsOf.Equal(request.SystemAsOf) ||
		trustedNow.IsZero() || trustedNow.Location() != time.UTC || trustedNow.Before(systemAsOf) {
		return time.Time{}, ErrInvalidSMASnapshot
	}
	return systemAsOf, nil
}

func parseCanonicalSMAUTC(value string) (time.Time, error) {
	if !strings.HasSuffix(value, "Z") {
		return time.Time{}, ErrInvalidSMASnapshot
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || parsed.Location() != time.UTC || parsed.UTC().Format(time.RFC3339Nano) != value {
		return time.Time{}, ErrInvalidSMASnapshot
	}
	return parsed, nil
}

func validSMAISODate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func containsSortedUnique(values []string, expected string) bool {
	if len(values) == 0 {
		return false
	}
	found := false
	for index, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || (index > 0 && values[index-1] >= value) {
			return false
		}
		if value == expected {
			found = true
		}
	}
	return found
}

func validSMAProviderSources(sources []CloseSeriesSource, allowedProviderCodes []string) bool {
	if len(sources) != 1 || !containsSortedUnique(allowedProviderCodes, smaSourceRef) {
		return false
	}
	source := sources[0]
	return source.SourceRef == smaSourceRef && source.SourceVersion == nil &&
		source.ProviderInterfaceVersion == smaProviderAPIVersion
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
