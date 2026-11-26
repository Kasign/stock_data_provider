package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/platform/security"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantregistry "example.com/stock-analysis-flow/internal/quant/registry"
	quantresult "example.com/stock-analysis-flow/internal/quant/result"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

// 端到端质量证据测试：Data 授权 Snapshot → Quant Factor → 结果落库 → Run/Stage 关联。
//
// 只替换持久化实现：marketdata/bootstrap.Module 需要 MariaDB，本文件用真实 Data
// Application（Snapshot Creator / Authorized Snapshot Reader / Authorization Owner）
// 与内存 Store 替代 Root 的数据库装配，不替换任何 Data 语义（manifest/payload 指纹、
// 覆盖率证据、授权签发与复核、PIT 证据校验全部由生产代码执行）。
// 因此 QualitySummary 中的质量事实必须来自 Data 自己记录的 coverage evidence，
// 而不是 Quant 测试为通过断言而构造的数字。

const (
	quantFactorEndToEndNow    = "2026-11-05T08:00:00Z"
	quantFactorEndToEndCutoff = "2026-11-04T00:00:00Z"
	quantFactorEndToEndSymbol = "600036.SH"
)

// quantFactorEndToEndStore 同时实现 Data 的 Snapshot Repository、授权决策 Store、
// 策略 Repository 与授权读取 Repository 四个 Port。
type quantFactorEndToEndStore struct {
	snapshots map[string]app.DailyBarsCloseSeriesSnapshot
	decisions map[string]app.DataAuthorizationDecision
	current   authorization.CurrentPolicy
}

func newQuantFactorEndToEndStore(current authorization.CurrentPolicy) *quantFactorEndToEndStore {
	return &quantFactorEndToEndStore{
		snapshots: map[string]app.DailyBarsCloseSeriesSnapshot{},
		decisions: map[string]app.DataAuthorizationDecision{},
		current:   current,
	}
}

func (store *quantFactorEndToEndStore) SaveDailyBarsCloseSeriesSnapshot(_ context.Context, snapshot app.DailyBarsCloseSeriesSnapshot) error {
	store.snapshots[snapshot.Manifest.SnapshotRef] = snapshot
	return nil
}

func (store *quantFactorEndToEndStore) GetDailyBarsCloseSeriesSnapshot(_ context.Context, snapshotRef string) (app.DailyBarsCloseSeriesSnapshot, error) {
	snapshot, present := store.snapshots[snapshotRef]
	if !present {
		return app.DailyBarsCloseSeriesSnapshot{}, errors.New("daily-bars close-series Snapshot not found")
	}
	return snapshot, nil
}

func (store *quantFactorEndToEndStore) PublishDataAuthorizationPolicyVersion(
	context.Context, string, authorization.PolicyVersion, app.DataAuthorizationPolicyPublicationAudit,
) error {
	return errors.New("policy publication is not part of the Quant end-to-end evidence")
}

func (store *quantFactorEndToEndStore) ResolveCurrentDataAuthorizationPolicy(context.Context) (authorization.CurrentPolicy, error) {
	return store.current, nil
}

func (store *quantFactorEndToEndStore) AppendDataAuthorizationDecision(_ context.Context, decision app.DataAuthorizationDecision) error {
	var ref struct {
		AuthorizationRef string `json:"authorization_ref"`
	}
	if err := json.Unmarshal(decision.AuthorizationRef, &ref); err != nil {
		return err
	}
	store.decisions[ref.AuthorizationRef] = decision
	return nil
}

func (store *quantFactorEndToEndStore) GetDataAuthorizationDecision(_ context.Context, authorizationRef string) (app.DataAuthorizationDecision, error) {
	decision, present := store.decisions[authorizationRef]
	if !present {
		return app.DataAuthorizationDecision{}, errors.New("data authorization decision not found")
	}
	return decision, nil
}

func (store *quantFactorEndToEndStore) ReadDataAuthorizedDailyBarsCloseSeriesSnapshot(
	_ context.Context, authorizationRef, snapshotRef string,
) (app.DataAuthorizationDecision, app.DailyBarsCloseSeriesSnapshot, authorization.CurrentPolicy, error) {
	decision, present := store.decisions[authorizationRef]
	if !present {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, errors.New("data authorization decision not found")
	}
	snapshot, present := store.snapshots[snapshotRef]
	if !present {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, errors.New("daily-bars close-series Snapshot not found")
	}
	return decision, snapshot, store.current, nil
}

