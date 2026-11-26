package stage

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
)

// Factor Stage Output 的强类型 DTO 与运行时 Validator。
//
// 这里的 Schema 属于 QNT-CTR-003（deferred）的 L2 机器契约，只约束首批
// Close-only Decimal 因子；不升级契约状态，也不代表 Factor 已接入生产 Stage。
// result_id 必须由 QNT-CTR-005 Quant Result Manifest 提供，Stage 侧不得推导。
const (
	FactorOutputType           = "quant.factor_result"
	FactorSchemaVersion        = "1.0.0"
	FactorQualitySchemaVersion = "1.0.0"
)

var ErrInvalidFactorStageOutput = errors.New("invalid Quant Factor stage output")

var (
	factorStageResultIDPattern   = regexp.MustCompile(`^quant_result:sha256:[0-9a-f]{64}$`)
	factorStageDecimalPattern    = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
	factorStageCalculationRE     = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	factorStageMinimumBarsByCode = map[string]int{
		"ma20_distance": 20,
		"momentum_5d":   6,
		"momentum_20d":  21,
	}
	factorStagePeriodByCode = map[string]string{
		"ma20_distance": "20d",
		"momentum_5d":   "5d",
		"momentum_20d":  "20d",
	}
)

// FactorOutput 是首批 Decimal 因子的 Researchflow Stage payload。
// Value 使用 json.RawMessage 是为了严格区分十进制字符串与 JSON null。
type FactorOutput struct {
	SchemaVersion      string          `json:"schema_version"`
	ResultID           string          `json:"result_id"`
	SnapshotRef        string          `json:"snapshot_ref"`
	MetricCode         string          `json:"metric_code"`
	Value              json.RawMessage `json:"value"`
	Unit               string          `json:"unit"`
	Period             string          `json:"period"`
	MetricStatus       string          `json:"metric_status"`
	SampleCount        int64           `json:"sample_count"`
	TradeDate          string          `json:"trade_date"`
	SystemAsOf         string          `json:"system_as_of"`
	QualityStatus      string          `json:"quality_status"`
	ResultStatus       string          `json:"result_status"`
	CalculationVersion string          `json:"calculation_version"`
}

// FactorQualitySummary 是首批 Decimal 因子的 Researchflow Stage quality_summary。
// minimum_required_bars 来自 Data CoverageEvidence，不是因子算法窗口；算法有效样本数由
// FactorOutput.sample_count 表达。observed_bars 不得大于 expected_trading_days。
type FactorQualitySummary struct {
	SchemaVersion         string `json:"schema_version"`
	MetricCode            string `json:"metric_code"`
	QualityStatus         string `json:"quality_status"`
	CalendarQualityStatus string `json:"calendar_quality_status"`
	ExpectedTradingDays   int    `json:"expected_trading_days"`
	ObservedBars          int    `json:"observed_bars"`
	MinimumRequiredBars   int    `json:"minimum_required_bars"`
	CoverageSufficient    bool   `json:"coverage_sufficient"`
}

func ValidateFactorStageOutputJSON(payload []byte) error {
	var output FactorOutput
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil || ensureJSONEOF(decoder) != nil {
		return ErrInvalidFactorStageOutput
	}
	minimumBars, knownCode := factorStageMinimumBarsByCode[output.MetricCode]
	period, _ := factorStagePeriodByCode[output.MetricCode]
	if !knownCode || output.SchemaVersion != FactorSchemaVersion ||
		!factorStageResultIDPattern.MatchString(output.ResultID) ||
		!factorStageOpaqueRef(output.SnapshotRef) || output.Unit != "ratio" ||
		output.Period != period || output.QualityStatus != "valid" ||
		!factorStageCalculationRE.MatchString(output.CalculationVersion) ||
		!validISODate(output.TradeDate) || !validCanonicalUTC(output.SystemAsOf) {
		return ErrInvalidFactorStageOutput
	}
	switch output.MetricStatus {
	case "available":
		var value string
		if output.ResultStatus != "completed" ||
			output.SampleCount != int64(minimumBars) ||
			json.Unmarshal(output.Value, &value) != nil ||
			!factorStageDecimalPattern.MatchString(value) {
			return ErrInvalidFactorStageOutput
		}
	case "insufficient_data":
		if output.ResultStatus != "insufficient_data" ||
			output.SampleCount < 1 || output.SampleCount >= int64(minimumBars) ||
			!bytes.Equal(bytes.TrimSpace(output.Value), []byte("null")) {
			return ErrInvalidFactorStageOutput
		}
	default:
		return ErrInvalidFactorStageOutput
	}
	return nil
}

func ValidateFactorQualitySummaryJSON(payload []byte) error {
	var summary FactorQualitySummary
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&summary); err != nil || ensureJSONEOF(decoder) != nil {
		return ErrInvalidFactorStageOutput
	}
	_, knownCode := factorStageMinimumBarsByCode[summary.MetricCode]
	if !knownCode || summary.SchemaVersion != FactorQualitySchemaVersion ||
		summary.QualityStatus != "valid" || summary.CalendarQualityStatus != "valid" ||
		summary.ExpectedTradingDays <= 0 || summary.ObservedBars <= 0 ||
		summary.MinimumRequiredBars <= 0 || summary.MinimumRequiredBars > summary.ObservedBars ||
		summary.ObservedBars > summary.ExpectedTradingDays || !summary.CoverageSufficient {
		return ErrInvalidFactorStageOutput
	}
	return nil
}

func factorStageOpaqueRef(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value
}
