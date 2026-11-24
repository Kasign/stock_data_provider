package indicator

import (
	"errors"
	"math/big"
	"regexp"
	"strings"
)

var (
	ErrNonTerminatingDecimal = errors.New("decimal SMA result has no finite decimal representation")
	decimalClosePattern      = regexp.MustCompile("^-?(?:0|[1-9][0-9]*)(?:\\.[0-9]+)?$")
)

const (
	maxDecimalCloseBytes = 128
	maxDecimalSMASamples = 10000
)

// DecimalResult 保留精确十进制结果，不经过 float64 转换。
type DecimalResult struct {
	Value       string
	Status      Status
	SampleCount int
}

// DecimalSMA 使用精确有理数计算最近 Window 个收盘价的算术平均值。
// 输入必须是 Data Snapshot 中的十进制字符串；不接受 float、指数形式或非正价格。
// 若平均值是无限循环小数，返回 ErrNonTerminatingDecimal，不执行隐式舍入。
func DecimalSMA(closes []string, p SMAParams) (DecimalResult, error) {
	if !validWindow(p.Window, 1) {
		return DecimalResult{}, ErrInvalidParameter
	}
	if len(closes) > maxDecimalSMASamples {
		return DecimalResult{}, ErrInvalidInput
	}

	parsed := make([]*big.Rat, len(closes))
	for index, raw := range closes {
		if len(raw) == 0 || len(raw) > maxDecimalCloseBytes || !decimalClosePattern.MatchString(raw) {
			return DecimalResult{}, ErrInvalidInput
		}
		value, ok := new(big.Rat).SetString(raw)
		if !ok || value.Sign() <= 0 {
			return DecimalResult{}, ErrInvalidInput
		}
		parsed[index] = value
	}
	if len(parsed) < p.Window {
		return DecimalResult{Status: StatusInsufficientData, SampleCount: len(parsed)}, nil
	}

	sum := new(big.Rat)
	for _, value := range parsed[len(parsed)-p.Window:] {
		sum.Add(sum, value)
	}
	average := new(big.Rat).Quo(sum, big.NewRat(int64(p.Window), 1))
	value, err := finiteDecimalString(average)
	if err != nil {
		return DecimalResult{}, err
	}
	return DecimalResult{Value: value, Status: StatusAvailable, SampleCount: p.Window}, nil
}

// finiteDecimalString converts a rational to its exact finite decimal representation.
// A denominator containing primes other than 2 or 5 is rejected rather than rounded.
func finiteDecimalString(value *big.Rat) (string, error) {
	if value == nil {
		return "", ErrInvalidInput
	}
	denominator := new(big.Int).Set(value.Denom())
	two, five := 0, 0
	zero := big.NewInt(0)
	twoFactor, fiveFactor := big.NewInt(2), big.NewInt(5)
	remainder := new(big.Int)
	for {
		remainder.Mod(denominator, twoFactor)
		if remainder.Cmp(zero) != 0 {
			break
		}
		denominator.Div(denominator, twoFactor)
		two++
	}
	for {
		remainder.Mod(denominator, fiveFactor)
		if remainder.Cmp(zero) != 0 {
			break
		}
		denominator.Div(denominator, fiveFactor)
		five++
	}
	if denominator.Cmp(big.NewInt(1)) != 0 {
		return "", ErrNonTerminatingDecimal
	}

	scale := two
	if five > scale {
		scale = five
	}
	multiplier := big.NewInt(1)
	if scale > two {
		multiplier.Mul(multiplier, new(big.Int).Exp(twoFactor, big.NewInt(int64(scale-two)), nil))
	}
	if scale > five {
		multiplier.Mul(multiplier, new(big.Int).Exp(fiveFactor, big.NewInt(int64(scale-five)), nil))
	}
	scaled := new(big.Int).Mul(value.Num(), multiplier)
	digits := scaled.String()
	if scale == 0 {
		return digits, nil
	}

	negative := strings.HasPrefix(digits, "-")
	if negative {
		digits = strings.TrimPrefix(digits, "-")
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
		return sign + integerPart, nil
	}
	return sign + integerPart + "." + fractionPart, nil
}
