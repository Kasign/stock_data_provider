package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// SaveProviderProbeRun 保存主动探测批次的完整脱敏结果。
func (s *Store) SaveProviderProbeRun(ctx context.Context, run *model.ProviderProbeRun) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB provider-probe storage disabled")
	}
	if run == nil || run.RunID == "" || run.StartedAt.IsZero() || run.FinishedAt.IsZero() {
		return errors.New("provider-probe run identity or timestamps are invalid")
	}
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO market_data_provider_probe_runs
(run_id, started_at, finished_at, status, metadata)
VALUES (?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE finished_at = VALUES(finished_at), status = VALUES(status), metadata = VALUES(metadata)`,
		run.RunID, run.StartedAt.UTC().Format("2006-01-02 15:04:05.999999"),
		run.FinishedAt.UTC().Format("2006-01-02 15:04:05.999999"), run.Status, raw)
	return err
}

// ListProviderProbeRuns 返回最近的主动探测批次。
func (s *Store) ListProviderProbeRuns(ctx context.Context, limit int) ([]model.ProviderProbeRun, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB provider-probe storage disabled")
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("provider-probe history limit must be between 1 and 100")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT metadata FROM market_data_provider_probe_runs ORDER BY started_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.ProviderProbeRun, 0, limit)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item model.ProviderProbeRun
		if err := json.Unmarshal(raw, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}
