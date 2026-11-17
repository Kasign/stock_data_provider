package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"
)

func TestNewMySQLDatabaseClockRejectsNilDatabase(t *testing.T) {
	if _, err := NewMySQLDatabaseClock(nil); err == nil {
		t.Fatal("nil SQL database was accepted")
	}
}

func TestMySQLDatabaseClockRejectsNilContext(t *testing.T) {
	clock, err := NewMySQLDatabaseClock(&sql.DB{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clock.NowUTC(nil); err == nil {
		t.Fatal("nil context was accepted")
	}
}

func TestMariaDBDatabaseClockUsesDatabaseUTC(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to verify the MariaDB database clock")
	}
	cfg := mysql.NewConfig()
	cfg.User = os.Getenv("PLATFORM_MARIADB_USER")
	cfg.Passwd = os.Getenv("PLATFORM_MARIADB_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = os.Getenv("PLATFORM_MARIADB_ADDR")
	cfg.DBName = os.Getenv("PLATFORM_MARIADB_DATABASE")
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	if cfg.User == "" || cfg.Addr == "" || cfg.DBName == "" {
		t.Fatal("MariaDB integration connection settings are incomplete")
	}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	read := func() (time.Time, error) {
		var value string
		if err := db.QueryRowContext(ctx, researchflowDatabaseClockQuery).Scan(&value); err != nil {
			return time.Time{}, err
		}
		return parseResearchflowTime(value)
	}
	before, err := read()
	if err != nil {
		t.Fatal(err)
	}
	clock, err := NewMySQLDatabaseClock(db)
	if err != nil {
		t.Fatal(err)
	}
	got, err := clock.NowUTC(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, err := read()
	if err != nil {
		t.Fatal(err)
	}
	if got.Location() != time.UTC || got.Before(before) || got.After(after) {
		t.Fatalf("database clock returned %s; expected UTC timestamp between %s and %s", got, before, after)
	}
}

func TestMySQLDatabaseClockReturnsQueryErrors(t *testing.T) {
	clock, err := NewMySQLDatabaseClock(&sql.DB{})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = clock.NowUTC(ctx)
	if err == nil || errors.Is(err, context.Canceled) == false {
		t.Fatalf("cancelled database clock query should preserve context error, got %v", err)
	}
	_ = fmt.Sprintf("%v", err)
}
