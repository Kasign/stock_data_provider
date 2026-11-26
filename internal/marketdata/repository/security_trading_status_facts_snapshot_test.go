package repository

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
)

func validRepositoryTradingFactsSnapshot(t *testing.T) app.SecurityTradingStatusFactsSnapshot {
	t.Helper()
	payload := app.SecurityTradingStatusFactsPayload{
		SchemaVersion: app.SecurityTradingStatusFactsPayloadSchemaVersion,
		PayloadKind:   app.SecurityTradingStatusFactsPayloadKind,
		Symbol:        "600519.SH",
		TradeDate:     "2026-11-25",
		QualityStatus: app.QualityValid,
		Facts: []app.SecurityTradingStatusFact{
			{Key: "risk_warning_status", Availability: app.TradingFactAvailable, Value: "normal"},
			{Key: "suspension_status", Availability: app.TradingFactAvailable, Value: "no_suspension_event"},
			{Key: "delisting_status", Availability: app.TradingFactAvailable, Value: "normal"},
			{Key: "listing_date", Availability: app.TradingFactAvailable, Value: "2001-08-27"},
			{Key: "is_trading_day", Availability: app.TradingFactAvailable, Value: "true"},
		},
	}
	snapshot, err := app.BuildSecurityTradingStatusFactsSnapshot(
		payload,
		[]app.DailyBarsCloseSeriesSource{{SourceRef: "tushare"}},
		"principal:service:researchflow",
		"risk_review",
		time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("BuildSecurityTradingStatusFactsSnapshot() error = %v", err)
	}
	return snapshot
}

func TestSecurityTradingStatusFactsSnapshotStorageDisabledFailsClosed(t *testing.T) {
	snapshot := validRepositoryTradingFactsSnapshot(t)
	if err := (*Store)(nil).SaveSecurityTradingStatusFactsSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("SaveSecurityTradingStatusFactsSnapshot() succeeded without storage")
	}
	if _, err := (*Store)(nil).GetSecurityTradingStatusFactsSnapshot(context.Background(), snapshot.Manifest.SnapshotRef); err == nil {
		t.Fatal("GetSecurityTradingStatusFactsSnapshot() succeeded without storage")
	}
	if err := (&Store{}).SaveSecurityTradingStatusFactsSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("SaveSecurityTradingStatusFactsSnapshot() succeeded with an unconfigured Store")
	}
}

func TestSecurityTradingStatusFactsSnapshotRefRejectsMalformedValues(t *testing.T) {
	store := &Store{}
	for _, value := range []string{"", "data_snapshot:sha256:xyz", "data_snapshot:sha256:" + "A2345678"} {
		if _, err := store.GetSecurityTradingStatusFactsSnapshot(context.Background(), value); err == nil {
			t.Fatalf("malformed SnapshotRef was accepted: %q", value)
		}
	}
}

func TestCanonicalSecurityTradingStatusFactsSnapshotJSONIsStable(t *testing.T) {
	snapshot := validRepositoryTradingFactsSnapshot(t)
	firstManifest, firstPayload, err := canonicalSecurityTradingStatusFactsSnapshotJSON(snapshot)
	if err != nil {
		t.Fatalf("canonicalSecurityTradingStatusFactsSnapshotJSON() error = %v", err)
	}
	secondManifest, secondPayload, err := canonicalSecurityTradingStatusFactsSnapshotJSON(snapshot)
	if err != nil {
		t.Fatalf("second canonicalization error = %v", err)
	}
	if string(firstManifest) != string(secondManifest) || string(firstPayload) != string(secondPayload) {
		t.Fatal("canonical JSON of the same Snapshot must be byte-identical")
	}
}

// 内容与指纹必须绑定：改动任一事实后，同一个 Snapshot 结构体不能再产出可写入的规范 JSON。
func TestCanonicalSecurityTradingStatusFactsJSONRejectsTamperedPayload(t *testing.T) {
	snapshot := validRepositoryTradingFactsSnapshot(t)
	snapshot.Payload.Facts[0].Value = "risk_warning"
	if _, _, err := canonicalSecurityTradingStatusFactsSnapshotJSON(snapshot); err == nil {
		t.Fatal("tampered payload produced canonical JSON")
	}
}
