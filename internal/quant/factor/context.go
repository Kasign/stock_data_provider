// Package factor 实现首批单证券时间序列因子；不承担横截面排序或中性化。
package factor

import (
	"time"

	"github.com/Kasign/stock_data_provider/internal/quant/domain"
)

// Momentum20WithContext 在执行 20 日动量前校验计算上下文和价格口径。
// 交易日连续性仍需由 Data Adapter 按其权威交易日历验证。
func Momentum20WithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64) (Result, error) {
	if err := ctx.Validate(trustedNow, true); err != nil {
		return Result{}, err
	}
	return Momentum20(closes)
}

// Momentum60WithContext 在执行 60 日动量前校验计算上下文和价格口径。
// 交易日连续性仍需由 Data Adapter 按其权威交易日历验证。
func Momentum60WithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64) (Result, error) {
	if err := ctx.Validate(trustedNow, true); err != nil {
		return Result{}, err
	}
	return Momentum60(closes)
}

// Volatility20WithContext 在执行 20 日波动率前校验计算上下文和价格口径。
// 交易日连续性仍需由 Data Adapter 按其权威交易日历验证。
func Volatility20WithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64) (Result, error) {
	if err := ctx.Validate(trustedNow, true); err != nil {
		return Result{}, err
	}
	return Volatility20(closes)
}
