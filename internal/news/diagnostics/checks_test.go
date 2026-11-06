package diagnostics

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/adapters/storage"
	"example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformdiag "example.com/stock-analysis-flow/internal/platform/diagnostics"
)

var _ StorageProbe = (*storage.MySQLRepository)(nil)

type storageProbeStub struct {
	pingErr   error
	missing   []string
	schemaErr error
}

func (s storageProbeStub) Ping(context.Context) error { return s.pingErr }
func (s storageProbeStub) MissingSchemaTables(context.Context) ([]string, error) {
	return append([]string(nil), s.missing...), s.schemaErr
}

type sourceCatalogStub struct {
	registration app.SourceEndpointRegistration
	err          error
}

func (s sourceCatalogStub) LookupEndpoint(context.Context, string, string) (app.SourceEndpointRegistration, error) {
	return s.registration, s.err
}

func TestProviderConfigChecksSkipUnconfiguredWithoutSecrets(t *testing.T) {
	report, err := (platformdiag.Runner{}).Run(context.Background(), ProviderConfigChecks(newsconfig.Config{}))
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Status != platformdiag.StatusSkipped || len(report.Results) != 4 {
		t.Fatalf("summary=%+v results=%d", report.Summary, len(report.Results))
	}
	for _, result := range report.Results {
		if result.Observation.Status != platformdiag.StatusSkipped {
			t.Errorf("provider check %s status=%s", result.CheckID, result.Observation.Status)
		}
		if len(result.Observation.Attributes) != 0 {
			t.Errorf("unconfigured provider exposed attributes: %#v", result.Observation.Attributes)
		}
	}
}

func TestParserChecksRunDeterministicLocalFixtures(t *testing.T) {
	report, err := (platformdiag.Runner{}).Run(context.Background(), ParserChecks())
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Status != platformdiag.StatusHealthy {
		t.Fatalf("summary=%+v", report.Summary)
	}
	for _, result := range report.Results {
		if result.Observation.Status != platformdiag.StatusHealthy {
			t.Errorf("parser check %s: %+v", result.CheckID, result.Observation)
		}
	}
}

func TestSourceEndpointCheckSkipsUnregisteredEndpoint(t *testing.T) {
	check := SourceEndpointCheck(sourceCatalogStub{err: app.ErrSourceEndpointNotFound}, "source-1", "endpoint-1", false)
	report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{check})
	if err != nil {
		t.Fatal(err)
	}
	if got := report.Results[0].Observation.Status; got != platformdiag.StatusSkipped {
		t.Fatalf("status=%s, want skipped", got)
	}
}

func TestSourceEndpointCheckDoesNotExposeCatalogErrors(t *testing.T) {
	check := SourceEndpointCheck(sourceCatalogStub{err: errors.New("private catalog connection details")}, "source-1", "endpoint-1", true)
	report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{check})
	if err != nil {
		t.Fatal(err)
	}
	result := report.Results[0]
	if result.Observation.Status != platformdiag.StatusUnavailable ||
		result.Observation.SafeMessage != "News source catalog lookup failed" {
		t.Fatalf("observation=%+v", result.Observation)
	}
}

func TestStorageChecksReportHealthyAndIncompleteSchema(t *testing.T) {
	checks := StorageChecks(storageProbeStub{missing: []string{"news_item_versions", "news_observations"}})
	report, err := (platformdiag.Runner{}).Run(context.Background(), checks)
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Observation.Status != platformdiag.StatusHealthy {
		t.Fatalf("connection=%+v", report.Results[0].Observation)
	}
	schema := report.Results[1].Observation
	if schema.Status != platformdiag.StatusDegraded || schema.Attributes["missing_table_count"] != "2" {
		t.Fatalf("schema=%+v", schema)
	}
	if schema.CheckedAt.IsZero() || schema.Duration < 0 || schema.SafeMessage == "" {
		t.Fatalf("invalid observation metadata: %+v", schema)
	}
}

func TestStorageChecksMapUnavailableProbeToSafeFault(t *testing.T) {
	checks := StorageChecks(storageProbeStub{pingErr: errors.New("private database address and credentials")})
	report, err := (platformdiag.Runner{}).Run(context.Background(), checks)
	if err != nil {
		t.Fatal(err)
	}
	result := report.Results[0]
	if result.Observation.Status != platformdiag.StatusUnavailable ||
		result.Observation.SafeMessage != "News storage is unavailable" ||
		result.Fault == nil || result.Fault.Code != "news.storage.connection_failed" {
		t.Fatalf("result=%+v", result)
	}
	if result.Observation.SafeMessage == "private database address and credentials" {
		t.Fatal("diagnostic exposed internal database error")
	}
}

func TestDiagnosticChecksHonorCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := (platformdiag.Runner{}).Run(ctx, ParserChecks())
	if err != nil {
		t.Fatal(err)
	}
	for _, result := range report.Results {
		if result.Observation.Status != platformdiag.StatusUnavailable {
			t.Errorf("check %s status=%s", result.CheckID, result.Observation.Status)
		}
	}
}

func TestProviderChecksHaveBoundedTimeouts(t *testing.T) {
	for _, check := range ProviderConfigChecks(newsconfig.Config{}) {
		if check.Timeout <= 0 || check.Timeout > time.Second {
			t.Errorf("check %s timeout=%s", check.ID, check.Timeout)
		}
	}
}

func TestSourceEndpointCheckReportsApprovedRegistration(t *testing.T) {
	catalog := sourceCatalogStub{registration: app.SourceEndpointRegistration{
		SourceID: "source-1", EndpointID: "endpoint-1", Enabled: true,
		URL: "https://feeds.example.com/rss", AccessMethod: domain.AccessRSS,
		SourcePolicyVersionID: "source-policy-v1",
		URLPolicy: domain.SourceURLPolicy{
			AllowedSchemes: []string{"https"},
			AllowedHosts:   []string{"feeds.example.com"},
		},
	}}
	report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{
		SourceEndpointCheck(catalog, "source-1", "endpoint-1", true),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation := report.Results[0].Observation
	if observation.Status != platformdiag.StatusHealthy ||
		observation.Attributes["access_method"] != string(domain.AccessRSS) {
		t.Fatalf("observation=%+v", observation)
	}
}
