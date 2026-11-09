package migration

import (
	"context"
	"database/sql"
	"reflect"
	"testing"

	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestRegisterRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		migrations []platformstorage.Migration
	}{
		{name: "empty namespace", namespace: "", migrations: []platformstorage.Migration{{Version: 1, Description: "init", Statements: []string{"SELECT 1"}}}},
		{name: "uppercase namespace", namespace: "Research", migrations: []platformstorage.Migration{{Version: 1, Description: "init", Statements: []string{"SELECT 1"}}}},
		{name: "empty migrations", namespace: "research"},
		{name: "zero version", namespace: "research", migrations: []platformstorage.Migration{{Description: "init", Statements: []string{"SELECT 1"}}}},
		{name: "missing description", namespace: "research", migrations: []platformstorage.Migration{{Version: 1, Statements: []string{"SELECT 1"}}}},
		{name: "missing statements", namespace: "research", migrations: []platformstorage.Migration{{Version: 1, Description: "init"}}},
		{name: "duplicate version", namespace: "research", migrations: []platformstorage.Migration{
			{Version: 1, Description: "init", Statements: []string{"SELECT 1"}},
			{Version: 1, Description: "duplicate", Statements: []string{"SELECT 2"}},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := NewRegistry()
			if err := registry.Register(tt.namespace, tt.migrations); err == nil {
				t.Fatal("Register() error = nil, want validation error")
			}
			if got := registry.Namespaces(); len(got) != 0 {
				t.Fatalf("registered namespaces after rejection = %v, want none", got)
			}
		})
	}
}

func TestRegisterRejectsDuplicateNamespace(t *testing.T) {
	registry := NewRegistry()
	migrations := []platformstorage.Migration{{Version: 1, Description: "init", Statements: []string{"SELECT 1"}}}
	if err := registry.Register("research", migrations); err != nil {
		t.Fatalf("first Register() error = %v", err)
	}
	if err := registry.Register("research", migrations); err == nil {
		t.Fatal("second Register() error = nil, want duplicate namespace error")
	}
}

func TestRegisterStoresImmutableSnapshot(t *testing.T) {
	registry := NewRegistry()
	migrations := []platformstorage.Migration{{Version: 1, Description: "init", Statements: []string{"CREATE TABLE sample (id INT)"}}}
	if err := registry.Register("research", migrations); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	migrations[0].Description = "mutated"
	migrations[0].Statements[0] = "DROP TABLE sample"
	if got := registry.entries["research"][0]; got.Description != "init" || got.Statements[0] != "CREATE TABLE sample (id INT)" {
		t.Fatalf("registered migration changed with caller input: %+v", got)
	}
}

func TestNamespacesAreSorted(t *testing.T) {
	registry := NewRegistry()
	migration := []platformstorage.Migration{{Version: 1, Description: "init", Statements: []string{"SELECT 1"}}}
	for _, namespace := range []string{"notification", "news", "research"} {
		if err := registry.Register(namespace, migration); err != nil {
			t.Fatalf("Register(%q) error = %v", namespace, err)
		}
	}
	want := []string{"news", "notification", "research"}
	if got := registry.Namespaces(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Namespaces() = %v, want %v", got, want)
	}
}

func TestRegisterOnNilRegistry(t *testing.T) {
	var registry *Registry
	if err := registry.Register("research", []platformstorage.Migration{{Version: 1, Description: "init", Statements: []string{"SELECT 1"}}}); err == nil {
		t.Fatal("Register() error = nil, want nil registry error")
	}
}

func TestRunValidatesInputsBeforeDatabaseAccess(t *testing.T) {
	registry := NewRegistry()
	emptyDB := &sql.DB{}

	if err := registry.Run(nil, emptyDB); err == nil {
		t.Fatal("Run() error = nil for nil context")
	}
	if err := registry.Run(context.Background(), nil); err == nil {
		t.Fatal("Run() error = nil for nil database")
	}
	if err := registry.Run(context.Background(), emptyDB); err == nil {
		t.Fatal("Run() error = nil for empty registry")
	}
}

func TestRunWithReportCapturesPreconditionFailure(t *testing.T) {
	registry := NewRegistry()
	report, err := registry.RunWithReport(context.Background(), &sql.DB{})
	if err == nil {
		t.Fatal("RunWithReport() error = nil, want empty registry error")
	}
	if report.Status != "failed" || report.StartedAt.IsZero() || report.FinishedAt.IsZero() ||
		report.FinishedAt.Before(report.StartedAt) || report.Duration < 0 {
		t.Fatalf("invalid failure report: %+v", report)
	}
	if len(report.Namespaces) != 0 {
		t.Fatalf("namespace results = %v, want none", report.Namespaces)
	}
}

func TestMigrationFailureDetails(t *testing.T) {
	tests := []struct {
		name             string
		err              error
		wantCode         string
		wantRecovery     bool
	}{
		{
			name: "incomplete migration requires recovery",
			err: platformfault.New(platformfault.ClassConflict, "platform.storage.migration_requires_recovery", "manual recovery required", nil),
			wantCode: "platform.storage.migration_requires_recovery",
			wantRecovery: true,
		},
		{
			name: "statement failure requires recovery",
			err: platformfault.New(platformfault.ClassInternal, "platform.storage.migration_apply_failed", "migration failed", nil),
			wantCode: "platform.storage.migration_apply_failed",
			wantRecovery: true,
		},
		{
			name: "history update failure requires inspection",
			err: platformfault.New(platformfault.ClassInternal, "platform.storage.migration_record_failed", "history update failed", nil),
			wantCode: "platform.storage.migration_record_failed",
			wantRecovery: true,
		},
		{
			name: "connection failure does not imply partial DDL",
			err: platformfault.New(platformfault.ClassUnavailable, "platform.storage.migration_connection_failed", "connection failed", nil),
			wantCode: "platform.storage.migration_connection_failed",
			wantRecovery: false,
		},
		{
			name: "invalid fault metadata is hidden",
			err: &platformfault.Fault{Class: platformfault.ClassInternal, Code: "INVALID CODE", SafeMessage: "invalid"},
			wantCode: "server.migration.namespace_failed",
			wantRecovery: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCode, gotRecovery := migrationFailureDetails(tt.err)
			if gotCode != tt.wantCode || gotRecovery != tt.wantRecovery {
				t.Fatalf("migrationFailureDetails() = (%q, %v), want (%q, %v)", gotCode, gotRecovery, tt.wantCode, tt.wantRecovery)
			}
		})
	}
}

func TestMigrationHistoryMismatchRequiresRecovery(t *testing.T) {
	gotCode, gotRecovery := migrationFailureDetails(fmt.Errorf("wrapped: %w", errMigrationHistoryMismatch))
	if gotCode != "server.migration.history_mismatch" || !gotRecovery {
		t.Fatalf("migrationFailureDetails() = (%q, %v), want history mismatch requiring recovery", gotCode, gotRecovery)
	}
}

func TestRegisterSortsMigrationsByVersion(t *testing.T) {
	registry := NewRegistry()
	migrations := []platformstorage.Migration{
		{Version: 30, Description: "third", Statements: []string{"SELECT 3"}},
		{Version: 10, Description: "first", Statements: []string{"SELECT 1"}},
		{Version: 20, Description: "second", Statements: []string{"SELECT 2"}},
	}
	if err := registry.Register("research", migrations); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	got := registry.entries["research"]
	want := []uint64{10, 20, 30}
	if len(got) != len(want) {
		t.Fatalf("migration count = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].Version != want[i] {
			t.Fatalf("migration version at index %d = %d, want %d", i, got[i].Version, want[i])
		}
	}
}
