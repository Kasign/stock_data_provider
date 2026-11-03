package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

type fakeMarketData struct {
	bundle   domain.StockBundle
	overview domain.MarketOverview
	err      error
	seenAsOf *string
}

func (f fakeMarketData) FetchStockBundle(_ context.Context, _ string, _ int, asOf string) (domain.StockBundle, error) {
	if f.seenAsOf != nil {
		*f.seenAsOf = asOf
	}
	return f.bundle, f.err
}
func (f fakeMarketData) FetchMarketOverview(context.Context, string, string) (domain.MarketOverview, error) {
	return f.overview, f.err
}

type fakeSearch struct {
	items []domain.NewsItem
	err   error
}

func (f fakeSearch) SearchStockNews(context.Context, string, string, int) ([]domain.NewsItem, error) {
	return f.items, f.err
}
func (f fakeSearch) SearchMarketNews(context.Context, string) ([]domain.NewsItem, error) {
	return f.items, f.err
}

type fakeAnalyzer struct {
	result                  domain.AnalysisResult
	report                  string
	err                     error
	stockCalls, marketCalls int
	lastStockContext        *domain.AnalysisContext
	lastStockNews           []domain.NewsItem
}

func (f *fakeAnalyzer) AnalyzeStock(_ context.Context, input domain.AnalysisContext, news []domain.NewsItem, _ string, _ string, _ string) (domain.AnalysisResult, error) {
	f.stockCalls++
	copy := input
	f.lastStockContext = &copy
	f.lastStockNews = append([]domain.NewsItem(nil), news...)
	return f.result, f.err
}
func (f *fakeAnalyzer) GenerateMarketReview(context.Context, domain.MarketOverview, []domain.NewsItem, string, string, string) (string, error) {
	f.marketCalls++
	return f.report, f.err
}

type fakePersistence struct {
	stockCalls, marketCalls int
	dailyBarsCalls          int
	persistedBars           []domain.DailyBar
	err                     error
}

func (f *fakePersistence) SaveDailyBars(_ context.Context, _ string, bars []domain.DailyBar, _ string) (int, error) {
	f.dailyBarsCalls++
	f.persistedBars = append([]domain.DailyBar(nil), bars...)
	return len(bars), f.err
}
func (f *fakePersistence) SaveStockAnalysis(context.Context, domain.AnalysisResult, domain.AnalysisContext, string, string) (int64, error) {
	f.stockCalls++
	return 12, f.err
}
func (f *fakePersistence) SaveMarketReview(context.Context, string, string, string, domain.MarketOverview) (int64, error) {
	f.marketCalls++
	return 13, f.err
}

type fakeNotifier struct {
	stockCalls, marketCalls int
	err                     error
}

func (f *fakeNotifier) NotifyStockAnalysis(context.Context, domain.AnalysisResult, string) error {
	f.stockCalls++
	return f.err
}
func (f *fakeNotifier) NotifyMarketReview(context.Context, string, string, string) error {
	f.marketCalls++
	return f.err
}

func TestStockFlowCompletesAndPersists(t *testing.T) {
	price := 12.5
	quote := &domain.Quote{Code: "600396", Price: &price}
	bars := make([]domain.DailyBar, 30)
	for i := range bars {
		bars[i] = domain.DailyBar{Date: time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), Close: float64(i + 1)}
	}
	data := fakeMarketData{bundle: domain.StockBundle{Code: "600396", Name: "测试股票", Quote: quote, DailyBars: bars}}
	analyzer := &fakeAnalyzer{result: domain.AnalysisResult{Success: true, OperationAdvice: "观望", AnalysisSummary: "fixture report"}}
	persistence := &fakePersistence{}
	notifier := &fakeNotifier{}
	flow := NewStockFlow(data, fakeSearch{items: []domain.NewsItem{{Title: "公告"}}}, analyzer, persistence, notifier)
	got, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396", QueryID: "test-query", SearchNews: boolPointer(true), Notify: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.Code != "600396" || got.Result.Name != "测试股票" || got.Result.QueryID != "test-query" {
		t.Fatalf("unexpected result: %#v", got.Result)
	}
	if analyzer.stockCalls != 1 || persistence.stockCalls != 1 || notifier.stockCalls != 1 {
		t.Fatalf("calls analyzer=%d persistence=%d notifier=%d", analyzer.stockCalls, persistence.stockCalls, notifier.stockCalls)
	}
	if len(got.Trace.Stages) < 5 {
		t.Fatalf("expected stage trace, got %#v", got.Trace.Stages)
	}
}

