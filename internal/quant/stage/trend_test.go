package stage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

type trendSnapshotCreatorStub struct {
	refs  SnapshotReferences
	err   error
	calls int
	last  SnapshotCreateRequest
}

func (stub *trendSnapshotCreatorStub) CreateDailyBarsCloseSeriesSnapshot(
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

func newTestTrendAdapter(
	t *testing.T,
	creator DailyBarsCloseSeriesSnapshotCreator,
	reader *factorSnapshotReaderStub,
) *TrendAdapter {
	t.Helper()
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	service, err := application.NewCreateTrendService(
		reader, quantRegistry, result.NewMemoryRepository(),
		func() time.Time { return time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewCreateTrendService() error = %v", err)
	}
	adapter, err := NewTrendAdapter(creator, service)
	if err != nil {
		t.Fatalf("NewTrendAdapter() error = %v", err)
	}
	return adapter
}

func validTrendStageRequest() TrendRequest {
	return TrendRequest{
		Symbol: "600000.SH", StartDate: "2026-06-01", EndDate: "2026-11-23",
		Limit:                61,
		SystemAsOf:           time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		AllowedProviderCodes: []string{QuantProviderCode},
	}
}

func trendStageReferences() SnapshotReferences {
	return SnapshotReferences{
		SnapshotRef:      factorStageSnapshotRef,
		AuthorizationRef: "data_authorization:sha256:" + strings.Repeat("c", 64),
	}
}

// flatCloses 构造可控序列：prefix 段固定值，tail 段固定值，最后一日覆盖值。
func flatCloses(prefixCount, tailCount int, prefix, tail, last string) []string {
	closes := make([]string, 0, prefixCount+tailCount+1)
	for index := 0; index < prefixCount; index++ {
		closes = append(closes, prefix)
	}
	for index := 0; index < tailCount; index++ {
		closes = append(closes, tail)
	}
	return append(closes, last)
}

func TestTrendAdapterProducesValidatedStageOutputForBullishSeries(t *testing.T) {
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 61))}
	creator := &trendSnapshotCreatorStub{refs: trendStageReferences()}
	adapter := newTestTrendAdapter(t, creator, reader)
	output, quality, err := adapter.Execute(context.Background(), validTrendStageRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if creator.calls != 1 || reader.calls != 1 || creator.last.Limit != 61 || creator.last.Symbol != "600000.SH" {
		t.Fatalf("Stage did not create exactly one Snapshot for limit=61: creator=%+v readerCalls=%d", creator, reader.calls)
	}
	if output.Direction != trend.DirectionBullish || output.Phase != trend.PhaseUnknown ||
		output.Strength != nil || output.Period != "20d" ||
		output.QualityStatus != trend.QualityValid || output.ResultStatus != "completed" {
		t.Fatalf("unexpected Trend Stage Output: %+v", output)
	}
	if len(output.EvidenceMetrics) != 4 || len(output.ConflictRefs) != 0 {
		t.Fatalf("Trend evidence is not the fixed D1 set: %+v", output.EvidenceMetrics)
	}
	wantCodes := []string{"ma20_distance", "momentum_5d", "momentum_20d", "momentum_60d"}
	wantSamples := []int{20, 6, 21, 61}
	for index, item := range output.EvidenceMetrics {
		if item.MetricCode != wantCodes[index] || item.SampleCount == nil || *item.SampleCount != wantSamples[index] ||
			item.MetricStatus != "available" || item.Unit != "ratio" {
			t.Fatalf("evidence[%d] = %+v, want code=%s samples=%d", index, item, wantCodes[index], wantSamples[index])
		}
	}
	if !strings.HasPrefix(output.ResultID, "quant_result:sha256:") {
		t.Fatalf("Trend Stage Output must carry a Manifest-issued result_id, got %q", output.ResultID)
	}
	if quality.TrendCode != trend.CloseTrendCode || quality.ObservedBars != 61 ||
		quality.ExpectedTradingDays != 61 || !quality.CoverageSufficient {
		t.Fatalf("unexpected Trend Stage quality summary: %+v", quality)
	}
	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTrendStageOutputJSON(payload); err != nil {
		t.Fatalf("Trend Stage Output failed its own validator: %v", err)
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTrendQualitySummaryJSON(qualityJSON); err != nil {
		t.Fatalf("Trend Stage quality summary failed its own validator: %v", err)
	}
}

func TestTrendAdapterDerivesSidewaysWithoutThresholds(t *testing.T) {
	// 前 41 根 100、随后 19 根 120、最后 105：收盘价低于 MA20（ma20_distance<0）
	// 但仍高于 21 根前的收盘价（momentum_20d>0），因此按 D2 纯符号判定为 sideways。
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(flatCloses(41, 19, "100", "120", "105"))}
	creator := &trendSnapshotCreatorStub{refs: trendStageReferences()}
	adapter := newTestTrendAdapter(t, creator, reader)
	output, _, err := adapter.Execute(context.Background(), validTrendStageRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Direction != trend.DirectionSideways || output.ResultStatus != "completed" || output.QualityStatus != trend.QualityValid {
		t.Fatalf("sign-conflict series must be sideways/completed/valid, got %+v", output)
	}
}

func TestTrendAdapterReportsInsufficientLongPeriodAsDegraded(t *testing.T) {
	// 30 根足以判定方向（MA20 与 20 日动量可用），但不足 61 根长周期证据。
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 30))}
	creator := &trendSnapshotCreatorStub{refs: trendStageReferences()}
	adapter := newTestTrendAdapter(t, creator, reader)
	output, quality, err := adapter.Execute(context.Background(), validTrendStageRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Direction != trend.DirectionBullish || output.ResultStatus != "completed" ||
		output.QualityStatus != trend.QualityDegraded {
		t.Fatalf("missing long-period evidence must stay degraded but completed, got %+v", output)
	}
	if output.EvidenceMetrics[3].MetricStatus != "insufficient_data" ||
		string(output.EvidenceMetrics[3].Value) != "null" {
		t.Fatalf("momentum_60d must be an explicit insufficient_data null, got %+v", output.EvidenceMetrics[3])
	}
	if quality.ObservedBars != 30 || quality.ExpectedTradingDays != 30 {
		t.Fatalf("unexpected quality summary: %+v", quality)
	}
	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTrendStageOutputJSON(payload); err != nil {
		t.Fatalf("degraded Trend Stage Output failed its validator: %v", err)
	}
}

