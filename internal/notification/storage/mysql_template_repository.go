package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/notification/template"
)

// MySQLTemplateRepository 持久化不可变模板版本；连接池由 Server 管理。
type MySQLTemplateRepository struct {
	db     *sql.DB
	limits template.Limits
}

var _ template.Repository = (*MySQLTemplateRepository)(nil)

func NewMySQLTemplateRepository(db *sql.DB, limits template.Limits) (*MySQLTemplateRepository, error) {
	if db == nil {
		return nil, errors.New("notification storage: database is required")
	}
	if err := limits.Validate(); err != nil {
		return nil, err
	}
	return &MySQLTemplateRepository{db: db, limits: limits}, nil
}

func (r *MySQLTemplateRepository) Create(ctx context.Context, definition template.Definition) (string, bool, error) {
	if r == nil || r.db == nil || ctx == nil {
		return "", false, errors.New("notification storage: repository is not configured")
	}
	if err := template.ValidateDefinition(definition, r.limits); err != nil {
		return "", false, err
	}
	digest, err := template.DefinitionDigest(definition)
	if err != nil {
		return "", false, err
	}
	bindings, err := json.Marshal(definition.VariableBindings)
	if err != nil {
		return "", false, fmt.Errorf("notification storage: encode template bindings: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO notification_template_versions (
		template_id, version, event_type, schema_version, format,
		subject_template, body_template, variable_bindings, digest
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		definition.TemplateID, definition.Version, definition.EventType, definition.SchemaVersion,
		string(definition.Format), definition.SubjectTemplate, definition.BodyTemplate, bindings, digest,
	)
	if err == nil {
		return digest, false, nil
	}
	if !isMySQLDuplicateKey(err) {
		return "", false, fmt.Errorf("notification storage: create template version: %w", err)
	}
	existing, existingDigest, found, loadErr := r.Get(ctx, definition.TemplateID, definition.Version)
	if loadErr != nil {
		return "", false, loadErr
	}
	if !found {
		return "", false, fmt.Errorf("notification storage: template unique constraint conflict: %w", err)
	}
	if existingDigest != digest {
		return "", false, template.ErrTemplateImmutable
	}
	// Digest 相同还需比较解码后的完整定义，防止数据库列与摘要不一致。
	if !equalTemplateDefinitions(existing, definition) {
		return "", false, errors.New("notification storage: persisted template digest/columns mismatch")
	}
	return digest, true, nil
}

func (r *MySQLTemplateRepository) Get(ctx context.Context, templateID string, version int64) (template.Definition, string, bool, error) {
	if r == nil || r.db == nil || ctx == nil || templateID == "" || version <= 0 {
		return template.Definition{}, "", false, errors.New("notification storage: repository or template identity is invalid")
	}
	var result template.Definition
	var format string
	var bindings []byte
	var storedDigest string
	err := r.db.QueryRowContext(ctx, `SELECT template_id, version, event_type, schema_version,
		format, subject_template, body_template, variable_bindings, digest
		FROM notification_template_versions WHERE template_id = ? AND version = ?`, templateID, version,
	).Scan(&result.TemplateID, &result.Version, &result.EventType, &result.SchemaVersion,
		&format, &result.SubjectTemplate, &result.BodyTemplate, &bindings, &storedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return template.Definition{}, "", false, nil
	}
	if err != nil {
		return template.Definition{}, "", false, fmt.Errorf("notification storage: get template version: %w", err)
	}
	result.Format = template.Format(format)
	if err := json.Unmarshal(bindings, &result.VariableBindings); err != nil {
		return template.Definition{}, "", false, fmt.Errorf("notification storage: decode template bindings: %w", err)
	}
	if err := template.ValidateDefinition(result, r.limits); err != nil {
		return template.Definition{}, "", false, fmt.Errorf("notification storage: persisted template definition is invalid: %w", err)
	}
	actualDigest, err := template.DefinitionDigest(result)
	if err != nil {
		return template.Definition{}, "", false, err
	}
	if actualDigest != storedDigest {
		return template.Definition{}, "", false, errors.New("notification storage: persisted template digest mismatch")
	}
	result.VariableBindings = cloneTemplateBindings(result.VariableBindings)
	return result, storedDigest, true, nil
}

func equalTemplateDefinitions(left, right template.Definition) bool {
	if left.TemplateID != right.TemplateID || left.Version != right.Version ||
		left.EventType != right.EventType || left.SchemaVersion != right.SchemaVersion ||
		left.Format != right.Format || left.SubjectTemplate != right.SubjectTemplate ||
		left.BodyTemplate != right.BodyTemplate || len(left.VariableBindings) != len(right.VariableBindings) {
		return false
	}
	for key, value := range left.VariableBindings {
		if right.VariableBindings[key] != value {
			return false
		}
	}
	return true
}

func cloneTemplateBindings(source map[string]string) map[string]string {
	if source == nil { return nil }
	result := make(map[string]string, len(source))
	for key, value := range source { result[key] = value }
	return result
}
