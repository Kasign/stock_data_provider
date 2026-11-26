package application

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

func newTestCreateTrendService(t *testing.T, reader CloseSeriesSnapshotReader, now time.Time) *CreateTrendService {
	t.Helper()
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	service, err := NewCreateTrendService(reader, quantRegistry, result.NewMemoryRepository(), func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewCreateTrendService() error = %v", err)
	}
	return service
}

func validCreateTrendRequest() CreateTrendRequest {
	return CreateTrendRequest{
		SnapshotRef:          "data_snapshot:sha256:" + strings.Repeat("a", 64),
		AuthorizationRef:     "data_authorization:sha256:" + strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		Parameters:           json.RawMessage(`{}`),
		AllowedProviderCodes: []string{"eastmoney"},
	}
}

// TestTrendEvidenceCodesMatchRegistry 断言 trend 包复述的证据 code 与 Registry 常量一致。
// trend 不能直接 import registry（registry 的测试反向引用 trend 会形成测试期循环依赖），
// 因此把一致性收敛到本包这一处断言，任何一侧改名都会在这里失败。
func TestTrendEvidenceCodesMatchRegistry(t *testing.T) {
	pairs := []struct {
		trendCode    string
		registryCode string
	}{
		{trend.EvidenceMADistanceCode, registry.ProductionMADistanceCode},
		{trend.EvidenceMomentum5Code, registry.ProductionMomentum5Code},
		{trend.EvidenceMomentum20Code, registry.ProductionMomentum20Code},
		{trend.EvidenceMomentum60Code, registry.ProductionMomentum60Code},
	}
	for _, pair := range pairs {
		if pair.trendCode != pair.registryCode {
			t.Fatalf("trend evidence code %q drifted from registry code %q", pair.trendCode, pair.registryCode)
		}
	}
	if trend.CloseTrendMinimumBars != 61 {
		t.Fatalf("CloseTrendMinimumBars = %d, want 61 (D1 周期集合 5/20/60)", trend.CloseTrendMinimumBars)
	}
}

func TestCreateTrendServicePersistsBullishTrendFromExactEvidence(t *testing.T) {
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 61))}
	service := newTestCreateTrendService(t, reader, factorExecutionNow())
	execution, err := service.Execute(context.Background(), validCreateTrendRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if execution.Result.Direction != trend.DirectionBullish || execution.Result.QualityStatus != trend.QualityValid {
		t.Fatalf("Result = %+v, want bullish/valid", execution.Result)
	}
	if execution.Manifest.ResultType != "trend" || execution.Manifest.ResultStatus != "completed" ||
		execution.Manifest.QualityStatus != "valid" || execution.Manifest.Period != trend.CloseTrendPeriod ||
		execution.Manifest.AlgorithmVersion != trend.CloseTrendAlgorithmVersion ||
		execution.Manifest.PayloadSchemaVersion != "1.0.0" {
		t.Fatalf("Manifest metadata does not describe a trend result: %+v", execution.Manifest)
	}
	if reader.lastPurpose != trendPurposeCode || reader.lastRef != execution.SnapshotRef || reader.calls != 1 {
		t.Fatalf("reader calls = %d purpose=%q ref=%q", reader.calls, reader.lastPurpose, reader.lastRef)
	}
	if execution.InputQualityStatus != "valid" || execution.ObservedBars != 61 ||
		execution.Coverage.ExpectedTradingDays != 61 {
		t.Fatalf("input quality evidence not carried: %+v", execution)
	}
	// evidence 顺序必须与 D1 固定集合一致，且 ratio 值不被改写为 percent。
	wantCodes := []string{"ma20_distance", "momentum_5d", "momentum_20d", "momentum_60d"}
	for index, item := range execution.Result.EvidenceMetrics {
		if item.MetricCode != wantCodes[index] || item.Unit != "ratio" || item.SampleCount == nil {
			t.Fatalf("evidence[%d] = %+v", index, item)
		}
	}
}

