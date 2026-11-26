package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/factor"
	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
)

type createFactorSnapshotReader struct {
	snapshot    CloseSeriesSnapshot
	err         error
	calls       int
	lastPurpose string
	lastRef     string
	lastAuthRef string
}

func (reader *createFactorSnapshotReader) ReadAuthorizedCloseSeriesSnapshot(ctx context.Context, snapshotRef, authorizationRef, purposeCode string) (CloseSeriesSnapshot, error) {
	reader.calls++
	reader.lastRef = snapshotRef
	reader.lastAuthRef = authorizationRef
	reader.lastPurpose = purposeCode
	if err := ctx.Err(); err != nil {
		return CloseSeriesSnapshot{}, err
	}
	if reader.err != nil {
		return CloseSeriesSnapshot{}, reader.err
	}
	return reader.snapshot, nil
}

func newTestCreateFactorService(t *testing.T, reader CloseSeriesSnapshotReader, now time.Time) *CreateFactorService {
	t.Helper()
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	service, err := NewCreateFactorService(reader, quantRegistry, result.NewMemoryRepository(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewCreateFactorService() error = %v", err)
	}
	return service
}

func validCreateFactorSnapshot(closes []string) CloseSeriesSnapshot {
	bars := make([]CloseSeriesBar, len(closes))
	for index, close := range closes {
		bars[index] = CloseSeriesBar{
			TradeDate:     time.Date(2026, 10, 1+index, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			Close:         close,
			QualityStatus: "valid",
		}
	}
	return CloseSeriesSnapshot{
		SnapshotRef:           "data_snapshot:sha256:" + strings.Repeat("a", 64),
		ManifestSchemaVersion: "2.0.0",
		PayloadSchemaVersion:  "1.0.0",
		PayloadKind:           "daily_bars_close_series",
		CallerRef:             "principal:service:researchflow",
		PurposeCode:           "quant_analysis",
		Sources:               []CloseSeriesSource{{SourceRef: "eastmoney", ProviderInterfaceVersion: "eastmoney-stock-api-v1"}},
		Symbol:                "600000.SH",
		StartDate:             bars[0].TradeDate,
		EndDate:               bars[len(bars)-1].TradeDate,
		AdjustmentBasis:       "none",
		SystemAsOf:            "2026-11-23T00:00:00Z",
		Interval:              "1d",
		TradingCalendarRef:    "trading_calendar:sha256:" + strings.Repeat("b", 64),
		QualityStatus:         "valid",
		Coverage: CloseSeriesCoverageEvidence{
			CalendarQualityStatus: "valid",
			ExpectedTradingDays:   len(bars),
			MinimumRequiredBars:   len(bars),
			CoverageSufficient:    true,
		},
		Bars: bars,
	}
}

func sequenceCloses(start, count int) []string {
	closes := make([]string, count)
	for index := range closes {
		closes[index] = fmt.Sprintf("%d", start+index)
	}
	return closes
}

func validCreateFactorRequest(code string) CreateFactorRequest {
	return CreateFactorRequest{
		FactorCode:           code,
		SnapshotRef:          "data_snapshot:sha256:" + strings.Repeat("a", 64),
		AuthorizationRef:     "data_authorization:sha256:" + strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		Parameters:           json.RawMessage(`{}`),
		AllowedProviderCodes: []string{"eastmoney"},
	}
}

func factorExecutionNow() time.Time {
	return time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
}

func TestCreateFactorServiceComputesAllThreeDecimalFactors(t *testing.T) {
	cases := []struct {
		code      string
		closes    []string
		wantValue string
		wantBars  int
	}{
		{registry.ProductionMADistanceCode, sequenceCloses(1, 20), "0.904761904762", 20},
		{registry.ProductionMomentum5Code, sequenceCloses(95, 6), "0.052631578947", 6},
		{registry.ProductionMomentum20Code, sequenceCloses(100, 21), "0.2", 21},
	}
	for _, testCase := range cases {
		t.Run(testCase.code, func(t *testing.T) {
			reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(testCase.closes)}
			service := newTestCreateFactorService(t, reader, factorExecutionNow())
			execution, err := service.Execute(context.Background(), validCreateFactorRequest(testCase.code))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if execution.Metric.MetricCode != testCase.code || execution.Metric.Value == nil ||
				*execution.Metric.Value != testCase.wantValue || execution.Metric.SampleCount != testCase.wantBars {
				t.Fatalf("Metric = %+v, want code=%s value=%s bars=%d", execution.Metric, testCase.code, testCase.wantValue, testCase.wantBars)
			}
			if execution.Metric.MetricStatus != factor.StatusAvailable || execution.Metric.Unit != "ratio" ||
				execution.Metric.Period == "" || execution.Metric.CalculationVersion == "" {
				t.Fatalf("Metric metadata incomplete: %+v", execution.Metric)
			}
			if reader.lastPurpose != "quant_analysis" || reader.lastRef != execution.SnapshotRef ||
				reader.lastAuthRef == "" || reader.calls != 1 {
				t.Fatalf("authorized Snapshot reader call = purpose %q ref %q auth %q calls %d",
					reader.lastPurpose, reader.lastRef, reader.lastAuthRef, reader.calls)
			}
			if execution.Manifest.ResultType != "factor" || execution.Manifest.Scope != "stock" ||
				execution.Manifest.ResultStatus != "completed" ||
				execution.Manifest.AlgorithmVersion != execution.Metric.CalculationVersion ||
				len(execution.Manifest.InputSnapshotRefs) != 1 ||
				execution.Manifest.InputSnapshotRefs[0] != execution.SnapshotRef ||
				execution.Manifest.ParameterFingerprint != execution.ParameterFingerprint {
				t.Fatalf("ManifestInput not bound to factor result/snapshot: %+v", execution.Manifest)
			}
			registryJSON, err := json.Marshal(execution.Metric)
			if err != nil {
				t.Fatalf("marshal metric: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(registryJSON, &fields); err != nil {
				t.Fatalf("unmarshal metric: %v", err)
			}
			if !strings.Contains(string(fields["value"]), testCase.wantValue) {
				t.Fatalf("serialized value = %s, want %s", fields["value"], testCase.wantValue)
			}
		})
	}
}

func TestCreateFactorServiceReturnsInsufficientDataAsNullMetric(t *testing.T) {
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(95, 5))}
	service := newTestCreateFactorService(t, reader, factorExecutionNow())
	execution, err := service.Execute(context.Background(), validCreateFactorRequest(registry.ProductionMomentum5Code))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if execution.Metric.Value != nil || execution.Metric.MetricStatus != factor.StatusInsufficientData ||
		execution.Metric.SampleCount != 5 {
		t.Fatalf("insufficient metric = %+v, want null value/status insufficient_data/5 bars", execution.Metric)
	}
	if execution.Manifest.ResultStatus != "insufficient_data" {
		t.Fatalf("ManifestInput.ResultStatus = %q, want insufficient_data", execution.Manifest.ResultStatus)
	}
}

