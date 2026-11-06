package template

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	htmltemplate "html/template"
	"io"
	"strconv"
	"strings"
	"sync"
	texttemplate "text/template"
	"text/template/parse"
	"unicode/utf8"

	"example.com/stock-analysis-flow/internal/notification/domain"
	"example.com/stock-analysis-flow/internal/notification/schema"
	"github.com/gowebpki/jcs"
)

type Format = domain.TemplateFormat

const (
	FormatText = domain.TemplateFormatText
	FormatHTML = domain.TemplateFormatHTML
)

var (
	ErrInvalidTemplateLimits       = errors.New("notification: invalid template limits")
	ErrInvalidTemplateDefinition   = errors.New("notification: invalid template definition")
	ErrTemplateImmutable           = errors.New("notification: template version is immutable")
	ErrUnknownTemplate             = errors.New("notification: unknown template version")
	ErrTemplateEventMismatch       = errors.New("notification: template event schema mismatch")
	ErrTemplateVariableNotAllowed = errors.New("notification: template variable is not allowlisted")
	ErrUnusedTemplateBinding       = errors.New("notification: unused template variable binding")
	ErrUnsupportedTemplateSyntax  = errors.New("notification: unsupported template syntax")
	ErrTemplateBindingMissing     = errors.New("notification: template binding path is missing")
	ErrTemplateBindingNotScalar   = errors.New("notification: template binding must resolve to a scalar")
	ErrTemplateResourceLimit      = errors.New("notification: template resource limit exceeded")
	ErrInvalidTemplateSubject     = errors.New("notification: invalid template subject")
)

type Limits struct {
	MaxTemplateBytes int
	MaxOutputBytes   int
	MaxPayloadDepth  int
}

func (l Limits) Validate() error {
	if l.MaxTemplateBytes <= 0 || l.MaxOutputBytes <= 0 || l.MaxPayloadDepth <= 0 {
		return ErrInvalidTemplateLimits
	}
	return nil
}

// Definition binds one immutable template version to one exact event schema.
// VariableBindings maps a template variable name to a JSON Pointer relative to payload.
type Definition struct {
	TemplateID       string            `json:"template_id"`
	Version          int64             `json:"version"`
	EventType        string            `json:"event_type"`
	SchemaVersion    string            `json:"schema_version"`
	Format           Format            `json:"format"`
	SubjectTemplate  string            `json:"subject_template,omitempty"`
	BodyTemplate     string            `json:"body_template"`
	VariableBindings map[string]string `json:"variable_bindings"`
}

type templateKey struct {
	id      string
	version int64
}

type compiledTemplate struct {
	definition Definition
	digest     string
	subject    *texttemplate.Template
	textBody   *texttemplate.Template
	htmlBody   *htmltemplate.Template
}

type Registry struct {
	mu        sync.RWMutex
	limits    Limits
	templates map[templateKey]*compiledTemplate
}

func NewRegistry(limits Limits) (*Registry, error) {
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return &Registry{limits: limits, templates: make(map[templateKey]*compiledTemplate)}, nil
}

func (r *Registry) Register(definition Definition) (string, error) {
	if r == nil {
		return "", ErrInvalidTemplateLimits
	}
	if err := validateDefinition(definition, r.limits); err != nil {
		return "", err
	}
	digest, err := definitionDigest(definition)
	if err != nil {
		return "", err
	}
	compiled, err := compileDefinition(definition, digest)
	if err != nil {
		return "", err
	}

	key := templateKey{id: definition.TemplateID, version: definition.Version}
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.templates[key]; ok {
		if existing.digest != digest {
			return "", ErrTemplateImmutable
		}
		return digest, nil
	}
	r.templates[key] = compiled
	return digest, nil
}

