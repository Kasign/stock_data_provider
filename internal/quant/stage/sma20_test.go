package stage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
)

type sma20SnapshotCreatorStub struct {
	refs SnapshotReferences
	request SnapshotCreateRequest
	err error
	calls int
}

func (creator *sma20SnapshotCreatorStub) CreateDailyBarsCloseSeriesSnapshot(ctx context.Context, request SnapshotCreateRequest) (SnapshotReferences, error) {
	creator.calls++
	creator.request = request
	if err := ctx.Err(); err != nil {
		return SnapshotReferences{}, err
	}
	if creator.err != nil {
		return SnapshotReferences{}, creator.err
	}
	return creator.refs, nil
}

type sma20SnapshotReaderStub struct {
	snapshot application.CloseSeriesSnapshot
	err error
	requestedSnapshotRef string
	requestedAuthorizationRef string
	requestedPurpose string
}

func (reader *sma20SnapshotReaderStub) ReadAuthorizedCloseSeriesSnapshot(ctx context.Context, snapshotRef, authorizationRef, purposeCode string) (application.CloseSeriesSnapshot, error) {
	reader.requestedSnapshotRef = snapshotRef
	reader.requestedAuthorizationRef = authorizationRef
	reader.requestedPurpose = purposeCode
	if err := ctx.Err(); err != nil {
		return application.CloseSeriesSnapshot{}, err
	}
	if reader.err != nil {
		return application.CloseSeriesSnapshot{}, reader.err
	}
	return reader.snapshot, nil
}

func TestSMA20AdapterCreatesDataSnapshotThenPersistsQuantResult(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	systemAsOf := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	refs := SnapshotReferences{
		SnapshotRef: "data_snapshot:sha256:" + strings.Repeat("a", 64),
		AuthorizationRef: "data_authorization:sha256:" + strings.Repeat("b", 64),
	}
	creator := &sma20SnapshotCreatorStub{refs: refs}
	reader := &sma20SnapshotReaderStub{snapshot: validSMA20Snapshot(refs.SnapshotRef)}
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("NewProductionRegistry() error = %v", err)
	}
	service, err := application.NewCreateSMAService(reader, quantRegistry, result.NewMemoryRepository(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewCreateSMAService() error = %v", err)
	}
	adapter, err := NewSMA20Adapter(creator, service)
	if err != nil {
		t.Fatalf("NewSMA20Adapter() error = %v", err)
	}
	output, quality, err := adapter.Execute(context.Background(), validSMA20Request(systemAsOf))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if creator.calls != 1 || creator.request.Symbol != "600000.SH" ||
		creator.request.StartDate != "2026-10-01" || creator.request.EndDate != "2026-10-20" ||
		creator.request.Limit != 20 || !creator.request.SystemAsOf.Equal(systemAsOf) {
		t.Fatalf("Data Snapshot create request = %+v, calls=%d", creator.request, creator.calls)
	}
	if reader.requestedSnapshotRef != refs.SnapshotRef || reader.requestedAuthorizationRef != refs.AuthorizationRef ||
		reader.requestedPurpose != SMA20PurposeCode {
		t.Fatalf("Quant did not read the just-created authorized Snapshot: %+v", reader)
	}
	if output.ResultID == "" || output.SnapshotRef != refs.SnapshotRef || output.MetricCode != "sma" ||
		string(output.Value) != "\"0.105\"" || output.Unit != "CNY/share" ||
		output.MetricStatus != "available" || output.SampleCount != 20 ||
		output.TradeDate != "2026-10-20" || output.SystemAsOf != "2026-11-23T00:00:00Z" {
		t.Fatalf("SMA20 output = %+v", output)
	}
	if quality.QualityStatus != "valid" || quality.ExpectedTradingDays != 20 ||
		quality.ObservedBars != 20 || quality.MinimumRequiredBars != 20 || !quality.CoverageSufficient {
		t.Fatalf("SMA20 quality summary = %+v", quality)
	}
}

