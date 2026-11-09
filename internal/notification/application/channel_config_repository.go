package application

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var (
	ErrChannelConfigVersionConflict       = errors.New("notification: channel config version conflict")
	ErrChannelConfigNotFound              = errors.New("notification: channel config version not found")
	ErrChannelConfigCorrupt               = errors.New("notification: persisted channel config is corrupt")
	ErrChannelConfigRepositoryUnavailable = errors.New("notification: channel config repository is unavailable")
)

// ChannelConfigRepository 保存不可变 ChannelConfigSnapshot。
type ChannelConfigRepository interface {
	Create(ctx context.Context, snapshot domain.ChannelConfigSnapshot) (digest string, duplicate bool, err error)
	Get(ctx context.Context, channelRef string, configVersion int64) (snapshot domain.ChannelConfigSnapshot, digest string, found bool, err error)
}

// LoadChannelConfigSnapshot 按精确版本读取并校验仓储返回的快照。
// Digest 内容校验由 Repository 实现负责；此处验证摘要格式、领域约束和身份，供 Server 安全装配 Adapter。
func LoadChannelConfigSnapshot(ctx context.Context, repository ChannelConfigRepository, channelRef string, configVersion int64) (domain.ChannelConfigSnapshot, error) {
	if ctx == nil || repository == nil || strings.TrimSpace(channelRef) == "" || configVersion <= 0 {
		return domain.ChannelConfigSnapshot{}, ErrChannelConfigRepositoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return domain.ChannelConfigSnapshot{}, err
	}
	snapshot, digest, found, err := repository.Get(ctx, channelRef, configVersion)
	if err != nil {
		return domain.ChannelConfigSnapshot{}, err
	}
	if !found {
		return domain.ChannelConfigSnapshot{}, ErrChannelConfigNotFound
	}
	if snapshot.Validate() != nil || snapshot.ChannelRef != channelRef ||
		snapshot.ConfigVersion != configVersion || !validChannelConfigDigest(digest) {
		return domain.ChannelConfigSnapshot{}, ErrChannelConfigCorrupt
	}
	if err := ctx.Err(); err != nil {
		return domain.ChannelConfigSnapshot{}, err
	}
	snapshot.RetryableErrorCodes = append([]string(nil), snapshot.RetryableErrorCodes...)
	return snapshot, nil
}

func validChannelConfigDigest(value string) bool {
	if len(value) != 64 || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
