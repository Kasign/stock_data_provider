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

type observationRetentionProbeStub struct {
	count  int
	oldest *time.Time
	err    error
}

func (s observationRetentionProbeStub) ObservationRetentionBacklog(context.Context, time.Time) (int, *time.Time, error) {
	return s.count, s.oldest, s.err
}

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

func TestObservationRetentionCheckReportsBacklogAndHealthyState(t *testing.T) {
	oldest := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	check := ObservationRetentionCheck(observationRetentionProbeStub{count: 3, oldest: &oldest})
	report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{check})
	if err != nil {
		t.Fatal(err)
	}
	observation := report.Results[0].Observation
	if observation.Status != platformdiag.StatusDegraded || observation.ReasonCode != "news.observation.retention_backlog" ||
		observation.Attributes["overdue_count"] != "3" || observation.Attributes["oldest_overdue_at"] != oldest.Format(time.RFC3339Nano) {
		t.Fatalf("retention backlog observation=%+v", observation)
	}
	report, err = (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{ObservationRetentionCheck(observationRetentionProbeStub{})})
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Observation.Status != platformdiag.StatusHealthy || report.Results[0].Observation.Attributes["overdue_count"] != "0" {
		t.Fatalf("empty retention backlog observation=%+v", report.Results[0].Observation)
	}
}

func TestObservationRetentionCheckRejectsInconsistentBacklogResult(t *testing.T) {
	oldest := time.Date(2026, 11, 8, 8, 0, 0, 0, time.UTC)
	cases := []struct {
		name  string
		probe observationRetentionProbeStub
	}{
		{name: "negative count", probe: observationRetentionProbeStub{count: -1}},
		{name: "positive count without oldest timestamp", probe: observationRetentionProbeStub{count: 1}},
		{name: "zero count with oldest timestamp", probe: observationRetentionProbeStub{oldest: &oldest}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{
				ObservationRetentionCheck(testCase.probe),
			})
			if err != nil {
				t.Fatal(err)
			}
			observation := report.Results[0].Observation
			if observation.Status != platformdiag.StatusFailed ||
				observation.ReasonCode != "news.observation.retention_backlog_invalid" {
				t.Fatalf("observation=%+v, want unavailable inconsistent-backlog result", observation)
			}
		})
	}
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
	healthyChecks := StorageChecks(storageProbeStub{})
	healthyReport, err := (platformdiag.Runner{}).Run(context.Background(), healthyChecks)
	if err != nil {
		t.Fatal(err)
	}
	healthySchema := healthyReport.Results[1].Observation
	if healthySchema.Status != platformdiag.StatusHealthy ||
		healthySchema.Attributes["required_tables"] != "10" {
		t.Fatalf("healthy schema=%+v, want all 10 News tables", healthySchema)
	}

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

func TestSourceEndpointCheckFailsClosedForUnsafeRegistrations(t *testing.T) {
	valid := app.SourceEndpointRegistration{
		SourceID: "source-1", EndpointID: "endpoint-1", Enabled: true,
		URL: "https://feeds.example.com/rss", AccessMethod: domain.AccessRSS,
		SourcePolicyVersionID: "source-policy-v1",
		URLPolicy: domain.SourceURLPolicy{
			AllowedSchemes: []string{"https"},
			AllowedHosts:   []string{"feeds.example.com"},
		},
	}
	tests := []struct {
		name         string
		registration app.SourceEndpointRegistration
		wantStatus   platformdiag.Status
		wantReason   string
		required     bool
	}{
		{
			name: "registration identity mismatch",
			registration: func() app.SourceEndpointRegistration {
				value := valid
				value.EndpointID = "other-endpoint"
				return value
			}(),
			wantStatus: platformdiag.StatusFailed,
			wantReason: "news.source.registration_mismatch",
		},
		{
			name: "disabled endpoint",
			registration: func() app.SourceEndpointRegistration {
				value := valid
				value.Enabled = false
				return value
			}(),
			wantStatus: platformdiag.StatusSkipped,
			wantReason: "news.source.endpoint_disabled",
			required:   false,
		},
		{
			name: "unsupported access method",
			registration: func() app.SourceEndpointRegistration {
				value := valid
				value.AccessMethod = domain.AccessOfficialAPI
				return value
			}(),
			wantStatus: platformdiag.StatusSkipped,
			wantReason: "news.source.access_method_unsupported",
			required:   false,
		},
		{
			name: "missing source policy version",
			registration: func() app.SourceEndpointRegistration {
				value := valid
				value.SourcePolicyVersionID = ""
				return value
			}(),
			wantStatus: platformdiag.StatusDegraded,
			wantReason: "news.source.policy_version_invalid",
		},
		{
			name: "URL outside allowlist",
			registration: func() app.SourceEndpointRegistration {
				value := valid
				value.URL = "https://untrusted.example/rss"
				return value
			}(),
			wantStatus: platformdiag.StatusFailed,
			wantReason: "news.source.endpoint_url_rejected",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			check := SourceEndpointCheck(sourceCatalogStub{registration: test.registration}, "source-1", "endpoint-1", test.required || (test.wantStatus != platformdiag.StatusSkipped))
			report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{check})
			if err != nil {
				t.Fatal(err)
			}
			observation := report.Results[0].Observation
			if observation.Status != test.wantStatus || observation.ReasonCode != test.wantReason {
				t.Fatalf("observation=%+v, want status=%s reason=%q", observation, test.wantStatus, test.wantReason)
			}
		})
	}
}