func (r *Registry) Digest(templateID string, version int64) (string, bool) {
	if r == nil {
		return "", false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	compiled, ok := r.templates[templateKey{id: templateID, version: version}]
	if !ok {
		return "", false
	}
	return compiled.digest, true
}

func (r *Registry) Render(ctx context.Context, event domain.EventEnvelope, templateID string, version int64) (domain.RenderedContent, error) {
	var result domain.RenderedContent
	if r == nil {
		return result, ErrInvalidTemplateLimits
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}

	r.mu.RLock()
	compiled, ok := r.templates[templateKey{id: templateID, version: version}]
	r.mu.RUnlock()
	if !ok {
		return result, ErrUnknownTemplate
	}
	if event.EventType != compiled.definition.EventType ||
		event.SchemaVersion != compiled.definition.SchemaVersion {
		return result, ErrTemplateEventMismatch
	}
	if err := schema.ValidateStrictJSON(event.Payload, r.limits.MaxPayloadDepth); err != nil {
		return result, err
	}
	if err := event.VerifyPayloadFingerprint(); err != nil {
		return result, err
	}

	values, err := resolveBindings(event.Payload, compiled.definition.VariableBindings)
	if err != nil {
		return result, err
	}

	subjectWriter := newLimitedWriter(r.limits.MaxOutputBytes)
	if compiled.subject != nil {
		if err := compiled.subject.Execute(subjectWriter, values); err != nil {
			if subjectWriter.exceeded {
				return result, ErrTemplateResourceLimit
			}
			return result, fmt.Errorf("notification: render template subject: %w", err)
		}
	}

	remaining := r.limits.MaxOutputBytes - subjectWriter.Len()
	bodyWriter := newLimitedWriter(remaining)
	switch {
	case compiled.htmlBody != nil:
		if err := compiled.htmlBody.Execute(bodyWriter, values); err != nil {
			if bodyWriter.exceeded {
				return result, ErrTemplateResourceLimit
			}
			return result, fmt.Errorf("notification: render HTML template body: %w", err)
		}
	case compiled.textBody != nil:
		if err := compiled.textBody.Execute(bodyWriter, values); err != nil {
			if bodyWriter.exceeded {
				return result, ErrTemplateResourceLimit
			}
			return result, fmt.Errorf("notification: render text template body: %w", err)
		}
	default:
		return result, ErrInvalidTemplateDefinition
	}

	subject := subjectWriter.String()
	if containsControlCharacter(subject) {
		return result, ErrInvalidTemplateSubject
	}

	result = domain.RenderedContent{
		TemplateID:              templateID,
		TemplateVersion:         version,
		TemplateDigest:          compiled.digest,
		EventPayloadFingerprint: event.PayloadFingerprint,
		Format:                  compiled.definition.Format,
		Subject:                 subject,
		Body:                    bodyWriter.String(),
	}
	return result, nil
}

func validateDefinition(definition Definition, limits Limits) error {
	if strings.TrimSpace(definition.TemplateID) == "" || definition.TemplateID != strings.TrimSpace(definition.TemplateID) ||
		len(definition.TemplateID) > 128 || len(definition.EventType) > 128 ||
		len(definition.SchemaVersion) > 64 || definition.Version <= 0 ||
		strings.TrimSpace(definition.EventType) == "" || definition.EventType != strings.TrimSpace(definition.EventType) ||
		strings.TrimSpace(definition.SchemaVersion) == "" || definition.SchemaVersion != strings.TrimSpace(definition.SchemaVersion) ||
		definition.BodyTemplate == "" ||
		(definition.Format != FormatText && definition.Format != FormatHTML) {
		return ErrInvalidTemplateDefinition
	}
	if !utf8.ValidString(definition.TemplateID) || !utf8.ValidString(definition.EventType) ||
		!utf8.ValidString(definition.SchemaVersion) ||
		!utf8.ValidString(definition.SubjectTemplate) || !utf8.ValidString(definition.BodyTemplate) {
		return ErrInvalidTemplateDefinition
	}
	templateBytes := len(definition.TemplateID) + len(definition.EventType) +
		len(definition.SchemaVersion) + len(definition.SubjectTemplate) + len(definition.BodyTemplate)
	for name, pointer := range definition.VariableBindings {
		templateBytes += len(name) + len(pointer)
	}
	if templateBytes > limits.MaxTemplateBytes {
		return ErrTemplateResourceLimit
	}

	for name, pointer := range definition.VariableBindings {
		if !validVariableName(name) || !utf8.ValidString(pointer) {
			return ErrInvalidTemplateDefinition
		}
		if _, err := parseJSONPointer(pointer); err != nil {
			return ErrInvalidTemplateDefinition
		}
	}

	used := make(map[string]struct{})
	for _, source := range []string{definition.SubjectTemplate, definition.BodyTemplate} {
		if source == "" {
			continue
		}
		variables, err := validateTemplateSource(source, definition.VariableBindings)
		if err != nil {
			return err
		}
		for name := range variables {
			used[name] = struct{}{}
		}
	}
	for name := range definition.VariableBindings {
		if _, exists := used[name]; !exists {
			return fmt.Errorf("%w: %s", ErrUnusedTemplateBinding, name)
		}
	}
	return nil
}

func compileDefinition(definition Definition, digest string) (*compiledTemplate, error) {
	compiled := &compiledTemplate{
		definition: definition,
		digest:     digest,
	}
	compiled.definition.VariableBindings = cloneBindings(definition.VariableBindings)

	if definition.SubjectTemplate != "" {
		subject, err := texttemplate.New("subject").Option("missingkey=error").Parse(definition.SubjectTemplate)
		if err != nil {
			return nil, fmt.Errorf("%w: subject: %v", ErrInvalidTemplateDefinition, err)
		}
		compiled.subject = subject
	}

	switch definition.Format {
	case FormatText:
		body, err := texttemplate.New("body").Option("missingkey=error").Parse(definition.BodyTemplate)
		if err != nil {
			return nil, fmt.Errorf("%w: body: %v", ErrInvalidTemplateDefinition, err)
		}
		compiled.textBody = body
	case FormatHTML:
		body, err := htmltemplate.New("body").Option("missingkey=error").Parse(definition.BodyTemplate)
		if err != nil {
			return nil, fmt.Errorf("%w: body: %v", ErrInvalidTemplateDefinition, err)
		}
		compiled.htmlBody = body
	}
	return compiled, nil
}

func validateTemplateSource(source string, bindings map[string]string) (map[string]struct{}, error) {
	trees, err := parse.Parse("notification", source, "{{", "}}")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTemplateDefinition, err)
	}
	if len(trees) != 1 || trees["notification"] == nil {
		return nil, ErrUnsupportedTemplateSyntax
	}

	used := make(map[string]struct{})
	for _, node := range trees["notification"].Root.Nodes {
		switch action := node.(type) {
		case *parse.TextNode:
			continue
		case *parse.ActionNode:
			if action.Pipe == nil || len(action.Pipe.Decl) != 0 || len(action.Pipe.Cmds) != 1 {
				return nil, ErrUnsupportedTemplateSyntax
			}
			command := action.Pipe.Cmds[0]
			if len(command.Args) != 1 {
				return nil, ErrUnsupportedTemplateSyntax
			}
			field, ok := command.Args[0].(*parse.FieldNode)
			if !ok || len(field.Ident) != 1 {
				return nil, ErrUnsupportedTemplateSyntax
			}
			name := field.Ident[0]
			if _, allowed := bindings[name]; !allowed {
				return nil, fmt.Errorf("%w: %s", ErrTemplateVariableNotAllowed, name)
			}
			used[name] = struct{}{}
		default:
			return nil, ErrUnsupportedTemplateSyntax
		}
	}
	return used, nil
}

