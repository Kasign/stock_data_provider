package result

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/risk"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

const EnvelopeSchemaVersion = "1.0.0"

var (
	ErrInvalidEnvelope    = errors.New("invalid quant result envelope")
	ErrUnsupportedPayload = errors.New("unsupported quant result payload")
)

// Payload 是已注册的强类型 Payload 联合；未注册的 result_type/schema 组合必须拒绝。
type Payload interface {
	resultType() string
	payloadSchemaVersion() string
	validate() error
	marshalPayload() ([]byte, error)
}

// TrendPayload 是 trend/1.0.0 的强类型载荷。
type TrendPayload struct {
	value trend.Result
}

func NewTrendPayload(value trend.Result) (TrendPayload, error) {
	copyValue := cloneTrendResult(value)
	if err := copyValue.Validate(); err != nil {
		return TrendPayload{}, fmt.Errorf("%w: trend payload: %v", ErrInvalidEnvelope, err)
	}
	return TrendPayload{value: copyValue}, nil
}

func (p TrendPayload) Result() trend.Result            { return cloneTrendResult(p.value) }
func (TrendPayload) resultType() string                { return "trend" }
func (TrendPayload) payloadSchemaVersion() string      { return "1.0.0" }
func (p TrendPayload) validate() error                 { return p.value.Validate() }
func (p TrendPayload) marshalPayload() ([]byte, error) { return json.Marshal(p.value) }
func (p TrendPayload) MarshalJSON() ([]byte, error)    { return p.marshalPayload() }

// RiskPayload 是 risk/1.0.0 的强类型载荷。
type RiskPayload struct {
	value risk.Decision
}

func NewRiskPayload(value risk.Decision) (RiskPayload, error) {
	copyValue := cloneRiskDecision(value)
	if err := copyValue.Validate(); err != nil {
		return RiskPayload{}, fmt.Errorf("%w: risk payload: %v", ErrInvalidEnvelope, err)
	}
	return RiskPayload{value: copyValue}, nil
}

func (p RiskPayload) Decision() risk.Decision         { return cloneRiskDecision(p.value) }
func (RiskPayload) resultType() string                { return "risk" }
func (RiskPayload) payloadSchemaVersion() string      { return "1.0.0" }
func (p RiskPayload) validate() error                 { return p.value.Validate() }
func (p RiskPayload) marshalPayload() ([]byte, error) { return json.Marshal(p.value) }
func (p RiskPayload) MarshalJSON() ([]byte, error)    { return p.marshalPayload() }

// DecodePayload 按 result_type + payload_schema_version 解码已注册的强类型载荷。
// 未注册组合 fail closed；禁止退回 map[string]any 或宽泛 JSON 对象。
func DecodePayload(resultType, schemaVersion string, raw json.RawMessage) (Payload, error) {
	if _, err := manifest.FingerprintPayload(raw); err != nil {
		return nil, fmt.Errorf("%w: payload JSON is invalid: %v", ErrInvalidEnvelope, err)
	}
	switch {
	case resultType == "trend" && schemaVersion == "1.0.0":
		var value trend.Result
		if err := decodeStrictPayload(raw, &value); err != nil {
			return nil, fmt.Errorf("%w: trend payload: %v", ErrInvalidEnvelope, err)
		}
		return NewTrendPayload(value)
	case resultType == "risk" && schemaVersion == "1.0.0":
		var value risk.Decision
		if err := decodeStrictPayload(raw, &value); err != nil {
			return nil, fmt.Errorf("%w: risk payload: %v", ErrInvalidEnvelope, err)
		}
		return NewRiskPayload(value)
	default:
		return nil, fmt.Errorf("%w: %s/%s", ErrUnsupportedPayload, resultType, schemaVersion)
	}
}

func decodeStrictPayload(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("payload contains trailing JSON")
	}
	return nil
}

