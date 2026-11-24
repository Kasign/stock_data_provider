package bootstrap

import (
	"context"
	"strings"
	"testing"
	"time"

	marketdataapp "example.com/stock-analysis-flow/internal/marketdata/app"
	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
)

type quantDataSnapshotPortStub struct {
	createRefs    marketdataapplication.DailyBarsCloseSeriesSnapshotReferences
	createQuery   marketdataapp.DailyBarsQuery
	createPurpose string
	createErr     error
	readSnapshot  marketdataapp.DailyBarsCloseSeriesSnapshot
	readRef       string
	readAuthRef   string
	readPurpose   string
	readErr       error
}

func (stub *quantDataSnapshotPortStub) CreateDailyBarsCloseSeriesSnapshot(_ context.Context, query marketdataapp.DailyBarsQuery, purpose string) (marketdataapplication.DailyBarsCloseSeriesSnapshotReferences, error) {
	stub.createQuery = query
	stub.createPurpose = purpose
	if stub.createErr != nil {
		return marketdataapplication.DailyBarsCloseSeriesSnapshotReferences{}, stub.createErr
	}
	return stub.createRefs, nil
}

func (stub *quantDataSnapshotPortStub) ReadAuthorizedDailyBarsCloseSeriesSnapshot(_ context.Context, snapshotRef, authorizationRef, purpose string) (marketdataapp.DailyBarsCloseSeriesSnapshot, error) {
	stub.readRef, stub.readAuthRef, stub.readPurpose = snapshotRef, authorizationRef, purpose
	if stub.readErr != nil {
		return marketdataapp.DailyBarsCloseSeriesSnapshot{}, stub.readErr
	}
	return stub.readSnapshot, nil
}

