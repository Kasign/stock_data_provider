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
)

var (
	ErrUnsupportedResultType       = errors.New("unsupported Quant result type")
	ErrUnsupportedSchema           = errors.New("unsupported Quant result schema")
	ErrInvalidQuantResult          = errors.New("invalid Quant result envelope")
	ErrInvalidQuantPayload         = errors.New("invalid Quant result payload")
	ErrPayloadValidatorUnavailable = errors.New("Quant payload validator is unavailable")
)

var schemaVersionRE = regexp.MustCompile(`^[0-9]+[.][0-9]+[.][0-9]+(-[0-9A-Za-z.-]+)?$`)

var supportedResultTypes = map[string]struct{}{
	"indicator": {}, "factor": {}, "trend": {}, "market": {}, "industry": {},
	"stock": {}, "strategy": {}, "backtest": {}, "risk": {}, "evaluation": {},
}

// QuantResultEnvelope is the stable Quant Application envelope. Payload stays
// raw until the exact result_type/schema_version validator accepts it.
type QuantResultEnvelope struct {
	SchemaVersion        string          `json:"schema_version"`
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
	CreatedAt            time.Time       `json:"created_at"`
	Payload              json.RawMessage `json:"payload"`
}

// PayloadValidator validates one exact result_type/schema_version payload
// against its frozen strong schema. Implementations must not accept arbitrary
// object properties or perform remote schema resolution.
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

func (r *Registry) Register(resultType, schemaVersion string, validator PayloadValidator) error {
	if r == nil || validator == nil {
		return ErrPayloadValidatorUnavailable
	}
	if _, ok := supportedResultTypes[resultType]; !ok {
		return fmt.Errorf("%w: %q", ErrUnsupportedResultType, resultType)
	}
	if !schemaVersionRE.MatchString(schemaVersion) || schemaVersion != strings.TrimSpace(schemaVersion) {
		return fmt.Errorf("%w: invalid schema version", ErrUnsupportedSchema)
	}
	key := schemaKey{resultType: resultType, version: schemaVersion}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.validators[key]; exists {
		return fmt.Errorf("%w: duplicate registration for %s/%s", ErrUnsupportedSchema, resultType, schemaVersion)
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
// payload to the exact registered validator. Non-completed or non-usable
// quality results are returned as non-usable facts, not promoted to errors.
func (r *Registry) Validate(result QuantResultEnvelope, policy ValidationPolicy) (QuantValidation, error) {
	if r == nil {
		return QuantValidation{}, ErrPayloadValidatorUnavailable
	}
	if _, ok := supportedResultTypes[result.ResultType]; !ok {
		return QuantValidation{}, fmt.Errorf("%w: %q", ErrUnsupportedResultType, result.ResultType)
	}
	if !schemaVersionRE.MatchString(result.SchemaVersion) || result.SchemaVersion != strings.TrimSpace(result.SchemaVersion) {
		return QuantValidation{}, fmt.Errorf("%w: %q", ErrUnsupportedSchema, result.SchemaVersion)
	}
	r.mu.RLock()
	validator, exists := r.validators[schemaKey{resultType: result.ResultType, version: result.SchemaVersion}]
	r.mu.RUnlock()
	if !exists {
		return QuantValidation{}, fmt.Errorf("%w: %s/%s", ErrUnsupportedSchema, result.ResultType, result.SchemaVersion)
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
	if strings.TrimSpace(result.ResultID) == "" || strings.TrimSpace(result.Scope) == "" ||
		strings.TrimSpace(result.Period) == "" || strings.TrimSpace(result.AlgorithmVersion) == "" ||
		strings.TrimSpace(result.ParameterFingerprint) == "" || strings.TrimSpace(result.AuthorizationRef) == "" {
		return fmt.Errorf("%w: required envelope field is empty", ErrInvalidQuantResult)
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
	if !isUTC(result.SystemAsOf) || !isUTC(result.CreatedAt) ||
		policy.ExpectedSystemAsOf.IsZero() || !isUTC(policy.ExpectedSystemAsOf) ||
		!result.SystemAsOf.Equal(policy.ExpectedSystemAsOf) {
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