// Envelope 是 Quant Application 返回给 Researchflow 的完整结果。
// 业务元数据投影自不可变 Manifest；AuthorizationRef 是本次请求的授权结果引用。
type Envelope struct {
	SchemaVersion        string   `json:"schema_version"`
	ResultID             string   `json:"result_id"`
	ResultType           string   `json:"result_type"`
	Scope                string   `json:"scope"`
	SecurityID           string   `json:"security_id,omitempty"`
	SystemAsOf           string   `json:"system_as_of"`
	TradeDate            string   `json:"trade_date,omitempty"`
	Period               string   `json:"period"`
	AlgorithmVersion     string   `json:"algorithm_version"`
	ParameterFingerprint string   `json:"parameter_fingerprint"`
	InputSnapshotRefs    []string `json:"input_snapshot_refs"`
	QualityStatus        string   `json:"quality_status"`
	ResultStatus         string   `json:"result_status"`
	AuthorizationRef     string   `json:"authorization_ref"`
	PayloadSchemaVersion string   `json:"payload_schema_version"`
	Payload              Payload  `json:"payload"`
}

// BuildEnvelope 从已验证 Manifest 投影 Envelope 元数据。
func BuildEnvelope(source manifest.Manifest, authorizationRef string, payload Payload) (Envelope, error) {
	if !supportedPayloadValue(payload) {
		return Envelope{}, ErrUnsupportedPayload
	}
	var snapshotRefs []string
	if source.InputSnapshotRefs != nil {
		snapshotRefs = append([]string{}, source.InputSnapshotRefs...)
	}
	envelope := Envelope{
		SchemaVersion: EnvelopeSchemaVersion,
		ResultID: source.ResultID,
		ResultType: source.ResultType,
		Scope: source.Scope,
		SecurityID: source.SecurityID,
		SystemAsOf: source.SystemAsOf,
		TradeDate: source.TradeDate,
		Period: source.Period,
		AlgorithmVersion: source.AlgorithmVersion,
		ParameterFingerprint: source.ParameterFingerprint,
		InputSnapshotRefs: snapshotRefs,
		QualityStatus: source.QualityStatus,
		ResultStatus: source.ResultStatus,
		AuthorizationRef: authorizationRef,
		PayloadSchemaVersion: source.PayloadSchemaVersion,
		Payload: payload,
	}
	if err := envelope.ValidateAgainstManifest(source); err != nil {
		return Envelope{}, err
	}
	return envelope, nil
}

// Validate 检查 Envelope 自身结构及 Payload/Envelope 的跨字段约束。
// 资源身份和内容指纹还必须通过 ValidateAgainstManifest 验证。
func (envelope Envelope) Validate() error {
	if envelope.SchemaVersion != EnvelopeSchemaVersion ||
		!validEnvelopeResultID(envelope.ResultID) ||
		!validEnvelopeScope(envelope.Scope) ||
		(envelope.Scope == "stock" && !validEnvelopeText(envelope.SecurityID)) ||
		(envelope.SecurityID != "" && !validEnvelopeText(envelope.SecurityID)) ||
		!canonicalEnvelopeTime(envelope.SystemAsOf) ||
		!validEnvelopeDate(envelope.TradeDate) ||
		!validEnvelopeText(envelope.Period) ||
		!validEnvelopeText(envelope.AlgorithmVersion) ||
		!validEnvelopeText(envelope.ParameterFingerprint) ||
		!validEnvelopeSnapshots(envelope.InputSnapshotRefs) ||
		!validEnvelopeQuality(envelope.QualityStatus) ||
		!validEnvelopeResultStatus(envelope.ResultStatus) ||
		!validEnvelopeText(envelope.AuthorizationRef) ||
		!supportedPayloadValue(envelope.Payload) ||
		envelope.Payload.resultType() != envelope.ResultType ||
		envelope.Payload.payloadSchemaVersion() != envelope.PayloadSchemaVersion ||
		envelope.Payload.validate() != nil {
		return ErrInvalidEnvelope
	}

	switch payload := envelope.Payload.(type) {
	case TrendPayload:
		value := payload.value
		if value.Period != envelope.Period ||
			value.SystemAsOf != envelope.SystemAsOf ||
			value.AlgorithmVersion != envelope.AlgorithmVersion ||
			string(value.QualityStatus) != envelope.QualityStatus {
			return ErrInvalidEnvelope
		}
	case RiskPayload:
		if payload.value.SystemAsOf != envelope.SystemAsOf {
			return ErrInvalidEnvelope
		}
	default:
		return ErrUnsupportedPayload
	}
	return nil
}

