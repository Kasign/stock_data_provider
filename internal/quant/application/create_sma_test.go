package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
)

type createSMASnapshotReader struct {
	snapshot CloseSeriesSnapshot
	err      error
	calls    int
}

func (reader *createSMASnapshotReader) ReadAuthorizedCloseSeriesSnapshot(ctx context.Context, snapshotRef, authorizationRef, purposeCode string) (CloseSeriesSnapshot, error) {
	reader.calls++
	if err := ctx.Err(); err != nil {
		return CloseSeriesSnapshot{}, err
	}
	if snapshotRef == "" || authorizationRef == "" || purposeCode != smaPurposeCode {
		return CloseSeriesSnapshot{}, ErrInvalidCreateSMARequest
	}
	if reader.err != nil {
		return CloseSeriesSnapshot{}, reader.err
	}
	return reader.snapshot, nil
}

type createSMARecordingRepository struct {
	inner   *result.MemoryRepository
	records []result.Record
}

func (repository *createSMARecordingRepository) Save(ctx context.Context, record result.Record) error {
	if err := repository.inner.Save(ctx, record); err != nil {
		return err
	}
	repository.records = append(repository.records, record)
	return nil
}

func TestCreateSMAServicePersistsExactSnapshotBoundResult(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	snapshot := validCreateSMASnapshot()
	reader := &createSMASnapshotReader{snapshot: snapshot}
	repository := &createSMARecordingRepository{inner: result.NewMemoryRepository()}
	service := newTestCreateSMAService(t, reader, repository, now)
	request := validCreateSMARequest()

	first, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	second, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() repeated error = %v", err)
	}
	if first.Manifest.ResultID != second.Manifest.ResultID || len(repository.records) != 2 {
		t.Fatalf("repeated Execute() result IDs or writes differ: first=%+v second=%+v writes=%d", first, second, len(repository.records))
	}
	if first.Manifest.ResultType != "indicator" || first.Manifest.Scope != "stock" || first.Manifest.SecurityID != snapshot.Symbol ||
		first.Manifest.TradeDate != snapshot.Bars[len(snapshot.Bars)-1].TradeDate || first.Manifest.SystemAsOf != snapshot.SystemAsOf ||
		len(first.Manifest.InputSnapshotRefs) != 1 || first.Manifest.InputSnapshotRefs[0] != snapshot.SnapshotRef {
		t.Fatalf("persisted SMA manifest is not bound to the source Snapshot: %+v", first)
	}
	var payload struct {
		IndicatorCode string          `json:"indicator_code"`
		Metrics       []metric.Metric `json:"metrics"`
	}
	if err := json.Unmarshal(repository.records[0].Payload, &payload); err != nil {
		t.Fatalf("decode persisted Indicator payload: %v", err)
	}
	if payload.IndicatorCode != "sma" || len(payload.Metrics) != 1 ||
		string(payload.Metrics[0].Value) != "\"0.105\"" ||
		payload.Metrics[0].MetricStatus != metric.StatusAvailable ||
		payload.Metrics[0].SampleCount == nil || *payload.Metrics[0].SampleCount != 20 ||
		string(first.Metric.Value) != "\"0.105\"" || first.ObservedBars != 20 || first.SnapshotRef != snapshot.SnapshotRef {
		t.Fatalf("persisted SMA Metric = %+v, want exact 0.105 with 20 bars", payload)
	}
}

func TestCreateSMAServiceUsesLastObservedTradingDateWhenQueryEndsOnNonTradingDay(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	snapshot := validCreateSMASnapshot()
	snapshot.EndDate = "2026-10-21"
	reader := &createSMASnapshotReader{snapshot: snapshot}
	repository := &createSMARecordingRepository{inner: result.NewMemoryRepository()}
	service := newTestCreateSMAService(t, reader, repository, now)
	execution, err := service.Execute(context.Background(), validCreateSMARequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if execution.Manifest.TradeDate != "2026-10-20" {
		t.Fatalf("TradeDate = %q, want last observed trading date 2026-10-20", execution.Manifest.TradeDate)
	}
}

func TestCreateSMAServiceRejectsSnapshotRunAsOfMismatch(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	reader := &createSMASnapshotReader{snapshot: validCreateSMASnapshot()}
	service := newTestCreateSMAService(t, reader, result.NewMemoryRepository(), now)
	request := validCreateSMARequest()
	request.SystemAsOf = request.SystemAsOf.Add(time.Minute)
	if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrInvalidSMASnapshot) {
		t.Fatalf("Execute() error = %v, want ErrInvalidSMASnapshot", err)
	}
}

