// Package factor 首批因子的精确十进制计算层。
//
// 与 indicator.DecimalSMA 不同，比率因子（MA20 距离、动量）是两个十进制数相除，
// 结果通常是无限循环小数，无法用有限十进制精确表示。因此本层用 big.Rat 完成
// 全部中间运算（无 float64 误差累积），再按固定 scale 与 half-even 舍入输出
// 十进制字符串。精度策略见 doc/quant/Factor精确计算精度策略决策.md。
package factor

import (
	"math/big"
	"regexp"
	"strings"
)

const (
	// DecimalFactorScale 是比率因子的固定输出小数位。
	DecimalFactorScale = 12

	maxDecimalFactorCloseBytes = 128
	maxDecimalFactorSamples    = 10000

	decimalFactorMADistanceCode = "ma20_distance"
	decimalFactorMomentum5Code  = "momentum_5d"
	decimalFactorMomentum20Code = "momentum_20d"
)

var (
	decimalFactorClosePattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
	decimalFactorOne          = big.NewRat(1, 1)
)

// DecimalResult 保留精确十进制因子结果，Value 是固定 scale 的十进制字符串，
// 不经 float64 转换。StatusInsufficientData 时 Value 为空串。
type DecimalResult struct {
	Code        string
	Value       string
	Status      Status
	SampleCount int
}

// DecimalMADistance20 以精确有理数计算最新收盘价相对最近 20 根收盘价均值的距离。
// 输入必须是 Data Snapshot 的十进制字符串；结果为 ratio（如 "0.05" 表示高于均值 5%）。
func DecimalMADistance20(closes []string) (DecimalResult, error) {
	const window = 20
	parsed, err := parseDecimalFactorCloses(closes)
	if err != nil {
		return DecimalResult{}, err
	}
	if len(parsed) < window {
		return DecimalResult{Code: decimalFactorMADistanceCode, Status: StatusInsufficientData, SampleCount: len(parsed)}, nil
	}
	sum := new(big.Rat)
	for _, value := range parsed[len(parsed)-window:] {
		sum.Add(sum, value)
	}
	average := new(big.Rat).Quo(sum, big.NewRat(int64(window), 1))
	last := parsed[len(parsed)-1]
	ratio := new(big.Rat).Quo(last, average)
	value := new(big.Rat).Sub(ratio, decimalFactorOne)
	return DecimalResult{
		Code:        decimalFactorMADistanceCode,
		Value:       roundRatHalfEvenToScale(value, DecimalFactorScale),
		Status:      StatusAvailable,
		SampleCount: window,
	}, nil
}

// DecimalMomentum5 以精确有理数计算 5 日动量（最新收盘价 / 6 根前收盘价 - 1）。
func DecimalMomentum5(closes []string) (DecimalResult, error) {
	return decimalMomentum(decimalFactorMomentum5Code, closes, 5)
}

// DecimalMomentum20 以精确有理数计算 20 日动量（最新收盘价 / 21 根前收盘价 - 1）。
func DecimalMomentum20(closes []string) (DecimalResult, error) {
	return decimalMomentum(decimalFactorMomentum20Code, closes, 20)
}

func decimalMomentum(code string, closes []string, window int) (DecimalResult, error) {
	if window < 1 {
		return DecimalResult{}, ErrInvalidWindow
	}
	parsed, err := parseDecimalFactorCloses(closes)
	if err != nil {
		return DecimalResult{}, err
	}
	if len(parsed) < window+1 {
		return DecimalResult{Code: code, Status: StatusInsufficientData, SampleCount: len(parsed)}, nil
	}
	base := parsed[len(parsed)-window-1]
	last := parsed[len(parsed)-1]
	value := new(big.Rat).Sub(new(big.Rat).Quo(last, base), decimalFactorOne)
	return DecimalResult{
		Code:        code,
		Value:       roundRatHalfEvenToScale(value, DecimalFactorScale),
		Status:      StatusAvailable,
		SampleCount: window + 1,
	}, nil
}

func parseDecimalFactorCloses(closes []string) ([]*big.Rat, error) {
	if len(closes) > maxDecimalFactorSamples {
		return nil, ErrInvalidInput
	}
	parsed := make([]*big.Rat, len(closes))
	for index, raw := range closes {
		if len(raw) == 0 || len(raw) > maxDecimalFactorCloseBytes || !decimalFactorClosePattern.MatchString(raw) {
			return nil, ErrInvalidInput
		}
		value, ok := new(big.Rat).SetString(raw)
		if !ok || value.Sign() <= 0 {
			return nil, ErrInvalidInput
		}
		parsed[index] = value
	}
	return parsed, nil
}

// roundRatHalfEvenToScale 把有理数按 half-even（银行家舍入）舍入到固定小数位，
// 返回不带尾部零的十进制字符串。它不是精确表示，舍入本身是本层声明的精度策略。
func roundRatHalfEvenToScale(value *big.Rat, scale int) string {
	if scale < 0 {
		scale = 0
	}
	negative := value.Sign() < 0
	absolute := new(big.Rat).Abs(value)
	multiplier := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
	scaled := new(big.Rat).Mul(absolute, new(big.Rat).SetInt(multiplier))

	quotient, remainder := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	twiceRemainder := new(big.Int).Mul(remainder, big.NewInt(2))
	comparison := twiceRemainder.Cmp(scaled.Denom())
	if comparison > 0 || (comparison == 0 && quotient.Bit(0) == 1) {
		quotient.Add(quotient, big.NewInt(1))
	}
	if quotient.Sign() == 0 {
		return "0"
	}

	digits := quotient.String()
	if scale == 0 {
		if negative {
			return "-" + digits
		}
		return digits
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale-len(digits)+1) + digits
	}
	integerPart := digits[:len(digits)-scale]
	fractionPart := strings.TrimRight(digits[len(digits)-scale:], "0")
	sign := ""
	if negative {
		sign = "-"
	}
	if fractionPart == "" {
		return sign + integerPart
	}
	return sign + integerPart + "." + fractionPart
}