// ValidateAgainstManifest 检查 Envelope 与不可变 Manifest 逐字段一致，并验证 Payload 指纹。
func (envelope Envelope) ValidateAgainstManifest(source manifest.Manifest) error {
	if err := source.Verify(); err != nil {
		return fmt.Errorf("%w: source manifest: %v", ErrInvalidEnvelope, err)
	}
	if err := envelope.Validate(); err != nil {
		return err
	}
	if envelope.ResultID != source.ResultID ||
		envelope.ResultType != source.ResultType ||
		envelope.Scope != source.Scope ||
		envelope.SecurityID != source.SecurityID ||
		envelope.SystemAsOf != source.SystemAsOf ||
		envelope.TradeDate != source.TradeDate ||
		envelope.Period != source.Period ||
		envelope.AlgorithmVersion != source.AlgorithmVersion ||
		envelope.ParameterFingerprint != source.ParameterFingerprint ||
		envelope.QualityStatus != source.QualityStatus ||
		envelope.ResultStatus != source.ResultStatus ||
		envelope.PayloadSchemaVersion != source.PayloadSchemaVersion ||
		!sameSnapshotRefs(envelope.InputSnapshotRefs, source.InputSnapshotRefs) {
		return ErrInvalidEnvelope
	}
	payloadJSON, err := envelope.Payload.marshalPayload()
	if err != nil {
		return fmt.Errorf("%w: marshal payload: %v", ErrInvalidEnvelope, err)
	}
	if err := source.VerifyPayload(payloadJSON); err != nil {
		return fmt.Errorf("%w: manifest payload binding: %v", ErrInvalidEnvelope, err)
	}
	return nil
}

func (envelope Envelope) MarshalJSON() ([]byte, error) {
	type envelopeAlias Envelope
	return json.Marshal(envelopeAlias(envelope))
}