// quantFactorEndToEndPITReader 返回一次 PIT V2 日线读取结果；
// CoverageEvidence 与 Bars 一起构成 Data 的覆盖率事实，Quant 只能原样消费。
type quantFactorEndToEndPITReader struct {
	snapshot app.DailyBarsV2Snapshot
}

func (reader *quantFactorEndToEndPITReader) GetDailyBarsV2(_ context.Context, query app.DailyBarsQuery) (app.DailyBarsV2Snapshot, error) {
	if query.Symbol != reader.snapshot.Symbol || query.SystemObservedAtCutoff == nil ||
		!query.SystemObservedAtCutoff.Equal(*reader.snapshot.SystemObservedAtCutoff) {
		return app.DailyBarsV2Snapshot{}, errors.New("PIT query does not match the recorded daily-bars read")
	}
	return reader.snapshot, nil
}

// quantFactorEndToEndDataPort 等价于 marketdata/bootstrap.Module 的两行委托，
// 只是把 MariaDB Store 换成内存 Store。
type quantFactorEndToEndDataPort struct {
	creator *marketdataapplication.DailyBarsCloseSeriesSnapshotCreator
	reader  *marketdataapplication.AuthorizedDailyBarsCloseSeriesSnapshotReader
}

func (port *quantFactorEndToEndDataPort) CreateDailyBarsCloseSeriesSnapshot(
	ctx context.Context, query app.DailyBarsQuery, purpose string,
) (marketdataapplication.DailyBarsCloseSeriesSnapshotReferences, error) {
	return port.creator.Create(ctx, query, purpose)
}

func (port *quantFactorEndToEndDataPort) ReadAuthorizedDailyBarsCloseSeriesSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.DailyBarsCloseSeriesSnapshot, error) {
	return port.reader.Read(ctx, snapshotRef, authorizationRef, purpose)
}

var _ quantDataSnapshotPort = (*quantFactorEndToEndDataPort)(nil)

// quantFactorEndToEndRecordingRepository 在真实落库之外记录实际写入的记录，
// 用于断言 Stage Output 的 result_id 就是持久化 Manifest 的 result_id。
type quantFactorEndToEndRecordingRepository struct {
	inner   quantresult.Repository
	records []quantresult.Record
}

func (repository *quantFactorEndToEndRecordingRepository) Save(ctx context.Context, record quantresult.Record) error {
	repository.records = append(repository.records, record)
	return repository.inner.Save(ctx, record)
}

type quantFactorEndToEndChain struct {
	adapter    *quantstage.FactorAdapter
	request    quantstage.FactorRequest
	store      *quantFactorEndToEndStore
	repository *quantFactorEndToEndRecordingRepository
	context    context.Context
}

func quantFactorEndToEndDates(count int) []string {
	dates := []string{
		"2026-10-08", "2026-10-09", "2026-10-12", "2026-10-13", "2026-10-14",
		"2026-10-15", "2026-10-16", "2026-10-19", "2026-10-20", "2026-10-21",
		"2026-10-22", "2026-10-23", "2026-10-26", "2026-10-27", "2026-10-28",
		"2026-10-29", "2026-10-30", "2026-11-02", "2026-11-03", "2026-11-04",
	}
	return append([]string(nil), dates[:count]...)
}

// quantFactorEndToEndCloses 是 20 根连续交易日的真实量级收盘价。
// 期望因子值由独立十进制计算得出，不在测试里复用被测算法。
var quantFactorEndToEndCloses = []string{
	"38.10", "38.42", "38.05", "37.88", "38.30",
	"38.66", "38.91", "38.44", "38.12", "37.95",
	"38.21", "38.58", "38.83", "39.11", "38.76",
	"38.49", "38.62", "38.90", "39.24", "39.05",
}

