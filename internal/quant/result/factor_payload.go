package result

import (
	"bytes"
	"encoding/json"
	"fmt"

	"example.com/stock-analysis-flow/internal/quant/factor"
	"example.com/stock-analysis-flow/internal/quant/manifest"
)

const (
	factorResultType           = "factor"
	factorPayloadSchemaVersion = "1.0.0"
)

// FactorPayload 是 factor/1.0.0 的强类型载荷，由 DEC-PLAT-002-058 授权注册进
// Envelope Payload 联合。它承载 DEC-057 已定义、经活动 Registry Entry 校验的
// DecimalQuantMetric：value 为十进制字符串或显式 null，不含浮点近似值。
// Payload 字段语义仍由 QNT-CTR-004 factor-quant-metric Schema 与领域 Validate 约束。
type FactorPayload struct {
	value factor.DecimalQuantMetric
}

// NewFactorPayload 构造并校验 factor/1.0.0 载荷；只接受领域 Validate 通过的强类型 Metric。
func NewFactorPayload(value factor.DecimalQuantMetric) (FactorPayload, error) {
	copyValue := cloneDecimalQuantMetric(value)
	if err := copyValue.Validate(); err != nil {
		return FactorPayload{}, fmt.Errorf("%w: factor payload: %v", ErrInvalidEnvelope, err)
	}
	return FactorPayload{value: copyValue}, nil
}

// Metric 返回与内部状态隔离的 Metric 深拷贝。
func (p FactorPayload) Metric() factor.DecimalQuantMetric {
	return cloneDecimalQuantMetric(p.value)
}

func (FactorPayload) resultType() string {
	return factorResultType
}

func (FactorPayload) payloadSchemaVersion() string {
	return factorPayloadSchemaVersion
}

func (p FactorPayload) validate() error {
	if err := p.value.Validate(); err != nil {
		return fmt.Errorf("%w: factor payload: %v", ErrInvalidEnvelope, err)
	}
	return nil
}

func (p FactorPayload) marshalPayload() ([]byte, error) {
	if err := p.validate(); err != nil {
		return nil, err
	}
	return json.Marshal(p.value)
}

func (p FactorPayload) MarshalJSON() ([]byte, error) {
	return p.marshalPayload()
}

// DecodeFactorPayload 解码并校验独立 factor/1.0.0 Payload。
// sample_count 必须为整数：领域 DTO 以 int 表示样本数，null/缺失无法区分"零样本"与"未提供"，
// 因此在强类型解码前显式拒绝，避免把外部伪造的 null 当成合法缺失值。
func DecodeFactorPayload(raw json.RawMessage) (FactorPayload, error) {
	if _, err := manifest.FingerprintPayload(raw); err != nil {
		return FactorPayload{}, fmt.Errorf("%w: factor payload JSON is invalid: %v", ErrInvalidEnvelope, err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return FactorPayload{}, fmt.Errorf("%w: factor payload: %v", ErrInvalidEnvelope, err)
	}
	sampleCount, ok := fields["sample_count"]
	if !ok || bytes.Equal(bytes.TrimSpace(sampleCount), []byte("null")) {
		return FactorPayload{}, fmt.Errorf("%w: factor payload sample_count must be an integer", ErrInvalidEnvelope)
	}
	var value factor.DecimalQuantMetric
	if err := decodeStrictPayload(raw, &value); err != nil {
		return FactorPayload{}, fmt.Errorf("%w: factor payload: %v", ErrInvalidEnvelope, err)
	}
	return NewFactorPayload(value)
}

func cloneDecimalQuantMetric(value factor.DecimalQuantMetric) factor.DecimalQuantMetric {
	copied := value
	if value.Value != nil {
		item := *value.Value
		copied.Value = &item
	}
	if value.LowerBound != nil {
		item := *value.LowerBound
		copied.LowerBound = &item
	}
	if value.UpperBound != nil {
		item := *value.UpperBound
		copied.UpperBound = &item
	}
	return copied
}
