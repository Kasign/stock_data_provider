package schema

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
	"github.com/gowebpki/jcs"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	envelopeSchemaFile = "notification-event-envelope-1.0.0.schema.json"
	envelopeSchemaID   = "urn:kasign:stock_data_provider:notification:envelope:1.0.0"
)

var (
	ErrUnknownSchema              = errors.New("notification: unknown event schema")
	ErrSourceModuleMismatch       = errors.New("notification: event source module does not match schema")
	ErrSchemaValidation           = errors.New("notification: event schema validation failed")
	ErrSchemaImmutable            = errors.New("notification: registered schema digest is immutable")
	ErrInvalidEventJSON           = errors.New("notification: invalid event JSON")
	ErrInvalidResourceLimits      = errors.New("notification: invalid event resource limits")
	ErrEventResourceLimitExceeded = errors.New("notification: event resource limit exceeded")
)

type SchemaKey struct {
	EventType string
	Version   string
}

type schemaResource struct {
	file         string
	key          SchemaKey
	sourceModule string
}

var payloadResources = []schemaResource{
	{
		file:         "researchflow-run-terminal-1.0.0.schema.json",
		key:          SchemaKey{EventType: "researchflow.run.terminal", Version: "1.0.0"},
		sourceModule: "researchflow",
	},
	{
		file:         "quant-risk-decision-changed-1.0.0.schema.json",
		key:          SchemaKey{EventType: "quant.risk_decision.changed", Version: "1.0.0"},
		sourceModule: "quant",
	},
	{
		file:         "research-event-lifecycle-changed-1.0.0.schema.json",
		key:          SchemaKey{EventType: "research.event.lifecycle_changed", Version: "1.0.0"},
		sourceModule: "research",
	},
}

type registryEntry struct {
	sourceModule string
	digest       string
	validator    *jsonschema.Schema
}

// Registry is read-only after construction and safe for concurrent validation.
type ResourceLimits struct {
	MaxEnvelopeBytes int
	MaxPayloadBytes  int
	MaxJSONDepth     int
}

func (l ResourceLimits) Validate() error {
	if l.MaxEnvelopeBytes <= 0 || l.MaxPayloadBytes <= 0 ||
		l.MaxPayloadBytes > l.MaxEnvelopeBytes || l.MaxJSONDepth <= 0 {
		return ErrInvalidResourceLimits
	}
	return nil
}

type Registry struct {
	envelope *jsonschema.Schema
	entries  map[SchemaKey]registryEntry
	limits   ResourceLimits
}

func NewRegistry(schemaFS fs.FS, limits ResourceLimits) (*Registry, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	if schemaFS == nil {
		return nil, errors.New("notification: schema filesystem is required")
	}

	compiler := jsonschema.NewCompiler()
	compiler.RegisterFormat(&jsonschema.Format{
		Name: "date-time",
		Validate: func(value any) error {
			text, ok := value.(string)
			if !ok {
				return nil
			}
			if !strings.HasSuffix(text, "Z") {
				return fmt.Errorf("date-time must use UTC Z suffix")
			}
			if _, err := time.Parse(time.RFC3339Nano, text); err != nil {
				return fmt.Errorf("invalid RFC3339Nano date-time: %w", err)
			}
			return nil
		},
	})
	compiler.AssertFormat()

	registry := &Registry{entries: make(map[SchemaKey]registryEntry), limits: limits}
	resources := append([]schemaResource(nil), payloadResources...)
	resources = append(resources, schemaResource{file: envelopeSchemaFile})

	for _, resource := range resources {
		raw, err := fs.ReadFile(schemaFS, resource.file)
		if err != nil {
			return nil, fmt.Errorf("notification: read schema %s: %w", resource.file, err)
		}
		canonical, err := jcs.Transform(raw)
		if err != nil {
			return nil, fmt.Errorf("notification: canonicalize schema %s: %w", resource.file, err)
		}
		var document any
		if err := json.Unmarshal(canonical, &document); err != nil {
			return nil, fmt.Errorf("notification: decode schema %s: %w", resource.file, err)
		}
		object, ok := document.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("notification: schema %s must be an object", resource.file)
		}
		schemaID, ok := object["$id"].(string)
		if !ok || schemaID == "" {
			return nil, fmt.Errorf("notification: schema %s has no absolute $id", resource.file)
		}
		if err := compiler.AddResource(schemaID, document); err != nil {
			return nil, fmt.Errorf("notification: register schema %s: %w", resource.file, err)
		}

		if resource.key.EventType != "" {
			payloadValidator, err := compiler.Compile(schemaID)
			if err != nil {
				return nil, fmt.Errorf("notification: compile payload schema %s: %w", resource.file, err)
			}
			sum := sha256.Sum256(canonical)
			if err := registry.registerDigest(resource.key, resource.sourceModule, hex.EncodeToString(sum[:]), payloadValidator); err != nil {
				return nil, err
			}
		}
	}

	compiled, err := compiler.Compile(envelopeSchemaID)
	if err != nil {
		return nil, fmt.Errorf("notification: compile envelope schema: %w", err)
	}
	registry.envelope = compiled
	return registry, nil
}