func TestTrendAdapterReportsUnknownDirectionAsInsufficientData(t *testing.T) {
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 10))}
	creator := &trendSnapshotCreatorStub{refs: trendStageReferences()}
	adapter := newTestTrendAdapter(t, creator, reader)
	output, _, err := adapter.Execute(context.Background(), validTrendStageRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if output.Direction != trend.DirectionUnknown || output.ResultStatus != "insufficient_data" ||
		output.QualityStatus != trend.QualityUnknown || output.Phase != trend.PhaseUnknown || output.Strength != nil {
		t.Fatalf("unable-to-assess series must be unknown/insufficient_data/unknown, got %+v", output)
	}
	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTrendStageOutputJSON(payload); err != nil {
		t.Fatalf("insufficient-data Trend Stage Output failed its validator: %v", err)
	}
}

func TestTrendAdapterRejectsInvalidRequestsBeforeCreatingSnapshot(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*TrendRequest)
	}{
		{"limit below trend minimum", func(r *TrendRequest) { r.Limit = 60 }},
		{"unsorted provider codes", func(r *TrendRequest) { r.AllowedProviderCodes = []string{"eastmoney", "baostock"} }},
		{"missing provider code", func(r *TrendRequest) { r.AllowedProviderCodes = []string{"baostock"} }},
		{"non UTC system as of", func(r *TrendRequest) { r.SystemAsOf = r.SystemAsOf.In(time.FixedZone("CST", 8*3600)) }},
		{"malformed symbol", func(r *TrendRequest) { r.Symbol = "600000" }},
		{"inverted date range", func(r *TrendRequest) { r.StartDate, r.EndDate = r.EndDate, r.StartDate }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			creator := &trendSnapshotCreatorStub{}
			reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 61))}
			adapter := newTestTrendAdapter(t, creator, reader)
			request := validTrendStageRequest()
			testCase.mutate(&request)
			if _, _, err := adapter.Execute(context.Background(), request); !errors.Is(err, ErrInvalidTrendStageRequest) {
				t.Fatalf("Execute() error = %v, want ErrInvalidTrendStageRequest", err)
			}
			if creator.calls != 0 || reader.calls != 0 {
				t.Fatalf("invalid request reached Data: creatorCalls=%d readerCalls=%d", creator.calls, reader.calls)
			}
		})
	}
}

func TestValidateTrendStageOutputRejectsSemanticDrift(t *testing.T) {
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 61))}
	creator := &trendSnapshotCreatorStub{refs: trendStageReferences()}
	adapter := newTestTrendAdapter(t, creator, reader)
	output, _, err := adapter.Execute(context.Background(), validTrendStageRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	base, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateTrendStageOutputJSON(base); err != nil {
		t.Fatalf("baseline output must be valid: %v", err)
	}
	cases := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"flipped direction", func(document map[string]any) { document["direction"] = "bearish" }},
		{"non-null strength", func(document map[string]any) { document["strength"] = 0.5 }},
		{"concrete phase", func(document map[string]any) { document["phase"] = "accelerating" }},
		{"conflict refs", func(document map[string]any) { document["conflict_refs"] = []string{"momentum_5d"} }},
		{"completed with unknown direction", func(document map[string]any) {
			document["direction"] = "unknown"
			document["result_status"] = "completed"
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal(base, &document); err != nil {
				t.Fatal(err)
			}
			testCase.mutate(document)
			drifted, err := json.Marshal(document)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateTrendStageOutputJSON(drifted); !errors.Is(err, ErrInvalidTrendStageOutput) {
				t.Fatalf("validator accepted drifted payload: err=%v", err)
			}
		})
	}
}

func TestTrendAdapterPropagatesSnapshotCreationFailure(t *testing.T) {
	creationErr := errors.New("data provider unavailable")
	creator := &trendSnapshotCreatorStub{err: creationErr}
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 61))}
	adapter := newTestTrendAdapter(t, creator, reader)
	if _, _, err := adapter.Execute(context.Background(), validTrendStageRequest()); !errors.Is(err, creationErr) {
		t.Fatalf("Execute() error = %v, want wrapped snapshot creation failure", err)
	}
}

func TestNewTrendAdapterRejectsMissingDependencies(t *testing.T) {
	reader := &factorSnapshotReaderStub{snapshot: factorStageSnapshot(factorStageCloses(1, 61))}
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatal(err)
	}
	service, err := application.NewCreateTrendService(
		reader, quantRegistry, result.NewMemoryRepository(),
		func() time.Time { return time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTrendAdapter(nil, service); !errors.Is(err, ErrInvalidTrendStageRequest) {
		t.Fatalf("nil snapshot creator error = %v", err)
	}
	if _, err := NewTrendAdapter(&trendSnapshotCreatorStub{}, nil); !errors.Is(err, ErrInvalidTrendStageRequest) {
		t.Fatalf("nil Trend service error = %v", err)
	}
}
