package app

import (
	"context"
	"errors"
	"time"
)

// ErrEvidenceSnapshotRetentionResultInvalid 表示 Snapshot 清理 Repository 返回了无效计数。
var ErrEvidenceSnapshotRetentionResultInvalid = errors.New("news.evidence.snapshot_retention_result_invalid")

// EvidenceSnapshotRetentionRepository 是固定分页 Snapshot TTL 清理端口。
type EvidenceSnapshotRetentionRepository interface {
	DeleteExpiredEvidenceSnapshots(context.Context, time.Time, int) (int64, error)
}

// EvidenceSnapshotRetentionCleaner 执行一次有界 Snapshot 清理，不负责调度。
type EvidenceSnapshotRetentionCleaner struct {
	repository EvidenceSnapshotRetentionRepository
}

// NewEvidenceSnapshotRetentionCleaner 创建固定分页 Snapshot 清理用例。
func NewEvidenceSnapshotRetentionCleaner(repository EvidenceSnapshotRetentionRepository) (*EvidenceSnapshotRetentionCleaner, error) {
	if repository == nil {
		return nil, ErrInvalidEvidenceSnapshot
	}
	return &EvidenceSnapshotRetentionCleaner{repository: repository}, nil
}

// Execute 删除 expires_at 不晚于 now 的 Snapshot；周期调度由 Server 组合根负责。
func (u *EvidenceSnapshotRetentionCleaner) Execute(ctx context.Context, now time.Time, limit int) (int64, error) {
	if u == nil || u.repository == nil || ctx == nil || now.IsZero() || limit < 1 || limit > 1000 {
		return 0, ErrInvalidEvidenceSnapshot
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	deleted, err := u.repository.DeleteExpiredEvidenceSnapshots(ctx, now.UTC(), limit)
	if err != nil {
		return 0, err
	}
	// Repository 返回成功表示删除语句已完成；此后 Context 取消不能抹去
	// 已确认的提交结果，否则调用方会把已删除记录误判为未完成。
	if deleted < 0 || deleted > int64(limit) {
		return 0, ErrEvidenceSnapshotRetentionResultInvalid
	}
	return deleted, nil
}
