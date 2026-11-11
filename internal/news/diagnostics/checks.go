package diagnostics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/adapters/parser"
	"example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformdiag "example.com/stock-analysis-flow/internal/platform/diagnostics"
	"example.com/stock-analysis-flow/internal/platform/fault"
)

// StorageProbe exposes only low-risk read-only checks. The implementation borrows
// the shared SQL pool; this package never owns a DB client or diagnostics runner.
type StorageProbe interface {
	Ping(context.Context) error
	MissingSchemaTables(context.Context) ([]string, error)
}

// ProviderConfigChecks reports optional Provider configuration without exposing
// endpoint values or credentials and without making billable upstream requests.
func ProviderConfigChecks(config newsconfig.Config) []platformdiag.Check {
	return []platformdiag.Check{
		providerConfigCheck("bocha", "Bocha", config.Bocha().Configured()),
		providerConfigCheck("tavily", "Tavily", config.Tavily().Configured()),
		providerConfigCheck("brave", "Brave", config.Brave().Configured()),
		providerConfigCheck("searxng", "SearXNG", config.SearXNG().Configured()),
	}
}

func providerConfigCheck(code, name string, configured bool) platformdiag.Check {
	return platformdiag.Check{
		ID:       "news.provider." + code + ".configuration",
		Name:     name + " provider configuration",
		Required: false,
		Timeout:  time.Second,
		Run: func(ctx context.Context) (platformdiag.Observation, error) {
			if err := ctx.Err(); err != nil {
				return platformdiag.Observation{}, err
			}
			if !configured {
				return platformdiag.Observation{
					Status: platformdiag.StatusSkipped, ReasonCode: "news.provider.not_configured",
					SafeMessage: "Provider is not configured", CheckedAt: time.Now().UTC(),
				}, nil
			}
			return platformdiag.Observation{
				Status: platformdiag.StatusHealthy, SafeMessage: "Provider configuration is present",
				CheckedAt: time.Now().UTC(), Attributes: map[string]string{"configured": "true"},
			}, nil
		},
	}
}

