package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

// 执行清单 1.5 的 Stage 层验收：Application 已经算出的唯一原因必须原样投影到
// Stage reason code，不允许再退化成 `*_execution_failed` 这种无法区分原因的通配码。

func wrappedInputInapplicable(reasonCode string) error {
	// 复刻真实调用链：Facade 把适用性失败包进自己的质量错误，Adapter 再包一层。
	facade := fmt.Errorf("%w: %w", quantapplication.ErrTrendSnapshotQuality, &quantapplication.InputNotApplicableError{ReasonCode: reasonCode})
	return fmt.Errorf("produce authorized Trend Quant Result: %w", facade)
}

func TestQuantInputInapplicableReasonCodeMapping(t *testing.T) {
	cases := []struct {
		reason string
		want   string
	}{
		{quantapplication.ReasonInputPITUnavailable, "stage_input_pit_unavailable"},
		{quantapplication.ReasonInputQualityInvalid, "stage_input_quality_invalid"},
		{quantapplication.ReasonInputCalendarInvalid, "stage_input_calendar_quality_invalid"},
		{quantapplication.ReasonInputCoverageInsufficient, "stage_input_coverage_insufficient"},
		{quantapplication.ReasonInputBarsBelowMinimum, "stage_input_bars_below_minimum"},
		{quantapplication.ReasonInputCoverageBelowPolicy, "stage_input_coverage_below_policy"},
		{quantapplication.ReasonInputBarQualityInvalid, "stage_input_bar_quality_invalid"},
	}
	seen := make(map[string]string, len(cases))
	for _, testCase := range cases {
		code, ok := quantInputInapplicableReasonCode("stage_", wrappedInputInapplicable(testCase.reason))
		if !ok || code != testCase.want {
			t.Fatalf("reason %q mapped to (%q, %v), want (%q, true)", testCase.reason, code, ok, testCase.want)
		}
		if previous, duplicated := seen[code]; duplicated {
			t.Fatalf("reason %q and %q map to the same Stage code %q", previous, testCase.reason, code)
		}
		seen[code] = testCase.reason
	}

	// 未登记的新原因必须保留"不可用"事实但不伪造具体原因；
	// 这条断言同时提醒：新增原因常量必须回到上面的表里补映射。
	if code, ok := quantInputInapplicableReasonCode("stage_", wrappedInputInapplicable("input_brand_new_reason")); !ok || code != "stage_input_not_applicable" {
		t.Fatalf("unknown reason mapped to (%q, %v), want generic fallback", code, ok)
	}

	// 非适用性失败不得被映射，调用方应继续走原有分支。
	if code, ok := quantInputInapplicableReasonCode("stage_", errors.New("boom")); ok || code != "" {
		t.Fatalf("non-applicability error mapped to (%q, %v), want no mapping", code, ok)
	}
	if code, ok := quantInputInapplicableReasonCode("stage_", nil); ok || code != "" {
		t.Fatalf("nil error mapped to (%q, %v), want no mapping", code, ok)
	}
	// 只有 Facade 的质量错误、没有原因码时同样不得猜测原因。
	if code, ok := quantInputInapplicableReasonCode("stage_", quantapplication.ErrTrendSnapshotQuality); ok || code != "" {
		t.Fatalf("reasonless quality error mapped to (%q, %v), want no mapping", code, ok)
	}
}

func TestQuantTrendStageExecutorMapsInputInapplicableReason(t *testing.T) {
	adapter := validQuantTrendStageAdapterStub(t)
	adapter.err = wrappedInputInapplicable(quantapplication.ReasonInputCoverageBelowPolicy)
	result, err := (quantTrendStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.TrendStageCode},
			Parameters:     validQuantTrendStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.AttemptStatus != researchflowdomain.AttemptFailed || result.StageStatus != researchflowdomain.StageFailed ||
		result.ProviderResult != nil {
		t.Fatalf("inapplicable input must fail without an output: %+v", result)
	}
	if len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "quant_trend_input_coverage_below_policy" {
		t.Fatalf("ReasonCodes = %v, want the exact applicability reason", result.ReasonCodes)
	}
}

func TestQuantFactorStageExecutorMapsInputInapplicableReason(t *testing.T) {
	adapter := validQuantFactorStageAdapterStub()
	adapter.err = wrappedInputInapplicable(quantapplication.ReasonInputPITUnavailable)
	result, err := (quantFactorStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.FactorStageCode},
			Parameters:     validQuantFactorStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "quant_factor_input_pit_unavailable" {
		t.Fatalf("ReasonCodes = %v, want quant_factor_input_pit_unavailable", result.ReasonCodes)
	}
}

func TestQuantSMA20StageExecutorMapsInputInapplicableReason(t *testing.T) {
	adapter := &quantSMA20StageAdapterStub{
		err: wrappedInputInapplicable(quantapplication.ReasonInputBarsBelowMinimum),
	}
	result, err := (quantSMA20StageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.SMA20StageCode},
			Parameters:     json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "quant_sma20_input_bars_below_minimum" {
		t.Fatalf("ReasonCodes = %v, want quant_sma20_input_bars_below_minimum", result.ReasonCodes)
	}
}
