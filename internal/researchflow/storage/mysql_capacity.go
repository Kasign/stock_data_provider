package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	// ErrQueueCapacityExceeded 表示全局排队 Run 容量已满；CreateRun 事务不会留下部分记录。
	ErrQueueCapacityExceeded = errors.New("Researchflow queued Run capacity exceeded")
	// ErrGlobalWorkerCapacityExceeded 表示全局有效 Worker Lease 已达到配置上限。
	ErrGlobalWorkerCapacityExceeded = errors.New("Researchflow global Worker capacity exceeded")
)

// CapacityLimits 是单个 Researchflow 部署实例共享的全局资源上限。
// 两个值都必须由 Server 配置显式提供，不提供隐式默认值。
type CapacityLimits struct {
	MaxQueuedRuns int
	MaxWorkers    int
}

func (c CapacityLimits) Validate() error {
	if c.MaxQueuedRuns < 1 || c.MaxWorkers < 1 {
		return errors.New("Researchflow MaxQueuedRuns and MaxWorkers must both be explicitly positive")
	}
	return nil
}

// lockCapacityGuard 必须在任何 Run/Stage/Attempt 行锁之前调用。
// CreateRun 和 ClaimAttempt 共用此锁，使容量计数与资源占用串行化。
func lockCapacityGuard(ctx context.Context, tx *sql.Tx) error {
	var guardID uint8
	if err := tx.QueryRowContext(ctx, `SELECT guard_id FROM researchflow_capacity_guard
		WHERE guard_id = 1 FOR UPDATE`).Scan(&guardID); err != nil {
		return fmt.Errorf("Researchflow capacity guard is unavailable; apply migration v4 before serving requests: %w", err)
	}
	if guardID != 1 {
		return errors.New("Researchflow capacity guard singleton row is invalid")
	}
	return nil
}
