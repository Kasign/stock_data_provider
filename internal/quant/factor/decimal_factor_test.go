package factor

import (
	"errors"
	"math/big"
	"strconv"
	"strings"
	"testing"
)

func TestDecimalMADistance20Exact(t *testing.T) {
	closes := make([]string, 20)
	for i := range closes {
		closes[i] = strconv.Itoa(i + 1)
	}
	got, err := DecimalMADistance20(closes)
	if err != nil {
		t.Fatal(err)
	}
	if got.Code != "ma20_distance" || got.Status != StatusAvailable || got.SampleCount != 20 {
		t.Fatalf("unexpected result metadata: %+v", got)
	}
	// 19/20 均值场景：last=20, avg=(1+..+20)/20=10.5 → 20/10.5-1=19/21=0.904761904761...
	if got.Value != "0.904761904762" {
		t.Fatalf("value = %s, want 0.904761904762 (half-even at scale 12)", got.Value)
	}
}

func TestDecimalMomentumExact(t *testing.T) {
	cases := []struct {
		name   string
		fn     func([]string) (DecimalResult, error)
		code   string
		closes []string
		want   string
		count  int
	}{
		{
			name:   "momentum5",
			fn:     DecimalMomentum5,
			code:   "momentum_5d",
			closes: []string{"95", "96", "97", "98", "99", "100"},
			want:   "0.052631578947", // 100/95-1 = 5/95 = 1/19 = 0.052631578947...
			count:  6,
		},
		{
			name:   "momentum20",
			fn:     DecimalMomentum20,
			code:   "momentum_20d",
			closes: sequentialCloses(100, 120),
			want:   "0.2", // 120/100-1 = 0.2 exactly
			count:  21,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn(tc.closes)
			if err != nil {
				t.Fatal(err)
			}
			if got.Code != tc.code || got.Status != StatusAvailable || got.SampleCount != tc.count {
				t.Fatalf("unexpected result metadata: %+v", got)
			}
			if got.Value != tc.want {
				t.Fatalf("value = %s, want %s", got.Value, tc.want)
			}
		})
	}
}

func TestDecimalFactorsRepeatingRatioIsRoundedNotRejected(t *testing.T) {
	// 19 根 3 与 1 根 1：avg=(19*3+1)/20=58/20=2.9, last=1 → 1/2.9-1=-0.655172413793...
	// 无限循环小数必须按 half-even 舍入输出，而不是像 DecimalSMA 那样返回 ErrNonTerminatingDecimal。
	closes := append(repeat("3", 19), "1")
	got, err := DecimalMADistance20(closes)
	if err != nil {
		t.Fatalf("repeating ratio must be rounded, got error: %v", err)
	}
	if got.Value != "-0.655172413793" {
		t.Fatalf("value = %s, want -0.655172413793", got.Value)
	}
}

func TestDecimalFactorsInsufficientData(t *testing.T) {
	cases := []struct {
		name  string
		fn    func([]string) (DecimalResult, error)
		code  string
		input []string
		wantN int
	}{
		{"madistance19", DecimalMADistance20, "ma20_distance", repeat("10", 19), 19},
		{"momentum5five", DecimalMomentum5, "momentum_5d", repeat("10", 5), 5},
		{"momentum20twenty", DecimalMomentum20, "momentum_20d", repeat("10", 20), 20},
		{"madistanceEmpty", DecimalMADistance20, "ma20_distance", nil, 0},
		{"momentum5Empty", DecimalMomentum5, "momentum_5d", nil, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := tc.fn(tc.input)
			if err != nil {
				t.Fatal(err)
			}
			if got.Status != StatusInsufficientData || got.Value != "" || got.SampleCount != tc.wantN || got.Code != tc.code {
				t.Fatalf("unexpected insufficient result: %+v", got)
			}
		})
	}
}

