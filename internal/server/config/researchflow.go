package config

import (
	"fmt"
	"strconv"
	"strings"

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