func TestCreateFactorServiceRejectsInvalidRequests(t *testing.T) {
	snapshot := validCreateFactorSnapshot(sequenceCloses(1, 20))
	now := factorExecutionNow()
	cases := []struct {
		name   string
		mutate func(*CreateFactorRequest)
		want   error
	}{
		{"unsupported code", func(request *CreateFactorRequest) { request.FactorCode = "unknown_factor" }, ErrUnsupportedFactorCode},
		{"empty code", func(request *CreateFactorRequest) { request.FactorCode = " " }, ErrInvalidCreateFactorRequest},
		{"empty snapshot ref", func(request *CreateFactorRequest) { request.SnapshotRef = "" }, ErrInvalidCreateFactorRequest},
		{"empty authorization ref", func(request *CreateFactorRequest) { request.AuthorizationRef = "" }, ErrInvalidCreateFactorRequest},
		{"non UTC as of", func(request *CreateFactorRequest) {
			request.SystemAsOf = request.SystemAsOf.In(time.FixedZone("CST", 8*3600))
		}, ErrInvalidCreateFactorRequest},
		{"bad parameters", func(request *CreateFactorRequest) { request.Parameters = json.RawMessage(`{"window":20}`) }, ErrInvalidCreateFactorRequest},
		{"null parameters", func(request *CreateFactorRequest) { request.Parameters = json.RawMessage(`null`) }, ErrInvalidCreateFactorRequest},
		{"trailing parameters", func(request *CreateFactorRequest) { request.Parameters = json.RawMessage(`{} {}`) }, ErrInvalidCreateFactorRequest},
		{"unapproved provider", func(request *CreateFactorRequest) { request.AllowedProviderCodes = []string{"other"} }, ErrInvalidCreateFactorRequest},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reader := &createFactorSnapshotReader{snapshot: snapshot}
			service := newTestCreateFactorService(t, reader, now)
			request := validCreateFactorRequest(registry.ProductionMADistanceCode)
			testCase.mutate(&request)
			if _, err := service.Execute(context.Background(), request); !errors.Is(err, testCase.want) {
				t.Fatalf("Execute() error = %v, want %v", err, testCase.want)
			}
			if testCase.name != "unsupported code" && reader.calls != 0 {
				t.Fatalf("invalid request must not read Snapshot; calls=%d", reader.calls)
			}
		})
	}
}

