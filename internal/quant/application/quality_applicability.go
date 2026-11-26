package application

import (
	"errors"
	"fmt"
)

// 输入质量事实 → Quant 适用性的正式映射（执行清单 1.5）。
//
// 背景：旧 Runner 用一个全局 `qualityAssessor.corePass`（"至少一只自选股有 ≥21 根
// 有效日 K"）决定 StrategyEligible，质量与适用性混在同一个全局开关里；新路径则
// 由 Data 与 Quant 各自提供事实与判定。本文件把 Quant 侧的判定收敛为一张封闭表：
// 输入只有 Data 已记录的 Snapshot 质量事实，输出只有"可计算 / 不可计算 + 唯一
// reason code"，不掺入算法窗口、策略偏好或综合评分。
//
// 与算法样本性的分界：本表只判断"这份输入是否可用于本用例"，不判断"算出来的值
// 是否可用"。后者由各用例的 Metric 状态（available/insufficient_data）表达。

// 不可计算原因的封闭取值。新增原因必须同时更新此处、表格与测试。
const (
	ReasonInputPITUnavailable       = "input_pit_unavailable"
	ReasonInputQualityInvalid       = "input_quality_invalid"
	ReasonInputCalendarInvalid      = "input_calendar_quality_invalid"
	ReasonInputCoverageInsufficient = "input_coverage_insufficient"
	ReasonInputBarsBelowMinimum     = "input_bars_below_minimum"
	ReasonInputCoverageBelowPolicy  = "input_coverage_below_policy"
	ReasonInputBarQualityInvalid    = "input_bar_quality_invalid"
)

// InputQualityFacts 是 Data Snapshot 已经记录的、用于适用性判定的全部事实。
// 其中 CoverageAtOrAbovePolicy 由调用方用精确有理数比较后给出，避免在映射表里
// 引入浮点比较；PITEvidenceAvailable 表达 system_as_of 是否可信且与请求一致。
type InputQualityFacts struct {
	PITEvidenceAvailable    bool
	SnapshotQuality         string
	CalendarQuality         string
	CoverageSufficient      bool
	ObservedBars            int
	MinimumRequiredBars     int
	CoverageAtOrAbovePolicy bool
	BarQualities            []string
}

// InputNotApplicableError 表达"输入存在但不允许用于本用例"，并携带唯一原因。
// 它 unwrap 到 ErrCloseSeriesSnapshotQuality，保持既有错误分类不变，同时让 Stage
// 层能给出精确 reason code，而不是把质量门禁失败笼统记为 execution_failed。
type InputNotApplicableError struct {
	ReasonCode string
}

func (e *InputNotApplicableError) Error() string {
	return "Quant input snapshot is not applicable: " + e.ReasonCode
}

func (e *InputNotApplicableError) Unwrap() error {
	return ErrCloseSeriesSnapshotQuality
}

// EvaluateInputApplicability 按固定顺序返回第一个不满足的条件。
// 顺序即优先级：PIT 缺失优先于质量状态，质量状态优先于覆盖率，避免同一份坏输入
// 因检查顺序不同而给出不同原因。
func EvaluateInputApplicability(facts InputQualityFacts) (bool, string) {
	switch {
	case !facts.PITEvidenceAvailable:
		return false, ReasonInputPITUnavailable
	case facts.SnapshotQuality != "valid":
		return false, ReasonInputQualityInvalid
	case facts.CalendarQuality != "valid":
		return false, ReasonInputCalendarInvalid
	case !facts.CoverageSufficient:
		return false, ReasonInputCoverageInsufficient
	case facts.ObservedBars < 1 || facts.MinimumRequiredBars < 1:
		return false, ReasonInputBarsBelowMinimum
	case facts.ObservedBars < facts.MinimumRequiredBars:
		return false, ReasonInputBarsBelowMinimum
	case !facts.CoverageAtOrAbovePolicy:
		return false, ReasonInputCoverageBelowPolicy
	}
	for _, quality := range facts.BarQualities {
		if quality != "valid" {
			return false, ReasonInputBarQualityInvalid
		}
	}
	return true, ""
}

// classifyCloseSeriesValidationError 是三个 Compute Facade 共用的错误分类：
// 保留各自既有的错误类型（SMA/Factor/Trend 语义不变），同时把适用性失败的
// reason code 挂在错误链上，避免 Facade 与 Stage 层各自解释一份质量判定。
//
// 不变量：返回值永远可 errors.Is 到 qualityError 或 invalidError 之一；
// 当输入属于"存在但不适用"时，错误链中必须仍可 errors.As 出 *InputNotApplicableError。
func classifyCloseSeriesValidationError(err, qualityError, invalidError error) error {
	if err == nil {
		return nil
	}
	var applicability *InputNotApplicableError
	if errors.As(err, &applicability) {
		return fmt.Errorf("%w: %w", qualityError, applicability)
	}
	if errors.Is(err, ErrCloseSeriesSnapshotQuality) {
		return qualityError
	}
	return invalidError
}
