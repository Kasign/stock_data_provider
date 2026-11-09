package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/server/resources"
	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	"example.com/stock-analysis-flow/internal/platform/fault"
)

// StorageChecks creates required read-only checks for shared Server resources.
// It does not create schemas or perform migrations.
func StorageChecks(shared *resources.Resources) []platformdiagnostics.Check {
	return []platformdiagnostics.Check{
		{
			ID: "server.storage.mariadb", Name: "MariaDB connection", Required: true, Timeout: 3 * time.Second,
			Run: func(ctx context.Context) (platformdiagnostics.Observation, error) {
				if shared == nil || shared.DB == nil {
					return platformdiagnostics.Observation{}, fault.New(
						fault.ClassUnavailable, "server.storage.mariadb_unavailable",
						"MariaDB resource is unavailable", nil,
					)
				}
				if err := shared.DB.PingContext(ctx); err != nil {
					return platformdiagnostics.Observation{}, fault.New(
						fault.ClassUnavailable, "server.storage.mariadb_ping_failed",
						"MariaDB connection is unavailable", err,
					)
				}
				return platformdiagnostics.Observation{
					Status: platformdiagnostics.StatusHealthy, SafeMessage: "MariaDB connection is healthy",
					CheckedAt: time.Now().UTC(), Attributes: map[string]string{"dependency": "mariadb"},
				}, nil
			},
		},
		{
			ID: "server.storage.redis", Name: "Redis connection", Required: true, Timeout: 3 * time.Second,
			Run: func(ctx context.Context) (platformdiagnostics.Observation, error) {
				if shared == nil || shared.Redis == nil {
					return platformdiagnostics.Observation{}, fault.New(
						fault.ClassUnavailable, "server.storage.redis_unavailable",
						"Redis resource is unavailable", nil,
					)
				}
				if err := shared.Redis.Ping(ctx).Err(); err != nil {
					if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
						return platformdiagnostics.Observation{}, err
					}
					return platformdiagnostics.Observation{}, fault.New(
						fault.ClassUnavailable, "server.storage.redis_ping_failed",
						"Redis connection is unavailable", fmt.Errorf("ping Redis: %w", err),
					)
				}
				return platformdiagnostics.Observation{
					Status: platformdiagnostics.StatusHealthy, SafeMessage: "Redis connection is healthy",
					CheckedAt: time.Now().UTC(), Attributes: map[string]string{"dependency": "redis"},
				}, nil
			},
		},
	}
}
