package trend

import (
	"encoding/json"
	"errors"
	"regexp"
	"time"

	"example.com/stock-analysis-flow/internal/quant/metric"
)

// Close-only 趋势判定的语义决议与实现边界。
//
// D1–D5（doc/quant/07-trend-following策略差距与契约结论.md 第 6 节，2026-11-26 通过）：
//   - D1 周期集合 5/20/60：ma20_distance 表达中期偏离，momentum_5d/20d/60d 表达短/中/长动量。
//   - D2 direction 纯符号：ma20_distance 与 momentum_20d 同时为正 → bullish，同时为负 →
//     bearish，其余（含 0 与符号不一致）→ sideways；两项任一不可用 → unknown。
//   - D3 phase v1 恒为 unknown：阶段判定涉及 emerging/mature/weakening 的业务阈值，尚无决议。
//   - D4 strength v1 恒为 null：DEC-PLAT-002-049 只规定取值范围，未给归一化公式。
//   - D5 只产出 TrendResult：交易候选（Signal）另立契约，QNT-008 仍 NOT_STARTED。
//
// 因此本包的职责边界是：把四个已精确计算的 Decimal 证据 Metric 映射为趋势状态，
// 不引入任何阈值、权重、多周期冲突判定或强度推导；也不改写证据 Metric 的值。
const (
	// CloseTrendCode 是 close-only 趋势状态的结果标识。
	CloseTrendCode = "close_trend_1.0.0"
	// CloseTrendPeriod 是 direction 的判定周期（MA20 偏离与 20 日动量）。
	CloseTrendPeriod = "20d"
	// CloseTrendAlgorithmVersion 是趋势判定语义的版本；SemVer，与证据 Metric 的
	// calculation_version 分离：后者属于因子算法，前者属于方向判定规则。
	CloseTrendAlgorithmVersion = "1.0.0"
	// CloseTrendMinimumBars 是 D1 周期集合的最大窗口（60 日动量需 61 根）。
	CloseTrendMinimumBars = 61

	// 证据 Metric 的 code 在 trend 包内复述，避免 trend → registry 的包依赖
	// （registry 的测试反向引用 trend 会形成测试期 import cycle）。
	// 一致性由 application 包的 TestTrendEvidenceCodesMatchRegistry 断言。
	EvidenceMADistanceCode = "ma20_distance"
	EvidenceMomentum5Code  = "momentum_5d"
	EvidenceMomentum20Code = "momentum_20d"
	EvidenceMomentum60Code = "momentum_60d"
)

var (
	ErrInvalidAssessmentInput = errors.New("invalid trend assessment input")
	trendDecimalPattern       = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
)

// evidenceSpec 是趋势证据的封闭规格表：顺序、周期、最小样本与是否参与方向判定。
// 顺序固定使 Golden 与 Stage Output 稳定，不依赖调用方的 map 迭代顺序。
type evidenceSpec struct {
	code                string
	period              string
	minimumSampleCount  int
	participatesInTrend bool
}

var closeTrendEvidence = []evidenceSpec{
	{code: EvidenceMADistanceCode, period: "20d", minimumSampleCount: 20, participatesInTrend: true},
	{code: EvidenceMomentum5Code, period: "5d", minimumSampleCount: 6},
	{code: EvidenceMomentum20Code, period: "20d", minimumSampleCount: 21, participatesInTrend: true},
	{code: EvidenceMomentum60Code, period: "60d", minimumSampleCount: 61},
}

// AssessmentInput 是一次趋势判定的输入：已由 Facade 精确计算并校验过的证据 Metric。
// SystemAsOf 必须与每个证据 Metric 的 system_as_of 完全一致（PIT 同一时点）。
type AssessmentInput struct {
	SystemAsOf time.Time
	Evidence   []metric.Metric
}

// Assess 把四个证据 Metric 判定为 TrendResult。它不做 I/O、不读 Snapshot、不落库。
func Assess(input AssessmentInput) (Result, error) {
	if input.SystemAsOf.IsZero() || input.SystemAsOf.Location() != time.UTC ||
		len(input.Evidence) != len(closeTrendEvidence) {
		return Result{}, ErrInvalidAssessmentInput
	}
	systemAsOf := input.SystemAsOf.UTC().Format(time.RFC3339Nano)
	for index, spec := range closeTrendEvidence {
		item := input.Evidence[index]
		if item.MetricCode != spec.code || item.Period != spec.period || item.Unit != "ratio" ||
			item.SystemAsOf != systemAsOf || item.QualityStatus != metric.QualityValid ||
			item.Validate() != nil || item.SampleCount == nil {
			return Result{}, ErrInvalidAssessmentInput
		}
		switch item.MetricStatus {
		case metric.StatusAvailable:
			if *item.SampleCount < spec.minimumSampleCount {
				return Result{}, ErrInvalidAssessmentInput
			}
		case metric.StatusInsufficientData:
			if *item.SampleCount >= spec.minimumSampleCount || !isJSONNull(item.Value) {
				return Result{}, ErrInvalidAssessmentInput
			}
		default:
			return Result{}, ErrInvalidAssessmentInput
		}
	}

	direction, err := assessDirection(input.Evidence)
	if err != nil {
		return Result{}, err
	}
	result := Result{
		TrendCode:        CloseTrendCode,
		Direction:        direction,
		Phase:            PhaseUnknown,
		Strength:         nil,
		Period:           CloseTrendPeriod,
		EvidenceMetrics:  cloneMetrics(input.Evidence),
		ConflictRefs:     []string{},
		QualityStatus:    assessQuality(input.Evidence, direction),
		SystemAsOf:       systemAsOf,
		AlgorithmVersion: CloseTrendAlgorithmVersion,
	}
	if err := result.Validate(); err != nil {
		return Result{}, err
	}
	return result, nil
}

