package storage

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
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

type migrationTestState struct {
	applied                  map[uint64]string
	pending                  map[uint64]string
	failed                   map[uint64]int
	failureCode              map[uint64]string
	started                  map[uint64]bool
	finished                 map[uint64]bool
	failStatement            bool
	statements               []string
	failLockAcquire          bool
	lockResultNull           bool
	lockAcquired             bool
	lockReleased             bool
	failLockRelease          bool
	skipAppliedHistoryUpdate bool
	physicalConnClosed       bool
	historyCreated           bool
	historyColumnsUpgraded   bool
}

type migrationTestConnector struct {
	state *migrationTestState
}

func (c migrationTestConnector) Connect(context.Context) (driver.Conn, error) {
	return migrationTestConn{state: c.state}, nil
}

func (c migrationTestConnector) Driver() driver.Driver {
	return migrationTestDriver{state: c.state}
}

type migrationTestDriver struct {
	state *migrationTestState
}

func (d migrationTestDriver) Open(string) (driver.Conn, error) {
	return migrationTestConn{state: d.state}, nil
}

type migrationTestConn struct {
	state *migrationTestState
}

func (c migrationTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not supported in this test driver")
}

func (c migrationTestConn) Close() error {
	c.state.physicalConnClosed = true
	return nil
}

func (c migrationTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported in this test driver")
}

func (c migrationTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "GET_LOCK"):
		if c.state.failLockAcquire {
			return nil, errors.New("private lock acquisition detail")
		}
		var value driver.Value = int64(0)
		if c.state.lockResultNull {
			value = nil
		} else if c.state.lockAcquired {
			value = int64(1)
		}
		return &migrationTestRows{columns: []string{"locked"}, values: [][]driver.Value{{value}}}, nil
	case strings.Contains(query, "RELEASE_LOCK"):
		if c.state.failLockRelease {
			return nil, errors.New("private lock release detail")
		}
		c.state.lockReleased = true
		return &migrationTestRows{columns: []string{"released"}, values: [][]driver.Value{{int64(1)}}}, nil
	case strings.Contains(query, "SELECT version, checksum, status, failure_statement"):
		versions := make([]uint64, 0, len(c.state.applied)+len(c.state.failed))
		for version := range c.state.applied {
			versions = append(versions, version)
		}
		for version := range c.state.failed {
			versions = append(versions, version)
		}
		sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
		values := make([][]driver.Value, 0, len(versions))
		for _, version := range versions {
			if failureStatement, failed := c.state.failed[version]; failed {
				values = append(values, []driver.Value{int64(version), c.state.pending[version], "failed", int64(failureStatement)})
				continue
			}
			values = append(values, []driver.Value{int64(version), c.state.applied[version], "applied", int64(0)})
		}
		return &migrationTestRows{columns: []string{"version", "checksum", "status", "failure_statement"}, values: values}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