func TestProviderOutputPolicyCheckReportsFailClosedBoundary(t *testing.T) {
	report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{
		ProviderOutputPolicyCheck(),
	})
	if err != nil {
		t.Fatal(err)
	}
	observation := report.Results[0].Observation
	if observation.Status != platformdiag.StatusDegraded ||
		observation.ReasonCode != "news.provider.output_policy_unavailable" ||
		observation.Attributes["provider_persistence"] != "blocked" ||
		observation.Attributes["policy_catalog"] != "unavailable" {
		t.Fatalf("provider policy readiness=%+v", observation)
	}
	if observation.SafeMessage == "" || observation.CheckedAt.IsZero() {
		t.Fatalf("incomplete diagnostic observation=%+v", observation)
	}
}

func TestProviderOutputPolicyCheckHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	report, err := (platformdiag.Runner{}).Run(ctx, []platformdiag.Check{ProviderOutputPolicyCheck()})
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Observation.Status != platformdiag.StatusUnavailable {
		t.Fatalf("status=%s, want unavailable", report.Results[0].Observation.Status)
	}
}

type providerOutputPolicyResolverStub struct {
	binding app.ProviderOutputPolicyBinding
	err     error
}

func (s providerOutputPolicyResolverStub) ResolveProviderOutputPolicy(context.Context, string) (app.ProviderOutputPolicyBinding, error) {
	return s.binding, s.err
}

func TestProviderOutputPolicyResolverCheckDistinguishesReadinessStates(t *testing.T) {
	run := func(name string, resolver app.ProviderOutputPolicyResolver, wantStatus platformdiag.Status, wantReason, wantState string) {
		t.Run(name, func(t *testing.T) {
			report, err := (platformdiag.Runner{}).Run(context.Background(), []platformdiag.Check{
				ProviderOutputPolicyResolverCheck(resolver, "brave"),
			})
			if err != nil {
				t.Fatal(err)
			}
			observation := report.Results[0].Observation
			if observation.Status != wantStatus || observation.ReasonCode != wantReason {
				t.Fatalf("observation=%+v", observation)
			}
			if observation.Attributes["policy_resolver"] != wantState ||
				observation.Attributes["provider_persistence"] != "blocked" {
				t.Fatalf("attributes=%v", observation.Attributes)
			}
			if observation.SafeMessage == "" || observation.CheckedAt.IsZero() {
				t.Fatalf("incomplete observation=%+v", observation)
			}
		})
	}

	run("resolver not configured", nil, platformdiag.StatusDegraded,
		"news.provider.output_policy_resolver_unavailable", "unavailable")
	run("lookup failed", providerOutputPolicyResolverStub{err: errors.New("private catalog detail")},
		platformdiag.StatusUnavailable, "news.provider.output_policy_lookup_failed", "lookup_failed")
	run("binding available but request not evaluated", providerOutputPolicyResolverStub{
		binding: app.ProviderOutputPolicyBinding{
			ProviderCode: "brave", PolicyRef: "brave-output", PolicyVersion: "policy-v4",
		},
	}, platformdiag.StatusDegraded, "news.provider.output_policy_not_evaluated", "available")
	run("binding identity mismatch", providerOutputPolicyResolverStub{
		binding: app.ProviderOutputPolicyBinding{
			ProviderCode: "tavily", PolicyRef: "tavily-output", PolicyVersion: "policy-v4",
		},
	}, platformdiag.StatusDegraded, "news.provider.output_policy_binding_invalid", "available")
}

