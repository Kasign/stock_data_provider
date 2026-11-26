package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
)

// 执行清单 1.5 的验收测试：输入质量事实 → 适用性判定的映射必须是封闭表，
// 且三个 Compute Facade 必须原样保留原因码，不允许各自拍脑袋改写。

func validInputQualityFacts() InputQualityFacts {
	return InputQualityFacts{
		PITEvidenceAvailable:    true,
		SnapshotQuality:         "valid",
		CalendarQuality:         "valid",
		CoverageSufficient:      true,
		ObservedBars:            61,
		MinimumRequiredBars:     61,
		CoverageAtOrAbovePolicy: true,
		BarQualities:            []string{"valid", "valid"},
	}
}

func TestEvaluateInputApplicabilityMapsEachReason(t *testing.T) {
	cases := []struct {
		name       string
		mutate     func(*InputQualityFacts)
		wantReason string
	}{
		{"pit unavailable", func(facts *InputQualityFacts) { facts.PITEvidenceAvailable = false }, ReasonInputPITUnavailable},
		{"snapshot quality invalid", func(facts *InputQualityFacts) { facts.SnapshotQuality = "degraded" }, ReasonInputQualityInvalid},
		{"calendar quality invalid", func(facts *InputQualityFacts) { facts.CalendarQuality = "unknown" }, ReasonInputCalendarInvalid},
		{"data-side coverage insufficient", func(facts *InputQualityFacts) { facts.CoverageSufficient = false }, ReasonInputCoverageInsufficient},
		{"observed bars below minimum", func(facts *InputQualityFacts) { facts.ObservedBars = 60 }, ReasonInputBarsBelowMinimum},
		{"minimum required bars is not positive", func(facts *InputQualityFacts) { facts.MinimumRequiredBars = 0 }, ReasonInputBarsBelowMinimum},
		{"coverage below registry policy", func(facts *InputQualityFacts) { facts.CoverageAtOrAbovePolicy = false }, ReasonInputCoverageBelowPolicy},
		{"bar quality invalid", func(facts *InputQualityFacts) { facts.BarQualities = []string{"valid", "suspect"} }, ReasonInputBarQualityInvalid},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			facts := validInputQualityFacts()
			testCase.mutate(&facts)
			applicable, reason := EvaluateInputApplicability(facts)
			if applicable || reason != testCase.wantReason {
				t.Fatalf("EvaluateInputApplicability() = (%v, %q), want (false, %q)", applicable, reason, testCase.wantReason)
			}
		})
	}
	if applicable, reason := EvaluateInputApplicability(validInputQualityFacts()); !applicable || reason != "" {
		t.Fatalf("valid facts = (%v, %q), want applicable with empty reason", applicable, reason)
	}
}

// TestEvaluateInputApplicabilityIsOrdered 固定优先级：同一份输入同时违反多条时，
// 结论只能由表格中的先后顺序决定，不能随调用点或 map 迭代顺序变化。
func TestEvaluateInputApplicabilityIsOrdered(t *testing.T) {
	facts := validInputQualityFacts()
	facts.PITEvidenceAvailable = false
	facts.SnapshotQuality = "invalid"
	facts.CalendarQuality = "invalid"
	facts.CoverageSufficient = false
	facts.ObservedBars = 1
	facts.CoverageAtOrAbovePolicy = false
	facts.BarQualities = []string{"invalid"}
	if applicable, reason := EvaluateInputApplicability(facts); applicable || reason != ReasonInputPITUnavailable {
		t.Fatalf("all-failing facts = (%v, %q), want PIT to win", applicable, reason)
	}

	facts = validInputQualityFacts()
	facts.SnapshotQuality = "invalid"
	facts.CalendarQuality = "invalid"
	if _, reason := EvaluateInputApplicability(facts); reason != ReasonInputQualityInvalid {
		t.Fatalf("quality vs calendar = %q, want snapshot quality to win", reason)
	}

	facts = validInputQualityFacts()
	facts.CoverageSufficient = false
	facts.ObservedBars = 1
	if _, reason := EvaluateInputApplicability(facts); reason != ReasonInputCoverageInsufficient {
		t.Fatalf("coverage flag vs bar count = %q, want Data coverage flag to win", reason)
	}

	facts = validInputQualityFacts()
	facts.ObservedBars = 60
	facts.CoverageAtOrAbovePolicy = false
	if _, reason := EvaluateInputApplicability(facts); reason != ReasonInputBarsBelowMinimum {
		t.Fatalf("bar count vs coverage ratio = %q, want bar count to win", reason)
	}
}

