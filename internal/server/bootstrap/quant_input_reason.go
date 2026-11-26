package bootstrap

import (
	"errors"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
)

// Quant Stage 对"输入存在但不适用于本用例"的 reason code 映射（执行清单 1.5）。
//
// 背景：Stage 层原先只能看到 Facade 的粗粒度错误（`ErrXxxSnapshotQuality`），
// 因此质量门禁失败与真正的执行故障共用 `*_execution_failed`，运维无法区分
// "数据没准备好"与"代码出错"。这里把 Application 层已经算出的唯一原因
// 投影为 Stage 级封闭 code：前缀区分 Stage，后缀沿用 Application 的原因名。
//
// 边界：本函数只做投影，不重新判定质量；Facade 没有给出原因时一律回退到
// 通用 code，绝不猜测具体原因。

// quantInputInapplicableReasonCode 返回 Stage 级 reason code。
// 第二个返回值为 false 表示错误链中没有适用性失败，调用方应继续走原有分支。
func quantInputInapplicableReasonCode(stagePrefix string, err error) (string, bool) {
	var applicability *quantapplication.InputNotApplicableError
	if !errors.As(err, &applicability) {
		return "", false
	}
	switch applicability.ReasonCode {
	case quantapplication.ReasonInputPITUnavailable:
		return stagePrefix + "input_pit_unavailable", true
	case quantapplication.ReasonInputQualityInvalid:
		return stagePrefix + "input_quality_invalid", true
	case quantapplication.ReasonInputCalendarInvalid:
		return stagePrefix + "input_calendar_quality_invalid", true
	case quantapplication.ReasonInputCoverageInsufficient:
		return stagePrefix + "input_coverage_insufficient", true
	case quantapplication.ReasonInputBarsBelowMinimum:
		return stagePrefix + "input_bars_below_minimum", true
	case quantapplication.ReasonInputCoverageBelowPolicy:
		return stagePrefix + "input_coverage_below_policy", true
	case quantapplication.ReasonInputBarQualityInvalid:
		return stagePrefix + "input_bar_quality_invalid", true
	default:
		// 新增原因未在此登记时仍要保留"不可用"的事实，但不得伪造具体原因。
		return stagePrefix + "input_not_applicable", true
	}
}