func TestCreateSMAServiceRejectsDegradedAndIncompleteSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*CloseSeriesSnapshot){
		"degraded":              func(snapshot *CloseSeriesSnapshot) { snapshot.QualityStatus = "degraded" },
		"calendar":              func(snapshot *CloseSeriesSnapshot) { snapshot.Coverage.CalendarQualityStatus = "degraded" },
		"insufficient coverage": func(snapshot *CloseSeriesSnapshot) { snapshot.Coverage.CoverageSufficient = false },
		"missing bar":           func(snapshot *CloseSeriesSnapshot) { snapshot.Bars[0].QualityStatus = "unknown" },
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := validCreateSMASnapshot()
			mutate(&snapshot)
			reader := &createSMASnapshotReader{snapshot: snapshot}
			service := newTestCreateSMAService(t, reader, result.NewMemoryRepository(), now)
			if _, err := service.Execute(context.Background(), validCreateSMARequest()); !errors.Is(err, ErrInvalidSMASnapshot) {
				t.Fatalf("Execute() error = %v, want ErrInvalidSMASnapshot", err)
			}
		})
	}
}

func TestCreateSMAServiceRejectsNonSMA20Parameters(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	reader := &createSMASnapshotReader{snapshot: validCreateSMASnapshot()}
	repository := &createSMARecordingRepository{inner: result.NewMemoryRepository()}
	service := newTestCreateSMAService(t, reader, repository, now)
	request := validCreateSMARequest()
	request.Parameters = json.RawMessage("{\"window\":3}")
	if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrInvalidCreateSMARequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidCreateSMARequest", err)
	}
	if len(repository.records) != 0 {
		t.Fatalf("non-SMA20 parameters must not persist a result; writes=%d", len(repository.records))
	}
}

func newTestCreateSMAService(t *testing.T, reader CloseSeriesSnapshotReader, repository result.Repository, now time.Time) *CreateSMAService {
	t.Helper()
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	service, err := NewCreateSMAService(reader, quantRegistry, repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewCreateSMAService() error = %v", err)
	}
	return service
}

func validCreateSMASnapshot() CloseSeriesSnapshot {
	bars := make([]CloseSeriesBar, 20)
	for index := range bars {
		bars[index] = CloseSeriesBar{
			TradeDate: time.Date(2026, 10, 1+index, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			Close:     fmt.Sprintf("0.%02d", index+1), QualityStatus: "valid",
		}
	}
	return CloseSeriesSnapshot{
		SnapshotRef:           "data_snapshot:sha256:" + strings.Repeat("a", 64),
		ManifestSchemaVersion: "2.0.0", PayloadSchemaVersion: "1.0.0", PayloadKind: "daily_bars_close_series",
		CallerRef: "principal:service:researchflow", PurposeCode: "quant_analysis",
		Sources: []CloseSeriesSource{{SourceRef: "eastmoney", ProviderInterfaceVersion: "eastmoney-stock-api-v1"}},
		Symbol:  "600000.SH", StartDate: "2026-10-01", EndDate: "2026-10-20",
		AdjustmentBasis: "none", SystemAsOf: "2026-11-23T00:00:00Z", Interval: "1d",
		TradingCalendarRef: "trading_calendar:sha256:" + strings.Repeat("b", 64),
		QualityStatus:      "valid",
		Coverage: CloseSeriesCoverageEvidence{
			CalendarQualityStatus: "valid", ExpectedTradingDays: 20,
			MinimumRequiredBars: 20, CoverageSufficient: true,
		},
		Bars: bars,
	}
}

func validCreateSMARequest() CreateSMARequest {
	return CreateSMARequest{
		SnapshotRef:          "data_snapshot:sha256:" + strings.Repeat("a", 64),
		AuthorizationRef:     "data_authorization:sha256:" + strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		Parameters:           json.RawMessage("{\"window\":20}"),
		AllowedProviderCodes: []string{"eastmoney"},
	}
}
