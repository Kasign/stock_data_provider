package application

import (
	"context"
	"errors"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var ErrChannelConfigVersionConflict = errors.New("notification: channel config version conflict")

// ChannelConfigRepository 保存不可变 ChannelConfigSnapshot。
type ChannelConfigRepository interface {
	Create(ctx context.Context, snapshot domain.ChannelConfigSnapshot) (digest string, duplicate bool, err error)
	Get(ctx context.Context, channelRef string, configVersion int64) (snapshot domain.ChannelConfigSnapshot, digest string, found bool, err error)
}
