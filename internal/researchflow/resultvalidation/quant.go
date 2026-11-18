package resultvalidation

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	quantresult "example.com/stock-analysis-flow/internal/quant/result"
)

var (
	ErrUnsupportedResultType       = errors.New("unsupported Quant result type")
	ErrUnsupportedSchema           = errors.New("unsupported Quant result schema")
	ErrInvalidQuantResult          = errors.New("invalid Quant result envelope")
	ErrInvalidQuantPayload         = errors.New("invalid Quant result payload")
	ErrPayloadValidatorUnavailable = errors.New("Quant payload validator is unavailable")
)

var schemaVersionRE = regexp.MustCompile(`^[0-9]+[.][0-9]+[.][0-9]+(-[0-9A-Za-z.-]+)?$`)
var quantResultIDRE = regexp.MustCompile(`^quant_result:sha256:[0-9a-f]{64}$`)

// Quant's frozen Envelope 1.0.0 currently supports only trend and risk payloads.
var supportedResultTypes = map[string]struct{}{
	"trend": {}, "risk": {},
}

// QuantResultEnvelope mirrors the Quant-owned Envelope fields used by Researchflow.
// Payload stays raw until the exact result_type/payload_schema_version validator accepts it.
type QuantResultEnvelope struct {
	SchemaVersion        string          `json:"schema_version"`
	PayloadSchemaVersion string          `json:"payload_schema_version"`
	ResultID             string          `json:"result_id"`
	ResultType           string          `json:"result_type"`
	Scope                string          `json:"scope"`
	SecurityID           string          `json:"security_id,omitempty"`
	SystemAsOf           time.Time       `json:"system_as_of"`
	TradeDate            string          `json:"trade_date,omitempty"`
	Period               string          `json:"period"`
	AlgorithmVersion     string          `json:"algorithm_version"`
	ParameterFingerprint string          `json:"parameter_fingerprint"`
	InputSnapshotRefs    []string        `json:"input_snapshot_refs"`
	QualityStatus        string          `json:"quality_status"`
	ResultStatus         string          `json:"result_status"`
	AuthorizationRef     string          `json:"authorization_ref"`
	Payload              json.RawMessage `json:"payload"`
}

// PayloadValidator validates one exact result_type/payload_schema_version payload.
// Implementations must not accept arbitrary object properties or perform remote schema resolution.
type PayloadValidator interface {
	ValidatePayload(payload json.RawMessage) error
}

type PayloadValidatorFunc func(json.RawMessage) error

func (f PayloadValidatorFunc) ValidatePayload(payload json.RawMessage) error {
	return f(payload)
}

type schemaKey struct {
	resultType string
	version    string
}

// Registry binds each supported discriminator pair to one payload validator.
// Unknown pairs fail closed; no fallback to older schema versions.
type Registry struct {
	mu         sync.RWMutex
	validators map[schemaKey]PayloadValidator
}

func NewRegistry() *Registry {
	return &Registry{validators: make(map[schemaKey]PayloadValidator)}
}