// ParserChecks executes deterministic local RSS/Atom fixtures. It performs no
// network access and never includes source payloads in diagnostic attributes.
func ParserChecks() []platformdiag.Check {
	return []platformdiag.Check{
		parserCheck("rss", parser.FeedRSS, []byte(`<rss version="2.0"><channel><title>News diagnostics</title><item><guid>check-1</guid><title>Probe</title></item></channel></rss>`)),
		parserCheck("atom", parser.FeedAtom, []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><title>News diagnostics</title><entry><id>urn:news-check:1</id><title>Probe</title></entry></feed>`)),
	}
}

func parserCheck(code string, expected parser.FeedFormat, payload []byte) platformdiag.Check {
	return platformdiag.Check{
		ID:       "news.parser." + code,
		Name:     code + " parser self-check",
		Required: true,
		Timeout:  2 * time.Second,
		Run: func(ctx context.Context) (platformdiag.Observation, error) {
			if err := ctx.Err(); err != nil {
				return platformdiag.Observation{}, err
			}
			feed, err := parser.ParseFeed(payload, time.Now().UTC())
			if err != nil || feed.Format != expected {
				return platformdiag.Observation{}, fault.New(
					fault.ClassInternal, "news.parser.self_check_failed",
					"News feed parser self-check failed", err,
				)
			}
			return platformdiag.Observation{
				Status: platformdiag.StatusHealthy, SafeMessage: "Feed parser self-check passed",
				CheckedAt: time.Now().UTC(),
				Attributes: map[string]string{
					"format": string(feed.Format), "fixture_entries": fmt.Sprintf("%d", len(feed.Entries)),
				},
			}, nil
		},
	}
}

// SourceEndpointCheck verifies that a registered endpoint is internally
// consistent and passes its configured URL allowlist. It does not fetch the URL.
func SourceEndpointCheck(catalog app.SourceCatalog, sourceID, endpointID string, required bool) platformdiag.Check {
	sum := sha256.Sum256([]byte(sourceID + "\x00" + endpointID))
	checkID := "news.source.endpoint." + hex.EncodeToString(sum[:12])
	return platformdiag.Check{
		ID: checkID, Name: "News source endpoint readiness", Required: required, Timeout: 3 * time.Second,
		Run: func(ctx context.Context) (platformdiag.Observation, error) {
			if err := ctx.Err(); err != nil {
				return platformdiag.Observation{}, err
			}
			if catalog == nil {
				return platformdiag.Observation{}, fault.New(
					fault.ClassUnavailable, "news.source.catalog_unavailable",
					"News source catalog is unavailable", nil,
				)
			}
			registration, err := catalog.LookupEndpoint(ctx, sourceID, endpointID)
			if err != nil {
				if errors.Is(err, app.ErrSourceEndpointNotFound) {
					return platformdiag.Observation{
						Status: platformdiag.StatusSkipped, ReasonCode: "news.source.endpoint_not_registered",
						SafeMessage: "Source endpoint is not registered", CheckedAt: time.Now().UTC(),
					}, nil
				}
				return platformdiag.Observation{}, fault.New(
					fault.ClassUnavailable, "news.source.catalog_lookup_failed",
					"News source catalog lookup failed", err,
				)
			}
			if registration.SourceID != sourceID || registration.EndpointID != endpointID {
				return platformdiag.Observation{}, fault.New(
					fault.ClassInternal, "news.source.registration_mismatch",
					"Source catalog returned a mismatched endpoint", nil,
				)
			}
			if !registration.Enabled {
				return platformdiag.Observation{
					Status: platformdiag.StatusSkipped, ReasonCode: "news.source.endpoint_disabled",
					SafeMessage: "Source endpoint is disabled", CheckedAt: time.Now().UTC(),
				}, nil
			}
			if registration.AccessMethod != domain.AccessRSS && registration.AccessMethod != domain.AccessLicensedFeed {
				return platformdiag.Observation{
					Status: platformdiag.StatusSkipped, ReasonCode: "news.source.access_method_unsupported",
					SafeMessage: "Source endpoint access method is not supported", CheckedAt: time.Now().UTC(),
				}, nil
			}
			if registration.SourcePolicyVersionID == "" ||
				(registration.EndpointPolicy != nil && registration.EndpointPolicyVersionID == "") ||
				(registration.EndpointPolicy == nil && registration.EndpointPolicyVersionID != "") {
				return platformdiag.Observation{
					Status: platformdiag.StatusDegraded, ReasonCode: "news.source.policy_version_invalid",
					SafeMessage: "Source endpoint policy version is incomplete", CheckedAt: time.Now().UTC(),
				}, nil
			}
			decision := domain.EvaluateSourceURL(registration.URL, registration.URLPolicy)
			if !decision.Allowed {
				return platformdiag.Observation{
					Status: platformdiag.StatusFailed, ReasonCode: "news.source.endpoint_url_rejected",
					SafeMessage: "Source endpoint URL is rejected by its allowlist", CheckedAt: time.Now().UTC(),
				}, nil
			}
			return platformdiag.Observation{
				Status: platformdiag.StatusHealthy, SafeMessage: "Source endpoint registration is ready",
				CheckedAt: time.Now().UTC(),
				Attributes: map[string]string{
					"access_method":    string(registration.AccessMethod),
					"policy_versioned": "true",
				},
			}, nil
		},
	}
}

// StorageChecks verifies the shared database connection and News-owned schema.
// It does not create tables, run migrations, or inspect stored article content.
func StorageChecks(probe StorageProbe) []platformdiag.Check {
	return []platformdiag.Check{
		{
			ID: "news.storage.connection", Name: "News storage connection", Required: true, Timeout: 3 * time.Second,
			Run: func(ctx context.Context) (platformdiag.Observation, error) {
				if probe == nil {
					return platformdiag.Observation{}, fault.New(
						fault.ClassUnavailable, "news.storage.probe_unavailable",
						"News storage probe is unavailable", nil,
					)
				}
				if err := probe.Ping(ctx); err != nil {
					return platformdiag.Observation{}, fault.New(
						fault.ClassUnavailable, "news.storage.connection_failed",
						"News storage is unavailable", err,
					)
				}
				return platformdiag.Observation{
					Status: platformdiag.StatusHealthy, SafeMessage: "News storage connection is healthy",
					CheckedAt: time.Now().UTC(), Attributes: map[string]string{"database": "mariadb"},
				}, nil
			},
		},
		{
			ID: "news.storage.schema", Name: "News storage schema", Required: true, Timeout: 5 * time.Second,
			Run: func(ctx context.Context) (platformdiag.Observation, error) {
				if probe == nil {
					return platformdiag.Observation{}, fault.New(
						fault.ClassUnavailable, "news.storage.probe_unavailable",
						"News storage probe is unavailable", nil,
					)
				}
				missing, err := probe.MissingSchemaTables(ctx)
				if err != nil {
					return platformdiag.Observation{}, fault.New(
						fault.ClassUnavailable, "news.storage.schema_check_failed",
						"News storage schema check failed", err,
					)
				}
				if len(missing) > 0 {
					return platformdiag.Observation{
						Status: platformdiag.StatusDegraded, ReasonCode: "news.storage.schema_incomplete",
						SafeMessage: "News storage schema is incomplete", CheckedAt: time.Now().UTC(),
						Attributes: map[string]string{
							"missing_table_count": fmt.Sprintf("%d", len(missing)),
							"missing_tables":      strings.Join(missing, ","),
						},
					}, nil
				}
				return platformdiag.Observation{
					Status: platformdiag.StatusHealthy, SafeMessage: "News storage schema is ready",
					CheckedAt: time.Now().UTC(), Attributes: map[string]string{"required_tables": "7"},
				}, nil
			},
		},
	}
}
