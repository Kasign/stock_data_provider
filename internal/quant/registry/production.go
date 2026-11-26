package registry

import "encoding/json"

const (
	ProductionSMA20Code             = "sma"
	ProductionSMA20SchemaVersion    = "1.0.0"
	ProductionSMA20AlgorithmVersion = "sma20-decimal-exact-v1"
	ProductionSMA20Unit             = "CNY/share"
)

const productionSMA20ParametersSchema = "{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"$id\":\"urn:kasign:quant:sma:1.0.0\",\"type\":\"object\",\"additionalProperties\":false,\"required\":[\"window\"],\"properties\":{\"window\":{\"type\":\"integer\",\"const\":20}}}"

// NewProductionRegistry 返回当前可执行的 Quant 指标 Registry。
// SMA20 使用 Eastmoney 原始 Close Token 做精确 Decimal 运算；不舍入，结果精度上限由输入 Token 长度推导。
func NewProductionRegistry() (*Registry, error) {
	registry := New()
	entry := Entry{
		Code:          ProductionSMA20Code,
		SchemaVersion: ProductionSMA20SchemaVersion,
		ValueType:     ValueTypeDecimalString,
		Unit:          ProductionSMA20Unit,
		Precision: Precision{
			MaxIntegerDigits:  128,
			MaxFractionDigits: 128,
			RoundingMode:      "exact",
			Tolerance:         json.RawMessage("\"0\""),
		},
		Range:            NumericRange{Unbounded: true},
		AlgorithmVersion: ProductionSMA20AlgorithmVersion,
		InputRequirements: []InputRequirement{{
			Fields:           []string{"close"},
			Interval:         "1d",
			Adjustment:       "none",
			PIT:              true,
			Quality:          []string{"valid"},
			RequiresSnapshot: true,
		}},
		ParametersSchema: json.RawMessage(productionSMA20ParametersSchema),
		QualityPolicy: QualityPolicy{
			MinimumCoverage:       1,
			MissingHandling:       "reject",
			QualityStates:         []string{"valid"},
			UnavailableConditions: []string{"insufficient_data"},
		},
		SampleCount: SampleCountPolicy{
			RequiredInMetric: true,
			Unit:             "bar",
			CountingRule:     "window_bars",
		},
		SupportsInterval: false,
	}
	if err := registry.Register(entry, true); err != nil {
		return nil, err
	}
	if err := registerProductionDecimalFactors(registry); err != nil {
		return nil, err
	}
	if err := registry.Freeze(); err != nil {
		return nil, err
	}
	return registry, nil
}
