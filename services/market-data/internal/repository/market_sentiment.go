package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// SaveMarketSentiment 追加保存一次市场情绪观测；同一交易日允许保留多次采集记录。
func (s *Store) SaveMarketSentiment(ctx context.Context, snapshot *model.MarketSentimentSnapshot) error {
	if s == nil || s.db == nil {
		return errors.New("MariaDB market-sentiment storage disabled")
	}
	if snapshot == nil || snapshot.Market != "cn_a_share" || snapshot.TradeDate == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("market-sentiment snapshot identity or fetched_at is invalid")
	}
	meta, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO market_data_sentiment_snapshots
(market, trade_date, fetched_at, source, quality_status, metadata)
VALUES (?, ?, ?, ?, ?, ?)`,
		snapshot.Market, snapshot.TradeDate,
		snapshot.FetchedAt.UTC().Format("2006-01-02 15:04:05.999999"),
		snapshot.Source, string(snapshot.QualityStatus), meta)
	return err
}

// ListMarketSentiment 按交易日返回每一天最近一次采集的快照。
func (s *Store) ListMarketSentiment(ctx context.Context, market, startDate, endDate string, limit int) ([]model.MarketSentimentSnapshot, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("MariaDB market-sentiment storage disabled")
	}
	if market != "cn_a_share" || limit < 1 || limit > 500 {
		return nil, errors.New("invalid market-sentiment query")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT s.metadata
FROM market_data_sentiment_snapshots s
JOIN (
  SELECT trade_date, MAX(id) AS latest_id
  FROM market_data_sentiment_snapshots
  WHERE market = ? AND trade_date >= ? AND trade_date <= ?
  GROUP BY trade_date
) latest ON latest.latest_id = s.id
ORDER BY s.trade_date DESC
LIMIT ?`, market, startDate, endDate, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.MarketSentimentSnapshot, 0)
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var item model.MarketSentimentSnapshot
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