func (c migrationTestConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	switch {
	case strings.HasPrefix(query, "CREATE TABLE IF NOT EXISTS platform_schema_migrations"):
		c.state.historyCreated = true
	case strings.HasPrefix(query, "ALTER TABLE platform_schema_migrations ADD COLUMN IF NOT EXISTS"):
		c.state.historyColumnsUpgraded = true
	case strings.Contains(query, "INSERT INTO platform_schema_migrations"):
		version, ok := args[1].Value.(int64)
		if !ok {
			return nil, fmt.Errorf("unexpected migration version argument: %T", args[1].Value)
		}
		checksum, ok := args[3].Value.(string)
		if !ok {
			return nil, fmt.Errorf("unexpected migration checksum argument: %T", args[3].Value)
		}
		c.state.pending[uint64(version)] = checksum
		if c.state.started == nil {
			c.state.started = make(map[uint64]bool)
		}
		c.state.started[uint64(version)] = strings.Contains(query, "started_at")
	case strings.Contains(query, "UPDATE platform_schema_migrations"):
		versionArg := args[len(args)-1].Value.(int64)
		version := uint64(versionArg)
		if strings.Contains(query, "status = 'applied'") && c.state.skipAppliedHistoryUpdate {
			return driver.RowsAffected(0), nil
		}
		if strings.Contains(query, "status = 'failed'") {
			c.state.failed[version] = int(args[0].Value.(int64))
			if c.state.failureCode == nil {
				c.state.failureCode = make(map[uint64]string)
			}
			c.state.failureCode[version] = "platform.storage.migration_apply_failed"
		} else if strings.Contains(query, "status = 'applied'") {
			c.state.applied[version] = c.state.pending[version]
			delete(c.state.pending, version)
			delete(c.state.failed, version)
		}
		if c.state.finished == nil {
			c.state.finished = make(map[uint64]bool)
		}
		c.state.finished[version] = strings.Contains(query, "finished_at")
	default:
		if c.state.failStatement && strings.Contains(query, "CREATE TABLE failing_table") {
			return nil, errors.New("private migration SQL failure detail")
		}
		c.state.statements = append(c.state.statements, query)
	}
	return driver.RowsAffected(1), nil
}

type migrationTestRows struct {
	columns []string
	values  [][]driver.Value
	index   int
}

func (r *migrationTestRows) Columns() []string { return r.columns }
func (r *migrationTestRows) Close() error      { return nil }
func (r *migrationTestRows) Next(destination []driver.Value) error {
	if r.index >= len(r.values) {
		return io.EOF
	}
	copy(destination, r.values[r.index])
	r.index++
	return nil
}

func TestApplyMigrationsAppliesInOrderAndIsIdempotent(t *testing.T) {
	state := &migrationTestState{applied: make(map[uint64]string), pending: make(map[uint64]string), failed: make(map[uint64]int), lockAcquired: true}
	db := sql.OpenDB(migrationTestConnector{state: state})
	defer db.Close()
	migrations := []Migration{
		{Version: 2, Description: "add index", Statements: []string{"CREATE INDEX idx_value ON sample(value)"}},
		{Version: 1, Description: "create table", Statements: []string{"CREATE TABLE sample (id BIGINT PRIMARY KEY)"}},
	}
	if err := ApplyMigrations(context.Background(), db, "analysis", migrations); err != nil {
		t.Fatal(err)
	}
	if !state.historyCreated || !state.historyColumnsUpgraded || !state.lockReleased {
		t.Fatalf("migration lifecycle incomplete: history=%t columnsUpgraded=%t lockReleased=%t", state.historyCreated, state.historyColumnsUpgraded, state.lockReleased)
	}
	if !state.started[1] || !state.finished[1] || !state.finished[2] {
		t.Fatalf("migration execution timestamps were not recorded: started=%v finished=%v", state.started, state.finished)
	}
	if len(state.statements) != 2 || !strings.HasPrefix(state.statements[0], "CREATE TABLE") || !strings.HasPrefix(state.statements[1], "CREATE INDEX") {
		t.Fatalf("migrations were not applied in version order: %v", state.statements)
	}
	if len(state.applied) != 2 {
		t.Fatalf("expected two migration records, got %d", len(state.applied))
	}
	if err := ApplyMigrations(context.Background(), db, "analysis", migrations); err != nil {
		t.Fatal(err)
	}
	if len(state.statements) != 2 {
		t.Fatalf("already applied migrations ran again: %v", state.statements)
	}
}

