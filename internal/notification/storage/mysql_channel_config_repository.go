package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
	"github.com/gowebpki/jcs"
)

// MySQLChannelConfigRepository 持久化不可变渠道配置快照；Secret 仅保存 SecretRef。
type MySQLChannelConfigRepository struct {
	db *sql.DB
}

var _ application.ChannelConfigRepository = (*MySQLChannelConfigRepository)(nil)

func NewMySQLChannelConfigRepository(db *sql.DB) (*MySQLChannelConfigRepository, error) {
	if db == nil {
		return nil, errors.New("notification storage: database is required")
	}
	return &MySQLChannelConfigRepository{db: db}, nil
}

func normalizeChannelConfig(snapshot domain.ChannelConfigSnapshot) domain.ChannelConfigSnapshot {
	result := snapshot
	result.RetryableErrorCodes = append([]string(nil), snapshot.RetryableErrorCodes...)
	sort.Strings(result.RetryableErrorCodes)
	return result
}

func channelConfigDigest(snapshot domain.ChannelConfigSnapshot) (string, []byte, error) {
	normalized := normalizeChannelConfig(snapshot)
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", nil, fmt.Errorf("notification storage: encode channel config: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", nil, fmt.Errorf("notification storage: canonicalize channel config: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), encoded, nil
}

func (r *MySQLChannelConfigRepository) Create(ctx context.Context, snapshot domain.ChannelConfigSnapshot) (string, bool, error) {
	if r == nil || r.db == nil || ctx == nil {
		return "", false, errors.New("notification storage: repository is not configured")
	}
	if err := snapshot.Validate(); err != nil {
		return "", false, err
	}
	digest, encoded, err := channelConfigDigest(snapshot)
	if err != nil {
		return "", false, err
	}
	_, err = r.db.ExecContext(ctx, `INSERT INTO notification_channel_config_versions (
		channel_ref, config_version, snapshot_json, digest
	) VALUES (?, ?, ?, ?)`, snapshot.ChannelRef, snapshot.ConfigVersion, encoded, digest)
	if err == nil {
		return digest, false, nil
	}
	if !isMySQLDuplicateKey(err) {
		return "", false, fmt.Errorf("notification storage: create channel config version: %w", err)
	}
	existing, existingDigest, found, loadErr := r.Get(ctx, snapshot.ChannelRef, snapshot.ConfigVersion)
	if loadErr != nil {
		return "", false, loadErr
	}
	if !found {
		return "", false, fmt.Errorf("notification storage: channel config unique constraint conflict: %w", err)
	}
	if existingDigest != digest {
		return "", false, application.ErrChannelConfigVersionConflict
	}
	return digest, true, nil
}

func (r *MySQLChannelConfigRepository) Get(ctx context.Context, channelRef string, configVersion int64) (domain.ChannelConfigSnapshot, string, bool, error) {
	if r == nil || r.db == nil || ctx == nil || channelRef == "" || configVersion <= 0 {
		return domain.ChannelConfigSnapshot{}, "", false, errors.New("notification storage: repository or channel identity is invalid")
	}
	var encoded []byte
	var storedDigest string
	err := r.db.QueryRowContext(ctx, `SELECT snapshot_json, digest FROM notification_channel_config_versions
		WHERE channel_ref = ? AND config_version = ?`, channelRef, configVersion).Scan(&encoded, &storedDigest)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ChannelConfigSnapshot{}, "", false, nil
	}
	if err != nil {
		return domain.ChannelConfigSnapshot{}, "", false, fmt.Errorf("notification storage: get channel config version: %w", err)
	}
	var snapshot domain.ChannelConfigSnapshot
	if err := json.Unmarshal(encoded, &snapshot); err != nil {
		return domain.ChannelConfigSnapshot{}, "", false, fmt.Errorf("notification storage: decode channel config: %w", err)
	}
	if err := snapshot.Validate(); err != nil {
		return domain.ChannelConfigSnapshot{}, "", false, fmt.Errorf("notification storage: persisted channel config is invalid: %w", err)
	}
	if snapshot.ChannelRef != channelRef || snapshot.ConfigVersion != configVersion {
		return domain.ChannelConfigSnapshot{}, "", false, errors.New("notification storage: persisted channel config identity mismatch")
	}
	actualDigest, _, err := channelConfigDigest(snapshot)
	if err != nil {
		return domain.ChannelConfigSnapshot{}, "", false, err
	}
	if actualDigest != storedDigest {
		return domain.ChannelConfigSnapshot{}, "", false, errors.New("notification storage: persisted channel config digest mismatch")
	}
	return normalizeChannelConfig(snapshot), storedDigest, true, nil
}
