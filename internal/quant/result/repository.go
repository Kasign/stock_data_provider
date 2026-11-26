package result

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/quant/manifest"
)

// BuildRecord 从 Quant 已注册的强类型 Payload 构造不可变结果记录。
// 只有 Envelope 当前支持的 result_type/schema 组合可以进入持久化写入端口。
func BuildRecord(input manifest.Input, payload Payload) (Record, error) {
	if !supportedPayloadValue(payload) {
		return Record{}, ErrUnsupportedPayload
	}
	if input.SchemaVersion != EnvelopeSchemaVersion ||
		input.ResultType != payload.resultType() ||
		input.PayloadSchemaVersion != payload.payloadSchemaVersion() {
		return Record{}, ErrInvalidEnvelope
	}
	if err := payload.validate(); err != nil {
		return Record{}, fmt.Errorf("%w: payload validation: %v", ErrInvalidEnvelope, err)
	}
	raw, err := payload.marshalPayload()
	if err != nil {
		return Record{}, fmt.Errorf("%w: marshal payload: %v", ErrInvalidEnvelope, err)
	}
	source, err := manifest.BuildWithPayload(input, raw)
	if err != nil {
		return Record{}, fmt.Errorf("%w: build manifest: %v", ErrInvalidEnvelope, err)
	}
	switch value := payload.(type) {
	case TrendPayload:
		trendResult := value.Result()
		if trendResult.Period != source.Period ||
			trendResult.SystemAsOf != source.SystemAsOf ||
			trendResult.AlgorithmVersion != source.AlgorithmVersion ||
			string(trendResult.QualityStatus) != source.QualityStatus {
			return Record{}, fmt.Errorf("%w: trend payload metadata differs from manifest", ErrInvalidEnvelope)
		}
	case RiskPayload:
		if value.Decision().SystemAsOf != source.SystemAsOf {
			return Record{}, fmt.Errorf("%w: risk payload timestamp differs from manifest", ErrInvalidEnvelope)
		}
	case IndicatorPayload:
		if err := validateIndicatorManifestMetadata(value.value, source.Period, source.SystemAsOf, source.AlgorithmVersion, source.QualityStatus, source.ResultStatus); err != nil {
			return Record{}, fmt.Errorf("%w: indicator payload metadata differs from manifest", ErrInvalidEnvelope)
		}
	case FactorPayload:
		if err := validateFactorManifestMetadata(value.value, source.Period, source.SystemAsOf, source.AlgorithmVersion, source.QualityStatus, source.ResultStatus); err != nil {
			return Record{}, fmt.Errorf("%w: factor payload metadata differs from manifest", ErrInvalidEnvelope)
		}
	default:
		return Record{}, ErrUnsupportedPayload
	}
	record := Record{Manifest: source, Payload: append(json.RawMessage(nil), raw...)}
	if err := record.Validate(); err != nil {
		return Record{}, fmt.Errorf("%w: validate record: %v", ErrInvalidEnvelope, err)
	}
	if _, err := DecodePayload(source.ResultType, source.PayloadSchemaVersion, record.Payload); err != nil {
		return Record{}, err
	}
	return record, nil
}

var ErrResultConflict = errors.New("quant result identity conflict")

// Record 是持久化边界中的不可变结果载荷。
// Payload 保持原始 JSON 字节；在 Envelope/各 result_type payload 子 Schema 冻结前，
// Repository 不解释或重塑其业务结构。
type Record struct {
	Manifest manifest.Manifest
	Payload  json.RawMessage
}

// Validate 校验 Manifest 与 payload 的内容寻址绑定。
func (record Record) Validate() error {
	if err := record.Manifest.VerifyPayload(record.Payload); err != nil {
		return fmt.Errorf("validate quant result record: %w", err)
	}
	return nil
}

// ValidatedSnapshot 校验记录后返回与调用方内存隔离的深拷贝。
// Repository 实现应在保存或异步处理前调用此方法，避免调用方随后修改切片底层数据。
func (record Record) ValidatedSnapshot() (Record, error) {
	if err := record.Validate(); err != nil {
		return Record{}, err
	}

	snapshot := Record{
		Manifest: record.Manifest,
		Payload:  append(json.RawMessage(nil), record.Payload...),
	}
	snapshot.Manifest.InputSnapshotRefs = append([]string{}, record.Manifest.InputSnapshotRefs...)
	return snapshot, nil
}

// Repository 是 Quant 结果的写入端口，不暴露未完成授权复核的读取能力。
// Save 必须满足：相同 result_id 且内容相同视为幂等成功；相同 result_id 但内容不同返回 ErrResultConflict。
// 实现方必须以单一事务/原子操作保证唯一性检查与写入不可分割。
type Repository interface {
	Save(ctx context.Context, record Record) error
}
