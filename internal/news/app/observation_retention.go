package app

import (
	"context"
	"errors"
	"time"
)

// ErrObservationRetentionResultInvalid 表示 Repository 返回了不可能的批量清理计数。
var ErrObservationRetentionResultInvalid = errors.New("news.observation.retention_result_invalid")

// ObservationPayloadPurgeResult 描述单次批量载荷清理结果。
// Scanned 是本批事务成功领取并锁定的到期记录数，不是全局 backlog 数。
// Purged 是本批实际清除载荷的记录数；全局剩余量应通过 ObservationRetentionBacklog 查询。
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
	if err := ctx.Err(); err != nil {
		return ObservationPayloadPurgeResult{}, err
	}
	result, err := u.repository.PurgeExpiredObservationPayloads(ctx, now.UTC(), limit)
	if err != nil {
		return ObservationPayloadPurgeResult{}, err
	}
	// Repository 返回成功表示事务已提交；提交后观察到 Context 取消不能
	// 将已确认的清理结果改报为取消，避免调用方误判并重复处理。
	if result.Scanned < 0 || result.Purged < 0 || result.Purged > result.Scanned || result.Scanned > limit {
		return ObservationPayloadPurgeResult{}, ErrObservationRetentionResultInvalid
	}
	return result, nil
}
