package result

import (
	"context"
	"errors"
	"sync"
)

// MemoryRepository 是 Repository 的并发安全内存实现，供单测和本地组合测试使用。
// 它不提供读取 API，不替代 MariaDB/Platform Storage，也不具备进程重启后的持久性。
type MemoryRepository struct {
	mu      sync.Mutex
	records map[string]Record
}

// NewMemoryRepository 创建空的内存结果仓库。
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{records: make(map[string]Record)}
}

// Save 使用锁将唯一性检查与写入合并为原子操作。
// 相同 result_id、manifest fingerprint 和 payload fingerprint 视为幂等成功；
// 相同 result_id 对应不同内容返回 ErrResultConflict。
func (repository *MemoryRepository) Save(ctx context.Context, record Record) error {
	if repository == nil {
		return errors.New("nil quant memory repository")
	}
	if ctx == nil {
		return errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	snapshot, err := record.ValidatedSnapshot()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	repository.mu.Lock()
	defer repository.mu.Unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	if existing, exists := repository.records[snapshot.Manifest.ResultID]; exists {
		if existing.Manifest.ManifestFingerprint == snapshot.Manifest.ManifestFingerprint &&
			existing.Manifest.ResultFingerprint == snapshot.Manifest.ResultFingerprint {
			return nil
		}
		return ErrResultConflict
	}
	repository.records[snapshot.Manifest.ResultID] = snapshot
	return nil
}
