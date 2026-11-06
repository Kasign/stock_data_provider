package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type ValueType string

const (
	ValueTypeDecimalString ValueType = "decimal_string"
	ValueTypeInteger       ValueType = "integer"
	ValueTypeNumber        ValueType = "number"
)

type Precision struct {
	MaxIntegerDigits   int             `json:"max_integer_digits"`
	MaxFractionDigits int             `json:"max_fraction_digits"`
	RoundingMode      string          `json:"rounding_mode"`
	Tolerance         json.RawMessage `json:"tolerance"`
}

type NumericRange struct {
	Minimum   json.RawMessage `json:"minimum,omitempty"`
	Maximum   json.RawMessage `json:"maximum,omitempty"`
	Unbounded bool            `json:"unbounded"`
}

type InputRequirement struct {
	Fields           []string `json:"fields"`
	Interval         string   `json:"interval"`
	Adjustment       string   `json:"adjustment"`
	PIT              bool     `json:"pit"`
	Quality          []string `json:"quality"`
	RequiresSnapshot bool     `json:"requires_snapshot"`
}

type QualityPolicy struct {
	MinimumCoverage      float64  `json:"minimum_coverage"`
	MissingHandling      string   `json:"missing_handling"`
	QualityStates        []string `json:"quality_states"`
	UnavailableConditions []string `json:"unavailable_conditions"`
}

// Entry 是 Quant 指标、Indicator 或 Factor 的不可变语义元数据。
// ParametersSchema 保留 JSON Schema 原文，避免把参数定义降级为无约束 map。
type Entry struct {
	Code              string              `json:"code"`
	SchemaVersion     string              `json:"schema_version"`
	ValueType         ValueType           `json:"value_type"`
	Unit              string              `json:"unit"`
	Precision         Precision           `json:"precision"`
	Range             NumericRange        `json:"range"`
	AlgorithmVersion  string              `json:"algorithm_version"`
	InputRequirements []InputRequirement  `json:"input_requirements"`
	ParametersSchema  json.RawMessage     `json:"parameters_schema"`
	QualityPolicy     QualityPolicy       `json:"quality_policy"`
	SupportsInterval  bool                `json:"supports_interval"`
}

var (
	ErrInvalidEntry       = errors.New("invalid registry entry")
	ErrDuplicateVersion   = errors.New("registry entry version already registered")
	ErrActiveVersionExists = errors.New("active algorithm version already exists")
	ErrUnknownEntry       = errors.New("registry entry not found")
	ErrNoActiveVersion    = errors.New("active algorithm version not found")
	ErrRegistryFrozen     = errors.New("registry is frozen")
)