func TestApplyMigrationsRejectsChecksumDriftAndUnknownHistory(t *testing.T) {
	migration := Migration{Version: 1, Description: "create table", Statements: []string{"CREATE TABLE sample (id BIGINT PRIMARY KEY)"}}
	_, checksums, err := prepareMigrations("analysis", []Migration{migration})
	if err != nil {
		t.Fatal(err)
	}
	for _, applied := range []map[uint64]string{
		{1: "different-checksum"},
		{2: "unknown-version"},
	} {
		state := &migrationTestState{applied: applied, pending: make(map[uint64]string), failed: make(map[uint64]int), lockAcquired: true}
		db := sql.OpenDB(migrationTestConnector{state: state})
		err := ApplyMigrations(context.Background(), db, "analysis", []Migration{migration})
		_ = db.Close()
		if err == nil {
			t.Fatal("migration history mismatch must be rejected")
		}
		if len(state.statements) != 0 {
			t.Fatalf("migration executed despite history mismatch: %v", state.statements)
		}
	}
	if checksums[1] == "" {
		t.Fatal("expected migration checksum")
	}
}

func TestApplyMigrationsRejectsHistoryUpdateWithoutAffectedRow(t *testing.T) {
	state := &migrationTestState{
		applied: make(map[uint64]string), pending: make(map[uint64]string),
		failed: make(map[uint64]int), lockAcquired: true,
		skipAppliedHistoryUpdate: true,
	}
	db := sql.OpenDB(migrationTestConnector{state: state})
	defer db.Close()

	err := ApplyMigrations(context.Background(), db, "analysis", []Migration{{
		Version: 1, Description: "create", Statements: []string{"CREATE TABLE sample (id INT)"},
	}})
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.migration_record_failed" {
		t.Fatalf("expected migration history update failure, got %v", err)
	}
	if len(state.statements) != 1 {
		t.Fatalf("migration SQL should have executed exactly once, got %v", state.statements)
	}
	if len(state.applied) != 0 || state.pending[1] == "" {
		t.Fatalf("migration must not be reported as applied when history update affects no row: applied=%v pending=%v", state.applied, state.pending)
	}
}

func TestApplyMigrationsDiscardsConnectionWhenLockAcquisitionIsAmbiguous(t *testing.T) {
	state := &migrationTestState{
		applied: make(map[uint64]string), pending: make(map[uint64]string),
		failed: make(map[uint64]int), lockAcquired: true, failLockAcquire: true,
	}
	db := sql.OpenDB(migrationTestConnector{state: state})
	defer db.Close()

	err := ApplyMigrations(context.Background(), db, "analysis", []Migration{{
		Version: 1, Description: "create", Statements: []string{"CREATE TABLE sample (id INT)"},
	}})
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.migration_lock_failed" {
		t.Fatalf("expected lock acquisition failure, got %v", err)
	}
	if !state.physicalConnClosed {
		t.Fatal("connection with an ambiguous advisory-lock result was returned to the pool")
	}
	if state.historyCreated || len(state.statements) != 0 {
		t.Fatalf("migration ran without confirmed lock ownership: %+v", state)
	}
}

func TestApplyMigrationsDiscardsConnectionWhenLockResultIsNull(t *testing.T) {
	state := &migrationTestState{
		applied: make(map[uint64]string), pending: make(map[uint64]string),
		failed: make(map[uint64]int), lockResultNull: true,
	}
	db := sql.OpenDB(migrationTestConnector{state: state})
	defer db.Close()

	err := ApplyMigrations(context.Background(), db, "analysis", []Migration{{
		Version: 1, Description: "create", Statements: []string{"CREATE TABLE sample (id INT)"},
	}})
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.migration_lock_failed" {
		t.Fatalf("expected ambiguous lock result failure, got %v", err)
	}
	if !state.physicalConnClosed {
		t.Fatal("connection with a NULL advisory-lock result was returned to the pool")
	}
	if state.historyCreated || len(state.statements) != 0 {
		t.Fatalf("migration ran without confirmed lock ownership: %+v", state)
	}
}

