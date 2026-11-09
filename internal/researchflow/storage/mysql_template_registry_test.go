package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

func TestMariaDBTemplateRegistryPublishesImmutableExactVersions(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run Researchflow MariaDB integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("PLATFORM_MARIADB_USER"),
		os.Getenv("PLATFORM_MARIADB_PASSWORD"),
		os.Getenv("PLATFORM_MARIADB_ADDR"),
		os.Getenv("PLATFORM_MARIADB_DATABASE"),
	)
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		t.Fatal(err)
	}
	db, err := platformstorage.OpenSQL(ctx, connector, platformstorage.DefaultSQLConfig())
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, Migrations()); err != nil {
		t.Fatalf("apply Researchflow migrations: %v", err)
	}
	registry, err := NewMySQLTemplateRegistry(db, nil)
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	value := registryTestTemplate("registry-"+suffix, "v1")
	if err := registry.Publish(ctx, value); err != nil {
		t.Fatalf("publish workflow template: %v", err)
	}
	// Re-publishing identical semantic content is idempotent.
	if err := registry.Publish(ctx, value); err != nil {
		t.Fatalf("repeat identical workflow template publication: %v", err)
	}
	loaded, err := registry.Get(ctx, value.Ref, value.Version)
	if err != nil {
		t.Fatalf("get exact workflow template: %v", err)
	}
	if loaded.Ref != value.Ref || loaded.Version != value.Version || loaded.Fingerprint == "" {
		t.Fatalf("loaded template identity/fingerprint mismatch: %#v", loaded)
	}
	loaded.Stages[0].Retry.RetryableReasonCodes[0] = "caller_mutation"
	again, err := registry.Get(ctx, value.Ref, value.Version)
	if err != nil {
		t.Fatalf("reload workflow template: %v", err)
	}
	if again.Stages[0].Retry.RetryableReasonCodes[0] != "provider_unavailable" {
		t.Fatal("caller mutation changed persisted template definition")
	}

	changed := registryTestTemplate(value.Ref, value.Version)
	changed.Stages[0].Priority++
	if err := registry.Publish(ctx, changed); !errors.Is(err, ErrWorkflowTemplateConflict) {
		t.Fatalf("changed same-version publication error = %v, want immutable version conflict", err)
	}
	if _, err := registry.Get(ctx, value.Ref, "missing"); !errors.Is(err, ErrWorkflowTemplateNotFound) {
		t.Fatalf("missing exact version error = %v, want ErrWorkflowTemplateNotFound", err)
	}
}

func registryTestTemplate(ref, version string) template.WorkflowTemplate {
	return template.WorkflowTemplate{
		Ref: ref, Version: version, ParameterSchemaRef: "params-v1",
		OutputRegistryVersion: "outputs-v1", ResourcePolicyRef: "resource-v1",
		Stages: []template.StageDefinition{{
			Code: "market", Required: true,
			Applicability: template.ApplicabilityRule{RuleRef: "always", RuleVersion: "1"},
			Retry: template.RetryPolicy{
				Ref: "retry-v1", MaxAttempts: 2,
				RetryableReasonCodes: []string{"provider_unavailable"},
				Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
				AttemptTimeoutMS:     5000,
			},
			Propagation: template.PropagationPolicy{
				OnDegraded: domain.PropagateAccept, OnSkipped: domain.PropagatePartial,
				OnFailed: domain.PropagateFail, OnTimedOut: domain.PropagateFail, OnBlocked: domain.PropagateFail,
			},
		}},
	}
}
