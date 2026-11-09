package security

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/storage"
)

const (
	OwnerCredentialMigrationNamespace = "platform.security"
	ownerCredentialTable              = "platform_owner_credentials"
	ownerCredentialID                 = 1
)

var (
	errOwnerCredentialNotFound = errors.New("owner credential does not exist")
	errOwnerCredentialConflict = errors.New("owner credential already exists or version changed")
)

// OwnerCredential is the single-owner credential snapshot. PasswordHash is a
// PHC-encoded Argon2id hash and must never be logged or exposed in API output.
type OwnerCredential struct {
	Subject           string
	PasswordHash      string
	CredentialVersion uint64
}

// CredentialStore persists the single Owner credential and its global
// revocation version. Password changes use compare-and-swap semantics.
type CredentialStore interface {
	GetOwnerCredential(ctx context.Context) (OwnerCredential, error)
	InitializeOwnerCredential(ctx context.Context, passwordHash string) (OwnerCredential, error)
	UpdateOwnerPassword(ctx context.Context, expectedVersion uint64, passwordHash string) (OwnerCredential, error)
}

// SQLCredentialStore is the MariaDB-backed implementation of CredentialStore.
// It never applies migrations implicitly; the Server Migration Registry must
// explicitly register OwnerCredentialMigrations.
type SQLCredentialStore struct {
	db        *sql.DB
	tableName string
}

func NewSQLCredentialStore(db *sql.DB) (*SQLCredentialStore, error) {
	if db == nil {
		return nil, fault.New(fault.ClassValidation, "platform.security.invalid_credential_store", "credential store database is not configured", nil)
	}
	return newSQLCredentialStore(db, ownerCredentialTable)
}

func newSQLCredentialStore(db *sql.DB, tableName string) (*SQLCredentialStore, error) {
	if db == nil || !validCredentialTableName(tableName) {
		return nil, fault.New(fault.ClassValidation, "platform.security.invalid_credential_store", "credential store database or table is not configured", nil)
	}
	return &SQLCredentialStore{db: db, tableName: tableName}, nil
}

func validCredentialTableName(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for index, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || (index > 0 && char == '_') {
			continue
		}
		return false
	}
	return true
}

// OwnerCredentialMigrations returns the Platform-owned schema migration.
// The Server is responsible for registering it with the Platform Migration Runner.
func OwnerCredentialMigrations() []storage.Migration {
	return ownerCredentialMigrations(ownerCredentialTable)
}

