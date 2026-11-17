package indicator

import (
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
)

// validateCalculationContext 统一校验价格序列指标的时间、快照、复权和质量策略边界。
func validateCalculationContext(ctx domain.CalculationContext, trustedNow time.Time) error {
	return ctx.Validate(trustedNow, true)
}

// SMAWithContext 在计算 SMA 前校验共享计算上下文。
func SMAWithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64, p SMAParams) (Result, error) {
	if err := validateCalculationContext(ctx, trustedNow); err != nil {
		return Result{}, err
	}
	return SMA(closes, p)
}

// EMAWithContext 在计算 EMA 前校验共享计算上下文。
func EMAWithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64, p EMAParams) (Result, error) {
	if err := validateCalculationContext(ctx, trustedNow); err != nil {
		return Result{}, err
	}
	return EMA(closes, p)
}

// ReturnWithContext 在计算收益率前校验共享计算上下文。
func ReturnWithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64, p ReturnParams) (Result, error) {
	if err := validateCalculationContext(ctx, trustedNow); err != nil {
		return Result{}, err
	}
	return Return(closes, p)
}

// VolatilityWithContext 在计算波动率前校验共享计算上下文。
func VolatilityWithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64, p VolatilityParams) (Result, error) {
	if err := validateCalculationContext(ctx, trustedNow); err != nil {
		return Result{}, err
	}
	return Volatility(closes, p)
}

// RSIWithContext 在计算 RSI 前校验共享计算上下文。
func RSIWithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64, p RSIParams) (Result, error) {
	if err := validateCalculationContext(ctx, trustedNow); err != nil {
		return Result{}, err
	}
	return RSI(closes, p)
}

// MACDWithContext 在计算 MACD 前校验共享计算上下文。
func MACDWithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64, p MACDParams) (MACDResult, error) {
	if err := validateCalculationContext(ctx, trustedNow); err != nil {
		return MACDResult{}, err
	}
	return MACD(closes, p)
}

// ATRWithContext 在计算 ATR 前校验共享计算上下文；previousClose 必须来自同一复权口径的前一交易日。
func ATRWithContext(ctx domain.CalculationContext, trustedNow time.Time, highs, lows, closes []float64, previousClose float64, p ATRParams) (Result, error) {
	if err := validateCalculationContext(ctx, trustedNow); err != nil {
		return Result{}, err
	}
	return ATR(highs, lows, closes, previousClose, p)
}

// BollingerWithContext 在计算布林带前校验共享计算上下文。
func BollingerWithContext(ctx domain.CalculationContext, trustedNow time.Time, closes []float64, p BollingerParams) (BandsResult, error) {
	if err := validateCalculationContext(ctx, trustedNow); err != nil {
		return BandsResult{}, err
	}
	return Bollinger(closes, p)
}
