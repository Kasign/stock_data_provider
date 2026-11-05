package diagnostics

import (
	"context"
	"errors"
	"strings"
	"time"
)

const (
	defaultCheckTimeout = 5 * time.Second
	maxCheckIDLength    = 128
)

// CheckFunc 必须遵守 context 取消约定；Runner 无法强制终止忽略 context 的检查实现。
type CheckFunc func(context.Context) (Observation, error)

// Check 描述一个可独立执行的诊断操作。
type Check struct {
	ID       string
	Name     string
	Required bool
	Timeout  time.Duration
	Run      CheckFunc
}

func (c Check) validate() error {
	if strings.TrimSpace(c.ID) == "" || len(c.ID) > maxCheckIDLength {
		return errors.New("check ID is required and must be at most 128 bytes")
	}
	if c.Run == nil {
		return errors.New("check function is required")
	}
	if c.Timeout < 0 {
		return errors.New("check timeout cannot be negative")
	}
	return nil
}
