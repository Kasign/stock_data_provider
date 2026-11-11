package config

import (
	"fmt"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

// Security contains credentials required by the Root Server HTTP boundary.
type Security struct {
	OwnerToken platformconfig.Secret
}

// LoadSecurity requires an explicit owner credential. Until module-specific
// service scopes are wired, legacy Analysis routes are owner-only.
func LoadSecurity(env platformconfig.Env) (Security, error) {
	token, err := env.RequiredSecret("PLATFORM_ADMIN_TOKEN")
	if err != nil {
		return Security{}, fmt.Errorf("安全配置无效: %w", err)
	}
	return Security{OwnerToken: token}, nil
}
