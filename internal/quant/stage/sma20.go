package stage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/quant/application"
)

const (
	SMA20StageCode     = "quant.sma20"
	SMA20OutputType    = "quant.sma20_result"
	SMA20SchemaVersion = "1.0.0"
	SMA20QualitySchema = "1.0.0"
	SMA20PurposeCode   = "quant_analysis"
	SMA20Window        = 20
	SMA20ProviderCode  = "eastmoney"
)

var (
	ErrInvalidSMA20StageRequest = errors.New("invalid Quant SMA20 stage request")
	ErrInvalidSMA20StageOutput  = errors.New("invalid Quant SMA20 stage output")
)

var sma20SymbolPattern = regexp.MustCompile(`^[0-9]{6}\.(SH|SZ|BJ)$`)
var sma20DecimalPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)

type SnapshotCreateRequest struct {
	Symbol     string
	StartDate  string
	EndDate    string
	Limit      int
	SystemAsOf time.Time
}

type SnapshotReferences struct {
	SnapshotRef      string
	AuthorizationRef string
}

type DailyBarsCloseSeriesSnapshotCreator interface {
	CreateDailyBarsCloseSeriesSnapshot(ctx context.Context, request SnapshotCreateRequest) (SnapshotReferences, error)
}

type SMA20Request struct {
	Symbol               string
	StartDate            string
	EndDate              string
	Limit                int
	SystemAsOf           time.Time
	AllowedProviderCodes []string
}

type SMA20Output struct {
	SchemaVersion string          `json:"schema_version"`
	ResultID      string          `json:"result_id"`
	SnapshotRef   string          `json:"snapshot_ref"`
	MetricCode    string          `json:"metric_code"`
	Value         json.RawMessage `json:"value"`
	Unit          string          `json:"unit"`
	Period        string          `json:"period"`
	MetricStatus  string          `json:"metric_status"`
	SampleCount   int64           `json:"sample_count"`
	TradeDate     string          `json:"trade_date"`
	SystemAsOf    string          `json:"system_as_of"`
	QualityStatus string          `json:"quality_status"`
	ResultStatus  string          `json:"result_status"`
}

type SMA20QualitySummary struct {
	SchemaVersion         string `json:"schema_version"`
	QualityStatus         string `json:"quality_status"`
	CalendarQualityStatus string `json:"calendar_quality_status"`
	ExpectedTradingDays   int    `json:"expected_trading_days"`
	ObservedBars          int    `json:"observed_bars"`
	MinimumRequiredBars   int    `json:"minimum_required_bars"`
	CoverageSufficient    bool   `json:"coverage_sufficient"`
}

type SMA20Adapter struct {
	snapshots DailyBarsCloseSeriesSnapshotCreator
	service   *application.CreateSMAService
}

func NewSMA20Adapter(snapshots DailyBarsCloseSeriesSnapshotCreator, service *application.CreateSMAService) (*SMA20Adapter, error) {
	if snapshots == nil || service == nil {
		return nil, ErrInvalidSMA20StageRequest
	}
	return &SMA20Adapter{snapshots: snapshots, service: service}, nil
}