// assessDirection 只对参与方向判定的两项取符号，其余证据不影响方向（D2/D5）。
func assessDirection(evidence []metric.Metric) (Direction, error) {
	var maDistance, momentum20 metric.Metric
	for index, spec := range closeTrendEvidence {
		if !spec.participatesInTrend {
			continue
		}
		switch spec.code {
		case EvidenceMADistanceCode:
			maDistance = evidence[index]
		case EvidenceMomentum20Code:
			momentum20 = evidence[index]
		}
	}
	if maDistance.MetricStatus != metric.StatusAvailable || momentum20.MetricStatus != metric.StatusAvailable {
		// 不可评估必须是显式 unknown，而不是 silent 的 sideways。
		return DirectionUnknown, nil
	}
	maValue, err := decimalString(maDistance.Value)
	if err != nil {
		return "", err
	}
	momentumValue, err := decimalString(momentum20.Value)
	if err != nil {
		return "", err
	}
	return DirectionOf(maValue, momentumValue)
}

// DirectionOf 是 D2 的纯符号判定：无阈值、无加权、无幅度比较。
// 任一输入为零都视为符号不一致，因为"零"既不构成正向也不构成负向证据。
func DirectionOf(movingAverageDistance, momentum20 string) (Direction, error) {
	maSign, err := decimalSign(movingAverageDistance)
	if err != nil {
		return "", err
	}
	momentumSign, err := decimalSign(momentum20)
	if err != nil {
		return "", err
	}
	switch {
	case maSign > 0 && momentumSign > 0:
		return DirectionBullish, nil
	case maSign < 0 && momentumSign < 0:
		return DirectionBearish, nil
	default:
		return DirectionSideways, nil
	}
}

// assessQuality 是趋势质量状态的封闭映射：
//   - 参与方向判定的证据缺失 → unknown（不可评估，不得伪装成 sideways 的 degraded）；
//   - 全部证据可用 → valid；
//   - 方向可判定但 5d/60d 证据不足 → degraded。
func assessQuality(evidence []metric.Metric, direction Direction) QualityStatus {
	if direction == DirectionUnknown {
		return QualityUnknown
	}
	for _, item := range evidence {
		if item.MetricStatus != metric.StatusAvailable {
			return QualityDegraded
		}
	}
	return QualityValid
}

func decimalString(value json.RawMessage) (string, error) {
	var raw string
	if err := json.Unmarshal(value, &raw); err != nil || !trendDecimalPattern.MatchString(raw) {
		return "", ErrInvalidAssessmentInput
	}
	return raw, nil
}

// decimalSign 只判断符号，不解析数值大小：避免在 Float/Int 转换中引入与精度策略冲突的舍入。
func decimalSign(raw string) (int, error) {
	if !trendDecimalPattern.MatchString(raw) {
		return 0, ErrInvalidAssessmentInput
	}
	digits := raw
	negative := false
	if digits[0] == '-' {
		negative = true
		digits = digits[1:]
	}
	nonZero := false
	for index := 0; index < len(digits); index++ {
		if digits[index] >= '1' && digits[index] <= '9' {
			nonZero = true
			break
		}
	}
	if !nonZero {
		return 0, nil
	}
	if negative {
		return -1, nil
	}
	return 1, nil
}

func isJSONNull(value json.RawMessage) bool {
	return string(trimJSONSpace(value)) == "null"
}

func trimJSONSpace(value []byte) []byte {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}

func cloneMetrics(source []metric.Metric) []metric.Metric {
	cloned := make([]metric.Metric, len(source))
	for index, item := range source {
		cloned[index] = item
		cloned[index].Value = append(json.RawMessage(nil), item.Value...)
		cloned[index].LowerBound = append(json.RawMessage(nil), item.LowerBound...)
		cloned[index].UpperBound = append(json.RawMessage(nil), item.UpperBound...)
		if item.SampleCount != nil {
			sampleCount := *item.SampleCount
			cloned[index].SampleCount = &sampleCount
		}
	}
	return cloned
}
