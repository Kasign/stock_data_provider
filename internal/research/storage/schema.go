package storage

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
)

var requiredResearchSchemaColumns = map[string][]string{
	"research_aggregate_heads": {
		"aggregate_type", "aggregate_id", "latest_version", "created_at", "updated_at",
	},
	"research_context_versions": {
		"context_id", "context_version", "system_as_of", "payload_sha256", "payload", "created_at",
	},
	"research_evidence_records": {
		"evidence_id", "source_owner", "source_ref", "source_version", "available_at",
		"system_observed_at", "system_as_of", "quality_status", "content_level",
		"authorization_ref", "content_fingerprint", "payload_sha256", "payload", "created_at",
	},
	"research_event_versions": {
		"event_id", "event_version", "status", "system_as_of", "payload_sha256", "payload", "created_at",
	},
	"research_claim_versions": {
		"claim_id", "claim_version", "epistemic_type", "status", "system_as_of",
		"payload_sha256", "payload", "created_at",
	},
	"research_notification_outbox": {
		"event_id", "event_type", "schema_version", "source_module", "occurred_at",
		"published_at", "subject_ref", "idempotency_key", "correlation_id",
		"payload_fingerprint", "payload", "delivery_status", "available_at",
		"lease_owner", "lease_token", "lease_until", "attempt_count", "last_error_code",
		"delivered_at", "created_at", "updated_at",
	},
}

type researchSchemaIndexDefinition struct {
	table   string
	name    string
	unique  bool
	columns []string
}

type researchSchemaIndexState struct {
	unique  bool
	columns []string
}

var requiredResearchSchemaIndexes = []researchSchemaIndexDefinition{
	{table: "research_aggregate_heads", name: "PRIMARY", unique: true, columns: []string{"aggregate_type", "aggregate_id"}},
	{table: "research_context_versions", name: "PRIMARY", unique: true, columns: []string{"context_id", "context_version"}},
	{table: "research_context_versions", name: "idx_research_context_asof", columns: []string{"context_id", "system_as_of"}},
	{table: "research_evidence_records", name: "PRIMARY", unique: true, columns: []string{"evidence_id"}},
	{table: "research_evidence_records", name: "idx_research_evidence_source", columns: []string{"source_owner", "source_ref", "source_version"}},
	{table: "research_evidence_records", name: "idx_research_evidence_pit", columns: []string{"system_as_of", "available_at"}},
	{table: "research_event_versions", name: "PRIMARY", unique: true, columns: []string{"event_id", "event_version"}},
	{table: "research_event_versions", name: "idx_research_event_status_asof", columns: []string{"status", "system_as_of"}},
	{table: "research_claim_versions", name: "PRIMARY", unique: true, columns: []string{"claim_id", "claim_version"}},
	{table: "research_claim_versions", name: "idx_research_claim_status_asof", columns: []string{"status", "system_as_of"}},
	{table: "research_notification_outbox", name: "PRIMARY", unique: true, columns: []string{"event_id"}},
	{table: "research_notification_outbox", name: "uq_research_outbox_idempotency", unique: true, columns: []string{"source_module", "idempotency_key"}},
	{table: "research_notification_outbox", name: "idx_research_outbox_created", columns: []string{"created_at"}},
	{table: "research_notification_outbox", name: "idx_research_outbox_delivery", columns: []string{"delivery_status", "available_at", "created_at"}},
}

// ValidateSchema performs a read-only check of the physical Research tables,
// required columns and indexes. Applied migration checksums do not detect
// manual DDL drift after a migration has been recorded as applied.
func ValidateSchema(ctx context.Context, db *sql.DB) error {
	if ctx == nil || db == nil {
		return fmt.Errorf("Research schema validation requires context and MariaDB")
	}

	const tableNames = "'research_aggregate_heads', 'research_context_versions', " +
		"'research_evidence_records', 'research_event_versions', 'research_claim_versions', " +
		"'research_notification_outbox'"
	rows, err := db.QueryContext(ctx, "SELECT table_name, column_name\n"+
		"FROM information_schema.columns\n"+
		"WHERE table_schema = DATABASE() AND table_name IN ("+tableNames+")\n"+
		"ORDER BY table_name, ordinal_position")
	if err != nil {
		return fmt.Errorf("inspect Research schema columns: %w", err)
	}
	columns := make(map[string]map[string]struct{}, len(requiredResearchSchemaColumns))
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read Research schema columns: %w", err)
		}
		if columns[table] == nil {
			columns[table] = make(map[string]struct{})
		}
		columns[table][column] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read Research schema columns: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close Research schema column inspection: %w", err)
	}
	if err := validateResearchSchemaColumns(columns); err != nil {
		return err
	}

	indexRows, err := db.QueryContext(ctx, "SELECT table_name, index_name, non_unique, seq_in_index, column_name\n"+
		"FROM information_schema.statistics\n"+
		"WHERE table_schema = DATABASE() AND table_name IN ("+tableNames+")\n"+
		"ORDER BY table_name, index_name, seq_in_index")
	if err != nil {
		return fmt.Errorf("inspect Research schema indexes: %w", err)
	}
	indexes := make(map[string]map[string]researchSchemaIndexState, len(requiredResearchSchemaColumns))
	for indexRows.Next() {
		var table, name, column string
		var nonUnique, sequence int
		if err := indexRows.Scan(&table, &name, &nonUnique, &sequence, &column); err != nil {
			_ = indexRows.Close()
			return fmt.Errorf("read Research schema indexes: %w", err)
		}
		if indexes[table] == nil {
			indexes[table] = make(map[string]researchSchemaIndexState)
		}
		state := indexes[table][name]
		state.unique = nonUnique == 0
		state.columns = append(state.columns, column)
		indexes[table][name] = state
	}
	if err := indexRows.Err(); err != nil {
		_ = indexRows.Close()
		return fmt.Errorf("read Research schema indexes: %w", err)
	}
	if err := indexRows.Close(); err != nil {
		return fmt.Errorf("close Research schema index inspection: %w", err)
	}
	if err := validateResearchSchemaIndexes(indexes); err != nil {
		return err
	}
	return nil
}

func validateResearchSchemaColumns(actual map[string]map[string]struct{}) error {
	tables := make([]string, 0, len(requiredResearchSchemaColumns))
	for table := range requiredResearchSchemaColumns {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		found, exists := actual[table]
		if !exists {
			return fmt.Errorf("Research schema is incomplete: required table %s is missing", table)
		}
		missing := make([]string, 0)
		for _, column := range requiredResearchSchemaColumns[table] {
			if _, ok := found[column]; !ok {
				missing = append(missing, column)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("Research schema is incomplete: table %s is missing required columns: %s", table, strings.Join(missing, ", "))
		}
	}
	return nil
}

func validateResearchSchemaIndexes(actual map[string]map[string]researchSchemaIndexState) error {
	for _, expected := range requiredResearchSchemaIndexes {
		found, exists := actual[expected.table][expected.name]
		if !exists {
			return fmt.Errorf("Research schema is incomplete: table %s is missing required index %s", expected.table, expected.name)
		}
		if found.unique != expected.unique || !equalResearchSchemaColumns(found.columns, expected.columns) {
			return fmt.Errorf(
				"Research schema index mismatch: table %s index %s has unique=%t columns=%v; want unique=%t columns=%v",
				expected.table, expected.name, found.unique, found.columns, expected.unique, expected.columns,
			)
		}
	}
	return nil
}

func equalResearchSchemaColumns(left, right []string) bool {
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