func definitionDigest(definition Definition) (string, error) {
	encoded, err := json.Marshal(definition)
	if err != nil {
		return "", fmt.Errorf("notification: encode template definition: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("notification: canonicalize template definition: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func resolveBindings(raw json.RawMessage, bindings map[string]string) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTemplateDefinition, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, ErrInvalidTemplateDefinition
	}

	values := make(map[string]any, len(bindings))
	for name, pointer := range bindings {
		segments, err := parseJSONPointer(pointer)
		if err != nil {
			return nil, ErrInvalidTemplateDefinition
		}
		value, err := resolveJSONPointer(payload, segments)
		if err != nil {
			return nil, err
		}
		switch value.(type) {
		case string, bool, json.Number:
			values[name] = value
		default:
			return nil, fmt.Errorf("%w: %s", ErrTemplateBindingNotScalar, name)
		}
	}
	return values, nil
}

func parseJSONPointer(pointer string) ([]string, error) {
	if pointer == "" || !strings.HasPrefix(pointer, "/") {
		return nil, ErrInvalidTemplateDefinition
	}
	rawSegments := strings.Split(pointer[1:], "/")
	segments := make([]string, len(rawSegments))
	for i, raw := range rawSegments {
		var decoded strings.Builder
		for j := 0; j < len(raw); j++ {
			if raw[j] != '~' {
				decoded.WriteByte(raw[j])
				continue
			}
			if j+1 >= len(raw) {
				return nil, ErrInvalidTemplateDefinition
			}
			j++
			switch raw[j] {
			case '0':
				decoded.WriteByte('~')
			case '1':
				decoded.WriteByte('/')
			default:
				return nil, ErrInvalidTemplateDefinition
			}
		}
		segments[i] = decoded.String()
	}
	return segments, nil
}

func resolveJSONPointer(root any, segments []string) (any, error) {
	current := root
	for _, segment := range segments {
		switch value := current.(type) {
		case map[string]any:
			next, ok := value[segment]
			if !ok {
				return nil, ErrTemplateBindingMissing
			}
			current = next
		case []any:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || strconv.Itoa(index) != segment || index >= len(value) {
				return nil, ErrTemplateBindingMissing
			}
			current = value[index]
		default:
			return nil, ErrTemplateBindingMissing
		}
	}
	return current, nil
}

func validVariableName(name string) bool {
	if len(name) == 0 || len(name) > 64 || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for i := 1; i < len(name); i++ {
		b := name[i]
		if (b < 'a' || b > 'z') && (b < '0' || b > '9') && b != '_' {
			return false
		}
	}
	return true
}

func cloneBindings(bindings map[string]string) map[string]string {
	if bindings == nil {
		return nil
	}
	cloned := make(map[string]string, len(bindings))
	for name, pointer := range bindings {
		cloned[name] = pointer
	}
	return cloned
}

type limitedWriter struct {
	buffer   bytes.Buffer
	maxBytes int
	exceeded bool
}

func newLimitedWriter(maxBytes int) *limitedWriter {
	return &limitedWriter{maxBytes: maxBytes}
}

func (w *limitedWriter) Write(data []byte) (int, error) {
	if len(data) > w.maxBytes-w.buffer.Len() {
		w.exceeded = true
		return 0, ErrTemplateResourceLimit
	}
	return w.buffer.Write(data)
}

func (w *limitedWriter) Len() int {
	return w.buffer.Len()
}

func (w *limitedWriter) String() string {
	return w.buffer.String()
}

func containsControlCharacter(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}