func ownerCredentialMigrations(tableName string) []storage.Migration {
	if !validCredentialTableName(tableName) {
		return nil
	}
	statement := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		owner_id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
		password_hash VARCHAR(512) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
		credential_version BIGINT UNSIGNED NOT NULL,
		created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
		updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
		CHECK (owner_id = 1)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`, tableName)
	return []storage.Migration{{
		Version: 1, Description: "create single owner credential store",
		Statements: []string{statement},
	}}
}

func (s *SQLCredentialStore) GetOwnerCredential(ctx context.Context) (OwnerCredential, error) {
	if err := s.validate(ctx); err != nil {
		return OwnerCredential{}, err
	}
	var credential OwnerCredential
	credential.Subject = "owner"
	err := s.db.QueryRowContext(ctx,
		"SELECT password_hash, credential_version FROM "+s.tableName+" WHERE owner_id = ?",
		ownerCredentialID,
	).Scan(&credential.PasswordHash, &credential.CredentialVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return OwnerCredential{}, fault.New(fault.ClassNotFound, "platform.security.owner_credential_not_found", "owner credential is not initialized", errOwnerCredentialNotFound)
	}
	if err != nil {
		return OwnerCredential{}, credentialStoreFault("could not read owner credential", err)
	}
	if credential.CredentialVersion == 0 {
		return OwnerCredential{}, fault.New(fault.ClassInternal, "platform.security.invalid_owner_credential", "stored owner credential is invalid", nil)
	}
	if _, _, _, err := parseArgon2idHash(credential.PasswordHash); err != nil {
		return OwnerCredential{}, fault.New(fault.ClassInternal, "platform.security.invalid_owner_credential", "stored owner credential is invalid", err)
	}
	return credential, nil
}

func (s *SQLCredentialStore) InitializeOwnerCredential(ctx context.Context, passwordHash string) (OwnerCredential, error) {
	if err := s.validate(ctx); err != nil {
		return OwnerCredential{}, err
	}
	if err := validateEncodedPasswordHash(passwordHash); err != nil {
		return OwnerCredential{}, fault.New(fault.ClassValidation, "platform.security.invalid_password_hash", "password hash is invalid", err)
	}
	_, err := s.db.ExecContext(ctx,
		"INSERT INTO "+s.tableName+" (owner_id, password_hash, credential_version) VALUES (?, ?, 1)",
		ownerCredentialID, passwordHash,
	)
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
			return OwnerCredential{}, fault.New(fault.ClassConflict, "platform.security.owner_credential_exists", "owner credential is already initialized", errOwnerCredentialConflict)
		}
		return OwnerCredential{}, credentialStoreFault("could not initialize owner credential", err)
	}
	return OwnerCredential{Subject: "owner", PasswordHash: passwordHash, CredentialVersion: 1}, nil
}

func (s *SQLCredentialStore) UpdateOwnerPassword(ctx context.Context, expectedVersion uint64, passwordHash string) (OwnerCredential, error) {
	if err := s.validate(ctx); err != nil {
		return OwnerCredential{}, err
	}
	if expectedVersion == 0 {
		return OwnerCredential{}, fault.New(fault.ClassValidation, "platform.security.invalid_credential_version", "credential version is invalid", nil)
	}
	if err := validateEncodedPasswordHash(passwordHash); err != nil {
		return OwnerCredential{}, fault.New(fault.ClassValidation, "platform.security.invalid_password_hash", "password hash is invalid", err)
	}
	if expectedVersion == ^uint64(0) {
		return OwnerCredential{}, fault.New(fault.ClassConflict, "platform.security.credential_version_exhausted", "owner credential version cannot be advanced", nil)
	}

	result, err := s.db.ExecContext(ctx,
		"UPDATE "+s.tableName+" SET password_hash = ?, credential_version = credential_version + 1 WHERE owner_id = ? AND credential_version = ?",
		passwordHash, ownerCredentialID, expectedVersion,
	)
	if err != nil {
		return OwnerCredential{}, credentialStoreFault("could not update owner credential", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return OwnerCredential{}, credentialStoreFault("could not confirm owner credential update", err)
	}
	if affected != 1 {
		var currentVersion uint64
		err := s.db.QueryRowContext(ctx,
			"SELECT credential_version FROM "+s.tableName+" WHERE owner_id = ?",
			ownerCredentialID,
		).Scan(&currentVersion)
		if errors.Is(err, sql.ErrNoRows) {
			return OwnerCredential{}, fault.New(fault.ClassNotFound, "platform.security.owner_credential_not_found", "owner credential is not initialized", errOwnerCredentialNotFound)
		}
		if err != nil {
			return OwnerCredential{}, credentialStoreFault("could not confirm owner credential version", err)
		}
		return OwnerCredential{}, fault.New(fault.ClassConflict, "platform.security.credential_version_conflict", "owner credential changed; reload before retrying", errOwnerCredentialConflict)
	}
	return OwnerCredential{Subject: "owner", PasswordHash: passwordHash, CredentialVersion: expectedVersion + 1}, nil
}

func (s *SQLCredentialStore) validate(ctx context.Context) error {
	if s == nil || s.db == nil || ctx == nil {
		return fault.New(fault.ClassValidation, "platform.security.invalid_credential_store", "credential store is not configured", nil)
	}
	return nil
}

func validateEncodedPasswordHash(encoded string) error {
	if _, _, _, err := parseArgon2idHash(encoded); err != nil {
		return fmt.Errorf("encoded Argon2id password hash is invalid: %w", err)
	}
	return nil
}

func credentialStoreFault(message string, cause error) *fault.Fault {
	return &fault.Fault{
		Class: fault.ClassUnavailable, Code: "platform.security.credential_store_unavailable",
		SafeMessage: message, Dependency: "mariadb", Retryable: false, Cause: cause,
	}
}
