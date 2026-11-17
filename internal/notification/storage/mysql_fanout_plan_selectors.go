package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	"example.com/stock-analysis-flow/internal/notification/template"
)

// MySQLFanoutChannelConfigSelector selects the greatest persisted config version.
// A disabled newest version is returned as disabled; callers must not fall back.
type MySQLFanoutChannelConfigSelector struct {
	db         *sql.DB
	repository application.ChannelConfigRepository
}

func NewMySQLFanoutChannelConfigSelector(db *sql.DB, repository application.ChannelConfigRepository) (*MySQLFanoutChannelConfigSelector, error) {
	if db == nil || repository == nil {
		return nil, application.ErrChannelConfigRepositoryUnavailable
	}
	return &MySQLFanoutChannelConfigSelector{db: db, repository: repository}, nil
}

func (s *MySQLFanoutChannelConfigSelector) ResolveForFanout(ctx context.Context, subscription domain.Subscription) (domain.ChannelConfigSnapshot, error) {
	if s == nil || s.db == nil || s.repository == nil || ctx == nil || subscription.ChannelRef == "" {
		return domain.ChannelConfigSnapshot{}, application.ErrChannelConfigRepositoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.ChannelConfigSnapshot{}, err
	}
	var version int64
	err := s.db.QueryRowContext(ctx, "SELECT config_version FROM notification_channel_config_versions WHERE channel_ref = ? ORDER BY config_version DESC LIMIT 1", subscription.ChannelRef).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ChannelConfigSnapshot{}, application.ErrChannelConfigNotFound
	}
	if err != nil {
		return domain.ChannelConfigSnapshot{}, fmt.Errorf("notification storage: select latest channel config version: %w", err)
	}
	return application.LoadChannelConfigSnapshot(ctx, s.repository, subscription.ChannelRef, version)
}

// MySQLFanoutTemplateVersionSelector selects the greatest template version compatible
// with the exact event_type and schema_version.
type MySQLFanoutTemplateVersionSelector struct {
	db         *sql.DB
	repository template.Repository
}

func NewMySQLFanoutTemplateVersionSelector(db *sql.DB, repository template.Repository) (*MySQLFanoutTemplateVersionSelector, error) {
	if db == nil || repository == nil {
		return nil, errors.New("notification storage: template selector requires database and repository")
	}
	return &MySQLFanoutTemplateVersionSelector{db: db, repository: repository}, nil
}

func (s *MySQLFanoutTemplateVersionSelector) ResolveForFanout(ctx context.Context, templateID, eventType, schemaVersion string) (int64, error) {
	if s == nil || s.db == nil || s.repository == nil || ctx == nil ||
		templateID == "" || eventType == "" || schemaVersion == "" {
		return 0, template.ErrUnknownTemplate
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	var version int64
	err := s.db.QueryRowContext(ctx, "SELECT version FROM notification_template_versions WHERE template_id = ? AND event_type = ? AND schema_version = ? ORDER BY version DESC LIMIT 1", templateID, eventType, schemaVersion).Scan(&version)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, template.ErrUnknownTemplate
	}
	if err != nil {
		return 0, fmt.Errorf("notification storage: select latest compatible template version: %w", err)
	}
	definition, _, found, err := s.repository.Get(ctx, templateID, version)
	if err != nil {
		return 0, err
	}
	if !found || definition.TemplateID != templateID || definition.Version != version ||
		definition.EventType != eventType || definition.SchemaVersion != schemaVersion {
		return 0, template.ErrRepositoryDigestMismatch
	}
	return version, nil
}

var _ application.FanoutChannelConfigSelector = (*MySQLFanoutChannelConfigSelector)(nil)
var _ application.FanoutTemplateVersionSelector = (*MySQLFanoutTemplateVersionSelector)(nil)
