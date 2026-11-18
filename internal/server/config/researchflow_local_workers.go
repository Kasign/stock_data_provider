package config

import (
	"fmt"
	"strconv"
	"strings"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

// LoadResearchflowLocalWorkers 读取单个进程内的 WorkerPool 并发。
// 它与数据库全局 RESEARCHFLOW_MAX_WORKERS 完全独立。
func LoadResearchflowLocalWorkers(env platformconfig.Env) (int, error) {
	raw := strings.TrimSpace(env.String("RESEARCHFLOW_LOCAL_WORKERS", ""))
	if raw == "" {
		return 0, fmt.Errorf("Researchflow Worker/All 角色启用 Capacity 时必须显式配置 RESEARCHFLOW_LOCAL_WORKERS")
	}
	workers, err := strconv.Atoi(raw)
	if err != nil || workers < 1 || workers > 32 {
		return 0, fmt.Errorf("RESEARCHFLOW_LOCAL_WORKERS 必须是 1～32 的整数")
	}
	return workers, nil
}