func TestCreateFactorServiceRejectsWeakSnapshot(t *testing.T) {
	now := factorExecutionNow()
	mutations := map[string]func(*CloseSeriesSnapshot){
		"degraded": func(snapshot *CloseSeriesSnapshot) { snapshot.QualityStatus = "degraded" },
		"calendar": func(snapshot *CloseSeriesSnapshot) { snapshot.Coverage.CalendarQualityStatus = "degraded" },
		"insufficient coverage": func(snapshot *CloseSeriesSnapshot) {
			snapshot.Coverage.CoverageSufficient = false
		},
		"missing bar":      func(snapshot *CloseSeriesSnapshot) { snapshot.Bars[0].QualityStatus = "unknown" },
		"wrong purpose":    func(snapshot *CloseSeriesSnapshot) { snapshot.PurposeCode = "other" },
		"wrong adjustment": func(snapshot *CloseSeriesSnapshot) { snapshot.AdjustmentBasis = "forward_adjusted" },
		"snapshot mismatch": func(snapshot *CloseSeriesSnapshot) {
			snapshot.SnapshotRef = "data_snapshot:sha256:" + strings.Repeat("d", 64)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			snapshot := validCreateFactorSnapshot(sequenceCloses(1, 20))
			mutate(&snapshot)
			reader := &createFactorSnapshotReader{snapshot: snapshot}
			service := newTestCreateFactorService(t, reader, now)
			if _, err := service.Execute(context.Background(), validCreateFactorRequest(registry.ProductionMADistanceCode)); !errors.Is(err, ErrInvalidFactorSnapshot) {
				t.Fatalf("Execute() error = %v, want ErrInvalidFactorSnapshot", err)
			}
		})
	}
	t.Run("as of mismatch", func(t *testing.T) {
		reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 20))}
		service := newTestCreateFactorService(t, reader, now)
		request := validCreateFactorRequest(registry.ProductionMADistanceCode)
		request.SystemAsOf = request.SystemAsOf.Add(time.Minute)
		if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrInvalidFactorSnapshot) {
			t.Fatalf("Execute() error = %v, want ErrInvalidFactorSnapshot", err)
		}
	})
}

