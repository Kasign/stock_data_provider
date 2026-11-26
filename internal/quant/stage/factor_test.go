package stage

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
)

type factorSnapshotCreatorStub struct {
	refs  SnapshotReferences
	err   error
	calls int
	last  SnapshotCreateRequest
}

func (stub *factorSnapshotCreatorStub) CreateDailyBarsCloseSeriesSnapshot(
	_ context.Context,
	request SnapshotCreateRequest,
) (SnapshotReferences, error) {
	stub.calls++
	stub.last = request
	if stub.err != nil {
		return SnapshotReferences{}, stub.err
	}
	return stub.refs, nil
}

type factorSnapshotReaderStub struct {
	snapshot application.CloseSeriesSnapshot
	err      error
	calls    int
}

func (stub *factorSnapshotReaderStub) ReadAuthorizedCloseSeriesSnapshot(
	_ context.Context,
	snapshotRef, _, purposeCode string,
) (application.CloseSeriesSnapshot, error) {
	stub.calls++
	if stub.err != nil {
		return application.CloseSeriesSnapshot{}, stub.err
	}
	if snapshotRef != stub.snapshot.SnapshotRef || purposeCode != QuantPurposeCode {
		return application.CloseSeriesSnapshot{}, errors.New("unexpected authorized Snapshot binding")
	}
	return stub.snapshot, nil
}

const factorStageSnapshotRef = "data_snapshot:sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func factorStageSnapshot(closes []string) application.CloseSeriesSnapshot {
	bars := make([]application.CloseSeriesBar, len(closes))
	for index, close := range closes {
		bars[index] = application.CloseSeriesBar{
			TradeDate:     time.Date(2026, 10, 1+index, 0, 0, 0, 0, time.UTC).Format("2006-01-02"),
			Close:         close,
			QualityStatus: "valid",
		}
	}
	minimumRequired := 1
	if len(bars) == 0 {
		minimumRequired = 0
	}
	return application.CloseSeriesSnapshot{
		SnapshotRef:           factorStageSnapshotRef,
		ManifestSchemaVersion: "2.0.0",
		PayloadSchemaVersion:  "1.0.0",
		PayloadKind:           "daily_bars_close_series",
		CallerRef:             "principal:service:researchflow",
		PurposeCode:           QuantPurposeCode,
		Sources: []application.CloseSeriesSource{{
			SourceRef: QuantProviderCode, ProviderInterfaceVersion: "eastmoney-stock-api-v1",
		}},
		Symbol:             "600000.SH",
		StartDate:          bars[0].TradeDate,
		EndDate:            bars[len(bars)-1].TradeDate,
		AdjustmentBasis:    "none",
		SystemAsOf:         "2026-11-23T00:00:00Z",
		Interval:           "1d",
		TradingCalendarRef: "trading_calendar:sha256:" + strings.Repeat("b", 64),
		QualityStatus:      "valid",
		Coverage: application.CloseSeriesCoverageEvidence{
			CalendarQualityStatus: "valid",
			ExpectedTradingDays:   len(bars),
			MinimumRequiredBars:   minimumRequired,
			CoverageSufficient:    true,
		},
		Bars: bars,
	}
}

func factorStageCloses(start, count int) []string {
	closes := make([]string, count)
	for index := range closes {
		closes[index] = strconv.Itoa(start + index)
	}
	return closes
}

