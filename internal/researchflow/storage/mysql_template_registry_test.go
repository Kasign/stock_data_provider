package storage

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
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

	corrupt := registryTestTemplate(value.Ref, "v2")
	if err := registry.Publish(ctx, corrupt); err != nil {
		t.Fatalf("publish corruption fixture: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE researchflow_workflow_templates SET definition = JSON_SET(definition, '$.unexpected_field', 'unexpected') WHERE template_ref = ? AND version = ?", corrupt.Ref, corrupt.Version); err != nil {
		t.Fatalf("corrupt template fixture: %v", err)
	}
	if _, err := registry.Get(ctx, corrupt.Ref, corrupt.Version); !errors.Is(err, ErrWorkflowTemplateCorrupt) {
		t.Fatalf("corrupt template error = %v, want ErrWorkflowTemplateCorrupt", err)
	}
	if err := registry.Publish(ctx, corrupt); !errors.Is(err, ErrWorkflowTemplateCorrupt) {
		t.Fatalf("re-publishing corrupt immutable version error = %v, want ErrWorkflowTemplateCorrupt", err)
	}

	// Concurrent publication of identical content must converge to one
	// immutable row and remain idempotent for every publisher.
	identical := registryTestTemplate("registry-concurrent-identical-"+suffix, "v1")
	const publishers = 8
	var identicalWait sync.WaitGroup
	identicalErrors := make(chan error, publishers)
	for i := 0; i < publishers; i++ {
		identicalWait.Add(1)
		go func() {
			defer identicalWait.Done()
			identicalErrors <- registry.Publish(ctx, identical)
		}()
	}
	identicalWait.Wait()
	close(identicalErrors)
	for publishErr := range identicalErrors {
		if publishErr != nil {
			t.Fatalf("concurrent identical publication failed: %v", publishErr)
		}
	}

	// Concurrent different definitions for one identity must have exactly
	// one winner; the losing publisher must observe an immutable conflict.
	conflictRef := "registry-concurrent-conflict-" + suffix
	conflictA := registryTestTemplate(conflictRef, "v1")
	conflictB := registryTestTemplate(conflictRef, "v1")
	conflictB.Stages[0].Priority++
	fingerprintA, err := template.Fingerprint(conflictA)
	if err != nil {
		t.Fatal(err)
	}
	fingerprintB, err := template.Fingerprint(conflictB)
	if err != nil {
		t.Fatal(err)
	}
	conflictResults := make(chan error, 2)
	var conflictWait sync.WaitGroup
	for _, candidate := range []template.WorkflowTemplate{conflictA, conflictB} {
		candidate := candidate
		conflictWait.Add(1)
		go func() {
			defer conflictWait.Done()
			conflictResults <- registry.Publish(ctx, candidate)
		}()
	}
	conflictWait.Wait()
	close(conflictResults)
	published, conflicted := 0, 0
	for publishErr := range conflictResults {
		switch {
		case publishErr == nil:
			published++
		case errors.Is(publishErr, ErrWorkflowTemplateConflict):
			conflicted++
		default:
			t.Fatalf("unexpected concurrent conflicting publication error: %v", publishErr)
		}
	}
	if published != 1 || conflicted != 1 {
		t.Fatalf("concurrent conflicting publications: success=%d conflict=%d, want 1/1", published, conflicted)
	}
	winner, err := registry.Get(ctx, conflictRef, "v1")
	if err != nil {
		t.Fatalf("read concurrent publication winner: %v", err)
	}
	if winner.Fingerprint != fingerprintA && winner.Fingerprint != fingerprintB {
		t.Fatal("persisted concurrent publication is neither submitted definition")
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
