package stageoutput

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidRegistry = errors.New("invalid Researchflow Stage Output Registry")
	ErrUnknownOutput   = errors.New("unregistered Researchflow Stage output")
	ErrInvalidEnvelope = errors.New("invalid Researchflow Stage Provider Result Envelope")
)

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
var codePattern = regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`)

// PayloadValidator validates one exact, immutable schema reference. Implementations
// must not perform remote schema resolution or silently fall back to another version.
type PayloadValidator interface {
	ValidateJSON(json.RawMessage) error
}

type ValidatorFunc func(json.RawMessage) error

func (f ValidatorFunc) ValidateJSON(value json.RawMessage) error { return f(value) }

// Entry binds one Stage output discriminator to its exact payload and quality schemas.
type Entry struct {
	StageCode                string
	OutputType               string
	SchemaVersion            string
	PayloadSchemaRef         string
	QualitySummarySchemaRef  string
	PayloadValidator         PayloadValidator
	QualitySummaryValidator  PayloadValidator
}

type fingerprintEntry struct {
	StageCode               string `json:"stage_code"`
	OutputType              string `json:"output_type"`
	SchemaVersion           string `json:"schema_version"`
	PayloadSchemaRef        string `json:"payload_schema_ref"`
	QualitySummarySchemaRef string `json:"quality_summary_schema_ref"`
}

type Registry struct {
	version     string
	fingerprint string
	entries     map[entryKey]Entry
}

type entryKey struct {
	stageCode     string
	outputType    string
	schemaVersion string
}

func NewRegistry(version string, entries []Entry) (*Registry, error) {
	if !versionPattern.MatchString(version) || version != strings.TrimSpace(version) || entries == nil {
		return nil, fmt.Errorf("%w: invalid version or nil entries", ErrInvalidRegistry)
	}
	copyEntries := append([]Entry(nil), entries...)
	sort.Slice(copyEntries, func(i, j int) bool {
		a, b := copyEntries[i], copyEntries[j]
		if a.StageCode != b.StageCode { return a.StageCode < b.StageCode }
		if a.OutputType != b.OutputType { return a.OutputType < b.OutputType }
		return a.SchemaVersion < b.SchemaVersion
	})
	registry := &Registry{version: version, entries: make(map[entryKey]Entry, len(copyEntries))}
	fingerprintEntries := make([]fingerprintEntry, 0, len(copyEntries))
	for _, entry := range copyEntries {
		if !codePattern.MatchString(entry.StageCode) || !codePattern.MatchString(entry.OutputType) ||
			!versionPattern.MatchString(entry.SchemaVersion) || entry.SchemaVersion != strings.TrimSpace(entry.SchemaVersion) ||
			strings.TrimSpace(entry.PayloadSchemaRef) == "" || entry.PayloadSchemaRef != strings.TrimSpace(entry.PayloadSchemaRef) ||
			strings.TrimSpace(entry.QualitySummarySchemaRef) == "" || entry.QualitySummarySchemaRef != strings.TrimSpace(entry.QualitySummarySchemaRef) ||
			entry.PayloadValidator == nil || entry.QualitySummaryValidator == nil {
			return nil, fmt.Errorf("%w: incomplete entry for stage %q", ErrInvalidRegistry, entry.StageCode)
		}
		key := entryKey{entry.StageCode, entry.OutputType, entry.SchemaVersion}
		if _, exists := registry.entries[key]; exists {
			return nil, fmt.Errorf("%w: duplicate stage/output/schema registration", ErrInvalidRegistry)
		}
		registry.entries[key] = entry
		fingerprintEntries = append(fingerprintEntries, fingerprintEntry{
			StageCode: entry.StageCode, OutputType: entry.OutputType, SchemaVersion: entry.SchemaVersion,
			PayloadSchemaRef: entry.PayloadSchemaRef, QualitySummarySchemaRef: entry.QualitySummarySchemaRef,
		})
	}
	canonical, err := json.Marshal(struct {
		Version string             `json:"version"`
		Entries []fingerprintEntry `json:"entries"`
	}{version, fingerprintEntries})
	if err != nil { return nil, fmt.Errorf("%w: fingerprint registry: %v", ErrInvalidRegistry, err) }
	sum := sha256.Sum256(canonical)
	registry.fingerprint = "sha256:" + hex.EncodeToString(sum[:])
	return registry, nil
}

func (r *Registry) Version() string {
	if r == nil { return "" }
	return r.version
}

func (r *Registry) Fingerprint() string {
	if r == nil { return "" }
	return r.fingerprint
}

// ProviderResult contains only provider-owned fields. Orchestrator identity and
// timing fields are added by BuildEnvelope and cannot be supplied by a provider.
type ProviderResult struct {
	OutputType      string
	SchemaVersion   string
	Payload         json.RawMessage
	QualitySummary  json.RawMessage
	DiagnosticRefs  []string
}

type ExecutionIdentity struct {
	RunRef          string
	StageRef        string
	AttemptRef      string
	StageCode       string
	TemplateVersion string
	RegistryVersion string
}

// Envelope is the frozen Stage Provider Result boundary. Payload and
// QualitySummary remain raw until the exact Registry entry validates them.
type Envelope struct {
	RunRef          string          `json:"run_ref"`
	StageRef        string          `json:"stage_ref"`
	AttemptRef      string          `json:"attempt_ref"`
	StageCode       string          `json:"stage_code"`
	TemplateVersion string          `json:"template_version"`
	OutputType      string          `json:"output_type"`
	SchemaVersion   string          `json:"schema_version"`
	Payload         json.RawMessage `json:"payload"`
	QualitySummary  json.RawMessage `json:"quality_summary"`
	DiagnosticRefs  []string        `json:"diagnostic_refs"`
	CompletedAt     time.Time       `json:"completed_at"`
}

func BuildEnvelope(identity ExecutionIdentity, result ProviderResult, completedAt time.Time) (Envelope, error) {
	if strings.TrimSpace(identity.RunRef) == "" || strings.TrimSpace(identity.StageRef) == "" ||
		strings.TrimSpace(identity.AttemptRef) == "" || !codePattern.MatchString(identity.StageCode) ||
		!versionPattern.MatchString(identity.TemplateVersion) || !versionPattern.MatchString(identity.RegistryVersion) ||
		!isUTC(completedAt) {
		return Envelope{}, fmt.Errorf("%w: invalid orchestrator execution identity", ErrInvalidEnvelope)
	}
	refs := append([]string{}, result.DiagnosticRefs...)
	for _, ref := range refs {
		if strings.TrimSpace(ref) == "" || ref != strings.TrimSpace(ref) {
			return Envelope{}, fmt.Errorf("%w: invalid diagnostic reference", ErrInvalidEnvelope)
		}
	}
	return Envelope{
		RunRef: identity.RunRef, StageRef: identity.StageRef, AttemptRef: identity.AttemptRef,
		StageCode: identity.StageCode, TemplateVersion: identity.TemplateVersion,
		OutputType: result.OutputType, SchemaVersion: result.SchemaVersion,
		Payload: append(json.RawMessage(nil), result.Payload...),
		QualitySummary: append(json.RawMessage(nil), result.QualitySummary...),
		DiagnosticRefs: refs, CompletedAt: completedAt,
	}, nil
}

type ValidationContext struct {
	Identity ExecutionIdentity
}

// Validate checks exact execution identity, registry version, discriminator,
// JSON structure and the two registered strong validators. There is no fallback.
func (r *Registry) Validate(envelope Envelope, context ValidationContext) error {
	if r == nil { return ErrInvalidRegistry }
	id := context.Identity
	if envelope.RunRef != id.RunRef || envelope.StageRef != id.StageRef ||
		envelope.AttemptRef != id.AttemptRef || envelope.StageCode != id.StageCode ||
		envelope.TemplateVersion != id.TemplateVersion || id.RegistryVersion != r.version ||
		!isUTC(envelope.CompletedAt) {
		return fmt.Errorf("%w: execution identity or registry version mismatch", ErrInvalidEnvelope)
	}
	key := entryKey{envelope.StageCode, envelope.OutputType, envelope.SchemaVersion}
	entry, ok := r.entries[key]
	if !ok { return fmt.Errorf("%w: %s/%s/%s", ErrUnknownOutput, key.stageCode, key.outputType, key.schemaVersion) }
	if err := validateJSONObject(envelope.Payload); err != nil {
		return fmt.Errorf("%w: payload: %v", ErrInvalidEnvelope, err)
	}
	if err := validateJSONObject(envelope.QualitySummary); err != nil {
		return fmt.Errorf("%w: quality_summary: %v", ErrInvalidEnvelope, err)
	}
	if envelope.DiagnosticRefs == nil {
		return fmt.Errorf("%w: diagnostic_refs must be an array", ErrInvalidEnvelope)
	}
	for _, ref := range envelope.DiagnosticRefs {
		if strings.TrimSpace(ref) == "" || ref != strings.TrimSpace(ref) {
			return fmt.Errorf("%w: invalid diagnostic reference", ErrInvalidEnvelope)
		}
	}
	if err := entry.PayloadValidator.ValidateJSON(append(json.RawMessage(nil), envelope.Payload...)); err != nil {
		return fmt.Errorf("%w: payload schema %q: %v", ErrInvalidEnvelope, entry.PayloadSchemaRef, err)
	}
	if err := entry.QualitySummaryValidator.ValidateJSON(append(json.RawMessage(nil), envelope.QualitySummary...)); err != nil {
		return fmt.Errorf("%w: quality summary schema %q: %v", ErrInvalidEnvelope, entry.QualitySummarySchemaRef, err)
	}
	return nil
}

func validateJSONObject(raw json.RawMessage) error {
	if len(raw) == 0 { return errors.New("value is required") }
	if err := rejectDuplicateJSONKeys(raw); err != nil { return err }
	var value map[string]json.RawMessage
	if err := json.Unmarshal(raw, &value); err != nil { return err }
	if value == nil { return errors.New("value must be an object") }
	return nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder); err != nil { return err }
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil { return errors.New("trailing JSON value") }
		return err
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil { return err }
	delim, ok := token.(json.Delim)
	if !ok { return nil }
	switch delim {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil { return err }
			key, ok := keyToken.(string)
			if !ok { return errors.New("object key is not a string") }
			if _, exists := seen[key]; exists { return fmt.Errorf("duplicate JSON object key %q", key) }
			seen[key] = struct{}{}
			if err := consumeJSONValue(decoder); err != nil { return err }
		}
		_, err = decoder.Token()
		return err
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil { return err }
		}
		_, err = decoder.Token()
		return err
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

func isUTC(value time.Time) bool {
	if value.IsZero() || value.Location() != time.UTC { return false }
	return strings.HasSuffix(value.Format(time.RFC3339Nano), "Z")
}
