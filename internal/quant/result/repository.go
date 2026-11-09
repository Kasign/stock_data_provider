package result

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/quant/manifest"
)

var ErrResultConflict = errors.New("quant result identity conflict")

// Record 是持久化边界中的不可变结果载荷。
// Payload 保持原始 JSON 字节；在 Envelope/各 result_type payload 子 Schema 冻结前，
// Repository 不解释或重塑其业务结构。
type Record struct {
	Manifest manifest.Manifest
	Payload  json.RawMessage
}

// Validate 校验 Manifest 与 payload 的内容寻址绑定。
func (record Record) Validate() error {
	if err := record.Manifest.VerifyPayload(record.Payload); err != nil {
		return fmt.Errorf("validate quant result record: %w", err)
	}
	return nil
}

// Repository 是 Quant 结果的写入端口，不暴露未完成授权复核的读取能力。
// Save 必须满足：相同 result_id 且内容相同视为幂等成功；相同 result_id 但内容不同返回 ErrResultConflict。
// 实现方必须以单一事务/原子操作保证唯一性检查与写入不可分割。
type Repository interface {
	Save(ctx context.Context, record Record) error
}