func newTestFactorAdapter(
	t *testing.T,
	creator DailyBarsCloseSeriesSnapshotCreator,
	reader *factorSnapshotReaderStub,
) *FactorAdapter {
	t.Helper()
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	service, err := application.NewCreateFactorService(
		reader, quantRegistry, result.NewMemoryRepository(),
		func() time.Time { return time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewCreateFactorService() error = %v", err)
	}
	adapter, err := NewFactorAdapter(creator, service)
	if err != nil {
		t.Fatalf("NewFactorAdapter() error = %v", err)
	}
	return adapter
}

func validFactorStageRequest(code string) FactorRequest {
	return FactorRequest{
		FactorCode: code, Symbol: "600000.SH",
		StartDate: "2026-10-01", EndDate: "2026-11-23",
		Limit:                1000,
		SystemAsOf:           time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		AllowedProviderCodes: []string{QuantProviderCode},
	}
}

func TestFactorAdapterProducesValidatedStageOutputForAllProductionFactors(t *testing.T) {
	cases := []struct {
		code      string
		closes    []string
		wantValue string
		wantBars  int
		wantPer   string
	}{
		{registry.ProductionMADistanceCode, factorStageCloses(1, 20), "0.904761904762", 20, "20d"},
		{registry.ProductionMomentum5Code, factorStageCloses(95, 6), "0.052631578947", 6, "5d"},
		{registry.ProductionMomentum20Code, factorStageCloses(100, 21), "0.2", 21, "20d"},
	}
	for _, testCase := range cases {
		t.Run(testCase.code, func(t *testing.T) {
			reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(testCase.closes)}
			creator := &factorSnapshotCreatorStub{refs: SnapshotReferences{
				SnapshotRef:      factorStageSnapshotRef,
				AuthorizationRef: "data_authorization:sha256:" + strings.Repeat("c", 64),
			}}
			adapter := newTestFactorAdapter(t, creator, reader)
			output, quality, err := adapter.Execute(context.Background(), validFactorStageRequest(testCase.code))
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if creator.calls != 1 || reader.calls != 1 ||
				creator.last.Limit != 1000 || creator.last.Symbol != "600000.SH" {
				t.Fatalf("Stage did not create exactly one Snapshot from the frozen parameters: creator=%+v readerCalls=%d", creator, reader.calls)
			}
			if output.MetricCode != testCase.code || output.Period != testCase.wantPer ||
				output.SampleCount != int64(testCase.wantBars) || output.MetricStatus != "available" ||
				output.ResultStatus != "completed" || output.Unit != "ratio" ||
				output.SnapshotRef != factorStageSnapshotRef {
				t.Fatalf("unexpected Factor Stage Output: %+v", output)
			}
			var value string
			if err := json.Unmarshal(output.Value, &value); err != nil || value != testCase.wantValue {
				t.Fatalf("Factor value = %s, want %s (err=%v)", output.Value, testCase.wantValue, err)
			}
			if !strings.HasPrefix(output.ResultID, "quant_result:sha256:") {
				t.Fatalf("Factor Stage Output must carry a Manifest-issued result_id, got %q", output.ResultID)
			}
			if quality.MetricCode != testCase.code || quality.ObservedBars != testCase.wantBars ||
				quality.ExpectedTradingDays != testCase.wantBars || !quality.CoverageSufficient {
				t.Fatalf("unexpected Factor Stage quality summary: %+v", quality)
			}
			payload, err := json.Marshal(output)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateFactorStageOutputJSON(payload); err != nil {
				t.Fatalf("Factor Stage Output failed its own validator: %v", err)
			}
			qualityJSON, err := json.Marshal(quality)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateFactorQualitySummaryJSON(qualityJSON); err != nil {
				t.Fatalf("Factor Stage quality summary failed its own validator: %v", err)
			}
		})
	}
}

func TestFactorAdapterReportsInsufficientDataWithExplicitNullValue(t *testing.T) {
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(95, 5))}
	creator := &factorSnapshotCreatorStub{refs: SnapshotReferences{
		SnapshotRef:      factorStageSnapshotRef,
		AuthorizationRef: "data_authorization:sha256:" + strings.Repeat("c", 64),
	}}
	adapter := newTestFactorAdapter(t, creator, reader)
	output, quality, err := adapter.Execute(context.Background(), validFactorStageRequest(registry.ProductionMomentum5Code))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.MetricStatus != "insufficient_data" || output.ResultStatus != "insufficient_data" ||
		output.SampleCount != 5 || string(output.Value) != "null" {
		t.Fatalf("insufficient sample was not reported as an explicit null result: %+v", output)
	}
	if quality.ObservedBars != 5 || quality.MinimumRequiredBars != 1 {
		t.Fatalf("unexpected quality summary for insufficient sample: %+v", quality)
	}
	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateFactorStageOutputJSON(payload); err != nil {
		t.Fatalf("insufficient-data Factor Stage Output failed its validator: %v", err)
	}
}

