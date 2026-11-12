package security

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"testing"
	"time"

	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

type auditWriterTestConnector struct {
	connection *auditWriterTestConnection
}

func (c auditWriterTestConnector) Connect(context.Context) (driver.Conn, error) {
	return c.connection, nil
}

func (c auditWriterTestConnector) Driver() driver.Driver {
	return auditWriterTestDriver{}
}

type auditWriterTestDriver struct{}

func (auditWriterTestDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("unexpected Open call")
}

type auditWriterTestConnection struct {
	query string
	args  []driver.NamedValue
	err   error
}

func (c *auditWriterTestConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("Prepare is not supported")
}

func (c *auditWriterTestConnection) Close() error {
	return nil
}

func (c *auditWriterTestConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (c *auditWriterTestConnection) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	c.query = query
	c.args = append([]driver.NamedValue(nil), args...)
	if c.err != nil {
		return nil, c.err
	}
	return driver.RowsAffected(1), nil
}

func newAuditWriterTestDB(connection *auditWriterTestConnection) *sql.DB {
	return sql.OpenDB(auditWriterTestConnector{connection: connection})
}

func validAuditEvent() platformsecurity.SecurityAuditEvent {
	return platformsecurity.SecurityAuditEvent{
		EventType: platformsecurity.AuditLoginSucceeded,
		Subject: "owner",
		Result: platformsecurity.AuditResultSuccess,
		OccurredAt: time.Date(2026, 11, 12, 9, 0, 0, 0, time.FixedZone("test", 8*60*60)),
		RequestID: "request-123",
		Source: platformsecurity.AuditSourceOwnerSession,
		ReasonCode: "credentials_verified",
	}
}

func TestSQLSecurityAuditWriterPersistsAllowlistedFields(t *testing.T) {
	connection := &auditWriterTestConnection{}
	db := newAuditWriterTestDB(connection)
	defer db.Close()
	writer, err := NewSQLSecurityAuditWriter(db)
	if err != nil {
		t.Fatalf("NewSQLSecurityAuditWriter() error = %v", err)
	}
	event := validAuditEvent()
	if err := writer.Write(context.Background(), event); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if !strings.Contains(connection.query, "INSERT INTO "+securityAuditTable) || len(connection.args) != 7 {
		t.Fatalf("unexpected audit insert: query=%q args=%+v", connection.query, connection.args)
	}
	if got := connection.args[3].Value.(time.Time); !got.Equal(event.OccurredAt.UTC()) || got.Location() != time.UTC {
		t.Fatalf("stored occurred_at = %v, want UTC %v", got, event.OccurredAt.UTC())
	}
	want := []string{"login.succeeded", "owner", "success", "request-123", "owner_session", "credentials_verified"}
	indices := []int{0, 1, 2, 4, 5, 6}
	for i, index := range indices {
		if got := connection.args[index].Value; got != want[i] {
			t.Fatalf("argument %d = %v, want %q", index, got, want[i])
		}
	}
}

func TestSQLSecurityAuditWriterRejectsInvalidEventBeforeDatabaseAccess(t *testing.T) {
	connection := &auditWriterTestConnection{}
	db := newAuditWriterTestDB(connection)
	defer db.Close()
	writer, err := NewSQLSecurityAuditWriter(db)
	if err != nil {
		t.Fatalf("NewSQLSecurityAuditWriter() error = %v", err)
	}
	event := validAuditEvent()
	event.ReasonCode = "contains secret or unbounded text!"
	if err := writer.Write(context.Background(), event); err == nil {
		t.Fatal("Write() accepted an invalid audit event")
	}
	if connection.query != "" {
		t.Fatalf("invalid event reached database: %q", connection.query)
	}
}

func TestSQLSecurityAuditWriterReturnsTypedDependencyFault(t *testing.T) {
	connection := &auditWriterTestConnection{err: errors.New("database unavailable")}
	db := newAuditWriterTestDB(connection)
	defer db.Close()
	writer, err := NewSQLSecurityAuditWriter(db)
	if err != nil {
		t.Fatalf("NewSQLSecurityAuditWriter() error = %v", err)
	}
	err = writer.Write(context.Background(), validAuditEvent())
	var fault *platformfault.Fault
	if !errors.As(err, &fault) || fault.Code != "server.security_audit.write_failed" ||
		fault.Dependency != "mariadb" || fault.Retryable {
		t.Fatalf("Write() error = %#v, want non-retryable MariaDB dependency Fault", err)
	}
}

func TestSQLSecurityAuditWriterRequiresDatabase(t *testing.T) {
	if _, err := NewSQLSecurityAuditWriter(nil); err == nil {
		t.Fatal("NewSQLSecurityAuditWriter(nil) error = nil")
	}
	var writer *SQLSecurityAuditWriter
	if err := writer.Write(context.Background(), validAuditEvent()); err == nil {
		t.Fatal("nil writer Write() error = nil")
	}
}

func TestSecurityAuditMigrationsExposeOneVersionedSchema(t *testing.T) {
	migrations := SecurityAuditMigrations()
	if len(migrations) != 1 || migrations[0].Version != 1 ||
		migrations[0].Description == "" || len(migrations[0].Statements) != 1 ||
		!strings.Contains(migrations[0].Statements[0], securityAuditTable) {
		t.Fatalf("invalid Security Audit migrations: %+v", migrations)
	}
}
