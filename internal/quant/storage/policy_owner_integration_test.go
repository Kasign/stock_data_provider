package storage

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/application"
)

func TestMySQLResultAccessPolicyPublishesImmutableVersionsAndRevokes(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx := context.Background()
	store, err := NewMySQLResultAccessPolicyStore(db)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := application.NewResultAccessPolicyManager(store)
	if err != nil {
		t.Fatal(err)
	}
	var auditBefore uint64
	if err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(audit_id), 0) FROM quant_policy_admin_audit").Scan(&auditBefore); err != nil {
		t.Fatal(err)
	}
	current, err := manager.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.Enabled {
		current, err = manager.Revoke(ctx, "owner", current.Version)
		if err != nil {
			t.Fatalf("normalize policy to revoked: %v", err)
		}
	}
	before := current.Version
	published, err := manager.Publish(ctx, "owner", before)
	if err != nil {
		t.Fatalf("publish policy: %v", err)
	}
	if !published.Enabled || published.Version != before+1 || published.Snapshot == nil ||
		published.Snapshot.SnapshotRef != resultAccessPolicySnapshotPrefix+strconv.FormatUint(published.Version, 10) {
		t.Fatalf("published policy state mismatch: %+v", published)
	}
	if _, err := manager.Revoke(ctx, "owner", before); !errors.Is(err, application.ErrPolicyVersionConflict) {
		t.Fatalf("stale revoke error = %v", err)
	}
	revoked, err := manager.Revoke(ctx, "owner", published.Version)
	if err != nil {
		t.Fatalf("revoke policy: %v", err)
	}
	if revoked.Enabled || revoked.Version != published.Version+1 || revoked.Snapshot == nil {
		t.Fatalf("revoked policy state mismatch: %+v", revoked)
	}
	var oldEnabled bool
	var oldSnapshot string
	if err := db.QueryRowContext(ctx, "SELECT enabled, policy_snapshot_json FROM quant_result_access_policy_versions WHERE policy_version = ?", published.Version).
		Scan(&oldEnabled, &oldSnapshot); err != nil {
		t.Fatal(err)
	}
	if !oldEnabled || !strings.Contains(oldSnapshot, published.Snapshot.SnapshotRef) {
		t.Fatal("published immutable policy version was modified by revoke")
	}
	reloaded, err := manager.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Version != revoked.Version || reloaded.Enabled {
		t.Fatalf("current policy pointer mismatch: %+v", reloaded)
	}

	rows, err := db.QueryContext(ctx, "SELECT actor_ref, action, previous_version, new_version FROM quant_policy_admin_audit WHERE audit_id > ? ORDER BY audit_id", auditBefore)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type auditRecord struct {
		actor string
		action string
		previous uint64
		next uint64
	}
	var auditRecords []auditRecord
	for rows.Next() {
		var item auditRecord
		if err := rows.Scan(&item.actor, &item.action, &item.previous, &item.next); err != nil {
			t.Fatal(err)
		}
		auditRecords = append(auditRecords, item)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	expectedAuditCount := 2
	if current.Enabled {
		expectedAuditCount++
	}
	if len(auditRecords) != expectedAuditCount {
		t.Fatalf("policy administration audit records=%d want=%d: %+v", len(auditRecords), expectedAuditCount, auditRecords)
	}
	publishRecord := auditRecords[len(auditRecords)-2]
	revokeRecord := auditRecords[len(auditRecords)-1]
	if publishRecord.actor != "owner" || publishRecord.action != "publish" ||
		publishRecord.previous != before || publishRecord.next != published.Version {
		t.Fatalf("publish audit record mismatch: %+v", publishRecord)
	}
	if revokeRecord.actor != "owner" || revokeRecord.action != "revoke" ||
		revokeRecord.previous != published.Version || revokeRecord.next != revoked.Version {
		t.Fatalf("revoke audit record mismatch: %+v", revokeRecord)
	}
}

func TestMySQLResultAccessPolicyConcurrentPublishUsesExpectedVersion(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx := context.Background()
	store, err := NewMySQLResultAccessPolicyStore(db)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := application.NewResultAccessPolicyManager(store)
	if err != nil {
		t.Fatal(err)
	}
	current, err := manager.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if current.Enabled {
		current, err = manager.Revoke(ctx, "owner", current.Version)
		if err != nil {
			t.Fatal(err)
		}
	}
	expected := current.Version
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := manager.Publish(ctx, "owner", expected)
			results <- err
		}()
	}
	wg.Wait()
	close(results)
	var succeeded, conflicted int
	for err := range results {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, application.ErrPolicyVersionConflict):
			conflicted++
		default:
			t.Fatalf("unexpected concurrent publish error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent publish outcomes: success=%d conflict=%d", succeeded, conflicted)
	}
	after, err := manager.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if after.Enabled {
		if _, err := manager.Revoke(ctx, "owner", after.Version); err != nil {
			t.Fatalf("restore fail-closed policy state: %v", err)
		}
	}
}

func TestMySQLResultAccessPolicyRejectsNonOwnerActor(t *testing.T) {
	db := openQuantIntegrationDB(t)
	store, err := NewMySQLResultAccessPolicyStore(db)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := application.NewResultAccessPolicyManager(store)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Publish(context.Background(), "service:researchflow", 0); !errors.Is(err, application.ErrInvalidPolicyChange) {
		t.Fatalf("non-owner actor error = %v", err)
	}
}
