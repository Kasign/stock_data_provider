package config

import (
	"fmt"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

// Migration controls the explicit schema migration step during bootstrap.
type Migration struct {
	Enabled bool
	Timeout time.Duration
}

// LoadMigration reads the opt-in migration policy. Production startup does not
// mutate schema unless MIGRATION_ENABLED is explicitly enabled.
func LoadMigration(env platformconfig.Env) (Migration, error) {
	cfg := Migration{Timeout: 2 * time.Minute}
	var err error
	if cfg.Enabled, err = env.Bool("MIGRATION_ENABLED", false); err != nil {
		return Migration{}, err
	}
	if cfg.Timeout, err = env.Duration("MIGRATION_TIMEOUT", cfg.Timeout); err != nil {
		return Migration{}, err
	}
	if cfg.Timeout <= 0 || cfg.Timeout > 30*time.Minute {
		return Migration{}, fmt.Errorf("MIGRATION_TIMEOUT must be greater than 0 and no more than 30m")
	}
	return cfg, nil
}