func (r *Registry) registerDigest(key SchemaKey, sourceModule, digest string, validator *jsonschema.Schema) error {
	if previous, exists := r.entries[key]; exists {
		if previous.sourceModule != sourceModule || previous.digest != digest {
			return ErrSchemaImmutable
		}
		return nil
	}
	r.entries[key] = registryEntry{sourceModule: sourceModule, digest: digest, validator: validator}
	return nil
}

// HasEventType reports whether at least one immutable version of an event type is registered.
func (r *Registry) HasEventType(eventType string) bool {
	if r == nil || eventType == "" {
		return false
	}
	for key := range r.entries {
		if key.EventType == eventType {
			return true
		}
	}
	return false
}

func (r *Registry) SchemaDigest(key SchemaKey) (string, bool) {
	entry, ok := r.entries[key]
	return entry.digest, ok
}

// ValidateEvent strictly parses, validates, and verifies an event before it can enter Inbox.
func (r *Registry) ValidateEvent(raw []byte) (domain.EventEnvelope, error) {
	var event domain.EventEnvelope
	if r == nil || r.envelope == nil {
		return event, errors.New("notification: schema registry is not initialized")
	}
	if len(raw) > r.limits.MaxEnvelopeBytes {
		return event, ErrEventResourceLimitExceeded
	}
	if err := validateStrictJSON(raw, r.limits.MaxJSONDepth); err != nil {
		if errors.Is(err, ErrEventResourceLimitExceeded) {
			return event, err
		}
		return event, fmt.Errorf("%w: %v", ErrInvalidEventJSON, err)
	}
	var rawFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &rawFields); err != nil || rawFields == nil {
		return event, fmt.Errorf("%w: envelope must be a JSON object", ErrInvalidEventJSON)
	}
	if payload, exists := rawFields["payload"]; exists && len(payload) > r.limits.MaxPayloadBytes {
		return event, ErrEventResourceLimitExceeded
	}

	canonical, err := jcs.Transform(raw)
	if err != nil {
		return event, fmt.Errorf("%w: %v", ErrInvalidEventJSON, err)
	}
	if err := json.Unmarshal(canonical, &event); err != nil {
		return event, fmt.Errorf("%w: %v", ErrInvalidEventJSON, err)
	}

	entry, exists := r.entries[SchemaKey{EventType: event.EventType, Version: event.SchemaVersion}]
	if !exists {
		return event, ErrUnknownSchema
	}
	if entry.sourceModule != event.SourceModule {
		return event, ErrSourceModuleMismatch
	}
	if entry.validator == nil {
		return event, ErrUnknownSchema
	}
	var payload any
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return event, fmt.Errorf("%w: invalid payload: %v", ErrInvalidEventJSON, err)
	}
	if err := entry.validator.Validate(payload); err != nil {
		return event, fmt.Errorf("%w: payload: %v", ErrSchemaValidation, err)
	}

	var instance any
	if err := json.Unmarshal(canonical, &instance); err != nil {
		return event, fmt.Errorf("%w: %v", ErrInvalidEventJSON, err)
	}
	if err := r.envelope.Validate(instance); err != nil {
		return event, fmt.Errorf("%w: %v", ErrSchemaValidation, err)
	}
	if err := event.VerifyPayloadFingerprint(); err != nil {
		return event, err
	}
	return event, nil
}