func TestStockFlowDoesNotPersistPartialIntradayBar(t *testing.T) {
	price := 10.5
	completed := domain.DailyBar{Date: "2026-10-19", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 100, Amount: 1000, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: "none", QualityStatus: "valid"}
	partial := domain.DailyBar{Date: "2026-10-20", Open: 10.5, High: 11, Low: 10.2, Close: 10.8, Volume: 25, Amount: 250, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: "none", QualityStatus: "degraded", IsPartial: true}
	persistence := &fakePersistence{}
	flow := NewStockFlow(
		fakeMarketData{bundle: domain.StockBundle{Code: "600396", Quote: &domain.Quote{Code: "600396", Price: &price}, DailyBars: []domain.DailyBar{completed, partial}}},
		nil,
		&fakeAnalyzer{result: domain.AnalysisResult{Success: true}},
		persistence,
		nil,
	)
	got, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396", SearchNews: boolPointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if len(persistence.persistedBars) != 1 || persistence.persistedBars[0].Date != completed.Date {
		t.Fatalf("partial candle leaked into completed-history persistence: %#v", persistence.persistedBars)
	}
	if len(got.Context.DailyBars) != 2 || !got.Context.DailyBars[1].IsPartial {
		t.Fatalf("partial candle should remain explicitly marked in analysis context: %#v", got.Context.DailyBars)
	}
	foundWarning := false
	for _, warning := range got.Warnings {
		if strings.Contains(warning, "excluded 1 intraday partial daily bar") {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Fatalf("expected partial-candle persistence warning, got %#v", got.Warnings)
	}
}

func TestStockFlowFailsWhenNoUsableMarketData(t *testing.T) {
	flow := NewStockFlow(fakeMarketData{bundle: domain.StockBundle{Code: "600396"}}, fakeSearch{}, &fakeAnalyzer{}, nil, nil)
	if _, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396"}); err == nil {
		t.Fatal("expected no-data error")
	}
}

func TestStockFlowDoesNotHideFetchFailure(t *testing.T) {
	flow := NewStockFlow(fakeMarketData{err: errors.New("provider unavailable")}, fakeSearch{}, &fakeAnalyzer{}, nil, nil)
	if _, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396"}); err == nil {
		t.Fatal("expected fetch error")
	}
}