func TestCreateFactorServicePropagatesReaderError(t *testing.T) {
	readerErr := errors.New("data authorization revoked")
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 20)), err: readerErr}
	service := newTestCreateFactorService(t, reader, factorExecutionNow())
	if _, err := service.Execute(context.Background(), validCreateFactorRequest(registry.ProductionMADistanceCode)); !errors.Is(err, readerErr) {
		t.Fatalf("Execute() error = %v, want reader error", err)
	}
}

func TestCreateFactorServiceRejectsCanceledContext(t *testing.T) {
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 20))}
	service := newTestCreateFactorService(t, reader, factorExecutionNow())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Execute(ctx, validCreateFactorRequest(registry.ProductionMADistanceCode)); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if reader.calls != 0 {
		t.Fatalf("canceled context must not read Snapshot; calls=%d", reader.calls)
	}
}

func TestDecimalFactorComputationsMatchRegistry(t *testing.T) {
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	for _, code := range []string{
		registry.ProductionMADistanceCode,
		registry.ProductionMomentum5Code,
		registry.ProductionMomentum20Code,
	} {
		compute, ok := decimalFactorComputationFor(code)
		if !ok || compute == nil {
			t.Fatalf("no decimal computation dispatched for %s", code)
		}
		entry, err := quantRegistry.LookupActive(code, registry.ProductionFactorSchemaVersion)
		if err != nil {
			t.Fatalf("LookupActive(%s) error = %v", code, err)
		}
		if err := validateDecimalFactorRegistryEntry(entry); err != nil {
			t.Fatalf("validateDecimalFactorRegistryEntry(%s) error = %v", code, err)
		}
		result, err := compute(sequenceCloses(1, 20))
		if err != nil || result.Code != code {
			t.Fatalf("compute(%s) = %+v, %v", code, result, err)
		}
	}
	if _, ok := decimalFactorComputationFor("momentum_60d"); ok {
		t.Fatalf("momentum_60d must not be dispatched by the Close-only Factor facade")
	}
}

func TestValidateDecimalFactorRegistryEntryRejectsPrecisionDrift(t *testing.T) {
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	base, err := quantRegistry.LookupActive(registry.ProductionMADistanceCode, registry.ProductionFactorSchemaVersion)
	if err != nil {
		t.Fatalf("LookupActive() error = %v", err)
	}
	mutations := map[string]func(*registry.Entry){
		"value type":     func(entry *registry.Entry) { entry.ValueType = registry.ValueTypeNumber },
		"unit":           func(entry *registry.Entry) { entry.Unit = "percent" },
		"scale":          func(entry *registry.Entry) { entry.Precision.MaxFractionDigits-- },
		"rounding":       func(entry *registry.Entry) { entry.Precision.RoundingMode = "half_up" },
		"tolerance":      func(entry *registry.Entry) { entry.Precision.Tolerance = json.RawMessage(`"0.1"`) },
		"sample count":   func(entry *registry.Entry) { entry.SampleCount.RequiredInMetric = false },
		"interval":       func(entry *registry.Entry) { entry.SupportsInterval = true },
		"unbounded":      func(entry *registry.Entry) { entry.Range.Unbounded = false },
		"missing policy": func(entry *registry.Entry) { entry.QualityPolicy.MissingHandling = "ignore" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			entry := base
			mutate(&entry)
			if err := validateDecimalFactorRegistryEntry(entry); !errors.Is(err, ErrUnsupportedFactorEntry) {
				t.Fatalf("validateDecimalFactorRegistryEntry() error = %v, want ErrUnsupportedFactorEntry", err)
			}
		})
	}
}

