package app

import (
	"context"
	"time"
)

// ObservationPayloadPurgeResult 描述单次批量载荷清理结果。
type ObservationPayloadPurgeResult struct {
	Scanned int
	Purged  int
}

// ObservationRetentionRepository 是 Observation 保留期清理的持久化端口。
type ObservationRetentionRepository interface {
	PurgeExpiredObservationPayloads(context.Context, time.Time, int) (ObservationPayloadPurgeResult, error)
}

// ObservationRetentionCleaner 执行一次有界的到期载荷清理，不负责调度。
type ObservationRetentionCleaner struct {
	repository ObservationRetentionRepository
}

// NewObservationRetentionCleaner 创建 Observation 到期清理用例。
func NewObservationRetentionCleaner(repository ObservationRetentionRepository) (*ObservationRetentionCleaner, error) {
	if repository == nil {
		return nil, ErrInvalidRequest
	}
	return &ObservationRetentionCleaner{repository: repository}, nil
}

// Execute 清理截至 now 已到期的候选载荷；调用方负责周期调度和运行诊断。
func (u *ObservationRetentionCleaner) Execute(ctx context.Context, now time.Time, limit int) (ObservationPayloadPurgeResult, error) {
	if u == nil || u.repository == nil || ctx == nil || now.IsZero() || limit < 1 || limit > 1000 {
		return ObservationPayloadPurgeResult{}, ErrInvalidRequest
	}
	result, err := u.repository.PurgeExpiredObservationPayloads(ctx, now.UTC(), limit)
	if err != nil {
		return ObservationPayloadPurgeResult{}, err
	}
	if result.Scanned < 0 || result.Purged < 0 || result.Purged > result.Scanned || result.Scanned > limit {
		return ObservationPayloadPurgeResult{}, ErrObservationRetentionResultInvalid
	}
	return result, nil
}