func TestApplyMigrationsDiscardsConnectionWhenLockReleaseFails(t *testing.T) {
	state := &migrationTestState{
		applied: make(map[uint64]string), pending: make(map[uint64]string),
		failed: make(map[uint64]int), lockAcquired: true, failLockRelease: true,
	}
	db := sql.OpenDB(migrationTestConnector{state: state})
	defer db.Close()

	err := ApplyMigrations(context.Background(), db, "analysis", []Migration{{
		Version: 1, Description: "create", Statements: []string{"CREATE TABLE sample (id INT)"},
	}})
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.migration_lock_release_failed" {
		t.Fatalf("expected lock release failure, got %v", err)
	}
	if state.lockReleased {
		t.Fatal("test driver unexpectedly released the advisory lock")
	}
	if !state.physicalConnClosed {
		t.Fatal("connection holding an unconfirmed advisory lock was returned to the pool")
	}
}

func TestApplyMigrationsRejectsConcurrentMigrationLock(t *testing.T) {
	state := &migrationTestState{applied: make(map[uint64]string), pending: make(map[uint64]string), failed: make(map[uint64]int), lockAcquired: false}
	db := sql.OpenDB(migrationTestConnector{state: state})
	defer db.Close()
	err := ApplyMigrations(context.Background(), db, "analysis", []Migration{{
		Version: 1, Description: "create", Statements: []string{"CREATE TABLE sample (id INT)"},
	}})
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.migration_lock_busy" {
		t.Fatalf("expected migration lock conflict, got %v", err)
	}
	if state.historyCreated || len(state.statements) != 0 || state.lockReleased {
		t.Fatalf("migration ran without owning lock: %+v", state)
	}
}

func TestApplyMigrationsRejectsMissingVersionBeforeAppliedVersion(t *testing.T) {
	migrations := []Migration{
		{Version: 1, Description: "first", Statements: []string{"CREATE TABLE first_table (id INT)"}},
		{Version: 2, Description: "second", Statements: []string{"CREATE TABLE second_table (id INT)"}},
	}
	_, checksums, err := prepareMigrations("analysis", migrations)
	if err != nil {
		t.Fatal(err)
	}
	state := &migrationTestState{
		applied:      map[uint64]string{2: checksums[2]},
		pending:      make(map[uint64]string),
		failed:       make(map[uint64]int),
		lockAcquired: true,
	}
	db := sql.OpenDB(migrationTestConnector{state: state})
	defer db.Close()
	if err := ApplyMigrations(context.Background(), db, "analysis", migrations); err == nil {
		t.Fatal("missing earlier migration must not be applied after a later version")
	}
	if len(state.statements) != 0 {
		t.Fatalf("migration statements ran despite out-of-order history: %v", state.statements)
	}
}

func TestApplyMigrationsRecordsFailureAndRequiresManualRecovery(t *testing.T) {
	state := &migrationTestState{
		applied:       make(map[uint64]string),
		pending:       make(map[uint64]string),
		failed:        make(map[uint64]int),
		lockAcquired:  true,
		failStatement: true,
	}
	db := sql.OpenDB(migrationTestConnector{state: state})
	defer db.Close()
	migrations := []Migration{{
		Version: 1, Description: "failing migration",
		Statements: []string{"CREATE TABLE failing_table (id INT)"},
	}}
	err := ApplyMigrations(context.Background(), db, "analysis", migrations)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.migration_apply_failed" {
		t.Fatalf("expected safe migration failure fault, got %v", err)
	}
	if strings.Contains(err.Error(), "private migration SQL failure detail") {
		t.Fatal("migration fault must not expose internal SQL error")
	}
	if state.failed[1] != 1 {
		t.Fatalf("expected failed migration statement 1 to be recorded, got %+v", state.failed)
	}
	if state.failureCode[1] != "platform.storage.migration_apply_failed" || !state.started[1] || !state.finished[1] {
		t.Fatalf("failed migration metadata is incomplete: code=%q started=%t finished=%t", state.failureCode[1], state.started[1], state.finished[1])
	}
	state.failStatement = false
	err = ApplyMigrations(context.Background(), db, "analysis", migrations)
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.storage.migration_requires_recovery" {
		t.Fatalf("expected manual recovery conflict, got %v", err)
	}
	if len(state.statements) != 0 {
		t.Fatalf("failed migration must not be retried automatically: %v", state.statements)
	}
}