// Execute 在同一可信 Run SystemAsOf 下创建 Data Snapshot，再调用 Quant Application 生产 SMA20。
func (adapter *SMA20Adapter) Execute(ctx context.Context, request SMA20Request) (SMA20Output, SMA20QualitySummary, error) {
	if adapter == nil || adapter.snapshots == nil || adapter.service == nil || ctx == nil ||
		!sma20SymbolPattern.MatchString(request.Symbol) ||
		!validISODate(request.StartDate) || !validISODate(request.EndDate) || request.StartDate > request.EndDate ||
		request.Limit < SMA20Window || request.Limit > 1000 ||
		request.SystemAsOf.IsZero() || request.SystemAsOf.Location() != time.UTC ||
		!sortedUniqueContains(request.AllowedProviderCodes, SMA20ProviderCode) {
		return SMA20Output{}, SMA20QualitySummary{}, ErrInvalidSMA20StageRequest
	}
	if err := ctx.Err(); err != nil {
		return SMA20Output{}, SMA20QualitySummary{}, err
	}
	refs, err := adapter.snapshots.CreateDailyBarsCloseSeriesSnapshot(ctx, SnapshotCreateRequest{
		Symbol: request.Symbol, StartDate: request.StartDate, EndDate: request.EndDate,
		Limit: request.Limit, SystemAsOf: request.SystemAsOf,
	})
	if err != nil {
		return SMA20Output{}, SMA20QualitySummary{}, fmt.Errorf("create authorized Data Snapshot for SMA20: %w", err)
	}
	if strings.TrimSpace(refs.SnapshotRef) == "" || strings.TrimSpace(refs.AuthorizationRef) == "" {
		return SMA20Output{}, SMA20QualitySummary{}, ErrInvalidSMA20StageRequest
	}
	execution, err := adapter.service.Execute(ctx, application.CreateSMARequest{
		SnapshotRef: refs.SnapshotRef, AuthorizationRef: refs.AuthorizationRef,
		SystemAsOf:           request.SystemAsOf,
		Parameters:           json.RawMessage("{\"window\":20}"),
		AllowedProviderCodes: request.AllowedProviderCodes,
	})
	if err != nil {
		return SMA20Output{}, SMA20QualitySummary{}, fmt.Errorf("produce authorized SMA20 Quant Result: %w", err)
	}
	if execution.Metric.SampleCount == nil {
		return SMA20Output{}, SMA20QualitySummary{}, ErrInvalidSMA20StageOutput
	}
	output := SMA20Output{
		SchemaVersion: SMA20SchemaVersion,
		ResultID:      execution.Manifest.ResultID,
		SnapshotRef:   execution.SnapshotRef,
		MetricCode:    "sma",
		Value:         append(json.RawMessage(nil), execution.Metric.Value...),
		Unit:          execution.Metric.Unit,
		Period:        execution.Metric.Period,
		MetricStatus:  string(execution.Metric.MetricStatus),
		SampleCount:   *execution.Metric.SampleCount,
		TradeDate:     execution.Manifest.TradeDate,
		SystemAsOf:    execution.Manifest.SystemAsOf,
		QualityStatus: execution.Manifest.QualityStatus,
		ResultStatus:  execution.Manifest.ResultStatus,
	}
	quality := SMA20QualitySummary{
		SchemaVersion:         SMA20QualitySchema,
		QualityStatus:         execution.Manifest.QualityStatus,
		CalendarQualityStatus: execution.Coverage.CalendarQualityStatus,
		ExpectedTradingDays:   execution.Coverage.ExpectedTradingDays,
		ObservedBars:          execution.ObservedBars,
		MinimumRequiredBars:   execution.Coverage.MinimumRequiredBars,
		CoverageSufficient:    execution.Coverage.CoverageSufficient,
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return SMA20Output{}, SMA20QualitySummary{}, err
	}
	if err := ValidateSMA20OutputJSON(payload); err != nil {
		return SMA20Output{}, SMA20QualitySummary{}, err
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return SMA20Output{}, SMA20QualitySummary{}, err
	}
	if err := ValidateSMA20QualitySummaryJSON(qualityJSON); err != nil {
		return SMA20Output{}, SMA20QualitySummary{}, err
	}
	return output, quality, nil
}

func ValidateSMA20OutputJSON(payload []byte) error {
	var output SMA20Output
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSMA20StageOutput, err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return ErrInvalidSMA20StageOutput
	}
	if output.SchemaVersion != SMA20SchemaVersion ||
		strings.TrimSpace(output.ResultID) == "" || strings.TrimSpace(output.SnapshotRef) == "" ||
		output.MetricCode != "sma" || output.Unit != "CNY/share" || output.Period != "20d" ||
		output.QualityStatus != "valid" || !validISODate(output.TradeDate) ||
		!validCanonicalUTC(output.SystemAsOf) || output.SampleCount < 1 || output.SampleCount > SMA20Window {
		return ErrInvalidSMA20StageOutput
	}
	switch output.MetricStatus {
	case "available":
		var value string
		if output.ResultStatus != "completed" || output.SampleCount != SMA20Window ||
			json.Unmarshal(output.Value, &value) != nil || !sma20DecimalPattern.MatchString(value) {
			return ErrInvalidSMA20StageOutput
		}
	case "insufficient_data":
		if output.ResultStatus != "insufficient_data" || output.SampleCount >= SMA20Window ||
			!bytes.Equal(bytes.TrimSpace(output.Value), []byte("null")) {
			return ErrInvalidSMA20StageOutput
		}
	default:
		return ErrInvalidSMA20StageOutput
	}
	return nil
}

func ValidateSMA20QualitySummaryJSON(payload []byte) error {
	var summary SMA20QualitySummary
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&summary); err != nil || ensureJSONEOF(decoder) != nil {
		return ErrInvalidSMA20StageOutput
	}
	if summary.SchemaVersion != SMA20QualitySchema || summary.QualityStatus != "valid" ||
		summary.CalendarQualityStatus != "valid" || summary.ExpectedTradingDays <= 0 ||
		summary.ObservedBars <= 0 || summary.ObservedBars > summary.ExpectedTradingDays ||
		summary.MinimumRequiredBars <= 0 || summary.MinimumRequiredBars > summary.ObservedBars ||
		!summary.CoverageSufficient {
		return ErrInvalidSMA20StageOutput
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ErrInvalidSMA20StageOutput
	}
	return nil
}

func validISODate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func validCanonicalUTC(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Location() == time.UTC && strings.HasSuffix(value, "Z") &&
		parsed.UTC().Format(time.RFC3339Nano) == value
}

func sortedUniqueContains(values []string, expected string) bool {
	if len(values) == 0 {
		return false
	}
	found := false
	for index, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) ||
			(index > 0 && values[index-1] >= value) {
			return false
		}
		if value == expected {
			found = true
		}
	}
	return found
}
