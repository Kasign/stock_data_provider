package factor

import (
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
)

var ErrQualityPolicyRejected = errors.New("factor result rejected by calculation quality policy")

// NewQuantMetricWithContext 将上下文门禁、质量策略和纯计算结果转换串联起来。
// strict 策略不允许把 degraded 输入包装成可用指标；allow_degraded 显式允许保留该质量状态。
func NewQuantMetricWithContext(
	ctx domain.CalculationContext,
	trustedNow time.Time,
	result Result,
	quality QualityStatus,
	calculationVersion string,
) (QuantMetric, error) {
	if err := ctx.Validate(trustedNow, true); err != nil {
		return QuantMetric{}, err
	}
	if quality == QualityDegraded && ctx.QualityPolicy == domain.QualityPolicyStrict {
		return QuantMetric{}, ErrQualityPolicyRejected
	}
	return NewQuantMetric(result, ctx.SystemAsOf, quality, calculationVersion)
}
