package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

// requiredNewsSchemaColumns 是从 News 当前 Migration V1 提取的物理列契约。
// Migration 仍是唯一 DDL 来源；这里仅声明启动校验需要的列名。
var requiredNewsSchemaColumns = map[string][]string{
	"news_items": {
		"news_id", "canonical_url", "canonical_url_hash", "title", "publisher_name", "publisher_domain", "author", "published_at", "content_json", "content_fingerprint", "created_at", "updated_at",
	},
	"news_identifiers": {
		"source_id", "identifier_hash", "identifier_value", "news_id", "created_at",
	},
	"news_observations": {
		"observation_id", "idempotency_key", "source_id", "provider_code", "query_dimension", "endpoint_id", "source_policy_version_id", "endpoint_policy_version_id", "provider_output_policy_ref", "provider_output_policy_version_id", "policy_evaluated_at", "candidate_json", "observed_at", "retention_until", "payload_purged_at", "identity_status", "news_id", "reason_code",
	},
	"news_item_versions": {
		"version_id", "news_id", "version_no", "fingerprint", "title", "publisher_name", "publisher_domain", "author", "published_at", "content_json", "observed_at", "observation_id",
	},
	"news_authorization_snapshots": {
		"authorization_ref", "subject_ref", "resource_ref", "decision", "purpose_code", "content_level", "decided_at", "expires_at", "decision_fingerprint", "payload_json", "payload_hash",
	},
	"news_evidence_snapshots": {
		"snapshot_ref", "caller_ref", "filters_fingerprint", "page_size", "system_as_of", "sort_version", "created_at", "expires_at", "item_count", "payload_json", "payload_hash",
	},
	"news_policy_versions": {
		"policy_type", "policy_ref", "policy_version", "payload_json", "payload_hash", "created_at",
	},
	"news_current_policy_versions": {
		"policy_type", "policy_ref", "policy_version", "updated_at",
	},
	"news_version_availability_evidence": {
		"evidence_id", "news_id", "version_id", "observation_id", "field_path", "raw_value", "source_timezone_or_offset", "source_precision", "parse_status", "parse_reason_code", "candidate_available_at_utc", "parser_version", "evidence_ref", "evidence_hash", "observed_at", "recorded_at", "retention_until", "payload_purged_at",
	},
	"news_version_availability_decisions": {
		"decision_id", "version_id", "decision_seq", "status", "selected_evidence_id", "considered_evidence_ids", "reason_code", "review_rule_version", "supersedes_decision_id", "recorded_at",
	},
}

type newsSchemaIndexDefinition struct {
	table   string
	name    string
	unique  bool
	columns []string
}

type newsSchemaIndexState struct {
	unique  bool
	columns []string
}

