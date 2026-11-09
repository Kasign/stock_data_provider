package security

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/storage"
)

func TestMariaDBCredentialStoreIntegration(t *testing.T) {
	if os.Getenv("PLATFORM_STORAGE_INTEGRATION") != "1" {
		t.Skip("set PLATFORM_STORAGE_INTEGRATION=1 to run MariaDB credential store integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	dsn := fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",
		os.Getenv("PLATFORM_MARIADB_USER"),
		os.Getenv("PLATFORM_MARIADB_PASSWORD"),
		os.Getenv("PLATFORM_MARIADB_ADDR"),
		os.Getenv("PLATFORM_MARIADB_DATABASE"),
	)
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	connector, err := mysql.NewConnector(config)
	if err != nil {
		t.Fatal(err)
	}
	db, err := storage.OpenSQL(ctx, connector, storage.DefaultSQLConfig())
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer db.Close()

	if err := storage.ApplyMigrations(ctx, db, OwnerCredentialMigrationNamespace, OwnerCredentialMigrations()); err != nil {
		t.Fatalf("apply owner credential migration: %v", err)
	}
	store, err := NewSQLCredentialStore(db)
	if err != nil {
		t.Fatal(err)
	}
	hasher := testPasswordHasher(t, 8*1024)
	initialHash, err := hasher.Hash([]byte("initial integration password"))
	if err != nil {
		t.Fatal(err)
	}
	credential, err := store.InitializeOwnerCredential(ctx, initialHash)
	if err != nil {
		t.Fatal(err)
	}
	if credential.Subject != "owner" || credential.CredentialVersion != 1 || credential.PasswordHash != initialHash {
		t.Fatalf("unexpected initialized credential: %+v", credential)
	}
	if _, err := store.InitializeOwnerCredential(ctx, initialHash); err == nil {
		t.Fatal("second initialization must conflict")
	} else {
		var storeFault *fault.Fault
		if !errors.As(err, &storeFault) || storeFault.Class != fault.ClassConflict {
			t.Fatalf("second initialization error = %v, want conflict", err)
		}
	}

	loaded, err := store.GetOwnerCredential(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CredentialVersion != 1 || loaded.PasswordHash != initialHash {
		t.Fatalf("loaded credential mismatch: %+v", loaded)
	}
	if ok, err := hasher.Verify(loaded.PasswordHash, []byte("initial integration password")); err != nil || !ok {
		t.Fatalf("stored password hash did not verify: ok=%v err=%v", ok, err)
	}

	updatedHash, err := hasher.Hash([]byte("updated integration password"))
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateOwnerPassword(ctx, 1, updatedHash)
	if err != nil {
		t.Fatal(err)
	}
	if updated.CredentialVersion != 2 || updated.PasswordHash != updatedHash {
		t.Fatalf("updated credential mismatch: %+v", updated)
	}
	if _, err := store.UpdateOwnerPassword(ctx, 1, initialHash); err == nil {
		t.Fatal("stale credential version must conflict")
	} else {
		var storeFault *fault.Fault
		if !errors.As(err, &storeFault) || storeFault.Class != fault.ClassConflict {
			t.Fatalf("stale update error = %v, want conflict", err)
		}
	}

	// Two simultaneous password changes using the same observed version must
	// produce exactly one winner and one conflict.
	hashA, err := hasher.Hash([]byte("concurrent password A"))
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := hasher.Hash([]byte("concurrent password B"))
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan error, 2)
	for _, hash := range []string{hashA, hashB} {
		wait.Add(1)
		go func(value string) {
			defer wait.Done()
			_, updateErr := store.UpdateOwnerPassword(ctx, 2, value)
			results <- updateErr
		}(hash)
	}
	wait.Wait()
	close(results)
	successes, conflicts := 0, 0
	for updateErr := range results {
		if updateErr == nil {
			successes++
			continue
		}
		var storeFault *fault.Fault
		if errors.As(updateErr, &storeFault) && storeFault.Class == fault.ClassConflict {
			conflicts++
			continue
		}
		t.Fatalf("unexpected concurrent update error: %v", updateErr)
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent updates: successes=%d conflicts=%d, want 1 each", successes, conflicts)
	}
	finalCredential, err := store.GetOwnerCredential(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if finalCredential.CredentialVersion != 3 {
		t.Fatalf("credential version after concurrent updates = %d, want 3", finalCredential.CredentialVersion)
	}
}
