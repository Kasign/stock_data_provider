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

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

var namespacePattern = regexp.MustCompile(`^[a-z0-9._-]{1,64}$`)

// Registry stores immutable snapshots of module migration definitions.
// Each namespace is applied independently in deterministic lexical order.
type Registry struct {
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
	if r.entries == nil {
		r.entries = make(map[string][]platformstorage.Migration)
	}
	if _, exists := r.entries[namespace]; exists {
		return fmt.Errorf("migration namespace %q is already registered", namespace)
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
	r.entries[namespace] = snapshot
	return nil
}

// Run applies every registered namespace in lexical order and stops at the
// first failure. Platform owns locking, checksums, history and SQL execution.
func (r *Registry) Run(ctx context.Context, db *sql.DB) error {
	if r == nil {
		return errors.New("migration registry is nil")
	}
	if ctx == nil {
		return errors.New("migration context is required")
	}
	if db == nil {
		return errors.New("migration database is required")
	}
	if len(r.entries) == 0 {
		return errors.New("no module migrations are registered")
	}

	namespaces := make([]string, 0, len(r.entries))
	for namespace := range r.entries {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	for _, namespace := range namespaces {
		if err := platformstorage.ApplyMigrations(ctx, db, namespace, r.entries[namespace]); err != nil {
			return fmt.Errorf("apply %s migrations: %w", namespace, err)
		}
	}
	return nil
}

// Namespaces returns the registered namespace names in deterministic order.
func (r *Registry) Namespaces() []string {
	if r == nil {
		return nil
	}
	namespaces := make([]string, 0, len(r.entries))
	for namespace := range r.entries {
		namespaces = append(namespaces, namespace)
	}
	sort.Strings(namespaces)
	return namespaces
}
