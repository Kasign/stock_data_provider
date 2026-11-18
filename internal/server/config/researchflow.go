package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

// ResearchflowCapacity contains the explicit database-wide admission limits.
// It is optional until the Researchflow runtime is enabled; when configured,
// both values are mandatory and no defaults are supplied.
type ResearchflowCapacity struct {
	Configured    bool
	MaxQueuedRuns int
	MaxWorkers    int
}

// LoadResearchflowCapacity reads Server-owned Researchflow capacity settings.
// A partial configuration is rejected to prevent instances using different
// or implicit limits.
func LoadResearchflowCapacity(env platformconfig.Env) (ResearchflowCapacity, error) {
	queuedRaw := strings.TrimSpace(env.String("RESEARCHFLOW_MAX_QUEUED_RUNS", ""))
	workersRaw := strings.TrimSpace(env.String("RESEARCHFLOW_MAX_WORKERS", ""))
	if queuedRaw == "" && workersRaw == "" {
		return ResearchflowCapacity{}, nil
	}
	if queuedRaw == "" || workersRaw == "" {
		return ResearchflowCapacity{}, fmt.Errorf("RESEARCHFLOW_MAX_QUEUED_RUNS and RESEARCHFLOW_MAX_WORKERS must be configured together")
	}
	maxQueuedRuns, err := strconv.Atoi(queuedRaw)
	if err != nil || maxQueuedRuns <= 0 {
		return ResearchflowCapacity{}, fmt.Errorf("RESEARCHFLOW_MAX_QUEUED_RUNS must be a positive integer")
	}
	maxWorkers, err := strconv.Atoi(workersRaw)
	if err != nil || maxWorkers <= 0 {
		return ResearchflowCapacity{}, fmt.Errorf("RESEARCHFLOW_MAX_WORKERS must be a positive integer")
	}
	return ResearchflowCapacity{
		Configured: true, MaxQueuedRuns: maxQueuedRuns, MaxWorkers: maxWorkers,
	}, nil
}

// LoadResearchflowRunDeadline reads the Server-owned absolute Run budget.
// It has no default: enabling Researchflow requires an explicit positive duration.
func LoadResearchflowRunDeadline(env platformconfig.Env, runtimeConfigured bool) (time.Duration, error) {
	raw := strings.TrimSpace(env.String("RESEARCHFLOW_RUN_DEADLINE", ""))
	if !runtimeConfigured {
		if raw != "" {
			return 0, fmt.Errorf("RESEARCHFLOW_RUN_DEADLINE requires Researchflow capacity configuration")
		}
		return 0, nil
	}
	if raw == "" {
		return 0, fmt.Errorf("RESEARCHFLOW_RUN_DEADLINE must be explicitly configured when Researchflow is enabled")
	}
	deadline, err := time.ParseDuration(raw)
	if err != nil || deadline <= 0 {
		return 0, fmt.Errorf("RESEARCHFLOW_RUN_DEADLINE must be a positive Go duration such as 30m or 2h")
	}
	return deadline, nil
}