func TestMarketReviewFlowCompletes(t *testing.T) {
	analyzer := &fakeAnalyzer{report: "market report"}
	persistence := &fakePersistence{}
	notifier := &fakeNotifier{}
	flow := NewMarketReviewFlow(fakeMarketData{overview: domain.MarketOverview{Date: "2026-10-20", Indices: []domain.MarketIndex{{Code: "000001", Name: "上证指数", Current: 3200}}}}, fakeSearch{}, analyzer, persistence, notifier)
	got, err := flow.Run(context.Background(), domain.MarketReviewRequest{Region: "cn", QueryID: "market-test", Notify: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.Report != "market report" || got.Result.SavedHistoryID != 13 {
		t.Fatalf("unexpected result %#v", got.Result)
	}
	if analyzer.marketCalls != 1 || persistence.marketCalls != 1 || notifier.marketCalls != 1 {
		t.Fatal("market workflow did not call all configured collaborators")
	}
}

func TestMarketReviewRejectsUnsupportedRegion(t *testing.T) {
	flow := NewMarketReviewFlow(fakeMarketData{}, fakeSearch{}, &fakeAnalyzer{}, nil, nil)
	if _, err := flow.Run(context.Background(), domain.MarketReviewRequest{Region: "moon"}); err == nil {
		t.Fatal("expected unsupported-region error")
	}
}

func boolPointer(value bool) *bool { return &value }

func TestMarketReviewSkipsNewsWhenSearchAdapterIsNil(t *testing.T) {
	analyzer := &fakeAnalyzer{report: "market report"}
	flow := NewMarketReviewFlow(fakeMarketData{overview: domain.MarketOverview{Date: "2026-10-20", Indices: []domain.MarketIndex{{Code: "000001", Name: "上证指数", Current: 3200}}}}, nil, analyzer, nil, nil)
	got, err := flow.Run(context.Background(), domain.MarketReviewRequest{Region: "cn"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.Report != "market report" {
		t.Fatalf("unexpected report: %q", got.Result.Report)
	}
	foundSkipped := false
	for _, stage := range got.Trace.Stages {
		if stage.Name == "search_market_news" && stage.Status == "skipped" {
			foundSkipped = true
		}
	}
	if !foundSkipped {
		t.Fatalf("expected skipped news stage, got %#v", got.Trace.Stages)
	}
}

func TestStockFlowDefaultsNewsSearchToEnabled(t *testing.T) {
	price := 12.5
	bars := make([]domain.DailyBar, 25)
	for i := range bars {
		bars[i] = domain.DailyBar{Date: time.Date(2026, 1, 1+i, 0, 0, 0, 0, time.UTC).Format("2006-01-02"), Close: float64(i + 1)}
	}
	search := fakeSearch{}
	flow := NewStockFlow(fakeMarketData{bundle: domain.StockBundle{Code: "600396", Name: "测试股票", Quote: &domain.Quote{Code: "600396", Price: &price}, DailyBars: bars}}, search, &fakeAnalyzer{result: domain.AnalysisResult{Success: true}}, nil, nil)
	got, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396"})
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range got.Trace.Stages {
		if stage.Name == "search_stock_news" {
			if stage.Status != "succeeded" {
				t.Fatalf("news search should default on, got stage %#v", stage)
			}
			return
		}
	}
	t.Fatal("search stage missing from trace")
}

func TestStockFlowDoesNotNotifyUnsuccessfulAnalysis(t *testing.T) {
	price := 10.0
	bars := []domain.DailyBar{{Date: "2026-10-19", Open: 9, High: 11, Low: 8, Close: 10, Volume: 100, Amount: 1000}}
	analyzer := &fakeAnalyzer{result: domain.AnalysisResult{Success: false, ErrorMessage: "model response invalid"}}
	notifier := &fakeNotifier{}
	flow := NewStockFlow(fakeMarketData{bundle: domain.StockBundle{Code: "600396", Name: "测试股票", Quote: &domain.Quote{Code: "600396", Price: &price}, DailyBars: bars}}, fakeSearch{}, analyzer, nil, notifier)
	got, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396", Notify: true, SearchNews: boolPointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if notifier.stockCalls != 0 {
		t.Fatalf("failed analysis must not be notified, got %d notifications", notifier.stockCalls)
	}
	for _, stage := range got.Trace.Stages {
		if stage.Name == "notify_stock_analysis" && (stage.Status != "skipped" || stage.Message != "skipped because analysis result is unsuccessful") {
			t.Fatalf("expected explicit notification skip, got %#v", stage)
		}
	}
}

func TestStockFlowPreservesAnalyzerContextSnapshotFields(t *testing.T) {
	price := 10.0
	bars := []domain.DailyBar{{Date: "2026-10-19", Open: 9, High: 11, Low: 8, Close: 10, Volume: 100, Amount: 1000}}
	analyzer := &fakeAnalyzer{result: domain.AnalysisResult{Success: true, ContextSnapshot: map[string]any{"legacy_key": "keep-me"}}}
	flow := NewStockFlow(fakeMarketData{bundle: domain.StockBundle{Code: "600396", Name: "测试股票", Quote: &domain.Quote{Code: "600396", Price: &price}, DailyBars: bars}}, nil, analyzer, nil, nil)
	got, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396", SearchNews: boolPointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if got.Result.ContextSnapshot["legacy_key"] != "keep-me" {
		t.Fatalf("existing context snapshot field was lost: %#v", got.Result.ContextSnapshot)
	}
	if _, ok := got.Result.ContextSnapshot["data_quality"]; !ok {
		t.Fatalf("Go quality metadata missing: %#v", got.Result.ContextSnapshot)
	}
}

func TestMarketReviewRejectsEmptyOverviewInsteadOfCallingLLM(t *testing.T) {
	analyzer := &fakeAnalyzer{report: "should not be used"}
	flow := NewMarketReviewFlow(fakeMarketData{overview: domain.MarketOverview{Date: "2026-10-20"}}, nil, analyzer, nil, nil)
	if _, err := flow.Run(context.Background(), domain.MarketReviewRequest{Region: "cn"}); err == nil {
		t.Fatal("expected empty overview to stop market review")
	}
	if analyzer.marketCalls != 0 {
		t.Fatalf("LLM must not be called with empty market data, got %d calls", analyzer.marketCalls)
	}
}

func TestMarketReviewRejectsHistoricalAsOfForLiveOnlyAdapter(t *testing.T) {
	analyzer := &fakeAnalyzer{report: "must not be generated"}
	flow := NewMarketReviewFlow(fakeMarketData{overview: domain.MarketOverview{Date: "2026-10-20", Indices: []domain.MarketIndex{{Code: "000001", Name: "上证指数", Current: 3200}}}}, nil, analyzer, nil, nil)
	_, err := flow.Run(context.Background(), domain.MarketReviewRequest{Region: "cn", AsOf: time.Now().Add(-24 * time.Hour)})
	if err == nil {
		t.Fatal("current Python market adapter must not pretend to support historical snapshots")
	}
	if analyzer.marketCalls != 0 {
		t.Fatalf("LLM must not analyze live data for a historical as_of request; calls=%d", analyzer.marketCalls)
	}
}

func TestStockFlowRejectsFutureAsOf(t *testing.T) {
	analyzer := &fakeAnalyzer{result: domain.AnalysisResult{Success: true}}
	flow := NewStockFlow(fakeMarketData{bundle: domain.StockBundle{Code: "600396"}}, nil, analyzer, nil, nil)
	_, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396", AsOf: time.Now().Add(5 * time.Second)})
	if err == nil {
		t.Fatal("future as_of should be rejected")
	}
	if analyzer.stockCalls != 0 {
		t.Fatalf("LLM must not run for a future as_of request; calls=%d", analyzer.stockCalls)
	}
}

func TestCompletedBarsForIndicatorsExcludesPartialCandles(t *testing.T) {
	bars := []domain.DailyBar{
		{Date: "2026-10-19", Close: 10, AdjustmentBasis: "none", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
		{Date: "2026-10-20", Close: 11, IsPartial: true, AdjustmentBasis: "none", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
	}
	got := completedBarsForIndicators(bars)
	if len(got) != 1 || got[0].Date != "2026-10-19" {
		t.Fatalf("partial daily candle must not affect technical indicators: %#v", got)
	}
}

func TestStockFlowDoesNotPersistBarsWithUnknownUnitsOrAdjustment(t *testing.T) {
	price := 10.5
	bar := domain.DailyBar{Date: "2026-10-19", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: 100, Amount: 1000, VolumeUnit: "unknown", AmountUnit: "unknown", AdjustmentBasis: "unknown", QualityStatus: "unknown", MissingFields: []string{"volume", "amount"}}
	persistence := &fakePersistence{}
	flow := NewStockFlow(
		fakeMarketData{bundle: domain.StockBundle{Code: "600396", Quote: &domain.Quote{Code: "600396", Price: &price}, DailyBars: []domain.DailyBar{bar}}},
		nil,
		&fakeAnalyzer{result: domain.AnalysisResult{Success: true}},
		persistence,
		nil,
	)
	got, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600396", SearchNews: boolPointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if persistence.dailyBarsCalls != 0 || len(persistence.persistedBars) != 0 {
		t.Fatalf("unverified bar unexpectedly persisted: %#v", persistence.persistedBars)
	}
	found := false
	for _, warning := range got.Warnings {
		if strings.Contains(warning, "unverified units, adjustment basis, quality, or required fields") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected data-contract warning, got %#v", got.Warnings)
	}
}

func TestMarkQuoteAfterAnalysisCutoffRejectsFutureQuote(t *testing.T) {
	cutoff := time.Date(2026, 10, 21, 3, 30, 0, 0, time.UTC)
	price := 1313.31
	quote := &domain.Quote{Code: "600519.SH", Price: &price, QuoteTime: "2026-10-21T03:30:01Z", QualityStatus: "valid"}
	if !markQuoteAfterAnalysisCutoff(quote, cutoff) {
		t.Fatal("expected a quote timestamp beyond the requested cutoff to be rejected")
	}
	if quote.QualityStatus != "unknown" || !containsField(quote.QualityReasons, "quote_timestamp_after_analysis_cutoff") {
		t.Fatalf("quote was not marked unusable: %+v", quote)
	}
}

func TestCompletedBarsForIndicatorsRequiresOneKnownAdjustmentBasis(t *testing.T) {
	bars := []domain.DailyBar{
		{Date: "2026-10-15", Close: 10, AdjustmentBasis: "forward_adjusted", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
		{Date: "2026-10-16", Close: 11, AdjustmentBasis: "none", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
		{Date: "2026-10-19", Close: 12, AdjustmentBasis: "unknown", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
		{Date: "2026-10-20", Close: 13, AdjustmentBasis: "forward_adjusted", IsPartial: true, Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
	}
	got := completedBarsForIndicators(bars)
	if len(got) != 0 {
		t.Fatalf("latest completed candle has unknown adjustment basis, so older indicators must not masquerade as current; got %+v", got)
	}
	consistent := []domain.DailyBar{
		{Date: "2026-10-15", Close: 10, AdjustmentBasis: "none", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
		{Date: "2026-10-16", Close: 11, AdjustmentBasis: "none", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
		{Date: "2026-10-19", Close: 12, AdjustmentBasis: "none", IsPartial: true, Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"},
	}
	got = completedBarsForIndicators(consistent)
	if len(got) != 2 || got[1].Date != "2026-10-16" {
		t.Fatalf("indicator series should keep the contiguous complete adjustment-consistent suffix: %+v", got)
	}
	if got := completedBarsForIndicators([]domain.DailyBar{{Date: "2026-10-19", Close: 12, AdjustmentBasis: "unknown", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"}}); len(got) != 0 {
		t.Fatalf("unknown-basis history must not enter technical indicators: %+v", got)
	}
}

func TestCompletedBarsForIndicatorsRejectsStaleUnknownAndMixedSourceSeries(t *testing.T) {
	base := func(date string, close float64) domain.DailyBar {
		return domain.DailyBar{Date: date, Close: close, AdjustmentBasis: "forward_adjusted", Source: "eastmoney", NormalizationVersion: "3", QualityStatus: "valid"}
	}
	stale := base("2026-10-20", 12)
	stale.QualityStatus = "stale"
	if got := completedBarsForIndicators([]domain.DailyBar{stale}); len(got) != 0 {
		t.Fatalf("stale history must not produce current indicators: %+v", got)
	}
	unknown := base("2026-10-20", 12)
	unknown.QualityStatus = "unknown"
	if got := completedBarsForIndicators([]domain.DailyBar{unknown}); len(got) != 0 {
		t.Fatalf("unknown-quality history must not produce indicators: %+v", got)
	}
	first := base("2026-10-19", 11)
	second := base("2026-10-20", 12)
	second.Source = "tencent"
	got := completedBarsForIndicators([]domain.DailyBar{first, second})
	if len(got) != 1 || got[0].Source != "tencent" {
		t.Fatalf("indicator series must use a coherent single-source suffix: %+v", got)
	}
	second = base("2026-10-20", 12)
	second.AdjustmentReferenceDate = "2026-10-20"
	got = completedBarsForIndicators([]domain.DailyBar{first, second})
	if len(got) != 1 {
		t.Fatalf("different adjustment references must not be combined: %+v", got)
	}
}

func TestStockFlowLiveModeUsesCompletedBundleReferenceInsteadOfStartCutoff(t *testing.T) {
	started := time.Date(2026, 10, 21, 3, 10, 0, 0, time.UTC)
	quoteTime := started.Add(time.Second)
	reference := started.Add(2 * time.Second)
	price := 1313.31
	quote := &domain.Quote{
		Code: "600519.SH", Price: &price, Source: "eastmoney",
		QuoteTime: quoteTime.Format(time.RFC3339Nano), ProviderTimestamp: &quoteTime,
		FetchedAt: reference, QualityStatus: "valid", AdjustmentBasis: "none",
		VolumeUnit: "shares", AmountUnit: "CNY",
	}
	seenAsOf := "unset"
	data := fakeMarketData{
		bundle:   domain.StockBundle{Code: "600519.SH", Quote: quote, FetchedAt: reference, AnalysisReferenceTime: reference},
		seenAsOf: &seenAsOf,
	}
	analyzer := &fakeAnalyzer{result: domain.AnalysisResult{Success: true}}
	flow := NewStockFlow(data, nil, analyzer, nil, nil)
	flow.now = func() time.Time { return started }
	got, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600519.SH", SearchNews: boolPointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if seenAsOf != "" {
		t.Fatalf("live request must not be sent as strict as_of=%q", seenAsOf)
	}
	if analyzer.lastStockContext == nil || !analyzer.lastStockContext.AsOf.Equal(reference) {
		t.Fatalf("analysis must use completed bundle reference time %s, got %#v", reference, analyzer.lastStockContext)
	}
	if analyzer.lastStockContext.Quote == nil || analyzer.lastStockContext.Quote.Price == nil || *analyzer.lastStockContext.Quote.Price != price {
		t.Fatalf("valid quote later than workflow start but before snapshot completion must be retained, got %#v", analyzer.lastStockContext.Quote)
	}
	if got.Result.ContextSnapshot["as_of_explicit"] != false {
		t.Fatalf("snapshot must record that caller did not explicitly set as_of: %#v", got.Result.ContextSnapshot)
	}
}

func TestStockFlowLiveModeDegradesClockSkewInsteadOfApplyingHistoricalCutoff(t *testing.T) {
	started := time.Date(2026, 10, 21, 3, 10, 0, 0, time.UTC)
	reference := started.Add(2 * time.Second)
	quoteTime := reference.Add(time.Second) // small provider/local clock skew
	price := 1313.31
	quote := &domain.Quote{
		Code: "600519.SH", Price: &price, Source: "eastmoney",
		QuoteTime: quoteTime.Format(time.RFC3339Nano), ProviderTimestamp: &quoteTime,
		FetchedAt: reference, QualityStatus: "valid", AdjustmentBasis: "none",
		VolumeUnit: "shares", AmountUnit: "CNY",
	}
	data := fakeMarketData{bundle: domain.StockBundle{Code: "600519.SH", Quote: quote, FetchedAt: reference, AnalysisReferenceTime: reference}}
	analyzer := &fakeAnalyzer{result: domain.AnalysisResult{Success: true}}
	flow := NewStockFlow(data, nil, analyzer, nil, nil)
	flow.now = func() time.Time { return started }
	got, err := flow.Run(context.Background(), domain.AnalysisRequest{StockCode: "600519.SH", SearchNews: boolPointer(false)})
	if err != nil {
		t.Fatal(err)
	}
	if analyzer.lastStockContext == nil || analyzer.lastStockContext.Quote == nil {
		t.Fatal("live quote should remain available as a degraded observation")
	}
	if analyzer.lastStockContext.Quote.QualityStatus != "degraded" {
		t.Fatalf("clock-skewed live quote should be degraded rather than rejected: %+v", analyzer.lastStockContext.Quote)
	}
	if !containsField(analyzer.lastStockContext.Quote.QualityReasons, "provider_timestamp_after_live_snapshot_reference") {
		t.Fatalf("expected explicit clock-skew quality reason: %+v", analyzer.lastStockContext.Quote.QualityReasons)
	}
	for _, warning := range got.Warnings {
		if strings.Contains(warning, "quote values were excluded") {
			t.Fatalf("live quote was incorrectly processed by strict historical cutoff: %#v", got.Warnings)
		}
	}
}