func TestInputNotApplicableErrorUnwrapsToQualitySentinel(t *testing.T) {
	err := error(&InputNotApplicableError{ReasonCode: ReasonInputCoverageInsufficient})
	if !errors.Is(err, ErrCloseSeriesSnapshotQuality) {
		t.Fatalf("InputNotApplicableError must unwrap to ErrCloseSeriesSnapshotQuality, got %v", err)
	}
	if errors.Is(err, ErrInvalidCloseSeriesSnapshot) {
		t.Fatal("InputNotApplicableError must not be classified as a structural Snapshot defect")
	}
}

func TestClassifyCloseSeriesValidationErrorKeepsReasonCode(t *testing.T) {
	if err := classifyCloseSeriesValidationError(nil, ErrSMASnapshotQuality, ErrInvalidSMASnapshot); err != nil {
		t.Fatalf("nil error must stay nil, got %v", err)
	}

	applicability := &InputNotApplicableError{ReasonCode: ReasonInputCalendarInvalid}
	classified := classifyCloseSeriesValidationError(applicability, ErrSMASnapshotQuality, ErrInvalidSMASnapshot)
	if !errors.Is(classified, ErrSMASnapshotQuality) {
		t.Fatalf("classified error = %v, want ErrSMASnapshotQuality in chain", classified)
	}
	var extracted *InputNotApplicableError
	if !errors.As(classified, &extracted) || extracted.ReasonCode != ReasonInputCalendarInvalid {
		t.Fatalf("reason code lost in classification: %v", classified)
	}

	if err := classifyCloseSeriesValidationError(ErrCloseSeriesSnapshotQuality, ErrSMASnapshotQuality, ErrInvalidSMASnapshot); err != ErrSMASnapshotQuality {
		t.Fatalf("plain quality error = %v, want ErrSMASnapshotQuality", err)
	}
	if err := classifyCloseSeriesValidationError(ErrInvalidCloseSeriesSnapshot, ErrSMASnapshotQuality, ErrInvalidSMASnapshot); err != ErrInvalidSMASnapshot {
		t.Fatalf("structural error = %v, want ErrInvalidSMASnapshot", err)
	}
}

// closeSeriesTestPolicy 与生产 Facade 使用同一构造入口（Registry Entry），
// 避免测试自造门槛而掩盖 Entry 与门禁的漂移。
func closeSeriesTestPolicy(t *testing.T) closeSeriesValidationPolicy {
	t.Helper()
	quantRegistry, err := registry.NewProductionRegistry()
	if err != nil {
		t.Fatalf("create production Quant Registry: %v", err)
	}
	entry, err := quantRegistry.LookupActive(registry.ProductionMomentum60Code, factorSchemaVersion)
	if err != nil {
		t.Fatalf("lookup momentum_60d entry: %v", err)
	}
	policy, err := closeSeriesValidationPolicyFromEntry(entry, factorPurposeCode, "eastmoney", "eastmoney-stock-api-v1")
	if err != nil {
		t.Fatalf("build validation policy: %v", err)
	}
	return policy
}

func TestValidateAuthorizedCloseSeriesSnapshotReportsExactReason(t *testing.T) {
	request := validCreateTrendRequest()
	trustedNow := factorExecutionNow()
	cases := []struct {
		name       string
		mutate     func(*CloseSeriesSnapshot)
		wantReason string
	}{
		{"pit evidence mismatched", func(snapshot *CloseSeriesSnapshot) {
			snapshot.SystemAsOf = "2026-11-22T00:00:00Z"
		}, ReasonInputPITUnavailable},
		{"snapshot quality invalid", func(snapshot *CloseSeriesSnapshot) {
			snapshot.QualityStatus = "degraded"
		}, ReasonInputQualityInvalid},
		{"calendar quality invalid", func(snapshot *CloseSeriesSnapshot) {
			snapshot.Coverage.CalendarQualityStatus = "degraded"
		}, ReasonInputCalendarInvalid},
		{"data coverage insufficient", func(snapshot *CloseSeriesSnapshot) {
			snapshot.Coverage.CoverageSufficient = false
		}, ReasonInputCoverageInsufficient},
		{"bars below minimum", func(snapshot *CloseSeriesSnapshot) {
			snapshot.Coverage.MinimumRequiredBars = 62
		}, ReasonInputBarsBelowMinimum},
		{"coverage below registry policy", func(snapshot *CloseSeriesSnapshot) {
			// 61/70 = 0.871 < momentum_60d Entry 的 minimum_coverage，但 Data 侧仍标记为充分。
			snapshot.Coverage.ExpectedTradingDays = 70
		}, ReasonInputCoverageBelowPolicy},
		{"bar quality invalid", func(snapshot *CloseSeriesSnapshot) {
			snapshot.Bars[0].QualityStatus = "suspect"
		}, ReasonInputBarQualityInvalid},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			snapshot := validCreateFactorSnapshot(sequenceCloses(1, 61))
			testCase.mutate(&snapshot)
			_, err := validateAuthorizedCloseSeriesSnapshot(
				snapshot, request.SnapshotRef, request.SystemAsOf, request.AllowedProviderCodes,
				closeSeriesTestPolicy(t), trustedNow,
			)
			var applicability *InputNotApplicableError
			if !errors.As(err, &applicability) || applicability.ReasonCode != testCase.wantReason {
				t.Fatalf("validateAuthorizedCloseSeriesSnapshot() error = %v, want reason %q", err, testCase.wantReason)
			}
			if !errors.Is(err, ErrCloseSeriesSnapshotQuality) {
				t.Fatalf("error %v lost ErrCloseSeriesSnapshotQuality", err)
			}
		})
	}
}