type cancellationStorageProbe struct{ cancel context.CancelFunc }

func (p cancellationStorageProbe) Ping(context.Context) error {
	p.cancel()
	return nil
}

func (p cancellationStorageProbe) MissingSchemaTables(context.Context) ([]string, error) {
	p.cancel()
	return nil, nil
}

type cancellationRetentionProbe struct{ cancel context.CancelFunc }

func (p cancellationRetentionProbe) ObservationRetentionBacklog(context.Context, time.Time) (int, *time.Time, error) {
	p.cancel()
	return 0, nil, nil
}

type cancellationSourceCatalog struct{ cancel context.CancelFunc }

func (p cancellationSourceCatalog) LookupEndpoint(context.Context, string, string) (app.SourceEndpointRegistration, error) {
	p.cancel()
	return app.SourceEndpointRegistration{
		SourceID: "source-1", EndpointID: "endpoint-1", Enabled: true,
		URL: "https://feeds.example.com/rss", AccessMethod: domain.AccessRSS,
		SourcePolicyVersionID: "source-policy-v1",
		URLPolicy:             domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"feeds.example.com"}},
	}, nil
}

func TestDiagnosticChecksStopWhenContextIsCanceledByDependency(t *testing.T) {
	t.Run("observation retention", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		check := ObservationRetentionCheck(cancellationRetentionProbe{cancel: cancel})
		report, err := (platformdiag.Runner{}).Run(ctx, []platformdiag.Check{check})
		if err != nil {
			t.Fatal(err)
		}
		if got := report.Results[0].Observation.Status; got != platformdiag.StatusUnavailable {
			t.Fatalf("status=%s, want unavailable", got)
		}
	})
	t.Run("source endpoint lookup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		check := SourceEndpointCheck(cancellationSourceCatalog{cancel: cancel}, "source-1", "endpoint-1", true)
		report, err := (platformdiag.Runner{}).Run(ctx, []platformdiag.Check{check})
		if err != nil {
			t.Fatal(err)
		}
		if got := report.Results[0].Observation.Status; got != platformdiag.StatusUnavailable {
			t.Fatalf("status=%s, want unavailable", got)
		}
	})
	t.Run("storage ping", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		report, err := (platformdiag.Runner{}).Run(ctx, StorageChecks(cancellationStorageProbe{cancel: cancel})[:1])
		if err != nil {
			t.Fatal(err)
		}
		if got := report.Results[0].Observation.Status; got != platformdiag.StatusUnavailable {
			t.Fatalf("status=%s, want unavailable", got)
		}
	})
	t.Run("storage schema", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		report, err := (platformdiag.Runner{}).Run(ctx, StorageChecks(cancellationStorageProbe{cancel: cancel})[1:])
		if err != nil {
			t.Fatal(err)
		}
		if got := report.Results[0].Observation.Status; got != platformdiag.StatusUnavailable {
			t.Fatalf("status=%s, want unavailable", got)
		}
	})
}


type countingStorageProbe struct {
	pingCalls   int
	schemaCalls int
}

func (p *countingStorageProbe) Ping(context.Context) error {
	p.pingCalls++
	return nil
}

func (p *countingStorageProbe) MissingSchemaTables(context.Context) ([]string, error) {
	p.schemaCalls++
	return nil, nil
}

func TestStorageChecksDoNotCallProbeWhenContextIsAlreadyCanceled(t *testing.T) {
	t.Run("ping", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		probe := &countingStorageProbe{}
		report, err := (platformdiag.Runner{}).Run(ctx, StorageChecks(probe)[:1])
		if err != nil {
			t.Fatal(err)
		}
		if probe.pingCalls != 0 {
			t.Fatalf("Ping calls=%d, want 0", probe.pingCalls)
		}
		if got := report.Results[0].Observation.Status; got != platformdiag.StatusUnavailable {
			t.Fatalf("status=%s, want unavailable", got)
		}
	})
	t.Run("schema", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		probe := &countingStorageProbe{}
		report, err := (platformdiag.Runner{}).Run(ctx, StorageChecks(probe)[1:])
		if err != nil {
			t.Fatal(err)
		}
		if probe.schemaCalls != 0 {
			t.Fatalf("MissingSchemaTables calls=%d, want 0", probe.schemaCalls)
		}
		if got := report.Results[0].Observation.Status; got != platformdiag.StatusUnavailable {
			t.Fatalf("status=%s, want unavailable", got)
		}
	})
}