func TestFactorAdapterRejectsInvalidRequestsBeforeCreatingSnapshot(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*FactorRequest)
	}{
		{"unknown factor code", func(r *FactorRequest) { r.FactorCode = "volatility_20d" }},
		{"limit below factor minimum", func(r *FactorRequest) { r.FactorCode = registry.ProductionMomentum20Code; r.Limit = 20 }},
		{"unsorted provider codes", func(r *FactorRequest) { r.AllowedProviderCodes = []string{"eastmoney", "baostock"} }},
		{"missing provider code", func(r *FactorRequest) { r.AllowedProviderCodes = []string{"baostock"} }},
		{"non UTC system as of", func(r *FactorRequest) {
			r.SystemAsOf = r.SystemAsOf.In(time.FixedZone("CST", 8*3600))
		}},
		{"malformed symbol", func(r *FactorRequest) { r.Symbol = "600000" }},
		{"inverted date range", func(r *FactorRequest) { r.StartDate, r.EndDate = r.EndDate, r.StartDate }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			creator := &factorSnapshotCreatorStub{}
			reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 20))}
			adapter := newTestFactorAdapter(t, creator, reader)
			request := validFactorStageRequest(registry.ProductionMADistanceCode)
			testCase.mutate(&request)
			if _, _, err := adapter.Execute(context.Background(), request); !errors.Is(err, ErrInvalidFactorStageRequest) {
				t.Fatalf("Execute() error = %v, want ErrInvalidFactorStageRequest", err)
			}
			if creator.calls != 0 || reader.calls != 0 {
				t.Fatalf("invalid request reached Data: creatorCalls=%d readerCalls=%d", creator.calls, reader.calls)
			}
		})
	}
}

func TestFactorAdapterRejectsIncompleteSnapshotReferences(t *testing.T) {
	creator := &factorSnapshotCreatorStub{refs: SnapshotReferences{SnapshotRef: factorStageSnapshotRef}}
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 20))}
	adapter := newTestFactorAdapter(t, creator, reader)
	if _, _, err := adapter.Execute(context.Background(), validFactorStageRequest(registry.ProductionMADistanceCode)); !errors.Is(err, ErrInvalidFactorStageRequest) {
		t.Fatalf("Execute() error = %v, want ErrInvalidFactorStageRequest", err)
	}
	if reader.calls != 0 {
		t.Fatalf("Snapshot without AuthorizationRef was read: calls=%d", reader.calls)
	}
}

func TestFactorAdapterPropagatesSnapshotCreationFailure(t *testing.T) {
	creationErr := errors.New("data provider unavailable")
	creator := &factorSnapshotCreatorStub{err: creationErr}
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 20))}
	adapter := newTestFactorAdapter(t, creator, reader)
	if _, _, err := adapter.Execute(context.Background(), validFactorStageRequest(registry.ProductionMADistanceCode)); !errors.Is(err, creationErr) {
		t.Fatalf("Execute() error = %v, want wrapped snapshot creation failure", err)
	}
}

func TestNewFactorAdapterRejectsMissingDependencies(t *testing.T) {
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 20))}
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatal(err)
	}
	service, err := application.NewCreateFactorService(
		reader, quantRegistry, result.NewMemoryRepository(),
		func() time.Time { return time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFactorAdapter(nil, service); !errors.Is(err, ErrInvalidFactorStageRequest) {
		t.Fatalf("nil snapshot creator error = %v", err)
	}
	if _, err := NewFactorAdapter(&factorSnapshotCreatorStub{}, nil); !errors.Is(err, ErrInvalidFactorStageRequest) {
		t.Fatalf("nil Factor service error = %v", err)
	}
}
