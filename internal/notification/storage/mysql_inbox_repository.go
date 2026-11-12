package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

// InboxFanoutWriter 在 Inbox 事务中写入对应 Fanout 工作项。
// 实现必须复用传入事务，不得自行提交或开启独立事务；字段、状态和游标语义以本模块 03 文档第 13 节为准。
type InboxFanoutWriter interface {
	InsertForAcceptedEvent(ctx context.Context, tx *sql.Tx, record domain.InboxRecord) error
}

// MySQLInboxRepository 使用数据库双唯一约束实现跨进程事件幂等。
// FanoutWriter 与 Inbox 记录共享同一事务，避免接收成功但丢失 Fanout 工作项。
type MySQLInboxRepository struct {
	db           *sql.DB
	fanoutWriter InboxFanoutWriter
}

var _ application.InboxRepository = (*MySQLInboxRepository)(nil)

// NewMySQLInboxRepository 必须注入事务内 Fanout 写入器；不允许降级为仅写 Inbox。
func NewMySQLInboxRepository(db *sql.DB, fanoutWriter InboxFanoutWriter) (*MySQLInboxRepository, error) {
	if db == nil || fanoutWriter == nil {
		return nil, errors.New("notification storage: database and transactional fanout writer are required")
	}
	return &MySQLInboxRepository{db: db, fanoutWriter: fanoutWriter}, nil
}

func (r *MySQLInboxRepository) Accept(
	ctx context.Context,
	event domain.EventEnvelope,
	receivedAt time.Time,
) (domain.InboxRecord, bool, error) {
	if r == nil || r.db == nil || r.fanoutWriter == nil || ctx == nil {
		return domain.InboxRecord{}, false, errors.New("notification storage: inbox repository is not configured")
	}
	if receivedAt.IsZero() || receivedAt.Location() != time.UTC {
		return domain.InboxRecord{}, false, errors.New("notification storage: inbox receive time must be UTC")
	}
	// 与 notification_inbox.received_at 的 DATETIME(6) 精度保持一致。
	receivedAt = receivedAt.Truncate(time.Microsecond)
	if err := event.VerifyPayloadFingerprint(); err != nil {
		return domain.InboxRecord{}, false, err
	}
	payload, err := json.Marshal(json.RawMessage(event.Payload))
	if err != nil {
		return domain.InboxRecord{}, false, fmt.Errorf("notification storage: encode inbox payload: %w", err)
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.InboxRecord{}, false, fmt.Errorf("notification storage: begin inbox accept: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, `INSERT INTO notification_inbox (
		source_module, event_id, idempotency_key, event_type, schema_version,
		subject_ref, occurred_at, published_at, correlation_id,
		payload_fingerprint, payload, received_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.SourceModule, event.EventID, event.IdempotencyKey, event.EventType,
		event.SchemaVersion, event.SubjectRef, event.OccurredAt, event.PublishedAt,
		event.CorrelationID, event.PayloadFingerprint, payload, receivedAt,
	)
	if isMySQLDuplicateKey(err) {
		_ = tx.Rollback()
		existing, found, loadErr := r.findByEitherIdentity(ctx, event)
		if loadErr != nil {
			return domain.InboxRecord{}, false, loadErr
		}
		if !found {
			return domain.InboxRecord{}, false, fmt.Errorf("notification storage: inbox unique constraint conflict without matching row: %w", err)
		}
		if replayErr := domain.ValidateInboxReplay(existing.Event, event); replayErr != nil {
			return domain.InboxRecord{}, false, replayErr
		}
		return existing, true, nil
	}
	if err != nil {
		return domain.InboxRecord{}, false, fmt.Errorf("notification storage: insert inbox event: %w", err)
	}

	record := domain.InboxRecord{Event: cloneInboxEvent(event), ReceivedAt: receivedAt}
	if err := r.fanoutWriter.InsertForAcceptedEvent(ctx, tx, record); err != nil {
		return domain.InboxRecord{}, false, fmt.Errorf("notification storage: insert fanout work item: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return domain.InboxRecord{}, false, fmt.Errorf("%w: notification storage: commit inbox accept: %w", application.ErrPersistenceOutcomeUnknown, err)
	}
	return record, false, nil
}

func (r *MySQLInboxRepository) findByEitherIdentity(
	ctx context.Context,
	event domain.EventEnvelope,
) (domain.InboxRecord, bool, error) {
	rows, err := r.db.QueryContext(ctx, inboxSelect+`
		WHERE source_module = ? AND (event_id = ? OR idempotency_key = ?)`,
		event.SourceModule, event.EventID, event.IdempotencyKey)
	if err != nil {
		return domain.InboxRecord{}, false, fmt.Errorf("notification storage: lookup inbox replay: %w", err)
	}
	defer rows.Close()

	var found *domain.InboxRecord
	for rows.Next() {
		record, scanErr := scanInboxRecord(rows)
		if scanErr != nil {
			return domain.InboxRecord{}, false, scanErr
		}
		if found != nil {
			return domain.InboxRecord{}, false, domain.ErrInboxIdempotencyConflict
		}
		copy := record
		found = &copy
	}
	if err := rows.Err(); err != nil {
		return domain.InboxRecord{}, false, fmt.Errorf("notification storage: iterate inbox replay: %w", err)
	}
	if found == nil {
		return domain.InboxRecord{}, false, nil
	}
	return *found, true, nil
}

const inboxSelect = `SELECT
	source_module, event_id, idempotency_key, event_type, schema_version,
	subject_ref, occurred_at, published_at, correlation_id,
	payload_fingerprint, payload, DATE_FORMAT(received_at, '%Y-%m-%d %H:%i:%s.%f')
	FROM notification_inbox`

type inboxScanner interface {
	Scan(dest ...any) error
}

func scanInboxRecord(row inboxScanner) (domain.InboxRecord, error) {
	var record domain.InboxRecord
	var payload, receivedAt []byte
	event := &record.Event
	if err := row.Scan(
		&event.SourceModule, &event.EventID, &event.IdempotencyKey,
		&event.EventType, &event.SchemaVersion, &event.SubjectRef,
		&event.OccurredAt, &event.PublishedAt, &event.CorrelationID,
		&event.PayloadFingerprint, &payload, &receivedAt,
	); err != nil {
		return domain.InboxRecord{}, fmt.Errorf("notification storage: scan inbox record: %w", err)
	}
	if !json.Valid(payload) {
		return domain.InboxRecord{}, errors.New("notification storage: stored inbox payload is invalid JSON")
	}
	event.Payload = append(json.RawMessage(nil), payload...)
	parsed, err := parseDatabaseTime(receivedAt)
	if err != nil {
		return domain.InboxRecord{}, fmt.Errorf("notification storage: parse inbox receive time: %w", err)
	}
	record.ReceivedAt = parsed
	if err := event.VerifyPayloadFingerprint(); err != nil {
		return domain.InboxRecord{}, fmt.Errorf("notification storage: stored inbox fingerprint is invalid: %w", err)
	}
	return record, nil
}

func cloneInboxEvent(event domain.EventEnvelope) domain.EventEnvelope {
	event.Payload = append(json.RawMessage(nil), event.Payload...)
	return event
}
