package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var (
	ErrWorkflowTemplateNotFound = errors.New("Researchflow workflow template version not found")
	ErrWorkflowTemplateConflict = errors.New("Researchflow workflow template version is immutable")
	ErrWorkflowTemplateCorrupt  = errors.New("Researchflow workflow template record is corrupt")
)

var _ application.TemplateRegistry = (*MySQLTemplateRegistry)(nil)

// MySQLTemplateRegistry stores immutable, exact workflow template versions.
// The caller owns the database connection and migration lifecycle.
type MySQLTemplateRegistry struct {
	db    *sql.DB
	gates template.HardRiskGateRegistry
}

func NewMySQLTemplateRegistry(db *sql.DB, gates template.HardRiskGateRegistry) (*MySQLTemplateRegistry, error) {
	if db == nil {
		return nil, errors.New("Researchflow Template Registry SQL database is required")
	}
	return &MySQLTemplateRegistry{db: db, gates: gates}, nil
}

// Get never resolves aliases or latest versions. A missing exact version is
// distinguishable from a corrupt persisted definition.
func (r *MySQLTemplateRegistry) Get(ctx context.Context, ref, version string) (template.WorkflowTemplate, error) {
	if r == nil || r.db == nil {
		return template.WorkflowTemplate{}, errors.New("Researchflow Template Registry is not configured")
	}
	if !validRegistryIdentity(ref) || !validTemplateVersion(version) {
		return template.WorkflowTemplate{}, fmt.Errorf("%w: template ref and version are required", template.ErrInvalidTemplate)
	}
	var fingerprint, definition string
	err := r.db.QueryRowContext(ctx, `SELECT fingerprint, definition
		FROM researchflow_workflow_templates WHERE template_ref = ? AND version = ?`, ref, version,
	).Scan(&fingerprint, &definition)
	if errors.Is(err, sql.ErrNoRows) {
		return template.WorkflowTemplate{}, fmt.Errorf("%w: %s@%s", ErrWorkflowTemplateNotFound, ref, version)
	}
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("read Researchflow workflow template %s@%s: %w", ref, version, err)
	}
	var result template.WorkflowTemplate
	decoder := json.NewDecoder(strings.NewReader(definition))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("%w: decode %s@%s: %v", ErrWorkflowTemplateCorrupt, ref, version, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return template.WorkflowTemplate{}, fmt.Errorf("%w: trailing content in %s@%s", ErrWorkflowTemplateCorrupt, ref, version)
	}
	if result.Ref != ref || result.Version != version {
		return template.WorkflowTemplate{}, fmt.Errorf("%w: stored identity does not match %s@%s", ErrWorkflowTemplateCorrupt, ref, version)
	}
	actual, err := template.Fingerprint(result)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("%w: fingerprint %s@%s: %v", ErrWorkflowTemplateCorrupt, ref, version, err)
	}
	if !validSHA256Fingerprint(fingerprint) || actual != fingerprint ||
		(result.Fingerprint != "" && result.Fingerprint != fingerprint) {
		return template.WorkflowTemplate{}, fmt.Errorf("%w: fingerprint mismatch for %s@%s", ErrWorkflowTemplateCorrupt, ref, version)
	}
	if err := template.Validate(result, r.gates); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("%w: validate %s@%s: %v", ErrWorkflowTemplateCorrupt, ref, version, err)
	}
	result.Fingerprint = fingerprint
	return cloneStoredWorkflowTemplate(result), nil
}

// Publish validates and inserts a version once. Re-publishing identical
// content is idempotent; changing content under the same identity is rejected.
func (r *MySQLTemplateRegistry) Publish(ctx context.Context, value template.WorkflowTemplate) error {
	if r == nil || r.db == nil {
		return errors.New("Researchflow Template Registry is not configured")
	}
	if !validRegistryIdentity(value.Ref) || !validTemplateVersion(value.Version) {
		return fmt.Errorf("%w: template ref and version are required", template.ErrInvalidTemplate)
	}
	if err := template.Validate(value, r.gates); err != nil {
		return err
	}
	fingerprint, err := template.Fingerprint(value)
	if err != nil {
		return fmt.Errorf("fingerprint Researchflow workflow template: %w", err)
	}
	if value.Fingerprint != "" && value.Fingerprint != fingerprint {
		return fmt.Errorf("%w: supplied template fingerprint does not match definition", ErrWorkflowTemplateCorrupt)
	}
	value = cloneStoredWorkflowTemplate(value)
	value.Fingerprint = fingerprint
	definition, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode Researchflow workflow template: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("begin Researchflow template publication: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	createdAt, err := readDatabaseUTC(tx, ctx)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO researchflow_workflow_templates
		(template_ref, version, fingerprint, definition, created_at)
		VALUES (?, ?, ?, ?, ?)`,
		value.Ref, value.Version, fingerprint, string(definition), formatResearchflowTimeValue(createdAt))
	if err != nil {
		if !isMySQLDuplicateKey(err) {
			return fmt.Errorf("insert Researchflow workflow template %s@%s: %w", value.Ref, value.Version, err)
		}
		_ = tx.Rollback()
		existing, readErr := r.Get(ctx, value.Ref, value.Version)
		if readErr != nil {
			return fmt.Errorf("resolve duplicate Researchflow template publication: %w", readErr)
		}
		if existing.Fingerprint == fingerprint {
			return nil
		}
		return fmt.Errorf("%w: %s@%s", ErrWorkflowTemplateConflict, value.Ref, value.Version)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit Researchflow workflow template publication: %w", err)
	}
	return nil
}

func validTemplateVersion(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && len(value) <= 64
}

func validRegistryIdentity(value string) bool {
	return strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value && len(value) <= 255
}

func cloneStoredWorkflowTemplate(value template.WorkflowTemplate) template.WorkflowTemplate {
	value.Stages = append([]template.StageDefinition(nil), value.Stages...)
	sort.Slice(value.Stages, func(i, j int) bool { return value.Stages[i].Code < value.Stages[j].Code })
	for i := range value.Stages {
		value.Stages[i].Dependencies = append([]string(nil), value.Stages[i].Dependencies...)
		sort.Strings(value.Stages[i].Dependencies)
		value.Stages[i].Retry.RetryableReasonCodes = append([]string(nil), value.Stages[i].Retry.RetryableReasonCodes...)
		sort.Strings(value.Stages[i].Retry.RetryableReasonCodes)
	}
	return value
}
