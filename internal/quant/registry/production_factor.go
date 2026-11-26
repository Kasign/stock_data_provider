package registry

import "encoding/json"

// 首批三个 Close-only 比率因子的生产 Registry Entry。
// 与 sma/1.0.0 的 exact/0 策略不同，比率因子是无限循环小数，必须按固定 scale 舍入并声明容差；
// 精度策略见 doc/quant/06-因子精确计算精度策略决策.md 与 DEC-PLAT-002-057。
const (
	ProductionFactorSchemaVersion = "1.0.0"
	ProductionFactorUnit          = "ratio"

	// AlgorithmVersion 必须是 semver（QNT-CTR-004 calculation_version pattern），
	// 且与结果级 Golden 的 calculation_version 一致；精度策略由 Precision 字段声明，
	// 不在版本字符串里重复表达。因子身份由 code 区分，三者各自 1.0.0。
	ProductionFactorAlgorithmVersion = "1.0.0"

	ProductionMADistanceCode             = "ma20_distance"
	ProductionMADistanceAlgorithmVersion = ProductionFactorAlgorithmVersion
	ProductionMADistanceCountingRule     = "Count 20 consecutive valid close bars in the window; result is one ratio."

	ProductionMomentum5Code             = "momentum_5d"
	ProductionMomentum5AlgorithmVersion = ProductionFactorAlgorithmVersion
	ProductionMomentum5CountingRule     = "Count 6 consecutive valid close bars (base plus latest); result is one ratio."

	ProductionMomentum20Code             = "momentum_20d"
	ProductionMomentum20AlgorithmVersion = ProductionFactorAlgorithmVersion
	ProductionMomentum20CountingRule     = "Count 21 consecutive valid close bars (base plus latest); result is one ratio."

	// ProductionMomentum60Code 是 Trend 长周期证据的唯一精度来源。
	// 它尚未进入 QNT-CTR-003/004 的已发布 Factor 结果契约，因此只登记 Entry、
	// 不被 Factor Facade 分派；调用方是 Trend Facade（D1 决议：周期集合 5/20/60）。
	ProductionMomentum60Code             = "momentum_60d"
	ProductionMomentum60AlgorithmVersion = ProductionFactorAlgorithmVersion
	ProductionMomentum60CountingRule     = "Count 61 consecutive valid close bars (base plus latest); result is one ratio."

	// productionFactorTolerance 是固定 scale=12 末位对应的容差（JSON 字符串，与 decimal_string 类型一致）。
	productionFactorTolerance = `"0.000000000001"`
)

const productionFactorParametersSchema = "{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"$id\":\"urn:kasign:quant:factor:1.0.0\",\"type\":\"object\",\"additionalProperties\":false,\"maxProperties\":0}"

// productionFactorEntries 返回首批比率因子的不可变 Entry 定义。
// 三项均复用 Data 已授权的 Close Series Snapshot（精确十进制 Close），不依赖通用 OHLC DTO。
func productionFactorEntries() []Entry {
	definitions := []struct {
		code             string
		algorithmVersion string
		countingRule     string
	}{
		{ProductionMADistanceCode, ProductionMADistanceAlgorithmVersion, ProductionMADistanceCountingRule},
		{ProductionMomentum5Code, ProductionMomentum5AlgorithmVersion, ProductionMomentum5CountingRule},
		{ProductionMomentum20Code, ProductionMomentum20AlgorithmVersion, ProductionMomentum20CountingRule},
		{ProductionMomentum60Code, ProductionMomentum60AlgorithmVersion, ProductionMomentum60CountingRule},
	}
	entries := make([]Entry, 0, len(definitions))
	for _, definition := range definitions {
		entries = append(entries, Entry{
			Code:          definition.code,
			SchemaVersion: ProductionFactorSchemaVersion,
			ValueType:     ValueTypeDecimalString,
			Unit:          ProductionFactorUnit,
			Precision: Precision{
				MaxIntegerDigits:  128,
				MaxFractionDigits: 12,
				RoundingMode:      "half_even",
				Tolerance:         json.RawMessage(productionFactorTolerance),
			},
			Range:            NumericRange{Unbounded: true},
			AlgorithmVersion: definition.algorithmVersion,
			InputRequirements: []InputRequirement{{
				Fields:           []string{"close"},
				Interval:         "1d",
				Adjustment:       "none",
				PIT:              true,
				Quality:          []string{"valid"},
				RequiresSnapshot: true,
			}},
			ParametersSchema: json.RawMessage(productionFactorParametersSchema),
			QualityPolicy: QualityPolicy{
				MinimumCoverage:       1,
				MissingHandling:       "reject",
				QualityStates:         []string{"valid"},
				UnavailableConditions: []string{"insufficient_data"},
			},
			SampleCount: SampleCountPolicy{
				RequiredInMetric: true,
				Unit:             "bar",
				CountingRule:     definition.countingRule,
			},
			SupportsInterval: false,
		})
	}
	return entries
}

// registerProductionDecimalFactors 在 Freeze 之前注册全部首批 Decimal Factor Entry。
func registerProductionDecimalFactors(registry *Registry) error {
	for _, entry := range productionFactorEntries() {
		if err := registry.Register(entry, true); err != nil {
			return err
		}
	}
	return nil
}
