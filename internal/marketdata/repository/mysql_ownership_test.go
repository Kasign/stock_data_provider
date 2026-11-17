package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"testing"
)

type ownershipTestConnector struct{}

func (ownershipTestConnector) Connect(context.Context) (driver.Conn, error) {
	return ownershipTestConn{}, nil
}

func (ownershipTestConnector) Driver() driver.Driver {
	return ownershipTestDriver{}
}

type ownershipTestDriver struct{}

func (ownershipTestDriver) Open(string) (driver.Conn, error) {
	return ownershipTestConn{}, nil
}

type ownershipTestConn struct{}

func (ownershipTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare is not supported")
}

func (ownershipTestConn) Close() error {
	return nil
}

func (ownershipTestConn) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions are not supported")
}

func (ownershipTestConn) Ping(context.Context) error {
	return nil
}

func TestNewWithBorrowedDBDoesNotCloseCallerPool(t *testing.T) {
	db := sql.OpenDB(ownershipTestConnector{})
	defer db.Close()

	store, err := NewWithBorrowedDB(db)
	if err != nil {
		t.Fatalf("NewWithBorrowedDB() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("borrowed Store.Close() error = %v", err)
	}
	if err := db.PingContext(context.Background()); err != nil {
		t.Fatalf("borrowed Store.Close() closed caller pool: %v", err)
	}
}

func TestOwnedStoreCloseClosesItsPool(t *testing.T) {
	db := sql.OpenDB(ownershipTestConnector{})
	store := &Store{db: db, ownsDB: true}

	if err := store.Close(); err != nil {
		t.Fatalf("owned Store.Close() error = %v", err)
	}
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("owned Store.Close() must close its pool")
	}
}

func TestNewWithBorrowedDBRejectsNil(t *testing.T) {
	if _, err := NewWithBorrowedDB(nil); err == nil {
		t.Fatal("NewWithBorrowedDB(nil) must return an error")
	}
}