// requiredNewsSchemaIndexes 覆盖 Migration V1 声明的全部主键、唯一键和普通索引。
var requiredNewsSchemaIndexes = []newsSchemaIndexDefinition{
	{table: "news_items", name: "PRIMARY", unique: true, columns: []string{"news_id"}},
	{table: "news_items", name: "uk_news_items_canonical_url_hash", unique: true, columns: []string{"canonical_url_hash"}},
	{table: "news_identifiers", name: "PRIMARY", unique: true, columns: []string{"source_id", "identifier_hash"}},
	{table: "news_identifiers", name: "idx_news_identifiers_news", unique: false, columns: []string{"news_id"}},
	{table: "news_observations", name: "PRIMARY", unique: true, columns: []string{"observation_id"}},
	{table: "news_observations", name: "uk_news_observations_idempotency", unique: true, columns: []string{"idempotency_key"}},
	{table: "news_observations", name: "uk_news_observations_id_news", unique: true, columns: []string{"observation_id", "news_id"}},
	{table: "news_observations", name: "idx_news_observations_source_time", unique: false, columns: []string{"source_id", "observed_at"}},
	{table: "news_observations", name: "idx_news_observations_news_time", unique: false, columns: []string{"news_id", "observed_at"}},
	{table: "news_observations", name: "idx_news_observations_retention", unique: false, columns: []string{"retention_until", "observation_id"}},
	{table: "news_item_versions", name: "PRIMARY", unique: true, columns: []string{"version_id"}},
	{table: "news_item_versions", name: "uk_news_versions_number", unique: true, columns: []string{"news_id", "version_no"}},
	{table: "news_item_versions", name: "uk_news_versions_id_news", unique: true, columns: []string{"version_id", "news_id"}},
	{table: "news_item_versions", name: "idx_news_versions_fingerprint", unique: false, columns: []string{"news_id", "fingerprint"}},
	{table: "news_item_versions", name: "idx_news_versions_observed", unique: false, columns: []string{"news_id", "observed_at", "version_no"}},
	{table: "news_authorization_snapshots", name: "PRIMARY", unique: true, columns: []string{"authorization_ref"}},
	{table: "news_authorization_snapshots", name: "idx_news_auth_subject_time", unique: false, columns: []string{"subject_ref", "decided_at"}},
	{table: "news_authorization_snapshots", name: "idx_news_auth_resource_time", unique: false, columns: []string{"resource_ref", "decided_at"}},
	{table: "news_authorization_snapshots", name: "idx_news_auth_expiry", unique: false, columns: []string{"expires_at"}},
	{table: "news_evidence_snapshots", name: "PRIMARY", unique: true, columns: []string{"snapshot_ref"}},
	{table: "news_evidence_snapshots", name: "idx_news_snapshot_caller_expiry", unique: false, columns: []string{"caller_ref", "expires_at"}},
	{table: "news_evidence_snapshots", name: "idx_news_snapshot_expiry", unique: false, columns: []string{"expires_at"}},
	{table: "news_policy_versions", name: "PRIMARY", unique: true, columns: []string{"policy_type", "policy_ref", "policy_version"}},
	{table: "news_policy_versions", name: "idx_news_policy_versions_created", unique: false, columns: []string{"policy_type", "policy_ref", "created_at"}},
	{table: "news_current_policy_versions", name: "PRIMARY", unique: true, columns: []string{"policy_type", "policy_ref"}},
	{table: "news_current_policy_versions", name: "idx_news_current_policy_version", unique: false, columns: []string{"policy_type", "policy_ref", "policy_version"}},
	{table: "news_version_availability_evidence", name: "PRIMARY", unique: true, columns: []string{"evidence_id"}},
	{table: "news_version_availability_evidence", name: "uk_news_availability_evidence_version", unique: true, columns: []string{"version_id", "evidence_id"}},
	{table: "news_version_availability_evidence", name: "idx_news_availability_evidence_pit", unique: false, columns: []string{"version_id", "observed_at", "recorded_at"}},
	{table: "news_version_availability_evidence", name: "idx_news_availability_evidence_observation", unique: false, columns: []string{"observation_id"}},
	{table: "news_version_availability_decisions", name: "PRIMARY", unique: true, columns: []string{"decision_id"}},
	{table: "news_version_availability_decisions", name: "uk_news_availability_decision_seq", unique: true, columns: []string{"version_id", "decision_seq"}},
	{table: "news_version_availability_decisions", name: "uk_news_availability_decision_version_id", unique: true, columns: []string{"version_id", "decision_id"}},
	{table: "news_version_availability_decisions", name: "uk_news_availability_decision_successor", unique: true, columns: []string{"supersedes_decision_id"}},
	{table: "news_version_availability_decisions", name: "idx_news_availability_decision_pit", unique: false, columns: []string{"version_id", "recorded_at", "decision_seq"}},
}