// TestComputeFacadesPreserveInputReasonCode 是 1.5 的核心不变式：
// 同一个"输入不适用"事实，在三个用例里必须落到各自的既有错误类型上，
// 同时错误链里仍能取出同一个原因码，供 Stage 层给出精确 reason code。
func TestComputeFacadesPreserveInputReasonCode(t *testing.T) {
	const wantReason = ReasonInputCoverageInsufficient
	factorSnapshot := validCreateFactorSnapshot(sequenceCloses(1, 61))
	factorSnapshot.Coverage.CoverageSufficient = false

	factorService := newTestCreateFactorService(t, &createFactorSnapshotReader{snapshot: factorSnapshot}, factorExecutionNow())
	_, err := factorService.Execute(context.Background(), validCreateFactorRequest(registry.ProductionMADistanceCode))
	assertReasonCode(t, err, ErrFactorSnapshotQuality, wantReason)

	smaSnapshot := validCreateSMASnapshot()
	smaSnapshot.Coverage.CoverageSufficient = false
	smaService := newTestCreateSMAService(t, &createSMASnapshotReader{snapshot: smaSnapshot}, result.NewMemoryRepository(), factorExecutionNow())
	_, err = smaService.Execute(context.Background(), validCreateSMARequest())
	assertReasonCode(t, err, ErrSMASnapshotQuality, wantReason)

	trendService := newTestCreateTrendService(t, &createFactorSnapshotReader{snapshot: factorSnapshot}, factorExecutionNow())
	_, err = trendService.Execute(context.Background(), validCreateTrendRequest())
	assertReasonCode(t, err, ErrTrendSnapshotQuality, wantReason)
}

func assertReasonCode(t *testing.T, err, sentinel error, wantReason string) {
	t.Helper()
	if !errors.Is(err, sentinel) {
		t.Fatalf("Execute() error = %v, want %v in chain", err, sentinel)
	}
	assertOptionalReasonCode(t, err, wantReason)
}

// assertOptionalReasonCode 断言错误链中的原因码：wantReason 为空时要求"没有原因码"，
// 这样结构错误若被误判成适用性失败也会立即失败。
func assertOptionalReasonCode(t *testing.T, err error, wantReason string) {
	t.Helper()
	var applicability *InputNotApplicableError
	found := errors.As(err, &applicability)
	if wantReason == "" {
		if found {
			t.Fatalf("error %v must not carry an applicability reason, got %q", err, applicability.ReasonCode)
		}
		return
	}
	if !found {
		t.Fatalf("error %v is missing *InputNotApplicableError, want reason %q", err, wantReason)
	}
	if applicability.ReasonCode != wantReason {
		t.Fatalf("reason code = %q, want %q", applicability.ReasonCode, wantReason)
	}
}

// TestComputeFacadesDoNotLeakStructuralErrorsIntoQualityClassification 断言
// 结构性缺陷仍然走各自的结构错误，不会被误报成"数据质量不达标"。
func TestComputeFacadesKeepStructuralErrorsSeparate(t *testing.T) {
	snapshot := validCreateFactorSnapshot(sequenceCloses(1, 61))
	snapshot.AdjustmentBasis = "qfq"

	service := newTestCreateFactorService(t, &createFactorSnapshotReader{snapshot: snapshot}, factorExecutionNow())
	_, err := service.Execute(context.Background(), validCreateFactorRequest(registry.ProductionMADistanceCode))
	if !errors.Is(err, ErrInvalidFactorSnapshot) {
		t.Fatalf("Execute() error = %v, want ErrInvalidFactorSnapshot", err)
	}
	var applicability *InputNotApplicableError
	if errors.As(err, &applicability) {
		t.Fatalf("structural defect must not carry an applicability reason: %v", err)
	}
	if errors.Is(err, ErrFactorSnapshotQuality) {
		t.Fatalf("structural defect must not be classified as a quality failure: %v", err)
	}
}