func TestNormalizeRedisOptionsAppliesBoundedDefaults(t *testing.T) {
	options, err := normalizeRedisOptions(redis.Options{Addr: "redis:6379"})
	if err != nil {
		t.Fatal(err)
	}
	if options.PoolSize != defaultRedisPoolSize || options.MaxActiveConns != maxRedisPoolSize || options.MaxIdleConns != defaultRedisPoolSize {
		t.Fatalf("Redis options did not receive bounded defaults: %+v", options)
	}
}

func TestNormalizeRedisOptionsRejectsInvalidLimitsAndTimeouts(t *testing.T) {
	tests := []struct {
		name    string
		options redis.Options
	}{
		{"pool size above maximum", redis.Options{Addr: "redis:6379", PoolSize: maxRedisPoolSize + 1}},
		{"active connections above maximum", redis.Options{Addr: "redis:6379", MaxActiveConns: maxRedisPoolSize + 1}},
		{"pool size exceeds active connections", redis.Options{Addr: "redis:6379", PoolSize: 20, MaxActiveConns: 10}},
		{"idle exceeds active", redis.Options{Addr: "redis:6379", PoolSize: 10, MaxActiveConns: 20, MaxIdleConns: 21}},
		{"minimum idle exceeds maximum idle", redis.Options{Addr: "redis:6379", PoolSize: 10, MinIdleConns: 11, MaxIdleConns: 10}},
		{"negative minimum idle", redis.Options{Addr: "redis:6379", MinIdleConns: -1}},
		{"negative pool timeout", redis.Options{Addr: "redis:6379", PoolTimeout: -time.Second}},
		{"negative idle timeout", redis.Options{Addr: "redis:6379", ConnMaxIdleTime: -time.Second}},
		{"negative lifetime", redis.Options{Addr: "redis:6379", ConnMaxLifetime: -time.Second}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeRedisOptions(test.options); err == nil {
				t.Fatal("expected invalid Redis options to fail")
			}
		})
	}
}

func TestNormalizeSQLConfigAppliesCompleteDefaults(t *testing.T) {
	config, err := normalizeSQLConfig(SQLConfig{})
	if err != nil {
		t.Fatal(err)
	}
	defaults := DefaultSQLConfig()
	if config.MaxOpenConns != defaults.MaxOpenConns ||
		config.MaxIdleConns != defaults.MaxIdleConns ||
		config.ConnMaxLifetime != defaults.ConnMaxLifetime ||
		config.ConnMaxIdleTime != defaults.ConnMaxIdleTime ||
		config.PingTimeout != defaults.PingTimeout {
		t.Fatalf("zero-value SQL config did not receive complete defaults: got %+v want %+v", config, defaults)
	}
}


func TestDependencyFaultRetryabilityIsConservative(t *testing.T) {
	tests := []struct {
		name      string
		cause     error
		class     fault.Class
		retryable bool
	}{
		{name: "context deadline", cause: context.DeadlineExceeded, class: fault.ClassTimeout},
		{name: "context cancellation", cause: context.Canceled, class: fault.ClassUnavailable},
		{name: "permanent database error", cause: errors.New("access denied"), class: fault.ClassUnavailable},
		{name: "network timeout", cause: &net.DNSError{Err: "temporary resolver failure", IsTimeout: true}, class: fault.ClassTimeout, retryable: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := dependencyFault("sql", "platform.storage.dependency_failed", "database operation failed", test.cause)
			if got.Class != test.class || got.Retryable != test.retryable {
				t.Fatalf("dependencyFault() = class %q retryable %v, want class %q retryable %v",
					got.Class, got.Retryable, test.class, test.retryable)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("dependencyFault() returned invalid fault: %v", err)
			}
		})
	}
}
