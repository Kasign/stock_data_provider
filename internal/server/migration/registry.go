// Package migration collects module-owned schema changes for the Root Server.
package migration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

var namespacePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// Registry stores immutable snapshots of module migration definitions.
// Each namespace is applied independently in deterministic lexical order.
type Registry struct {
	mu      sync.RWMutex
	entries map[string][]platformstorage.Migration
}

// NewRegistry creates an empty migration registry.
func NewRegistry() *Registry {
	return &Registry{entries: make(map[string][]platformstorage.Migration)}
}

// Register adds one module's complete migration set. Module SQL remains owned
// by the module; the Server only records and executes the supplied definitions.
func (r *Registry) Register(namespace string, migrations []platformstorage.Migration) error {
	if r == nil {
		return errors.New("migration registry is nil")
	}
	namespace = strings.TrimSpace(namespace)
	if !namespacePattern.MatchString(namespace) {
		return fmt.Errorf("migration namespace %q is invalid", namespace)
	}
	if len(migrations) == 0 {
		return fmt.Errorf("migration namespace %q has no migrations", namespace)
	}
	snapshot := make([]platformstorage.Migration, len(migrations))
	versions := make(map[uint64]struct{}, len(migrations))
	for i, item := range migrations {
		if item.Version == 0 || strings.TrimSpace(item.Description) == "" || len(item.Statements) == 0 {
			return fmt.Errorf("migration namespace %q contains an incomplete migration at index %d", namespace, i)
		}
		if _, exists := versions[item.Version]; exists {
			return fmt.Errorf("migration namespace %q contains duplicate version %d", namespace, item.Version)
		}
		versions[item.Version] = struct{}{}
		snapshot[i] = platformstorage.Migration{
			Version: item.Version, Description: item.Description,
			Statements: append([]string(nil), item.Statements...),
		}
	}
	sort.Slice(snapshot, func(i, j int) bool {
		return snapshot[i].Version < snapshot[j].Version
	})

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.entries == nil {
		r.entries = make(map[string][]platformstorage.Migration)
	}
	if _, exists := r.entries[namespace]; exists {
		return fmt.Errorf("migration namespace %q is already registered", namespace)
	}
	r.entries[namespace] = snapshot
	return nil
}

// NamespaceResult records one namespace execution attempt. It deliberately
// does not expose SQL or driver error text in structured diagnostics.
type NamespaceResult struct {
	Namespace        string        `json:"namespace"`
	Status           string        `json:"status"`
	Duration         time.Duration `json:"duration"`
	ReasonCode       string        `json:"reason_code,omitempty"`
	RecoveryRequired bool          `json:"recovery_required"`
}

// RunReport is the bounded, deterministic result of one Registry run.
type RunReport struct {
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt time.Time         `json:"finished_at"`
	Duration   time.Duration     `json:"duration"`
	Status     string            `json:"status"`
	Namespaces []NamespaceResult `json:"namespaces"`
}

// Run preserves the original error-only API for existing callers.
func (r *Registry) Run(ctx context.Context, db *sql.DB) error {
	_, err := r.RunWithReport(ctx, db)
	return err
}

// RunWithReport applies namespaces in lexical order and stops at the first
// failure. Platform owns locking, checksums, history and SQL execution.
func (r *Registry) RunWithReport(ctx context.Context, db *sql.DB) (RunReport, error) {
	startedAt := time.Now()
	report := RunReport{StartedAt: startedAt, Status: "failed", Namespaces: make([]NamespaceResult, 0)}
	finish := func(err error) (RunReport, error) {
		report.FinishedAt = time.Now()
		report.Duration = report.FinishedAt.Sub(startedAt)
		if err == nil {
			report.Status = "succeeded"
		}
		return report, err
	}
	if r == nil {
		return finish(errors.New("migration registry is nil"))
	}
	if ctx == nil {
		return finish(errors.New("migration context is required"))
	}
	if db == nil {
		return finish(errors.New("migration database is required"))
	}
	entries := r.snapshot()
	if len(entries) == 0 {
		return finish(errors.New("no module migrations are registered"))
	}

	namespaces := make([]string, 0, len(entries))
	for namespace := range entries {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	for _, namespace := range namespaces {
		itemStarted := time.Now()
		err := platformstorage.ApplyMigrations(ctx, db, namespace, entries[namespace])
		item := NamespaceResult{
			Namespace: namespace,
			Status:    "succeeded",
			Duration:  time.Since(itemStarted),
		}
		if err != nil {
			item.Status = "failed"
			item.ReasonCode, item.RecoveryRequired = migrationFailureDetails(err)
			item.Duration = time.Since(itemStarted)
			report.Namespaces = append(report.Namespaces, item)
			return finish(fmt.Errorf("apply %s migrations: %w", namespace, err))
		}
		report.Namespaces = append(report.Namespaces, item)
	}
	return finish(nil)
}

// migrationFailureDetails converts internal migration failures to stable,
// non-sensitive diagnostics and marks cases that require operator inspection.
func migrationFailureDetails(err error) (reasonCode string, recoveryRequired bool) {
	if errors.Is(err, errMigrationHistoryMismatch) {
		return "server.migration.history_mismatch", true
	}
	var platformErr *platformfault.Fault
	if !errors.As(err, &platformErr) || platformErr.Validate() != nil {
		return "server.migration.namespace_failed", false
	}
	switch platformErr.Code {
	case "platform.storage.migration_requires_recovery",
		"platform.storage.migration_apply_failed",
		"platform.storage.migration_record_failed",
		"platform.storage.migration_lock_release_failed":
		return platformErr.Code, true
	default:
		return platformErr.Code, false
	}
}

// Namespaces returns the registered namespace names in deterministic order.
func (r *Registry) Namespaces() []string {
	if r == nil {
		return nil
	}
	entries := r.snapshot()
	namespaces := make([]string, 0, len(entries))
	for namespace := range entries {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	return namespaces
}

// snapshot copies the registry map while holding a read lock. Registered
// migration slices are immutable after insertion and can safely be shared.
func (r *Registry) snapshot() map[string][]platformstorage.Migration {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make(map[string][]platformstorage.Migration, len(r.entries))
	for namespace, migrations := range r.entries {
		entries[namespace] = migrations
	}
	return entries
}