// NewQuantPayloadRegistry registers only the payload types currently supported
// by Quant's frozen Envelope 1.0.0 union. Validation delegates to Quant's own
// strict typed decoder so Researchflow does not maintain duplicate payload rules.
func NewQuantPayloadRegistry() (*Registry, error) {
	registry := NewRegistry()
	for _, item := range []struct {
		resultType string
		version    string
	}{
		{resultType: "trend", version: "1.0.0"},
		{resultType: "risk", version: "1.0.0"},
	} {
		resultType, version := item.resultType, item.version
		validator := PayloadValidatorFunc(func(payload json.RawMessage) error {
			_, err := quantresult.DecodePayload(resultType, version, payload)
			return err
		})
		if err := registry.Register(resultType, version, validator); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

// FromQuantEnvelope validates and adapts the Quant-owned typed Envelope into
// Researchflow's composition input. Envelope and payload schema versions stay
// separate; CreatedAt belongs to QuantResultRef, not the Envelope.
func FromQuantEnvelope(envelope quantresult.Envelope) (QuantResultEnvelope, error) {
	if err := envelope.Validate(); err != nil {
		return QuantResultEnvelope{}, fmt.Errorf("%w: Quant envelope: %v", ErrInvalidQuantResult, err)
	}
	systemAsOf, err := time.Parse(time.RFC3339Nano, envelope.SystemAsOf)
	if err != nil {
		return QuantResultEnvelope{}, fmt.Errorf("%w: invalid Quant system_as_of: %v", ErrInvalidQuantResult, err)
	}
	payload, err := json.Marshal(envelope.Payload)
	if err != nil {
		return QuantResultEnvelope{}, fmt.Errorf("%w: marshal Quant payload: %v", ErrInvalidQuantPayload, err)
	}
	return QuantResultEnvelope{
		SchemaVersion: envelope.SchemaVersion, PayloadSchemaVersion: envelope.PayloadSchemaVersion,
		ResultID: envelope.ResultID, ResultType: envelope.ResultType, Scope: envelope.Scope,
		SecurityID: envelope.SecurityID, SystemAsOf: systemAsOf, TradeDate: envelope.TradeDate,
		Period: envelope.Period, AlgorithmVersion: envelope.AlgorithmVersion,
		ParameterFingerprint: envelope.ParameterFingerprint,
		InputSnapshotRefs: append([]string{}, envelope.InputSnapshotRefs...),
		QualityStatus: envelope.QualityStatus, ResultStatus: envelope.ResultStatus,
		AuthorizationRef: envelope.AuthorizationRef, Payload: json.RawMessage(payload),
	}, nil
}

func (r *Registry) Register(resultType, payloadSchemaVersion string, validator PayloadValidator) error {
	if r == nil || validator == nil {
		return ErrPayloadValidatorUnavailable
	}
	if _, ok := supportedResultTypes[resultType]; !ok {
		return fmt.Errorf("%w: %q", ErrUnsupportedResultType, resultType)
	}
	if !schemaVersionRE.MatchString(payloadSchemaVersion) || payloadSchemaVersion != strings.TrimSpace(payloadSchemaVersion) {
		return fmt.Errorf("%w: invalid payload schema version", ErrUnsupportedSchema)
	}
	key := schemaKey{resultType: resultType, version: payloadSchemaVersion}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.validators[key]; exists {
		return fmt.Errorf("%w: duplicate registration for %s/%s", ErrUnsupportedSchema, resultType, payloadSchemaVersion)
	}
	r.validators[key] = validator
	return nil
}

type ValidationPolicy struct {
	ExpectedSystemAsOf time.Time
	AllowEmptyInputs   bool
	AllowDegraded      bool
}

type QuantValidation struct {
	Result      QuantResultEnvelope
	Usable      bool
	ReasonCodes []string
}

// Validate checks common envelope semantics and delegates the discriminated
// payload to the exact registered payload schema validator. Non-completed or
// non-usable quality results are returned as non-usable facts, not promoted to errors.
func (r *Registry) Validate(result QuantResultEnvelope, policy ValidationPolicy) (QuantValidation, error) {
	if r == nil {
		return QuantValidation{}, ErrPayloadValidatorUnavailable
	}
	if _, ok := supportedResultTypes[result.ResultType]; !ok {
		return QuantValidation{}, fmt.Errorf("%w: %q", ErrUnsupportedResultType, result.ResultType)
	}
	if result.SchemaVersion != quantresult.EnvelopeSchemaVersion {
		return QuantValidation{}, fmt.Errorf("%w: envelope schema %q", ErrUnsupportedSchema, result.SchemaVersion)
	}
	if !schemaVersionRE.MatchString(result.PayloadSchemaVersion) || result.PayloadSchemaVersion != strings.TrimSpace(result.PayloadSchemaVersion) {
		return QuantValidation{}, fmt.Errorf("%w: payload schema %q", ErrUnsupportedSchema, result.PayloadSchemaVersion)
	}
	r.mu.RLock()
	validator, exists := r.validators[schemaKey{resultType: result.ResultType, version: result.PayloadSchemaVersion}]
	r.mu.RUnlock()
	if !exists {
		return QuantValidation{}, fmt.Errorf("%w: %s/%s", ErrUnsupportedSchema, result.ResultType, result.PayloadSchemaVersion)
	}

	if err := validateEnvelope(result, policy); err != nil {
		return QuantValidation{}, err
	}
	payload := append(json.RawMessage(nil), result.Payload...)
	if err := validator.ValidatePayload(payload); err != nil {
		return QuantValidation{}, fmt.Errorf("%w: %v", ErrInvalidQuantPayload, err)
	}

	reasons := make([]string, 0, 2)
	if result.ResultStatus != "completed" {
		reasons = append(reasons, "result_not_completed")
	}
	switch result.QualityStatus {
	case "invalid", "unknown":
		reasons = append(reasons, "quality_not_usable")
	case "degraded":
		if !policy.AllowDegraded {
			reasons = append(reasons, "degraded_not_allowed")
		}
	}
	sort.Strings(reasons)
	return QuantValidation{
		Result: cloneQuantResult(result), Usable: len(reasons) == 0, ReasonCodes: reasons,
	}, nil
}

func validateEnvelope(result QuantResultEnvelope, policy ValidationPolicy) error {
	if !quantResultIDRE.MatchString(result.ResultID) || strings.TrimSpace(result.Scope) == "" ||
		strings.TrimSpace(result.Period) == "" || strings.TrimSpace(result.AlgorithmVersion) == "" ||
		strings.TrimSpace(result.ParameterFingerprint) == "" || strings.TrimSpace(result.AuthorizationRef) == "" {
		return fmt.Errorf("%w: required envelope field is empty or invalid", ErrInvalidQuantResult)
	}
	if !validScope(result.Scope) {
		return fmt.Errorf("%w: unsupported scope %q", ErrInvalidQuantResult, result.Scope)
	}
	if result.Scope == "stock" && strings.TrimSpace(result.SecurityID) == "" {
		return fmt.Errorf("%w: stock result requires security_id", ErrInvalidQuantResult)
	}
	if result.SecurityID != "" && result.SecurityID != strings.TrimSpace(result.SecurityID) {
		return fmt.Errorf("%w: security_id is not canonical", ErrInvalidQuantResult)
	}
	if !validQuality(result.QualityStatus) || !validResultStatus(result.ResultStatus) {
		return fmt.Errorf("%w: invalid result or quality status", ErrInvalidQuantResult)
	}
	if result.ParameterFingerprint != strings.TrimSpace(result.ParameterFingerprint) {
		return fmt.Errorf("%w: parameter_fingerprint is not canonical", ErrInvalidQuantResult)
	}
	if result.InputSnapshotRefs == nil || (!policy.AllowEmptyInputs && len(result.InputSnapshotRefs) == 0) ||
		hasDuplicateOrInvalidRefs(result.InputSnapshotRefs) || !sort.StringsAreSorted(result.InputSnapshotRefs) {
		return fmt.Errorf("%w: input_snapshot_refs is missing, empty or invalid", ErrInvalidQuantResult)
	}
	if !isUTC(result.SystemAsOf) || policy.ExpectedSystemAsOf.IsZero() ||
		!isUTC(policy.ExpectedSystemAsOf) || !result.SystemAsOf.Equal(policy.ExpectedSystemAsOf) {
		return fmt.Errorf("%w: system_as_of must match the Run and timestamps must be UTC", ErrInvalidQuantResult)
	}
	if result.TradeDate != "" {
		date, err := time.Parse("2006-01-02", result.TradeDate)
		if err != nil || date.Format("2006-01-02") != result.TradeDate {
			return fmt.Errorf("%w: trade_date must use YYYY-MM-DD", ErrInvalidQuantResult)
		}
	}
	if len(result.Payload) == 0 || !json.Valid(result.Payload) {
		return fmt.Errorf("%w: payload must be valid JSON", ErrInvalidQuantPayload)
	}
	trimmed := strings.TrimSpace(string(result.Payload))
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return fmt.Errorf("%w: payload must be a JSON object", ErrInvalidQuantPayload)
	}
	return nil
}

func validScope(value string) bool {
	switch value {
	case "stock", "market", "industry", "portfolio":
		return true
	default:
		return false
	}
}

func validQuality(value string) bool {
	switch value {
	case "valid", "degraded", "invalid", "unknown":
		return true
	default:
		return false
	}
}

func validResultStatus(value string) bool {
	switch value {
	case "completed", "insufficient_data", "insufficient_sample", "rejected", "failed":
		return true
	default:
		return false
	}
}

func isUTC(value time.Time) bool {
	if value.IsZero() {
		return false
	}
	_, offset := value.Zone()
	return offset == 0
}

func hasDuplicateOrInvalidRefs(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
			return true
		}
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func cloneQuantResult(value QuantResultEnvelope) QuantResultEnvelope {
	value.InputSnapshotRefs = append([]string{}, value.InputSnapshotRefs...)
	value.Payload = append(json.RawMessage(nil), value.Payload...)
	return value
}