// ValidateSchema 只读检查 News 物理表、必需列和 Migration V1 索引。
// Migration History/Checksum 无法发现迁移完成后的人工 DDL 漂移；
// 因此 Root Server 必须在创建业务 Repository 前执行本检查。
func ValidateSchema(ctx context.Context, db *sql.DB) error {
	if ctx == nil || db == nil {
		return fmt.Errorf("News schema validation requires context and MariaDB")
	}

	const tableNames = "'news_items', 'news_identifiers', 'news_observations', 'news_item_versions', 'news_authorization_snapshots', 'news_evidence_snapshots', 'news_policy_versions', 'news_current_policy_versions', 'news_version_availability_evidence', 'news_version_availability_decisions'"
	tableRows, err := db.QueryContext(ctx, "SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND LEFT(table_name, 5) = 'news_' ORDER BY table_name")
	if err != nil {
		return fmt.Errorf("inspect News schema tables: %w", err)
	}
	actualTables := make([]string, 0, len(requiredNewsSchemaColumns))
	for tableRows.Next() {
		var table string
		if err := tableRows.Scan(&table); err != nil {
			_ = tableRows.Close()
			return fmt.Errorf("read News schema tables: %w", err)
		}
		actualTables = append(actualTables, table)
	}
	if err := tableRows.Err(); err != nil {
		_ = tableRows.Close()
		return fmt.Errorf("read News schema tables: %w", err)
	}
	if err := tableRows.Close(); err != nil {
		return fmt.Errorf("close News schema table inspection: %w", err)
	}
	if err := validateNewsSchemaTableSet(actualTables); err != nil {
		return err
	}
	rows, err := db.QueryContext(ctx, "SELECT table_name, column_name\n"+
		"FROM information_schema.columns\n"+
		"WHERE table_schema = DATABASE() AND table_name IN ("+tableNames+")\n"+
		"ORDER BY table_name, ordinal_position")
	if err != nil {
		return fmt.Errorf("inspect News schema columns: %w", err)
	}
	columns := make(map[string]map[string]struct{}, len(requiredNewsSchemaColumns))
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read News schema columns: %w", err)
		}
		if columns[table] == nil {
			columns[table] = make(map[string]struct{})
		}
		columns[table][column] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read News schema columns: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close News schema column inspection: %w", err)
	}
	if err := validateNewsSchemaColumns(columns); err != nil {
		return err
	}

	indexRows, err := db.QueryContext(ctx, "SELECT table_name, index_name, non_unique, seq_in_index, column_name\n"+
		"FROM information_schema.statistics\n"+
		"WHERE table_schema = DATABASE() AND table_name IN ("+tableNames+")\n"+
		"ORDER BY table_name, index_name, seq_in_index")
	if err != nil {
		return fmt.Errorf("inspect News schema indexes: %w", err)
	}
	indexes := make(map[string]map[string]newsSchemaIndexState, len(requiredNewsSchemaColumns))
	for indexRows.Next() {
		var table, name, column string
		var nonUnique, sequence int
		if err := indexRows.Scan(&table, &name, &nonUnique, &sequence, &column); err != nil {
			_ = indexRows.Close()
			return fmt.Errorf("read News schema indexes: %w", err)
		}
		if indexes[table] == nil {
			indexes[table] = make(map[string]newsSchemaIndexState)
		}
		state := indexes[table][name]
		state.unique = nonUnique == 0
		state.columns = append(state.columns, column)
		indexes[table][name] = state
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return fmt.Errorf("read News schema indexes: %w", err)
	}
	if err := indexRows.Close(); err != nil {
		return fmt.Errorf("close News schema index inspection: %w", err)
	}
	return validateNewsSchemaIndexes(indexes)
}

func validateNewsSchemaTableSet(actual []string) error {
	expected := make([]string, 0, len(requiredNewsSchemaColumns))
	for table := range requiredNewsSchemaColumns {
		expected = append(expected, table)
	}
	sort.Strings(expected)
	actual = append([]string(nil), actual...)
	sort.Strings(actual)
	if len(actual) != len(expected) {
		return fmt.Errorf("News schema table set mismatch: found %v; want %v", actual, expected)
	}
	for i := range expected {
		if actual[i] != expected[i] {
			return fmt.Errorf("News schema table set mismatch: found %v; want %v", actual, expected)
		}
	}
	return nil
}

func validateNewsSchemaColumns(actual map[string]map[string]struct{}) error {
	tables := make([]string, 0, len(requiredNewsSchemaColumns))
	for table := range requiredNewsSchemaColumns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		found, exists := actual[table]
		if !exists {
			return fmt.Errorf("News schema is incomplete: required table %s is missing", table)
		}
		missing := make([]string, 0)
		for _, column := range requiredNewsSchemaColumns[table] {
			if _, ok := found[column]; !ok {
				missing = append(missing, column)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("News schema is incomplete: table %s is missing required columns: %s", table, strings.Join(missing, ", "))
		}
	}
	return nil
}

func validateNewsSchemaIndexes(actual map[string]map[string]newsSchemaIndexState) error {
	for _, expected := range requiredNewsSchemaIndexes {
		found, exists := actual[expected.table][expected.name]
		if !exists {
			return fmt.Errorf("News schema is incomplete: table %s is missing required index %s", expected.table, expected.name)
		}
		if found.unique != expected.unique || !equalNewsSchemaColumns(found.columns, expected.columns) {
			return fmt.Errorf(
				"News schema index mismatch: table %s index %s has unique=%t columns=%v; want unique=%t columns=%v",
				expected.table, expected.name, found.unique, found.columns, expected.unique, expected.columns,
			)
		}
	}
	return nil
}

func equalNewsSchemaColumns(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
