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
	query := deliverySelectColumns + `,
		lease_worker_id, DATE_FORMAT(lease_expires_at, '%Y-%m-%d %H:%i:%s.%f'),
		fencing_token
		FROM notification_deliveries
		WHERE status = ? AND lease_worker_id IS NOT NULL
		AND lease_expires_at <= UTC_TIMESTAMP(6)
		ORDER BY lease_expires_at ASC, delivery_id ASC
		LIMIT ?`
	rows, err := r.db.QueryContext(ctx, query, string(domain.DeliverySending), limit)
	if err != nil {
		return nil, fmt.Errorf("notification storage: query expired delivery lease snapshots: %w", err)
	}
	defer rows.Close()

	claims := make([]application.DeliveryClaim, 0, limit)
	for rows.Next() {
		var workerID sql.NullString
		var expiresAt []byte
		var fencingToken int64
		delivery, err := scanDelivery(deliveryLeaseRow{
			deliveryScanner: rows, workerID: &workerID,
			expiresAt: &expiresAt, fencingToken: &fencingToken,
		})
		if err != nil {
			return nil, fmt.Errorf("notification storage: scan expired delivery lease snapshot: %w", err)
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
			Lease: application.DeliveryLease{
				WorkerID: workerID.String, FencingToken: fencingToken, ExpiresAt: expiry,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("notification storage: iterate expired delivery lease snapshots: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("notification storage: close expired delivery lease scan: %w", err)
	}

	return claims, nil
}

type deliveryLeaseRow struct {
	deliveryScanner
	workerID     *sql.NullString
	expiresAt    *[]byte
	fencingToken *int64
}

func (row deliveryLeaseRow) Scan(dest ...any) error {
	return row.deliveryScanner.Scan(append(dest, row.workerID, row.expiresAt, row.fencingToken)...)
}