func TestDecimalFactorsRejectInvalidInput(t *testing.T) {
	validMADistance := repeat("10", 20)
	validMomentum5 := repeat("10", 6)
	invalid := [][]string{
		{"100", "0", "100", "100", "100", "100"}, // 零价
		{"100", "-1", "100", "100", "100", "100"},
		{"100", "1e2", "100", "100", "100", "100"}, // 指数形式
		{"100", " 100", "100", "100", "100", "100"},
		{"100", "01", "100", "100", "100", "100"},
		{strings.Repeat("1", 129)},
	}
	for _, input := range invalid {
		if _, err := DecimalMADistance20(input); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("MADistance20(%v) error = %v, want ErrInvalidInput", input, err)
		}
	}
	for _, input := range invalid {
		if _, err := DecimalMomentum5(input); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("Momentum5(%v) error = %v, want ErrInvalidInput", input, err)
		}
	}
	// 确保有效输入不被上面的用例污染
	if _, err := DecimalMADistance20(validMADistance); err != nil {
		t.Fatalf("valid MADistance20 rejected: %v", err)
	}
	if _, err := DecimalMomentum5(validMomentum5); err != nil {
		t.Fatalf("valid Momentum5 rejected: %v", err)
	}
}

func TestDecimalFactorsAgreeWithFloatWithinTolerance(t *testing.T) {
	// Decimal 计算层的输出转回 float 后，应与现有 float64 纯函数在既有 1e-12 容差内一致。
	closes := sequentialCloses(100, 120)
	decimalMAD, err := DecimalMADistance20(closes)
	if err != nil {
		t.Fatal(err)
	}
	floatMAD, err := MADistance20(toFloat(closes))
	if err != nil {
		t.Fatal(err)
	}
	assertClose(t, "ma20_distance", decimalMAD.Value, floatMAD.Value, 1e-12)

	decMom5, err := DecimalMomentum5([]string{"95", "96", "97", "98", "99", "100"})
	if err != nil {
		t.Fatal(err)
	}
	floatMom5, err := Momentum5([]float64{95, 96, 97, 98, 99, 100})
	if err != nil {
		t.Fatal(err)
	}
	assertClose(t, "momentum_5d", decMom5.Value, floatMom5.Value, 1e-12)
}

func TestRoundRatHalfEvenToScale(t *testing.T) {
	// 正向与负向的中点舍入到偶数位；固定 scale 输出不带尾部零。
	cases := []struct {
		num   string
		den   string
		scale int
		want  string
	}{
		{"1", "2", 0, "0"},       // 0.5 → 0（偶数）
		{"3", "2", 0, "2"},       // 1.5 → 2（偶数）
		{"5", "2", 0, "2"},       // 2.5 → 2（偶数）
		{"1", "8", 3, "0.125"},   // 精确有限
		{"-1", "2", 0, "0"},      // -0.5 → 0
		{"-3", "2", 0, "-2"},     // -1.5 → -2（偶数）
		{"2", "1", 3, "2"},       // 整数不带尾部零
		{"1", "3", 4, "0.3333"},  // 1/3 截断为 0.3333（第 5 位 3 < 5）
		{"2", "3", 4, "0.6667"},  // 2/3 第 5 位 6 > 5，进位
		{"1", "200", 3, "0.005"}, // 0.005 精确
	}
	for _, tc := range cases {
		num, _ := new(big.Int).SetString(tc.num, 10)
		den, _ := new(big.Int).SetString(tc.den, 10)
		value := new(big.Rat).SetFrac(num, den)
		if got := roundRatHalfEvenToScale(value, tc.scale); got != tc.want {
			t.Errorf("%s/%s scale=%d: got %s, want %s", tc.num, tc.den, tc.scale, got, tc.want)
		}
	}
}

func sequentialCloses(from, to int) []string {
	out := make([]string, 0, to-from+1)
	for i := from; i <= to; i++ {
		out = append(out, strconv.Itoa(i))
	}
	return out
}

func repeat(value string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = value
	}
	return out
}

func toFloat(values []string) []float64 {
	out := make([]float64, len(values))
	for i, raw := range values {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			panic(err)
		}
		out[i] = parsed
	}
	return out
}

func assertClose(t *testing.T, code, decimalValue string, floatValue, tolerance float64) {
	t.Helper()
	got, err := strconv.ParseFloat(decimalValue, 64)
	if err != nil {
		t.Fatalf("%s: parse decimal value %q: %v", code, decimalValue, err)
	}
	diff := got - floatValue
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Fatalf("%s: decimal=%s float=%.15g diff=%.3g > tolerance %.3g", code, decimalValue, floatValue, diff, tolerance)
	}
}