func TestQuantDataSnapshotAdapterCreatesPITUnadjustedSnapshotForQuantPurpose(t *testing.T) {
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	stub := &quantDataSnapshotPortStub{
		createRefs: marketdataapplication.DailyBarsCloseSeriesSnapshotReferences{
			SnapshotRef:      "data_snapshot:sha256:" + strings.Repeat("a", 64),
			AuthorizationRef: "data_authorization:sha256:" + strings.Repeat("b", 64),
		},
	}
	adapter, err := newQuantDataSnapshotAdapter(stub)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.CreateDailyBarsCloseSeriesSnapshot(context.Background(), quantstage.SnapshotCreateRequest{
		Symbol: "600036.SH", StartDate: "2026-10-01", EndDate: "2026-10-30",
		Limit: 20, SystemAsOf: cutoff, AllowedProviderCodes: []string{"eastmoney"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stub.createPurpose != quantstage.SMA20PurposeCode ||
		stub.createQuery.Symbol != "600036.SH" || stub.createQuery.StartDate != "2026-10-01" ||
		stub.createQuery.EndDate != "2026-10-30" || stub.createQuery.Limit != 20 ||
		stub.createQuery.AdjustmentBasis != marketdataapp.AdjustmentNone ||
		stub.createQuery.SystemObservedAtCutoff == nil || !stub.createQuery.SystemObservedAtCutoff.Equal(cutoff) ||
		len(stub.createQuery.AllowedProviderCodes) != 1 || stub.createQuery.AllowedProviderCodes[0] != "eastmoney" {
		t.Fatalf("Data Snapshot query/purpose mismatch: %+v purpose=%q", stub.createQuery, stub.createPurpose)
	}
	if got.SnapshotRef != stub.createRefs.SnapshotRef || got.AuthorizationRef != stub.createRefs.AuthorizationRef {
		t.Fatalf("Snapshot references were not preserved: %+v", got)
	}
}

func TestQuantDataSnapshotAdapterMapsOnlyVerifiedAuthorizedSnapshot(t *testing.T) {
	snapshot := validQuantDataCloseSeriesSnapshot(t)
	stub := &quantDataSnapshotPortStub{readSnapshot: snapshot}
	adapter, err := newQuantDataSnapshotAdapter(stub)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.ReadAuthorizedCloseSeriesSnapshot(
		context.Background(), snapshot.Manifest.SnapshotRef, "data_authorization:sha256:"+strings.Repeat("b", 64),
		quantstage.SMA20PurposeCode,
	)
	if err != nil {
		t.Fatal(err)
	}
	if stub.readRef != snapshot.Manifest.SnapshotRef || stub.readPurpose != quantstage.SMA20PurposeCode {
		t.Fatalf("Data authorized reader request mismatch: %+v", stub)
	}
	if got.CallerRef != "principal:service:quant" || got.PurposeCode != quantstage.SMA20PurposeCode ||
		got.ManifestSchemaVersion != "2.0.0" || got.PayloadSchemaVersion != "1.0.0" ||
		got.PayloadKind != "daily_bars_close_series" || got.AdjustmentBasis != "none" ||
		got.Interval != "1d" || got.QualityStatus != "valid" ||
		len(got.Sources) != 1 || got.Sources[0].SourceRef != "eastmoney" ||
		got.Sources[0].ProviderInterfaceVersion != "eastmoney-stock-api-v1" || len(got.Bars) != 20 ||
		got.Bars[0].Close == "" || got.Coverage.MinimumRequiredBars != 20 || !got.Coverage.CoverageSufficient {
		t.Fatalf("Data Snapshot evidence was not mapped to Quant DTO: %+v", got)
	}
}

func TestQuantDataSnapshotAdapterRejectsSnapshotIdentityMismatch(t *testing.T) {
	snapshot := validQuantDataCloseSeriesSnapshot(t)
	stub := &quantDataSnapshotPortStub{readSnapshot: snapshot}
	adapter, err := newQuantDataSnapshotAdapter(stub)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.ReadAuthorizedCloseSeriesSnapshot(
		context.Background(), "data_snapshot:sha256:"+strings.Repeat("c", 64),
		"data_authorization:sha256:"+strings.Repeat("b", 64), quantstage.SMA20PurposeCode,
	); err == nil {
		t.Fatal("Snapshot reference mismatch must be rejected")
	}
}

func validQuantDataCloseSeriesSnapshot(t *testing.T) marketdataapp.DailyBarsCloseSeriesSnapshot {
	t.Helper()
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	observed := cutoff.Add(-time.Minute)
	query := marketdataapp.DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-10-01", EndDate: "2026-10-30",
		Limit: 20, AdjustmentBasis: marketdataapp.AdjustmentNone, SystemObservedAtCutoff: &cutoff,
	}
	bars := make([]marketdataapp.DailyBar, 20)
	for index := range bars {
		date := time.Date(2026, 10, 1+index, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
		closeValue := "43." + strings.Repeat("0", 2)
		fraction := 2
		bars[index] = marketdataapp.DailyBar{
			Symbol: query.Symbol, TradeDate: date, Period: "1d",
			AdjustmentBasis: marketdataapp.AdjustmentNone,
			Source:          "eastmoney", SourceVersion: "eastmoney-stock-api-v1",
			FetchedAt: cutoff.Add(-2 * time.Minute), NormalizedAt: observed,
			SystemObservedAt: &observed, NormalizationVersion: "4",
			QualityStatus: marketdataapp.QualityValid,
			ExactValues: &marketdataapp.DailyBarExactValues{
				Close: closeValue,
				CloseEvidence: &marketdataapp.DailyBarDecimalEvidence{
					EvidenceKind: "observed_token", SourceToken: closeValue, SourceFractionDigits: &fraction,
				},
			},
		}
	}
	calendarRef := "data_calendar:sha256:" + strings.Repeat("a", 64)
	dailyBars := marketdataapp.DailyBarsV2Snapshot{
		Symbol: query.Symbol, Interval: "1d", AdjustmentBasis: marketdataapp.AdjustmentNone,
		QualityStatus: marketdataapp.QualityValid, SystemObservedAtCutoff: &cutoff,
		TradingCalendarRef: &calendarRef,
		CoverageEvidence: marketdataapp.DailyBarCoverageEvidence{
			CalendarQualityStatus: marketdataapp.QualityValid, ExpectedTradingDays: 20,
			MinimumRequiredBars: 20, CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		},
		Bars: bars,
	}
	input, err := marketdataapp.BuildDailyBarsCloseSeriesInput(query, dailyBars)
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesInput() error = %v", err)
	}
	snapshot, err := marketdataapp.BuildDailyBarsCloseSeriesSnapshot(input, "principal:service:quant", quantstage.SMA20PurposeCode)
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() error = %v", err)
	}
	return snapshot
}
