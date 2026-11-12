package storage

import (
	"context"
	"database/sql"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

var _ app.NewsPolicyVersionWriter = (*MySQLRepository)(nil)

// PublishNewsPolicyVersion 在同一 InnoDB 事务中写入不可变版本并切换当前指针。
// 历史 payload/hash 从不更新；相同版本的不同载荷会被拒绝。
func (r *MySQLRepository) PublishNewsPolicyVersion(ctx context.Context, value domain.NewsPolicyVersion) error {
	if r == nil || r.db == nil || ctx == nil || domain.ValidateNewsPolicyVersion(value) != nil {
		return app.ErrNewsPolicyVersionStore
	}
	sum := sha256.Sum256(value.Payload)
	actualHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(actualHash), []byte(value.PayloadHash)) != 1 {
		return app.ErrNewsPolicyVersionStore
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return errors.Join(app.ErrNewsPolicyVersionStore, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// ON DUPLICATE KEY 的更新仅写回主键本身，不触碰历史 payload/hash。
	_, err = tx.ExecContext(ctx, `INSERT INTO news_policy_versions
		(policy_type, policy_ref, policy_version, payload_json, payload_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE policy_version = VALUES(policy_version)`,
		value.PolicyType, value.PolicyRef, value.PolicyVersion, value.Payload,
		value.PayloadHash, value.CreatedAt.UTC())
	if err != nil {
		return errors.Join(app.ErrNewsPolicyVersionStore, err)
	}

	var storedHash string
	if err := tx.QueryRowContext(ctx, `SELECT payload_hash
		FROM news_policy_versions
		WHERE policy_type = ? AND policy_ref = ? AND policy_version = ?
		FOR UPDATE`,
		value.PolicyType, value.PolicyRef, value.PolicyVersion,
	).Scan(&storedHash); err != nil {
		return errors.Join(app.ErrNewsPolicyVersionStore, err)
	}
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(value.PayloadHash)) != 1 {
		return app.ErrNewsPolicyVersionConflict
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO news_current_policy_versions
		(policy_type, policy_ref, policy_version, updated_at)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			policy_version = VALUES(policy_version),
			updated_at = VALUES(updated_at)`,
		value.PolicyType, value.PolicyRef, value.PolicyVersion, time.Now().UTC())
	if err != nil {
		return errors.Join(app.ErrNewsPolicyVersionStore, err)
	}
	if err := tx.Commit(); err != nil {
		return errors.Join(app.ErrNewsPolicyVersionStore, err)
	}
	committed = true
	return nil
}