func TestCreateFactorServiceRejectsNilDependencies(t *testing.T) {
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 20))}
	now := func() time.Time { return factorExecutionNow() }
	repository := result.NewMemoryRepository()
	if _, err := NewCreateFactorService(nil, quantRegistry, repository, now); !errors.Is(err, ErrInvalidCreateFactorRequest) {
		t.Fatalf("nil reader error = %v", err)
	}
	if _, err := NewCreateFactorService(reader, nil, repository, now); !errors.Is(err, ErrInvalidCreateFactorRequest) {
		t.Fatalf("nil registry error = %v", err)
	}
	if _, err := NewCreateFactorService(reader, quantRegistry, nil, now); !errors.Is(err, ErrInvalidCreateFactorRequest) {
		t.Fatalf("nil repository error = %v", err)
	}
	if _, err := NewCreateFactorService(reader, quantRegistry, repository, nil); !errors.Is(err, ErrInvalidCreateFactorRequest) {
		t.Fatalf("nil clock error = %v", err)
	}
}

func TestCreateFactorServicePersistsContentAddressedFactorRecord(t *testing.T) {
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 20))}
	repository := &createFactorRecordingRepository{inner: result.NewMemoryRepository()}
	service := newTestCreateFactorServiceWithRepository(t, reader, repository, factorExecutionNow())
	request := validCreateFactorRequest(registry.ProductionMADistanceCode)

	first, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	second, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("repeated Execute() error = %v", err)
	}
	if len(repository.records) != 2 || first.Manifest.ResultID != second.Manifest.ResultID {
		t.Fatalf("repeated Execute() wrote %d records or changed result id: first=%s second=%s",
			len(repository.records), first.Manifest.ResultID, second.Manifest.ResultID)
	}
	if first.Manifest.ResultType != "factor" || first.Manifest.PayloadSchemaVersion != "1.0.0" ||
		first.Manifest.SchemaVersion != result.EnvelopeSchemaVersion || first.Manifest.ResultID == "" {
		t.Fatalf("Manifest missing persistence-ready metadata: %+v", first.Manifest)
	}
	if err := first.Manifest.Verify(); err != nil {
		t.Fatalf("persisted Manifest is not content-addressed: %v", err)
	}
	record := repository.records[0]
	if err := record.Validate(); err != nil {
		t.Fatalf("persisted Record is not payload-bound: %v", err)
	}
	decoded, err := result.DecodePayload(record.Manifest.ResultType, record.Manifest.PayloadSchemaVersion, record.Payload)
	if err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	factorPayload, ok := decoded.(result.FactorPayload)
	if !ok {
		t.Fatalf("decoded payload type = %T, want result.FactorPayload", decoded)
	}
	if got := factorPayload.Metric(); got.MetricCode != registry.ProductionMADistanceCode ||
		got.Value == nil || *got.Value != "0.904761904762" || got.SampleCount != 20 {
		t.Fatalf("decoded factor metric = %+v, want ma20_distance 0.904761904762/20", got)
	}
}

func TestCreateFactorServiceDoesNotPersistOnInvalidInput(t *testing.T) {
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 20))}
	repository := &createFactorRecordingRepository{inner: result.NewMemoryRepository()}
	service := newTestCreateFactorServiceWithRepository(t, reader, repository, factorExecutionNow())
	request := validCreateFactorRequest(registry.ProductionMADistanceCode)
	request.AllowedProviderCodes = []string{"unapproved"}
	if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrInvalidCreateFactorRequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidCreateFactorRequest", err)
	}
	if len(repository.records) != 0 {
		t.Fatalf("invalid request must not persist; writes=%d", len(repository.records))
	}
}

type createFactorRecordingRepository struct {
	inner   *result.MemoryRepository
	records []result.Record
}

func (repository *createFactorRecordingRepository) Save(ctx context.Context, record result.Record) error {
	if err := repository.inner.Save(ctx, record); err != nil {
		return err
	}
	repository.records = append(repository.records, record)
	return nil
}

func newTestCreateFactorServiceWithRepository(t *testing.T, reader CloseSeriesSnapshotReader, repository result.Repository, now time.Time) *CreateFactorService {
	t.Helper()
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	service, err := NewCreateFactorService(reader, quantRegistry, repository, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewCreateFactorService() error = %v", err)
	}
	return service
}