var (
	codePattern    = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	decimalPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)(?:\.[0-9]+)?$`)
	integerPattern = regexp.MustCompile(`^-?(?:0|[1-9][0-9]*)$`)
)

func (e Entry) Validate() error {
	if !codePattern.MatchString(e.Code) || !versionPattern.MatchString(e.SchemaVersion) || strings.TrimSpace(e.AlgorithmVersion) == "" {
		return invalid("code, schema_version or algorithm_version")
	}
	if strings.TrimSpace(e.Unit) == "" {
		return invalid("unit")
	}
	switch e.ValueType {
	case ValueTypeDecimalString, ValueTypeInteger, ValueTypeNumber:
	default:
		return invalid("value_type")
	}
	if e.Precision.MaxIntegerDigits < 1 || e.Precision.MaxFractionDigits < 0 || strings.TrimSpace(e.Precision.RoundingMode) == "" {
		return invalid("precision")
	}
	tolerance, err := parseNumeric(e.Precision.Tolerance, e.ValueType)
	if err != nil || tolerance.Sign() < 0 {
		return invalid("precision.tolerance")
	}
	if err := e.Range.validate(e.ValueType); err != nil {
		return err
	}
	if len(e.InputRequirements) == 0 {
		return invalid("input_requirements")
	}
	for i, requirement := range e.InputRequirements {
		if len(requirement.Fields) == 0 || strings.TrimSpace(requirement.Interval) == "" || strings.TrimSpace(requirement.Adjustment) == "" || len(requirement.Quality) == 0 {
			return invalid(fmt.Sprintf("input_requirements[%d]", i))
		}
		if hasBlankOrDuplicate(requirement.Fields) || hasBlankOrDuplicate(requirement.Quality) {
			return invalid(fmt.Sprintf("input_requirements[%d] contains blank or duplicate values", i))
		}
	}
	if !isJSONObject(e.ParametersSchema) {
		return invalid("parameters_schema must be a JSON object")
	}
	if math.IsNaN(e.QualityPolicy.MinimumCoverage) || math.IsInf(e.QualityPolicy.MinimumCoverage, 0) || e.QualityPolicy.MinimumCoverage < 0 || e.QualityPolicy.MinimumCoverage > 1 {
		return invalid("quality_policy.minimum_coverage")
	}
	if strings.TrimSpace(e.QualityPolicy.MissingHandling) == "" || len(e.QualityPolicy.QualityStates) == 0 || len(e.QualityPolicy.UnavailableConditions) == 0 {
		return invalid("quality_policy")
	}
	if hasBlankOrDuplicate(e.QualityPolicy.QualityStates) || hasBlankOrDuplicate(e.QualityPolicy.UnavailableConditions) {
		return invalid("quality_policy contains blank or duplicate values")
	}
	return nil
}

func (r NumericRange) validate(valueType ValueType) error {
	if r.Unbounded {
		if len(r.Minimum) != 0 || len(r.Maximum) != 0 {
			return invalid("range: unbounded cannot include minimum/maximum")
		}
		return nil
	}
	if len(r.Minimum) == 0 || len(r.Maximum) == 0 {
		return invalid("range: bounded range requires minimum and maximum")
	}
	minimum, err := parseNumeric(r.Minimum, valueType)
	if err != nil {
		return invalid("range.minimum")
	}
	maximum, err := parseNumeric(r.Maximum, valueType)
	if err != nil {
		return invalid("range.maximum")
	}
	if minimum.Cmp(maximum) > 0 {
		return invalid("range.minimum must not exceed range.maximum")
	}
	return nil
}

func parseNumeric(raw json.RawMessage, valueType ValueType) (*big.Rat, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, errors.New("missing numeric value")
	}
	var text string
	switch valueType {
	case ValueTypeDecimalString:
		if err := json.Unmarshal(raw, &text); err != nil || !decimalPattern.MatchString(text) {
			return nil, errors.New("invalid decimal string")
		}
	case ValueTypeInteger:
		var number json.Number
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&number); err != nil || !integerPattern.MatchString(number.String()) {
			return nil, errors.New("invalid integer")
		}
		text = number.String()
	case ValueTypeNumber:
		var number json.Number
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&number); err != nil {
			return nil, err
		}
		value, err := strconv.ParseFloat(number.String(), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, errors.New("invalid finite number")
		}
		rat := new(big.Rat)
		if rat.SetFloat64(value) == nil {
			return nil, errors.New("invalid number")
		}
		return rat, nil
	default:
		return nil, errors.New("unsupported value type")
	}
	rat := new(big.Rat)
	if _, ok := rat.SetString(text); !ok {
		return nil, errors.New("invalid numeric value")
	}
	return rat, nil
}

func isJSONObject(raw json.RawMessage) bool {
	var value map[string]json.RawMessage
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil || value == nil {
		return false
	}
	return true
}

func hasBlankOrDuplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func invalid(field string) error {
	return fmt.Errorf("%w: %s", ErrInvalidEntry, field)
}

type metricKey struct {
	code          string
	schemaVersion string
}

type versionKey struct {
	metricKey
	algorithmVersion string
}

// Registry 只管理 Quant 自身的指标语义，不承担跨模块通用插件注册职责。
type Registry struct {
	mu      sync.RWMutex
	frozen  bool
	entries map[versionKey]Entry
	active  map[metricKey]string
}

func New() *Registry {
	return &Registry{
		entries: make(map[versionKey]Entry),
		active:  make(map[metricKey]string),
	}
}

// Register 注册一个明确版本。active=true 表示该 code/schema_version 的活动算法版本。
// 活动版本必须显式指定，不依赖注册顺序；每个 code/schema_version 最多一个活动版本。
func (r *Registry) Register(entry Entry, active bool) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return ErrRegistryFrozen
	}
	key := metricKey{code: entry.Code, schemaVersion: entry.SchemaVersion}
	version := versionKey{metricKey: key, algorithmVersion: entry.AlgorithmVersion}
	if _, exists := r.entries[version]; exists {
		return ErrDuplicateVersion
	}
	if active {
		if _, exists := r.active[key]; exists {
			return ErrActiveVersionExists
		}
	}
	r.entries[version] = cloneEntry(entry)
	if active {
		r.active[key] = entry.AlgorithmVersion
	}
	return nil
}

func (r *Registry) LookupActive(code, schemaVersion string) (Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := metricKey{code: code, schemaVersion: schemaVersion}
	algorithmVersion, exists := r.active[key]
	if !exists {
		return Entry{}, ErrNoActiveVersion
	}
	entry, exists := r.entries[versionKey{metricKey: key, algorithmVersion: algorithmVersion}]
	if !exists {
		return Entry{}, ErrUnknownEntry
	}
	return cloneEntry(entry), nil
}

func (r *Registry) LookupVersion(code, schemaVersion, algorithmVersion string) (Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, exists := r.entries[versionKey{
		metricKey: metricKey{code: code, schemaVersion: schemaVersion},
		algorithmVersion: algorithmVersion,
	}]
	if !exists {
		return Entry{}, ErrUnknownEntry
	}
	return cloneEntry(entry), nil
}

func (r *Registry) Freeze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

func (r *Registry) IsFrozen() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frozen
}

// Entries 返回稳定排序的深拷贝快照，不暴露内部可变切片或 JSON buffer。
func (r *Registry) Entries() []Entry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	keys := make([]versionKey, 0, len(r.entries))
	for key := range r.entries {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].code != keys[j].code {
			return keys[i].code < keys[j].code
		}
		if keys[i].schemaVersion != keys[j].schemaVersion {
			return keys[i].schemaVersion < keys[j].schemaVersion
		}
		return keys[i].algorithmVersion < keys[j].algorithmVersion
	})
	entries := make([]Entry, 0, len(keys))
	for _, key := range keys {
		entries = append(entries, cloneEntry(r.entries[key]))
	}
	return entries
}

func cloneEntry(entry Entry) Entry {
	entry.Precision.Tolerance = bytes.Clone(entry.Precision.Tolerance)
	entry.Range.Minimum = bytes.Clone(entry.Range.Minimum)
	entry.Range.Maximum = bytes.Clone(entry.Range.Maximum)
	entry.ParametersSchema = bytes.Clone(entry.ParametersSchema)
	entry.InputRequirements = append([]InputRequirement(nil), entry.InputRequirements...)
	for i := range entry.InputRequirements {
		entry.InputRequirements[i].Fields = append([]string(nil), entry.InputRequirements[i].Fields...)
		entry.InputRequirements[i].Quality = append([]string(nil), entry.InputRequirements[i].Quality...)
	}
	entry.QualityPolicy.QualityStates = append([]string(nil), entry.QualityPolicy.QualityStates...)
	entry.QualityPolicy.UnavailableConditions = append([]string(nil), entry.QualityPolicy.UnavailableConditions...)
	return entry
}