func TestCreateTrendServiceKeepsDegradedAndInsufficientResultsPersistable(t *testing.T) {
	cases := []struct {
		name          string
		bars          int
		wantDirection trend.Direction
		wantQuality   string
		wantStatus    string
	}{
		{"missing long period", 30, trend.DirectionBullish, "degraded", "completed"},
		{"unable to assess", 10, trend.DirectionUnknown, "unknown", "insufficient_data"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, testCase.bars))}
			service := newTestCreateTrendService(t, reader, factorExecutionNow())
			execution, err := service.Execute(context.Background(), validCreateTrendRequest())
			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if execution.Result.Direction != testCase.wantDirection ||
				string(execution.Result.QualityStatus) != testCase.wantQuality ||
				execution.Manifest.ResultStatus != testCase.wantStatus {
				t.Fatalf("Result/Manifest = %+v / %+v, want %s/%s/%s", execution.Result, execution.Manifest,
					testCase.wantDirection, testCase.wantQuality, testCase.wantStatus)
			}
			// QNT-CTR-005 绑定：payload quality_status 必须等于 Manifest quality_status。
			if execution.Manifest.QualityStatus != string(execution.Result.QualityStatus) {
				t.Fatalf("Manifest quality %q differs from payload quality %q",
					execution.Manifest.QualityStatus, execution.Result.QualityStatus)
			}
			if len(execution.Manifest.ResultID) == 0 {
				t.Fatal("degraded/insufficient trend result must still be persisted with a content-addressed result_id")
			}
			if execution.InputQualityStatus != "valid" {
				t.Fatalf("input quality must stay valid independently of the assessment: %q", execution.InputQualityStatus)
			}
		})
	}
}

func TestCreateTrendServiceRejectsInvalidRequests(t *testing.T) {
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 61))}
	service := newTestCreateTrendService(t, reader, factorExecutionNow())
	cases := []struct {
		name   string
		mutate func(*CreateTrendRequest)
	}{
		{"non empty parameters", func(r *CreateTrendRequest) { r.Parameters = json.RawMessage(`{"period":"60d"}`) }},
		{"malformed parameters", func(r *CreateTrendRequest) { r.Parameters = json.RawMessage(`{} {}`) }},
		{"missing snapshot ref", func(r *CreateTrendRequest) { r.SnapshotRef = " " }},
		{"missing authorization ref", func(r *CreateTrendRequest) { r.AuthorizationRef = "" }},
		{"non UTC system as of", func(r *CreateTrendRequest) { r.SystemAsOf = r.SystemAsOf.In(time.FixedZone("CST", 8*3600)) }},
		{"missing provider code", func(r *CreateTrendRequest) { r.AllowedProviderCodes = []string{"baostock"} }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := validCreateTrendRequest()
			testCase.mutate(&request)
			if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrInvalidCreateTrendRequest) {
				t.Fatalf("Execute() error = %v, want ErrInvalidCreateTrendRequest", err)
			}
			if reader.calls != 0 {
				t.Fatalf("invalid request reached the authorized Snapshot reader: calls=%d", reader.calls)
			}
		})
	}
}

func TestCreateTrendServiceMapsSnapshotQualityFailure(t *testing.T) {
	snapshot := validCreateFactorSnapshot(sequenceCloses(1, 61))
	snapshot.Coverage.CoverageSufficient = false
	reader := &createFactorSnapshotReader{snapshot: snapshot}
	service := newTestCreateTrendService(t, reader, factorExecutionNow())
	_, err := service.Execute(context.Background(), validCreateTrendRequest())
	if !errors.Is(err, ErrTrendSnapshotQuality) {
		t.Fatalf("Execute() error = %v, want ErrTrendSnapshotQuality", err)
	}
	assertOptionalReasonCode(t, err, ReasonInputCoverageInsufficient)
}

func TestCreateTrendServiceStopsOnCanceledContext(t *testing.T) {
	reader := &createFactorSnapshotReader{snapshot: validCreateFactorSnapshot(sequenceCloses(1, 61))}
	service := newTestCreateTrendService(t, reader, factorExecutionNow())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Execute(ctx, validCreateTrendRequest()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if reader.calls != 0 {
		t.Fatalf("canceled context must not read Snapshot; calls=%d", reader.calls)
	}
}