func TestSMA20AdapterRejectsInvalidRequestBeforeCreatingSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	reader := &sma20SnapshotReaderStub{snapshot: validSMA20Snapshot("data_snapshot:sha256:" + strings.Repeat("a", 64))}
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("NewProductionRegistry() error = %v", err)
	}
	service, err := application.NewCreateSMAService(reader, quantRegistry, result.NewMemoryRepository(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewCreateSMAService() error = %v", err)
	}
	creator := &sma20SnapshotCreatorStub{refs: SnapshotReferences{SnapshotRef: "ref", AuthorizationRef: "auth"}}
	adapter, err := NewSMA20Adapter(creator, service)
	if err != nil {
		t.Fatalf("NewSMA20Adapter() error = %v", err)
	}
	request := validSMA20Request(time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC))
	request.Symbol = "not-a-stock"
	if _, _, err := adapter.Execute(context.Background(), request); !errors.Is(err, ErrInvalidSMA20StageRequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidSMA20StageRequest", err)
	}
	if creator.calls != 0 {
		t.Fatalf("invalid request must not create a Data Snapshot; calls=%d", creator.calls)
	}
}

func TestSMA20AdapterPropagatesDataSnapshotCreationFailure(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	reader := &sma20SnapshotReaderStub{snapshot: validSMA20Snapshot("data_snapshot:sha256:" + strings.Repeat("a", 64))}
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("NewProductionRegistry() error = %v", err)
	}
	service, err := application.NewCreateSMAService(reader, quantRegistry, result.NewMemoryRepository(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewCreateSMAService() error = %v", err)
	}
	expected := errors.New("Data snapshot unavailable")
	creator := &sma20SnapshotCreatorStub{err: expected}
	adapter, err := NewSMA20Adapter(creator, service)
	if err != nil {
		t.Fatalf("NewSMA20Adapter() error = %v", err)
	}
	_, _, err = adapter.Execute(context.Background(), validSMA20Request(time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)))
	if !errors.Is(err, expected) {
		t.Fatalf("Execute() error = %v, want wrapped Data error", err)
	}
	if reader.requestedSnapshotRef != "" {
		t.Fatalf("Quant must not read or persist when Data Snapshot creation fails: %+v", reader)
	}
}

func validSMA20Request(systemAsOf time.Time) SMA20Request {
	return SMA20Request{
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-10-20",
		Limit: 20, SystemAsOf: systemAsOf, AllowedProviderCodes: []string{"eastmoney"},
	}
}

func validSMA20Snapshot(snapshotRef string) application.CloseSeriesSnapshot {
	bars := make([]application.CloseSeriesBar, 20)
	for index := range bars {
		bars[index] = application.CloseSeriesBar{
			TradeDate: time.Date(2026, 10, 1+index, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			Close: fmt.Sprintf("0.%02d", index+1), QualityStatus: "valid",
		}
	}
	return application.CloseSeriesSnapshot{
		SnapshotRef: snapshotRef,
		ManifestSchemaVersion: "2.0.0", PayloadSchemaVersion: "1.0.0",
		PayloadKind: "daily_bars_close_series",
		CallerRef: "principal:service:researchflow", PurposeCode: SMA20PurposeCode,
		Sources: []application.CloseSeriesSource{{SourceRef: "eastmoney", ProviderInterfaceVersion: "eastmoney-stock-api-v1"}},
		Symbol: "600000.SH", StartDate: "2026-10-01", EndDate: "2026-10-20",
		AdjustmentBasis: "none", SystemAsOf: "2026-11-23T00:00:00Z", Interval: "1d",
		TradingCalendarRef: "trading_calendar:sha256:" + strings.Repeat("c", 64),
		QualityStatus: "valid",
		Coverage: application.CloseSeriesCoverageEvidence{
			CalendarQualityStatus: "valid", ExpectedTradingDays: 20,
			MinimumRequiredBars: 20, CoverageSufficient: true,
		},
		Bars: bars,
	}
}
