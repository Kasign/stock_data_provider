package storage

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"example.com/stock-analysis-flow/internal/quant/application"
)

var (
	ErrInvalidPolicyStore = errors.New("Quant result access policy store is invalid")
	ErrPolicyStoreUnavailable = errors.New("Quant result access policy storage is unavailable")
)

const (
	resultAccessPolicyType = "quant_result_access"
	resultAccessPolicyRef = "quant-result-access"
	resultAccessPolicySnapshotPrefix = "policy-snapshot:quant:result-access:"
)

// MySQLResultAccessPolicyStore owns immutable Quant result-access policy
// versions. The head row is the shared serialization point for publish,
// revoke, and (later) authorized reads.
type MySQLResultAccessPolicyStore struct {
	db *sql.DB
}

func NewMySQLResultAccessPolicyStore(db *sql.DB) (*MySQLResultAccessPolicyStore, error) {
	if db == nil {
		return nil, ErrInvalidPolicyStore
	}
	return &MySQLResultAccessPolicyStore{db: db}, nil
}

var _ application.ResultAccessPolicyStore = (*MySQLResultAccessPolicyStore)(nil)

func (store *MySQLResultAccessPolicyStore) CurrentResultAccessPolicy(ctx context.Context) (application.ResultAccessPolicyState, error) {
	if store == nil || store.db == nil || ctx == nil {
		return application.ResultAccessPolicyState{}, ErrInvalidPolicyStore
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()
	state, err := loadCurrentResultAccessPolicy(ctx, tx)
	if err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	if err := tx.Commit(); err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	return state, nil
}

func (store *MySQLResultAccessPolicyStore) SetResultAccessPolicyEnabled(
	ctx context.Context,
	actorRef string,
	expectedVersion uint64,
	enabled bool,
) (application.ResultAccessPolicyState, error) {
	if store == nil || store.db == nil || ctx == nil || actorRef != "owner" {
		return application.ResultAccessPolicyState{}, ErrInvalidPolicyStore
	}
	if err := ctx.Err(); err != nil {
		return application.ResultAccessPolicyState{}, err
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	defer func() { _ = tx.Rollback() }()

	var currentVersion uint64
	if err := tx.QueryRowContext(ctx, "SELECT current_version FROM quant_result_access_policy_head WHERE policy_type = ? AND policy_ref = ? FOR UPDATE",
		resultAccessPolicyType, resultAccessPolicyRef).Scan(&currentVersion); err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	if currentVersion != expectedVersion {
		return application.ResultAccessPolicyState{}, application.ErrPolicyVersionConflict
	}
	current, err := loadPolicyVersion(ctx, tx, currentVersion)
	if err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	if current.Enabled == enabled {
		if err := tx.Commit(); err != nil {
			return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
		}
		return current, nil
	}
	if currentVersion == ^uint64(0) {
		return application.ResultAccessPolicyState{}, ErrPolicyStoreUnavailable
	}

	nextVersion := currentVersion + 1
	snapshotRef := resultAccessPolicySnapshotPrefix + strconv.FormatUint(nextVersion, 10)
	snapshot := application.PolicySnapshot{
		SchemaVersion: "1.0.0",
		SnapshotRef: snapshotRef,
		Policies: []application.PolicyVersion{{
			PolicyType: resultAccessPolicyType,
			PolicyRef: resultAccessPolicyRef,
			PolicyVersion: strconv.FormatUint(nextVersion, 10),
		}},
	}
	snapshotJSON, err := json.Marshal(snapshot)
	if err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO quant_result_access_policy_versions (policy_version, enabled, snapshot_ref, policy_snapshot_json, created_at) VALUES (?, ?, ?, ?, DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z'))",
		nextVersion, enabled, snapshotRef, string(snapshotJSON))
	if err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	update, err := tx.ExecContext(ctx, "UPDATE quant_result_access_policy_head SET current_version = ?, updated_at = DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z') WHERE policy_type = ? AND policy_ref = ? AND current_version = ?",
		nextVersion, resultAccessPolicyType, resultAccessPolicyRef, currentVersion)
	if err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	affected, err := update.RowsAffected()
	if err != nil || affected != 1 {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	action := "revoke"
	if enabled {
		action = "publish"
	}
	_, err = tx.ExecContext(ctx, "INSERT INTO quant_policy_admin_audit (actor_ref, policy_type, policy_ref, action, previous_version, new_version, occurred_at) VALUES (?, ?, ?, ?, ?, ?, DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z'))",
		actorRef, resultAccessPolicyType, resultAccessPolicyRef, action, currentVersion, nextVersion)
	if err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	if err := tx.Commit(); err != nil {
		return application.ResultAccessPolicyState{}, errors.Join(ErrPolicyStoreUnavailable, err)
	}
	return application.ResultAccessPolicyState{Version: nextVersion, Enabled: enabled, Snapshot: &snapshot}, nil
}

func loadCurrentResultAccessPolicy(ctx context.Context, tx *sql.Tx) (application.ResultAccessPolicyState, error) {
	var version uint64
	if err := tx.QueryRowContext(ctx, "SELECT current_version FROM quant_result_access_policy_head WHERE policy_type = ? AND policy_ref = ? FOR UPDATE",
		resultAccessPolicyType, resultAccessPolicyRef).Scan(&version); err != nil {
		return application.ResultAccessPolicyState{}, err
	}
	return loadPolicyVersion(ctx, tx, version)
}

type policyQueryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func loadPolicyVersion(ctx context.Context, queryer policyQueryer, version uint64) (application.ResultAccessPolicyState, error) {
	if version == 0 {
		return application.ResultAccessPolicyState{Version: 0, Enabled: false}, nil
	}
	var enabled bool
	var snapshotRef string
	var snapshotJSON []byte
	if err := queryer.QueryRowContext(ctx, "SELECT enabled, snapshot_ref, policy_snapshot_json FROM quant_result_access_policy_versions WHERE policy_version = ?", version).
		Scan(&enabled, &snapshotRef, &snapshotJSON); err != nil {
		return application.ResultAccessPolicyState{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(snapshotJSON))
	decoder.DisallowUnknownFields()
	var snapshot application.PolicySnapshot
	if err := decoder.Decode(&snapshot); err != nil {
		return application.ResultAccessPolicyState{}, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return application.ResultAccessPolicyState{}, errors.New("Quant policy snapshot contains trailing JSON")
	}
	expectedRef := resultAccessPolicySnapshotPrefix + strconv.FormatUint(version, 10)
	if snapshot.SchemaVersion != "1.0.0" || snapshot.SnapshotRef != expectedRef ||
		snapshotRef != expectedRef || len(snapshot.Policies) != 1 ||
		snapshot.Policies[0].PolicyType != resultAccessPolicyType ||
		snapshot.Policies[0].PolicyRef != resultAccessPolicyRef ||
		snapshot.Policies[0].PolicyVersion != strconv.FormatUint(version, 10) {
		return application.ResultAccessPolicyState{}, errors.New("Quant policy snapshot identity is corrupt")
	}
	return application.ResultAccessPolicyState{Version: version, Enabled: enabled, Snapshot: &snapshot}, nil
}
