package storage

import (
	"context"
	"database/sql/driver"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

type testConnector struct {
	pingErr error
	closed  atomic.Bool
}

func (c *testConnector) Connect(context.Context) (driver.Conn, error) {
	return &testConn{owner: c}, nil
}

func (c *testConnector) Driver() driver.Driver {
	return testDriver{owner: c}
}

type testDriver struct {
	owner *testConnector
}

func (d testDriver) Open(string) (driver.Conn, error) {
	return &testConn{owner: d.owner}, nil
}

type testConn struct {
	owner *testConnector
}

func (c *testConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("not implemented")
}

func (c *testConn) Close() error {
	c.owner.closed.Store(true)
	return nil
}

func (c *testConn) Begin() (driver.Tx, error) {
	return nil, errors.New("not implemented")
}

func (c *testConn) Ping(context.Context) error {
	return c.owner.pingErr
}

func TestOpenSQLAppliesPoolSettingsAndCloses(t *testing.T) {
	connector := &testConnector{}
	db, err := OpenSQL(context.Background(), connector, SQLConfig{
		MaxOpenConns: 3, MaxIdleConns: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := db.Stats().MaxOpenConnections; got != 3 {
		t.Fatalf("MaxOpenConnections = %d, want 3", got)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if !connector.closed.Load() {
		t.Fatal("expected underlying connection to be closed")
	}
}

func TestOpenSQLClosesDatabaseWhenPingFails(t *testing.T) {
	connector := &testConnector{pingErr: errors.New("private connection detail")}
	db, err := OpenSQL(context.Background(), connector, SQLConfig{})
	if db != nil {
		t.Fatal("database must not be returned when ping fails")
	}
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.sql_unavailable" {
		t.Fatalf("expected SQL unavailable fault, got %v", err)
	}
	if !connector.closed.Load() {
		t.Fatal("failed database connection must be closed")
	}
	if strings.Contains(err.Error(), "private connection detail") {
		t.Fatal("safe fault message must not expose internal connection details")
	}
}

func TestOpenSQLRejectsInvalidPoolSettings(t *testing.T) {
	for _, config := range []SQLConfig{
		{MaxOpenConns: 2, MaxIdleConns: 3},
		{MaxOpenConns: maxSQLMaxOpenConns + 1},
		{MaxIdleConns: -1},
		{ConnMaxLifetime: -time.Second},
		{PingTimeout: maxPingTimeout + time.Second},
	} {
		if db, err := OpenSQL(context.Background(), &testConnector{}, config); err == nil || db != nil {
			t.Fatalf("expected invalid SQL config to fail: %+v", config)
		}
	}
}

func TestOpenSQLRejectsMissingDependencies(t *testing.T) {
	if db, err := OpenSQL(context.Background(), nil, SQLConfig{}); err == nil || db != nil {
		t.Fatal("nil connector must be rejected")
	}
	if err := PingSQL(context.Background(), nil); err == nil {
		t.Fatal("nil database must be rejected")
	}
}

func TestOpenRedisClosesClientWhenPingFails(t *testing.T) {
	options := redis.Options{
		Addr: "redis.invalid:6379",
		Dialer: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return nil, errors.New("private redis connection detail")
		},
	}
	client, err := OpenRedis(context.Background(), options, time.Second)
	if client != nil {
		t.Fatal("Redis client must not be returned when ping fails")
	}
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.redis_unavailable" {
		t.Fatalf("expected Redis unavailable fault, got %v", err)
	}
	if strings.Contains(err.Error(), "private redis connection detail") {
		t.Fatal("safe fault message must not expose internal Redis connection details")
	}
}

func TestOpenRedisReturnsTimeoutFault(t *testing.T) {
	options := redis.Options{
		Addr: "redis.invalid:6379",
		Dialer: func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	client, err := OpenRedis(context.Background(), options, 10*time.Millisecond)
	if client != nil {
		t.Fatal("Redis client must not be returned after timeout")
	}
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Class != fault.ClassTimeout {
		t.Fatalf("expected Redis timeout fault, got %v", err)
	}
}

func TestOpenRedisRejectsInvalidOptions(t *testing.T) {
	for _, options := range []redis.Options{
		{},
		{Addr: "redis:6379", DB: -1},
		{Addr: "redis:6379", PoolSize: maxSQLMaxOpenConns + 1},
		{Addr: "redis:6379", DialTimeout: -time.Second},
	} {
		if client, err := OpenRedis(context.Background(), options, time.Second); err == nil || client != nil {
			t.Fatalf("expected invalid Redis options to fail: %+v", options)
		}
	}
}

func TestPrepareMigrationsSortsAndChecksums(t *testing.T) {
	migrations := []Migration{
		{Version: 2, Description: "add index", Statements: []string{"CREATE INDEX idx_x ON t(x)"}},
		{Version: 1, Description: "create table", Statements: []string{"CREATE TABLE t (id INT PRIMARY KEY)"}},
	}
	ordered, checksums, err := prepareMigrations("analysis", migrations)
	if err != nil {
		t.Fatal(err)
	}
	if ordered[0].Version != 1 || ordered[1].Version != 2 {
		t.Fatalf("migrations were not sorted: %+v", ordered)
	}
	changed := []Migration{{Version: 1, Description: "create table", Statements: []string{"CREATE TABLE t (id BIGINT PRIMARY KEY)"}}}
	_, changedChecksums, err := prepareMigrations("analysis", changed)
	if err != nil {
		t.Fatal(err)
	}
	if checksums[1] == changedChecksums[1] {
		t.Fatal("changing migration SQL must change its checksum")
	}
}

func TestPrepareMigrationsRejectsInvalidDefinitions(t *testing.T) {
	tests := []struct {
		name       string
		namespace  string
		migrations []Migration
	}{
		{name: "empty namespace", namespace: "", migrations: []Migration{{Version: 1, Description: "one", Statements: []string{"SELECT 1"}}}},
		{name: "uppercase namespace", namespace: "Analysis", migrations: []Migration{{Version: 1, Description: "one", Statements: []string{"SELECT 1"}}}},
		{name: "duplicate version", namespace: "analysis", migrations: []Migration{
			{Version: 1, Description: "one", Statements: []string{"SELECT 1"}},
			{Version: 1, Description: "duplicate", Statements: []string{"SELECT 2"}},
		}},
		{name: "zero version", namespace: "analysis", migrations: []Migration{{Version: 0, Description: "zero", Statements: []string{"SELECT 1"}}}},
		{name: "empty statement", namespace: "analysis", migrations: []Migration{{Version: 1, Description: "empty", Statements: []string{" "}}}},
		{name: "empty description", namespace: "analysis", migrations: []Migration{{Version: 1, Statements: []string{"SELECT 1"}}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := prepareMigrations(test.namespace, test.migrations); err == nil {
				t.Fatal("expected invalid migration definition to fail")
			}
		})
	}
}

func TestMigrationLockNameIsBoundedAndNamespaceScoped(t *testing.T) {
	first := migrationLockName("analysis")
	second := migrationLockName("news")
	if first == second {
		t.Fatal("different namespaces must use different advisory locks")
	}
	if len(first) > 64 {
		t.Fatalf("MariaDB lock name exceeds 64 characters: %d", len(first))
	}
}

func TestApplyMigrationsRejectsMissingDatabase(t *testing.T) {
	migration := []Migration{{Version: 1, Description: "create", Statements: []string{"SELECT 1"}}}
	if err := ApplyMigrations(context.Background(), nil, "analysis", migration); err == nil {
		t.Fatal("nil database must be rejected")
	}
}

var _ driver.Pinger = (*testConn)(nil)
var _ driver.Connector = (*testConnector)(nil)
