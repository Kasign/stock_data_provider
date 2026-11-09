package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/notification/application"
	"example.com/stock-analysis-flow/internal/notification/domain"
)

// MySQLDeliveryRepository also provides a bounded, oldest-expiry-first scan.
var _ application.ExpiredDeliveryLeaseScanner = (*MySQLDeliveryRepository)(nil)

func (r *MySQLDeliveryRepository) ListExpiredLeases(ctx context.Context, limit int) ([]application.DeliveryClaim, error) {
	if r == nil || r.db == nil || ctx == nil || limit < 1 || limit > 1000 {
		return nil, errors.New("notification storage: invalid expired lease scan")
	}
	rows, err := r.db.QueryContext(ctx, `SELECT delivery_id
		FROM notification_deliveries
		WHERE status = ? AND lease_worker_id IS NOT NULL
		AND lease_expires_at <= UTC_TIMESTAMP(6)
		ORDER BY lease_expires_at ASC, delivery_id ASC
		LIMIT ?`, string(domain.DeliverySending), limit)
	if err != nil {
		return nil, fmt.Errorf("notification storage: query expired delivery lease ids: %w", err)
	}
	ids := make([]string, 0, limit)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("notification storage: scan expired delivery id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("notification storage: iterate expired delivery ids: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("notification storage: close expired delivery scan: %w", err)
	}

	claims := make([]application.DeliveryClaim, 0, len(ids))
	for _, id := range ids {
		delivery, found, err := r.Get(ctx, id)
		if err != nil {
			return nil, err
		}
		if !found || delivery.Status != domain.DeliverySending {
			continue
		}
		var workerID sql.NullString
		var expiresAt []byte
		var fencingToken int64
		err = r.db.QueryRowContext(ctx, `SELECT lease_worker_id,
			DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'), fencing_token
			FROM notification_deliveries WHERE delivery_id = ? AND status = ?`,
			id, string(domain.DeliverySending)).Scan(&workerID, &expiresAt, &fencingToken)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("notification storage: read expired delivery lease: %w", err)
		}
		if !workerID.Valid || len(expiresAt) == 0 || fencingToken < 1 {
			continue
		}
		expiry, err := parseDatabaseTime(expiresAt)
		if err != nil {
			return nil, err
		}
		claims = append(claims, application.DeliveryClaim{
			Delivery: delivery,
			Lease: application.DeliveryLease{WorkerID: workerID.String, FencingToken: fencingToken, ExpiresAt: expiry},
		})
	}
	return claims, nil
}