func (envelope *Envelope) UnmarshalJSON(data []byte) error {
	if envelope == nil {
		return ErrInvalidEnvelope
	}
	var wire struct {
		SchemaVersion string `json:"schema_version"`
		ResultID string `json:"result_id"`
		ResultType string `json:"result_type"`
		Scope string `json:"scope"`
		SecurityID string `json:"security_id"`
		SystemAsOf string `json:"system_as_of"`
		TradeDate string `json:"trade_date"`
		Period string `json:"period"`
		AlgorithmVersion string `json:"algorithm_version"`
		ParameterFingerprint string `json:"parameter_fingerprint"`
		InputSnapshotRefs []string `json:"input_snapshot_refs"`
		QualityStatus string `json:"quality_status"`
		ResultStatus string `json:"result_status"`
		AuthorizationRef string `json:"authorization_ref"`
		PayloadSchemaVersion string `json:"payload_schema_version"`
		Payload json.RawMessage `json:"payload"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&wire); err != nil {
		return fmt.Errorf("%w: decode envelope: %v", ErrInvalidEnvelope, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return ErrInvalidEnvelope
	}
	payload, err := DecodePayload(wire.ResultType, wire.PayloadSchemaVersion, wire.Payload)
	if err != nil {
		return err
	}
	var snapshotRefs []string
	if wire.InputSnapshotRefs != nil {
		snapshotRefs = append([]string{}, wire.InputSnapshotRefs...)
	}
	decoded := Envelope{
		SchemaVersion: wire.SchemaVersion,
		ResultID: wire.ResultID,
		ResultType: wire.ResultType,
		Scope: wire.Scope,
		SecurityID: wire.SecurityID,
		SystemAsOf: wire.SystemAsOf,
		TradeDate: wire.TradeDate,
		Period: wire.Period,
		AlgorithmVersion: wire.AlgorithmVersion,
		ParameterFingerprint: wire.ParameterFingerprint,
		InputSnapshotRefs: snapshotRefs,
		QualityStatus: wire.QualityStatus,
		ResultStatus: wire.ResultStatus,
		AuthorizationRef: wire.AuthorizationRef,
		PayloadSchemaVersion: wire.PayloadSchemaVersion,
		Payload: payload,
	}
	if err := decoded.Validate(); err != nil {
		return err
	}
	*envelope = decoded
	return nil
}

func supportedPayloadValue(payload Payload) bool {
	switch payload.(type) {
	case TrendPayload, RiskPayload:
		return true
	default:
		return false
	}
}

func cloneTrendResult(value trend.Result) trend.Result {
	copyValue := value
	if value.Strength != nil {
		strength := *value.Strength
		copyValue.Strength = &strength
	}
	copyValue.EvidenceMetrics = append([]metric.Metric{}, value.EvidenceMetrics...)
	for index := range copyValue.EvidenceMetrics {
		copyValue.EvidenceMetrics[index].Value = append(json.RawMessage{}, value.EvidenceMetrics[index].Value...)
		copyValue.EvidenceMetrics[index].LowerBound = append(json.RawMessage{}, value.EvidenceMetrics[index].LowerBound...)
		copyValue.EvidenceMetrics[index].UpperBound = append(json.RawMessage{}, value.EvidenceMetrics[index].UpperBound...)
		if value.EvidenceMetrics[index].SampleCount != nil {
			count := *value.EvidenceMetrics[index].SampleCount
			copyValue.EvidenceMetrics[index].SampleCount = &count
		}
	}
	copyValue.ConflictRefs = append([]string{}, value.ConflictRefs...)
	return copyValue
}

func cloneRiskDecision(value risk.Decision) risk.Decision {
	copyValue := value
	copyValue.Reasons = append([]risk.Reason{}, value.Reasons...)
	for index := range copyValue.Reasons {
		copyValue.Reasons[index].EvidenceRefs = append([]string{}, value.Reasons[index].EvidenceRefs...)
	}
	copyValue.InputRefs = append([]string{}, value.InputRefs...)
	return copyValue
}

func sameSnapshotRefs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validEnvelopeResultID(value string) bool {
	const prefix = "quant_result:sha256:"
	digest := strings.TrimPrefix(value, prefix)
	if digest == value || len(digest) != 64 {
		return false
	}
	for _, character := range digest {
		if !((character >= '0' && character <= '9') || (character >= 'a' && character <= 'f')) {
			return false
		}
	}
	return true
}

func validEnvelopeSnapshots(refs []string) bool {
	if refs == nil {
		return false
	}
	seen := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		if !validEnvelopeText(ref) {
			return false
		}
		if _, exists := seen[ref]; exists {
			return false
		}
		seen[ref] = struct{}{}
	}
	return true
}

func validEnvelopeScope(value string) bool {
	switch value {
	case "stock", "market", "industry", "portfolio":
		return true
	default:
		return false
	}
}

func validEnvelopeQuality(value string) bool {
	switch value {
	case "valid", "degraded", "invalid", "unknown":
		return true
	default:
		return false
	}
}

func validEnvelopeResultStatus(value string) bool {
	switch value {
	case "completed", "insufficient_data", "insufficient_sample", "rejected", "failed":
		return true
	default:
		return false
	}
}

func validEnvelopeText(value string) bool {
	return utf8.ValidString(value) && value != "" && strings.TrimSpace(value) == value
}

func validEnvelopeDate(value string) bool {
	if value == "" {
		return true
	}
	if len(value) != len("2006-01-02") {
		return false
	}
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}

func canonicalEnvelopeTime(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Location() == time.UTC &&
		strings.HasSuffix(value, "Z") && parsed.UTC().Format(time.RFC3339Nano) == value
}
