package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/application"
)

var _ application.RunAccessAuthorizer = (*MySQLRunRepository)(nil)

// AuthorizeRunRead permits access only to Runs created by the authenticated caller.
func (r *MySQLRunRepository) AuthorizeRunRead(ctx context.Context, callerRef, runRef string) error {
	return r.authorizeRunOwner(ctx, callerRef, runRef)
}

// AuthorizeRunCancellation applies the same persisted creator association to write access.
func (r *MySQLRunRepository) AuthorizeRunCancellation(ctx context.Context, callerRef, runRef string) error {
	return r.authorizeRunOwner(ctx, callerRef, runRef)
}

func (r *MySQLRunRepository) authorizeRunOwner(ctx context.Context, callerRef, runRef string) error {
	if r == nil || r.db == nil {
		return errors.New("Researchflow Run access repository is not configured")
	}
	if strings.TrimSpace(callerRef) == "" || callerRef != strings.TrimSpace(callerRef) ||
		strings.TrimSpace(runRef) == "" || runRef != strings.TrimSpace(runRef) || len(runRef) > 255 {
		return errors.New("Researchflow Run access arguments are invalid")
	}
	var exists int
	err := r.db.QueryRowContext(ctx, `SELECT 1 FROM researchflow_run_idempotency
		WHERE caller_ref = ? AND operation = 'CreateRun' AND run_ref = ? LIMIT 1`,
		callerRef, runRef,
	).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return application.ErrRunAccessDenied
	}
	if err != nil {
		return fmt.Errorf("verify Researchflow Run creator association: %w", err)
	}
	return nil
}
