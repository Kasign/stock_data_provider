package relay

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	notificationapplication "example.com/stock-analysis-flow/internal/notification/application"
	notificationschema "example.com/stock-analysis-flow/internal/notification/schema"
	notificationstorage "example.com/stock-analysis-flow/internal/notification/storage"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
)

type failOnceNotificationReceiver struct {
	receiver *notificationapplication.EventReceiver
	failed   bool
}

func (r *failOnceNotificationReceiver) Receive(ctx context.Context, raw []byte) error {
	if !r.failed {
		r.failed = true
		return errors.New("temporary Notification acceptance failure")
	}
	_, err := r.receiver.Receive(ctx, raw)
	return err
}

func TestResearchflowOutboxToNotificationInboxMariaDB(t *testing.T) {
	dsn := os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("SERVER_RELAY_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	adminDB, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB admin connection: %v", err)
	}
	defer adminDB.Close()
	if err := adminDB.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}
	databaseName := fmt.Sprintf("researchflow_relay_%d", time.Now().UnixNano())
	if _, err := adminDB.ExecContext(ctx, "CREATE DATABASE `"+databaseName+"`"); err != nil {
		t.Fatalf("create isolated Researchflow relay database: %v", err)
	}
	defer func() {
		_, _ = adminDB.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+databaseName+"`")
	}()
	mysqlConfig, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse MariaDB DSN: %v", err)
	}
	mysqlConfig.DBName = databaseName
	connector, err := mysql.NewConnector(mysqlConfig)
	if err != nil {
		t.Fatalf("create isolated MariaDB connector: %v", err)
	}
	db := sql.OpenDB(connector)
	defer db.Close()
	db.SetMaxOpenConns(8)

	migrations := servermigration.NewRegistry()
	if err := migrations.Register(researchflowstorage.MigrationNamespace, researchflowstorage.Migrations()); err != nil {
		t.Fatalf("register Researchflow migrations: %v", err)
	}
	if err := migrations.Register("notification", notificationstorage.Migrations()); err != nil {
		t.Fatalf("register Notification migrations: %v", err)
	}
	if _, err := migrations.RunWithReport(ctx, db); err != nil {
		t.Fatalf("apply Researchflow and Notification migrations: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	runRef := fmt.Sprintf("run_ntf_%d", now.UnixNano())
	outboxRef := fmt.Sprintf("outbox_ntf_%d", now.UnixNano())
	summaryRef := "summary_" + runRef
	correlationID := "corr_" + runRef
	systemAsOf := now.Add(-time.Hour).Format("2006-01-02T15:04:05.000000000Z")
	finishedAt := now.Format("2006-01-02T15:04:05.000000000Z")
	createdAt := now.Add(-time.Minute).Format("2006-01-02T15:04:05.000000000Z")
	deadlineAt := now.Add(time.Hour).Format("2006-01-02T15:04:05.000000000Z")

	payload := researchflowTerminalPayload{
		SourceEventRef: runRef, RunRef: runRef, Status: "succeeded",
		SystemAsOf: systemAsOf, FinishedAt: finishedAt,
		ResultSummaryRef: &summaryRef, TemplateRef: "daily_stock_analysis",
		TemplateVersion: "1.0.0",
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	payloadDigest := sha256.Sum256(payloadJSON)
	idempotencyKey := "run-terminal:" + runRef

	_, err = db.ExecContext(ctx, `INSERT INTO researchflow_runs (
		run_ref, schema_version, status, version, system_as_of, deadline_at,
		template_ref, template_version, template_fingerprint, input_snapshot_refs,
		resolved_template_snapshot, parameters_snapshot, originating_request_id,
		correlation_id, result_summary_ref, termination_reason, created_at, started_at, finished_at
	) VALUES (?, '1.0.0', 'succeeded', 3, ?, ?, ?, ?, ?, '[]', '{}', '{}', NULL, ?, ?, NULL, ?, ?, ?)`,
		runRef, systemAsOf, deadlineAt, payload.TemplateRef, payload.TemplateVersion,
		strings.Repeat("a", 64), correlationID, summaryRef, createdAt, createdAt, finishedAt)
	if err != nil {
		t.Fatalf("insert terminal Researchflow Run fixture: %v", err)
	}
	_, err = db.ExecContext(ctx, `INSERT INTO researchflow_outbox (
		outbox_ref, run_ref, event_type, schema_version, idempotency_key,
		correlation_id, payload_fingerprint, payload, created_at,
		published_at, attempt_count, last_error_code
	) VALUES (?, ?, 'researchflow.run.terminal', '1.0.0', ?, ?, ?, ?, ?, NULL, 0, NULL)`,
		outboxRef, runRef, idempotencyKey, correlationID,
		hex.EncodeToString(payloadDigest[:]), string(payloadJSON), finishedAt)
	if err != nil {
		t.Fatalf("insert Researchflow terminal Outbox fixture: %v", err)
	}

	runStore, err := researchflowstorage.NewMySQLRunRepository(db, researchflowstorage.CapacityLimits{
		MaxQueuedRuns: 10, MaxWorkers: 2,
	})
	if err != nil {
		t.Fatalf("create Researchflow Outbox Store: %v", err)
	}
	fanoutWriter, err := notificationstorage.NewMySQLFanoutWorkItemRepository(db)
	if err != nil {
		t.Fatalf("create Notification Fanout Writer: %v", err)
	}
	inbox, err := notificationstorage.NewMySQLInboxRepository(db, fanoutWriter)
	if err != nil {
		t.Fatalf("create Notification Inbox Repository: %v", err)
	}
	registry, err := notificationschema.NewRegistry(os.DirFS("../../../doc/notification/schema"), notificationschema.ResourceLimits{
		MaxEnvelopeBytes: 8192, MaxPayloadBytes: 4096, MaxJSONDepth: 16,
	})
	if err != nil {
		t.Fatalf("create Notification Schema Registry: %v", err)
	}
	receiver, err := notificationapplication.NewEventReceiver(registry, inbox, relayIntegrationClock{})
	if err != nil {
		t.Fatalf("create Notification EventReceiver: %v", err)
	}
	relay, err := NewResearchflowNotificationRelay(runStore, &failOnceNotificationReceiver{receiver: receiver}, ResearchflowNotificationConfig{
		WorkerID: "researchflow-notification-integration", BatchSize: 4,
		PollInterval: time.Second, LeaseDuration: 30 * time.Second,
		PublishTimeout: 10 * time.Second,
	}, nil)
	if err != nil {
		t.Fatalf("create Researchflow Notification Relay: %v", err)
	}

	firstDispatch, err := relay.ProcessOnce(ctx)
	if err != nil || firstDispatch.Claimed != 1 || firstDispatch.Published != 0 || firstDispatch.Retried != 1 {
		t.Fatalf("failed Notification acceptance should schedule Researchflow Outbox retry: result=%+v err=%v", firstDispatch, err)
	}
	var publishedAt, nextAttemptAt, lastErrorCode sql.NullString
	if err := db.QueryRowContext(ctx,
		"SELECT published_at, next_attempt_at, last_error_code FROM researchflow_outbox WHERE outbox_ref = ?", outboxRef,
	).Scan(&publishedAt, &nextAttemptAt, &lastErrorCode); err != nil {
		t.Fatalf("read Researchflow Outbox retry state: %v", err)
	}
	if publishedAt.Valid || !nextAttemptAt.Valid || lastErrorCode.String != "notification_receive_unavailable" {
		t.Fatalf("failed Notification acceptance was not persisted as retryable: published=%v next_attempt=%v error_code=%q", publishedAt, nextAttemptAt, lastErrorCode.String)
	}
	time.Sleep(1200 * time.Millisecond)
	retryDispatch, err := relay.ProcessOnce(ctx)
	if err != nil || retryDispatch.Claimed != 1 || retryDispatch.Published != 1 || retryDispatch.Retried != 0 {
		t.Fatalf("retry Researchflow Outbox through Notification Inbox: result=%+v err=%v", retryDispatch, err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT published_at FROM researchflow_outbox WHERE outbox_ref = ?", outboxRef,
	).Scan(&publishedAt); err != nil {
		t.Fatalf("read Researchflow Outbox delivery state: %v", err)
	}
	if !publishedAt.Valid || publishedAt.String == "" {
		t.Fatal("Researchflow Outbox was not acknowledged after Notification acceptance")
	}
	var inboxCount, fanoutCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_inbox WHERE source_module='researchflow' AND event_id=?",
		outboxRef,
	).Scan(&inboxCount); err != nil {
		t.Fatalf("count Notification Inbox records: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_fanout_work_items WHERE source_module='researchflow' AND event_id=?",
		outboxRef,
	).Scan(&fanoutCount); err != nil {
		t.Fatalf("count Notification Fanout WorkItems: %v", err)
	}
	if inboxCount != 1 || fanoutCount != 1 {
		t.Fatalf("cross-module delivery counts = inbox:%d fanout:%d, want exactly one each", inboxCount, fanoutCount)
	}

	if _, err := relay.ProcessOnce(ctx); err != nil {
		t.Fatalf("repeat Researchflow Outbox dispatch: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_inbox WHERE source_module='researchflow' AND event_id=?",
		outboxRef,
	).Scan(&inboxCount); err != nil {
		t.Fatalf("recount Notification Inbox records: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM notification_fanout_work_items WHERE source_module='researchflow' AND event_id=?",
		outboxRef,
	).Scan(&fanoutCount); err != nil {
		t.Fatalf("recount Notification Fanout WorkItems: %v", err)
	}
	if inboxCount != 1 || fanoutCount != 1 {
		t.Fatalf("replayed Researchflow event duplicated records: inbox:%d fanout:%d", inboxCount, fanoutCount)
	}
}
