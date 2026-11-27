package repository

import (
	"bytes"
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func validRepositoryOrderBookFactsSnapshot(t *testing.T) app.OrderBookFactsSnapshot {
	t.Helper()
	observed := time.Date(2026, 11, 25, 7, 0, 0, 0, time.UTC)
	book := model.OrderBook{
		Symbol: "600519.SH",
		Bids: []model.OrderBookLevel{
			{Price: 1313.1, Volume: 1200},
			{Price: 1313.0, Volume: 900},
			{Price: 1312.9, Volume: 700},
			{Price: 1312.8, Volume: 500},
			{Price: 1312.7, Volume: 300},
		},
		Asks: []model.OrderBookLevel{
			{Price: 1313.2, Volume: 1100},
			{Price: 1313.3, Volume: 800},
			{Price: 1313.4, Volume: 600},
			{Price: 1313.5, Volume: 400},
			{Price: 1313.6, Volume: 200},
		},
		ObservedAt:       observed,
		SystemObservedAt: observed.Add(time.Second),
		Source:           "eastmoney",
		QualityStatus:    model.QualityValid,
	}
	payload, err := app.BuildOrderBookFactsPayload(book)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := app.BuildOrderBookFactsSnapshot(
		payload,
		[]app.DailyBarsCloseSeriesSource{{SourceRef: "eastmoney"}},
		"principal:service:researchflow", "risk_review", observed.Add(2*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestOrderBookFactsSnapshotStorageDisabledFailsClosed(t *testing.T) {
	snapshot := validRepositoryOrderBookFactsSnapshot(t)
	if err := (*Store)(nil).SaveOrderBookFactsSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("SaveOrderBookFactsSnapshot() succeeded without storage")
	}
	if _, err := (*Store)(nil).GetOrderBookFactsSnapshot(context.Background(), snapshot.Manifest.SnapshotRef); err == nil {
		t.Fatal("GetOrderBookFactsSnapshot() succeeded without storage")
	}
	if err := (&Store{}).SaveOrderBookFactsSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("SaveOrderBookFactsSnapshot() succeeded with an unconfigured Store")
	}
}

func TestCanonicalOrderBookFactsSnapshotJSONIsStable(t *testing.T) {
	snapshot := validRepositoryOrderBookFactsSnapshot(t)
	firstManifest, firstPayload, err := canonicalOrderBookFactsSnapshotJSON(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	secondManifest, secondPayload, err := canonicalOrderBookFactsSnapshotJSON(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstManifest, secondManifest) || !bytes.Equal(firstPayload, secondPayload) {
		t.Fatal("canonical order-book facts JSON is not stable")
	}
	snapshot.Payload.TradableVolumeShares++
	if _, _, err := canonicalOrderBookFactsSnapshotJSON(snapshot); err == nil {
		t.Fatal("canonicalization accepted a snapshot that fails Verify()")
	}
}