func newQuantFactorEndToEndChain(t *testing.T, bars int) quantFactorEndToEndChain {
	t.Helper()
	cutoff := mustQuantFactorEndToEndTime(t, quantFactorEndToEndCutoff)
	now := mustQuantFactorEndToEndTime(t, quantFactorEndToEndNow)
	dates := quantFactorEndToEndDates(bars)
	closes := append([]string(nil), quantFactorEndToEndCloses[:bars]...)

	barsRead := make([]app.DailyBar, bars)
	for index := range barsRead {
		fetchedAt := cutoff.Add(-2 * time.Hour)
		observedAt := cutoff.Add(-time.Hour)
		fractionDigits := 2
		closeEvidence := &app.DailyBarDecimalEvidence{
			EvidenceKind: "observed_token", SourceToken: closes[index],
			SourceFractionDigits: &fractionDigits,
		}
		barsRead[index] = app.DailyBar{
			Symbol: quantFactorEndToEndSymbol, TradeDate: dates[index], Period: "1d",
			Close: 0, AdjustmentBasis: app.AdjustmentNone,
			Source: "eastmoney", SourceVersion: "eastmoney-stock-api-v1",
			FetchedAt: fetchedAt, NormalizedAt: fetchedAt, SystemObservedAt: &observedAt,
			ProviderTime: &observedAt, NormalizationVersion: "daily-bars-normalize-v1",
			QualityStatus: app.QualityValid,
			ExactValues:   &app.DailyBarExactValues{Close: closes[index], CloseEvidence: closeEvidence},
		}
	}
	calendarRef := "trading_calendar:sha256:" + strings.Repeat("7", 64)
	// Data 对该用途要求至少 90% 交易日有数据；这里刻意让它与观测根数不同，
	// 使 QualitySummary 无法用 observed_bars 冒充 minimum_required_bars。
	minimumRequiredBars := bars - 2
	if minimumRequiredBars < 1 {
		minimumRequiredBars = 1
	}
	pit := &quantFactorEndToEndPITReader{snapshot: app.DailyBarsV2Snapshot{
		Symbol: quantFactorEndToEndSymbol, Interval: "1d", AdjustmentBasis: app.AdjustmentNone,
		QualityStatus: app.QualityValid, Warnings: []string{},
		SystemObservedAtCutoff: &cutoff, TradingCalendarRef: &calendarRef,
		CoverageEvidence: app.DailyBarCoverageEvidence{
			CalendarQualityStatus: app.QualityValid,
			ExpectedTradingDays:   bars,
			MinimumRequiredBars:   minimumRequiredBars,
			CoverageBasis:         "exchange_calendar",
			CoverageSufficient:    true,
			Warnings:              []string{},
		},
		Bars: barsRead,
	}}

	policy := authorization.DataAccessPolicy{
		SchemaVersion:   authorization.DataAccessPolicySchemaVersion,
		Status:          authorization.PolicyApproved,
		AllowedSubjects: []string{"principal:service:researchflow"},
		AllowedPurposes: []string{quantstage.QuantPurposeCode},
		RequiredScopes:  []string{security.ScopeDataRead},
		EffectiveAt:     cutoff.Add(-24 * time.Hour).UTC(),
	}
	policyPayload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	policySum := sha256.Sum256(policyPayload)
	store := newQuantFactorEndToEndStore(authorization.CurrentPolicy{
		Version: authorization.PolicyVersion{
			PolicyType: authorization.DataAccessPolicyType, PolicyRef: authorization.DailyBarsV2PolicyRef,
			PolicyVersion: "policy-2026-11", Payload: policyPayload,
			PayloadHash: hex.EncodeToString(policySum[:]), CreatedAt: cutoff.Add(-24 * time.Hour).UTC(),
		},
		Policy: policy,
	})

	owner, err := marketdataapplication.NewDataAuthorizationOwner(store, store)
	if err != nil {
		t.Fatal(err)
	}
	creator, err := marketdataapplication.NewDailyBarsCloseSeriesSnapshotCreator(pit, store, owner)
	if err != nil {
		t.Fatal(err)
	}
	authorizedReader, err := marketdataapplication.NewAuthorizedDailyBarsCloseSeriesSnapshotReader(store)
	if err != nil {
		t.Fatal(err)
	}
	dataAdapter, err := newQuantDataSnapshotAdapter(&quantFactorEndToEndDataPort{
		creator: creator, reader: authorizedReader,
	})
	if err != nil {
		t.Fatal(err)
	}
	quantRegistry, err := quantregistry.NewProductionRegistry()
	if err != nil {
		t.Fatal(err)
	}
	repository := &quantFactorEndToEndRecordingRepository{inner: quantresult.NewMemoryRepository()}
	service, err := quantapplication.NewCreateFactorService(
		dataAdapter, quantRegistry, repository, func() time.Time { return now },
	)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := quantstage.NewFactorAdapter(dataAdapter, service)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := security.WithPrincipal(context.Background(), security.Principal{
		Kind: security.IdentityService, AuthMethod: security.AuthMethodServiceBearer,
		Subject: "researchflow", Scopes: []string{security.ScopeDataRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	return quantFactorEndToEndChain{
		adapter: adapter, store: store, repository: repository, context: ctx,
		request: quantstage.FactorRequest{
			FactorCode: quantregistry.ProductionMADistanceCode, Symbol: quantFactorEndToEndSymbol,
			StartDate: dates[0], EndDate: dates[bars-1], Limit: bars,
			SystemAsOf: cutoff, AllowedProviderCodes: []string{quantstage.QuantProviderCode},
		},
	}
}

func mustQuantFactorEndToEndTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatal(err)
	}
	return parsed.UTC()
}

func TestQuantFactorStageEndToEndCarriesRealDataQualityEvidence(t *testing.T) {
	chain := newQuantFactorEndToEndChain(t, 20)
	output, quality, err := chain.adapter.Execute(chain.context, chain.request)
	if err != nil {
		t.Fatalf("Factor Stage execution over a real Data Snapshot failed: %v", err)
	}
	if output.MetricStatus != "available" || output.ResultStatus != "completed" || output.SampleCount != 20 {
		t.Fatalf("unexpected Factor Stage Output status: %+v", output)
	}
	var value string
	if err := json.Unmarshal(output.Value, &value); err != nil {
		t.Fatalf("Factor value is not a decimal string: %s (%v)", output.Value, err)
	}
	// 期望值由独立十进制计算得到：(39.05 / 38.531 - 1) 四舍五入到 12 位小数。
	if value != "0.013469673769" {
		t.Fatalf("Factor value = %q, want 0.013469673769", value)
	}

	snapshot, present := chain.store.snapshots[output.SnapshotRef]
	if !present {
		t.Fatalf("Stage Output snapshot_ref %q does not resolve to the Data Snapshot created in the run", output.SnapshotRef)
	}
	evidence := snapshot.Payload.CoverageEvidence
	if quality.CalendarQualityStatus != string(evidence.CalendarQualityStatus) ||
		quality.ExpectedTradingDays != evidence.ExpectedTradingDays ||
		quality.MinimumRequiredBars != evidence.MinimumRequiredBars ||
		quality.CoverageSufficient != evidence.CoverageSufficient ||
		quality.ObservedBars != len(snapshot.Payload.Bars) ||
		quality.QualityStatus != string(snapshot.Payload.QualityStatus) {
		t.Fatalf("QualitySummary did not derive from the Data Snapshot coverage evidence: summary=%+v evidence=%+v", quality, evidence)
	}
	if output.QualityStatus != string(snapshot.Payload.QualityStatus) ||
		output.TradeDate != snapshot.Payload.EndDate ||
		output.SystemAsOf != snapshot.Manifest.SystemAsOf ||
		output.SnapshotRef != snapshot.Manifest.SnapshotRef {
		t.Fatalf("Stage Output identity drifted from the Data Snapshot manifest: output=%+v manifest=%+v", output, snapshot.Manifest)
	}
	if !strings.HasPrefix(output.ResultID, "quant_result:sha256:") {
		t.Fatalf("Stage Output must carry a Manifest-issued result_id, got %q", output.ResultID)
	}
	if len(chain.repository.records) != 1 {
		t.Fatalf("Factor execution persisted %d records, want exactly 1", len(chain.repository.records))
	}
	if persisted := chain.repository.records[0].Manifest; persisted.ResultID != output.ResultID ||
		persisted.QualityStatus != output.QualityStatus || persisted.ResultStatus != output.ResultStatus {
		t.Fatalf("Stage Output result_id/status does not match the persisted Manifest: output=%+v manifest=%+v", output, persisted)
	}
	if execution := chain.repository.records[0].Payload; len(execution) == 0 {
		t.Fatal("persisted Factor record has no Envelope payload")
	}
}

func TestQuantFactorStageEndToEndReportsInsufficientSampleFromRealSnapshot(t *testing.T) {
	chain := newQuantFactorEndToEndChain(t, 6)
	// 请求 20 根，但该日期区间只有 6 个交易日：Data 覆盖率充足（6/6），
	// 因子算法窗口不足，因此必须在 Stage 层显式降级而不是伪造数值。
	chain.request.Limit = 20
	output, quality, err := chain.adapter.Execute(chain.context, chain.request)
	if err != nil {
		t.Fatalf("insufficient-sample Factor Stage execution must not fail the Stage: %v", err)
	}
	if output.MetricStatus != "insufficient_data" || output.ResultStatus != "insufficient_data" ||
		output.SampleCount != 6 || string(output.Value) != "null" {
		t.Fatalf("insufficient sample was not reported explicitly: %+v", output)
	}
	if quality.ObservedBars != 6 || quality.ExpectedTradingDays != 6 ||
		quality.MinimumRequiredBars != 4 || !quality.CoverageSufficient {
		t.Fatalf("insufficient-sample QualitySummary lost the Data coverage facts: %+v", quality)
	}
	if len(chain.repository.records) != 1 ||
		chain.repository.records[0].Manifest.ResultStatus != "insufficient_data" {
		t.Fatalf("insufficient sample was not persisted as an auditable result: %+v", chain.repository.records)
	}
	if !strings.HasPrefix(output.ResultID, "quant_result:sha256:") {
		t.Fatalf("insufficient sample still requires a persisted result_id binding, got %q", output.ResultID)
	}
}

func TestQuantFactorStageEndToEndResultIDBindsResearchflowRunStage(t *testing.T) {
	chain := newQuantFactorEndToEndChain(t, 20)
	output, quality, err := chain.adapter.Execute(chain.context, chain.request)
	if err != nil {
		t.Fatalf("Factor Stage execution over a real Data Snapshot failed: %v", err)
	}
	payload, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	qualityPayload, err := json.Marshal(quality)
	if err != nil {
		t.Fatal(err)
	}
	runs := &quantAssociationRunReaderStub{result: quantAssociationRun(
		quantAssociationTestRunRef,
		quantAssociationStageStage(quantAssociationTestRunRef, quantstage.FactorStageCode, researchflowdomain.StageSucceeded),
	)}
	outputs := &quantAssociationOutputReaderStub{resources: map[string]researchflowapplication.StageOutputResource{
		quantstage.FactorStageCode: {Content: payload},
	}}
	verifier := quantResultAssociationVerifier{runs: runs, outputs: outputs}
	if err := verifier.VerifyResultAssociation(chain.context, quantAssociationTestRunRef, output.ResultID); err != nil {
		t.Fatalf("result_id produced end-to-end is not readable as a Run/Stage association: %v", err)
	}
	spec, supported := quantStageAssociationSpecFor(quantstage.FactorStageCode)
	if !supported {
		t.Fatal("quant.factor is missing from the Quant result association spec table")
	}
	observed, err := spec.ResultID(payload)
	if err != nil || observed != output.ResultID {
		t.Fatalf("association spec extracted result_id %q (err=%v), want %q", observed, err, output.ResultID)
	}
	if err := spec.PayloadValidator(payload); err != nil {
		t.Fatalf("association spec rejected the end-to-end payload: %v", err)
	}
	if err := spec.QualitySummaryValidator(qualityPayload); err != nil {
		t.Fatalf("association spec rejected the end-to-end quality summary: %v", err)
	}
	if err := verifier.VerifyResultAssociation(chain.context, quantAssociationTestRunRef, quantAssociationUnrelatedResultID); !errors.Is(err, quantapplication.ErrResultAssociationNotFound) {
		t.Fatalf("association verifier accepted an unrelated result_id: %v", err)
	}
}
