package contextpack

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/market"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

func BuildStockContext(bundle domain.StockBundle, technical domain.TechnicalSnapshot, news []domain.NewsItem, asOf string) domain.AnalysisContext {
	missing := make([]string, 0, 4)
	warnings := append([]string(nil), bundle.Warnings...)
	if bundle.QualityStatus != "" && bundle.QualityStatus != "valid" {
		warnings = append(warnings, "market_data_bundle_quality_"+bundle.QualityStatus)
	}
	if len(bundle.DailyBars) > 0 && bundle.Quote != nil {
		barBasis := ""
		for i := len(bundle.DailyBars) - 1; i >= 0; i-- {
			if !bundle.DailyBars[i].IsPartial {
				barBasis = bundle.DailyBars[i].AdjustmentBasis
				break
			}
		}
		quoteBasis := bundle.Quote.AdjustmentBasis
		if barBasis == "" || quoteBasis == "" || quoteBasis == "unknown" || barBasis == "unknown" {
			warnings = append(warnings, "实时行情或历史日线复权口径未知；禁止将实时价格与日线均线/支撑位直接比较")
		} else if barBasis != quoteBasis {
			warnings = append(warnings, "实时行情与历史日线复权口径不同；不得直接比较实时价和复权均线/支撑位")
		}
	}
	if bundle.Quote != nil {
		if !trustedRealtimeVolume(bundle.Quote) {
			warnings = append(warnings, "实时行情成交量单位未被可靠确认；不把成交量数值传给分析模型")
		}
		if bundle.Quote.AmountUnit != "CNY" {
			warnings = append(warnings, "实时行情成交额单位未确认为人民币元；不把成交额数值传给分析模型")
		}
	}
	if !quoteFreshAndTrustworthy(bundle.Quote) {
		missing = append(missing, "realtime_quote")
		warnings = append(warnings, "实时行情缺少可信质量状态/供应商时间，或已过期；不把其实时数值传给分析模型")
	}
	completedCount := 0
	for _, bar := range bundle.DailyBars {
		if !bar.IsPartial {
			completedCount++
		}
	}
	if completedCount == 0 {
		missing = append(missing, "completed_daily_bars")
		warnings = append(warnings, "没有可用的完整日K；不能将盘中未完成K线当作收盘数据")
	}
	if bundle.Fundamentals == nil {
		missing = append(missing, "fundamentals")
	}
	if bundle.Chip == nil {
		missing = append(missing, "chip_distribution")
	}
	if len(bundle.Boards) == 0 {
		missing = append(missing, "stock_boards")
		warnings = append(warnings, "未获得可信的个股所属板块快照")
	}
	if bundle.CapitalFlow == nil {
		missing = append(missing, "capital_flow")
		warnings = append(warnings, "未获得可用的个股资金流数据")
	}
	dailyMetricMissing := map[string]struct{}{}
	basisSeen := map[string]struct{}{}
	latestCompletedBasis := ""
	for i := len(bundle.DailyBars) - 1; i >= 0; i-- {
		if !bundle.DailyBars[i].IsPartial {
			if knownAdjustmentBasis(bundle.DailyBars[i].AdjustmentBasis) {
				latestCompletedBasis = bundle.DailyBars[i].AdjustmentBasis
			}
			break
		}
	}
	for _, bar := range bundle.DailyBars {
		if !bar.IsPartial && knownAdjustmentBasis(bar.AdjustmentBasis) {
			basisSeen[bar.AdjustmentBasis] = struct{}{}
		}
		if bar.IsPartial {
			warnings = append(warnings, "最新日线为盘中未完成K线，不可当作已收盘日线解读")
			continue
		}
		for _, field := range bar.MissingFields {
			dailyMetricMissing[field] = struct{}{}
		}
		if bar.VolumeUnit != "shares" {
			dailyMetricMissing["volume"] = struct{}{}
		}
		if bar.AmountUnit != "CNY" {
			dailyMetricMissing["amount"] = struct{}{}
		}
		if !knownAdjustmentBasis(bar.AdjustmentBasis) {
			warnings = append(warnings, "历史K线复权口径未知；相关价格序列不用于Go技术指标计算")
		}
		if latestCompletedBasis == "" || bar.AdjustmentBasis != latestCompletedBasis || !knownAdjustmentBasis(bar.AdjustmentBasis) {
			for _, field := range []string{"open", "high", "low", "close", "pct_chg"} {
				dailyMetricMissing[field] = struct{}{}
			}
		}
	}
	if len(basisSeen) > 1 {
		warnings = append(warnings, "历史K线混合复权口径；仅最新完整K线所属口径的连续价格序列可用于分析")
	}
	for _, field := range []string{"open", "high", "low", "close", "volume", "amount", "pct_chg"} {
		if _, ok := dailyMetricMissing[field]; ok {
			missing = append(missing, "daily_bars."+field)
			warnings = append(warnings, "历史K线部分记录缺少字段 "+field+"；该字段按未知处理，不按零值解释")
		}
	}
	if completedCount > 0 && completedCount < 20 {
		warnings = append(warnings, "完整历史K线不足20条，长周期指标不可用")
	}
	if len(news) == 0 {
		warnings = append(warnings, "没有获得新闻证据；不得在结论中虚构新闻来源")
	}
	asOfTime, err := time.Parse(time.RFC3339, asOf)
	if err != nil {
		asOfTime = time.Now()
	}
	phase := market.PhaseContext(inferMarket(bundle.Code), asOfTime)
	warnings = append(warnings, "交易日历状态未校验；市场阶段仅按时钟时间估算")
	warnings = uniqueStrings(warnings)
	qualityScore := 100 - len(missing)*15 - min(30, len(warnings)*5)
	if qualityScore < 0 {
		qualityScore = 0
	}
	level := "good"
	if qualityScore < 50 {
		level = "poor"
	} else if qualityScore < 80 {
		level = "partial"
	}
	return domain.AnalysisContext{
		Code: bundle.Code, Name: bundle.Name, Market: inferMarket(bundle.Code), AsOf: asOfTime,
		Quote: bundle.Quote, DailyBars: bundle.DailyBars, Technical: technical, Phase: phase,
		Chip: bundle.Chip, CapitalFlow: bundle.CapitalFlow, DragonTiger: bundle.DragonTiger,
		Fundamentals: bundle.Fundamentals, Boards: bundle.Boards,
		News: news, Warnings: warnings,
		DataQuality: domain.DataQuality{Level: level, Score: qualityScore, Missing: missing, Warnings: warnings, Sources: bundle.Sources},
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// ToLegacyAnalyzerContext translates the stable Go domain model into the current
// Python GeminiAnalyzer context shape. This translation deliberately lives at the
// bridge boundary, not inside the core workflow.
func ToLegacyAnalyzerContext(input domain.AnalysisContext) map[string]any {
	ctx := map[string]any{
		"code":                          input.Code,
		"stock_name":                    input.Name,
		"market":                        input.Market,
		"date":                          analysisDate(input),
		"data_quality":                  input.DataQuality,
		"analysis_context_pack_summary": QualitySummary(input.DataQuality),
		"analysis_warnings":             input.Warnings,
		"market_phase_context":          input.Phase,
		"daily_data":                    input.DailyBars,
		"history":                       input.DailyBars,
		"trend": map[string]any{
			"trend_status": input.Technical.TrendStatus,
			"ma_alignment": input.Technical.MAAlignment,
			"ma5":          input.Technical.MA5, "ma10": input.Technical.MA10,
			"ma20": input.Technical.MA20, "ma60": input.Technical.MA60,
			"rsi14": input.Technical.RSI14,
		},
		"technical_indicators": input.Technical,
		"fundamental_context":  input.Fundamentals,
		"belong_boards":        input.Boards,
		"capital_flow_context": input.CapitalFlow,
		"capital_flow":         input.CapitalFlow,
		"dragon_tiger_context": input.DragonTiger,
		"dragon_tiger":         input.DragonTiger,
	}
	// The legacy GeminiAnalyzer expects a `today`/`yesterday` price-table shape,
	// a separate realtime overlay, and the singular `chip` key. Completed daily
	// candles own today/yesterday; an intraday partial bar is exposed separately.
	completedBars := make([]domain.DailyBar, 0, len(input.DailyBars))
	var latestPartial *domain.DailyBar
	for i := range input.DailyBars {
		bar := input.DailyBars[i]
		if bar.IsPartial {
			copy := bar
			latestPartial = &copy
			continue
		}
		completedBars = append(completedBars, bar)
	}
	activeBasis := ""
	activeBasisStart := len(completedBars)
	if len(completedBars) > 0 && knownAdjustmentBasis(completedBars[len(completedBars)-1].AdjustmentBasis) {
		activeBasis = completedBars[len(completedBars)-1].AdjustmentBasis
		activeBasisStart = len(completedBars) - 1
		for activeBasisStart > 0 && completedBars[activeBasisStart-1].AdjustmentBasis == activeBasis {
			activeBasisStart--
		}
	}
	legacyBars := make([]map[string]any, 0, len(completedBars))
	for i, bar := range completedBars {
		expectedBasis := activeBasis
		if activeBasis == "" || i < activeBasisStart {
			expectedBasis = ""
		}
		legacyBars = append(legacyBars, legacyDailyBarForBasis(bar, expectedBasis))
	}
	// Serialize through the legacy boundary adapter rather than passing domain
	// structs directly. That adapter nulls fields marked missing; otherwise an
	// unknown-unit number would still leak via daily_data/history despite its
	// missing_fields marker.
	ctx["daily_data"] = legacyBars
	ctx["history"] = legacyBars
	if len(completedBars) > 0 {
		n := len(completedBars)
		last := completedBars[n-1]
		today := legacyDailyBarForBasis(last, activeBasis)
		putFloat(today, "ma5", input.Technical.MA5)
		putFloat(today, "ma10", input.Technical.MA10)
		putFloat(today, "ma20", input.Technical.MA20)
		putFloat(today, "ma60", input.Technical.MA60)
		ctx["today"] = today
		if n > 1 {
			ctx["yesterday"] = legacyDailyBarForBasis(completedBars[n-2], activeBasis)
		} else {
			ctx["yesterday"] = map[string]any{}
		}
		if !quoteFreshAndTrustworthy(input.Quote) {
			setLastCompletedCloseReference(ctx, last, "no trustworthy real-time quote; the latest completed close is historical, not a current price")
		}
	} else if latestPartial != nil {
		ctx["intraday_partial_bar"] = legacyDailyBarForBasis(*latestPartial, activeBasis)
		ctx["current_price_omitted_reason"] = "only an unfinished intraday candle is available"
	}
	if latestPartial != nil {
		partial := legacyDailyBarForBasis(*latestPartial, activeBasis)
		partial["is_partial"] = true
		partial["interpretation"] = "盘中未完成K线，不可视为当日收盘价"
		ctx["intraday_partial_bar"] = partial
	}
	if input.Quote != nil {
		realtime := map[string]any{
			"code": input.Quote.Code, "name": input.Quote.Name, "source": input.Quote.Source,
			"source_version": input.Quote.SourceVersion, "timestamp": input.Quote.Timestamp,
			"quote_time": input.Quote.QuoteTime, "adjustment_basis": input.Quote.AdjustmentBasis,
			"volume_unit": input.Quote.VolumeUnit, "amount_unit": input.Quote.AmountUnit,
			"quality_status": input.Quote.QualityStatus,
		}
		if !trustedRealtimeVolume(input.Quote) {
			realtime["volume_omitted_due_to_unknown_unit"] = true
		}
		// These are standalone quote facts; they do not require the real-time
		// price basis to match the historical K-line basis.
		putFloat(realtime, "volume_ratio", input.Quote.VolumeRatio)
		putFloat(realtime, "turnover_rate", input.Quote.TurnoverRate)
		if input.Quote.AmountUnit != "CNY" {
			realtime["amount_omitted_due_to_unknown_unit"] = true
		}
		if !quoteFreshAndTrustworthy(input.Quote) {
			realtime["price_omitted_due_to_quality"] = true
		} else {
			useRealtimePrice := true
			if len(completedBars) > 0 {
				last := completedBars[len(completedBars)-1]
				if !knownAdjustmentBasis(input.Quote.AdjustmentBasis) || !knownAdjustmentBasis(last.AdjustmentBasis) || input.Quote.AdjustmentBasis != last.AdjustmentBasis {
					useRealtimePrice = false
					// Preserve the quote as a factual live price, but explicitly mark it
					// non-comparable to adjusted historical bars/indicators.
					putFloat(realtime, "price", input.Quote.Price)
					putFloat(realtime, "change", input.Quote.Change)
					putFloat(realtime, "change_pct", input.Quote.ChangePct)
					realtime["price_basis_comparable_to_daily_bars"] = false
					realtime["price_omitted_from_cross_basis_comparisons"] = true
					setLastCompletedCloseReference(ctx, last, "live quote is preserved as a separate fact; do not compare it numerically to indicators computed on adjusted daily bars")
				}
			}
			if useRealtimePrice {
				putFloat(realtime, "price", input.Quote.Price)
				putFloat(realtime, "change", input.Quote.Change)
				putFloat(realtime, "change_pct", input.Quote.ChangePct)
				putFloat(realtime, "pct_chg", input.Quote.ChangePct)
				putFloat(realtime, "open", input.Quote.Open)
				putFloat(realtime, "high", input.Quote.High)
				putFloat(realtime, "low", input.Quote.Low)
				putFloat(realtime, "prev_close", input.Quote.PrevClose)
				if trustedRealtimeVolume(input.Quote) {
					putFloat(realtime, "volume", input.Quote.Volume)
				}
				if input.Quote.AmountUnit == "CNY" {
					putFloat(realtime, "amount", input.Quote.Amount)
				}
				realtime["price_basis_comparable_to_daily_bars"] = true
				ctx["current_price"] = *input.Quote.Price
				if input.Quote.ChangePct != nil {
					ctx["change_pct"] = *input.Quote.ChangePct
				}
				if input.Quote.VolumeRatio != nil {
					ctx["volume_change_ratio"] = *input.Quote.VolumeRatio
				}
			}
		}
		ctx["realtime"] = realtime
	}
	if input.Chip != nil && (input.Chip.ProfitRatio != nil || input.Chip.Concentration90 != nil || input.Chip.CostAverage != nil) {
		chip := map[string]any{}
		putFloat(chip, "profit_ratio", input.Chip.ProfitRatio)
		putFloat(chip, "concentration_90", input.Chip.Concentration90)
		putFloat(chip, "concentration_70", input.Chip.Concentration70)
		putFloat(chip, "avg_cost", input.Chip.CostAverage)
		if input.Chip.Raw != nil {
			for key, value := range input.Chip.Raw {
				if _, exists := chip[key]; !exists {
					chip[key] = value
				}
			}
		}
		ctx["chip"] = chip
	}
	maStatus := "均线数据不足"
	switch input.Technical.MAAlignment {
	case "bullish":
		maStatus = "多头排列"
	case "bearish":
		maStatus = "空头排列"
	case "mixed":
		maStatus = "均线缠绕/分歧"
	}
	ctx["ma_status"] = maStatus
	trendAnalysis := map[string]any{"trend_status": input.Technical.TrendStatus, "ma_alignment": input.Technical.MAAlignment, "signal_reasons": []string{}, "risk_factors": []string{}}
	if input.Technical.TrendStatus == "uptrend" {
		trendAnalysis["signal_reasons"] = []string{"MA5 > MA10 > MA20"}
	}
	if input.Technical.TrendStatus == "downtrend" {
		trendAnalysis["risk_factors"] = []string{"MA5 < MA10 < MA20"}
	}
	putFloat(trendAnalysis, "bias_ma5", input.Technical.BiasMA5Pct)
	ctx["trend_analysis"] = trendAnalysis
	return ctx
}

// quoteFreshAndTrustworthy is intentionally conservative at the LLM boundary.
// A numeric price is not enough: it needs an explicit accepted quality status
// and a provider timestamp. Freshness failures must not be converted into a
// seemingly authoritative current_price field.
func trustedRealtimeVolume(q *domain.Quote) bool {
	if q == nil || q.Volume == nil || q.VolumeUnit != "shares" {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(q.RawVolumeUnit)) {
	case "hands", "shares", "hands_inferred", "shares_inferred", "hands_inferred_from_turnover", "shares_inferred_from_turnover":
		return true
	default:
		// In particular, do not feed a fallback value whose unit was simply
		// assumed when Tencent's inconsistent field-6 unit couldn't be checked.
		return false
	}
}

func quoteFreshAndTrustworthy(q *domain.Quote) bool {
	if q == nil || q.Price == nil || !finitePositive(*q.Price) {
		return false
	}
	if q.QualityStatus != "valid" && q.QualityStatus != "degraded" {
		return false
	}
	if strings.TrimSpace(q.Source) == "" {
		return false
	}
	timestamp := strings.TrimSpace(q.QuoteTime)
	if timestamp == "" {
		timestamp = strings.TrimSpace(q.Timestamp)
	}
	if timestamp == "" {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		return false
	}
	for _, reason := range q.QualityReasons {
		switch reason {
		case "provider_timestamp_missing", "quote_older_than_180_seconds_during_trading_clock", "quote_older_than_24_hours_outside_trading_clock", "provider_timestamp_more_than_60_seconds_in_future":
			return false
		}
	}
	return true
}

func setLastCompletedCloseReference(ctx map[string]any, bar domain.DailyBar, reason string) {
	delete(ctx, "current_price")
	ctx["current_price_omitted_reason"] = reason
	if knownAdjustmentBasis(bar.AdjustmentBasis) && finitePositive(bar.Close) {
		ctx["last_completed_close"] = bar.Close
		ctx["last_completed_close_date"] = bar.Date
		ctx["last_completed_close_adjustment_basis"] = bar.AdjustmentBasis
		ctx["last_completed_close_semantics"] = "historical_completed_daily_close_not_realtime_price"
	} else {
		delete(ctx, "last_completed_close")
		delete(ctx, "last_completed_close_date")
		delete(ctx, "last_completed_close_adjustment_basis")
		delete(ctx, "last_completed_close_semantics")
	}
}

func knownAdjustmentBasis(basis string) bool {
	switch basis {
	case "none", "forward_adjusted", "backward_adjusted":
		return true
	default:
		return false
	}
}

func finitePositive(v float64) bool {
	return v > 0 && !math.IsNaN(v) && !math.IsInf(v, 0)
}

func legacyDailyBar(bar domain.DailyBar) map[string]any {
	return legacyDailyBarForBasis(bar, bar.AdjustmentBasis)
}

func legacyDailyBarForBasis(bar domain.DailyBar, expectedBasis string) map[string]any {
	result := map[string]any{
		"date": bar.Date, "open": bar.Open, "high": bar.High, "low": bar.Low, "close": bar.Close,
		"volume": bar.Volume, "amount": bar.Amount, "pct_chg": bar.PctChg,
	}
	missingSet := map[string]struct{}{}
	for _, field := range bar.MissingFields {
		switch field {
		case "open", "high", "low", "close", "volume", "amount", "pct_chg":
			missingSet[field] = struct{}{}
		}
	}
	priceBasisSafe := knownAdjustmentBasis(bar.AdjustmentBasis) && knownAdjustmentBasis(expectedBasis) && bar.AdjustmentBasis == expectedBasis
	if !priceBasisSafe {
		for _, field := range []string{"open", "high", "low", "close", "pct_chg"} {
			missingSet[field] = struct{}{}
			result[field] = nil
		}
		result["price_fields_omitted_reason"] = "unknown_or_mixed_adjustment_basis"
	}
	// A numeric field whose unit is unknown is not safe for a model prompt,
	// even if an older provider omitted the explicit missing_fields marker.
	if bar.VolumeUnit != "shares" {
		missingSet["volume"] = struct{}{}
	}
	if bar.AmountUnit != "CNY" {
		missingSet["amount"] = struct{}{}
	}
	missing := make([]string, 0, len(missingSet))
	for _, field := range []string{"open", "high", "low", "close", "volume", "amount", "pct_chg"} {
		if _, ok := missingSet[field]; ok {
			result[field] = nil
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		result["missing_fields"] = missing
	}
	// Carry quality provenance to the legacy LLM context. In particular, a
	// bar from the old Python fetcher must not silently lose its unknown units
	// and adjustment basis at the last serialization boundary.
	for key, value := range map[string]string{
		"source": bar.Source, "source_version": bar.SourceVersion,
		"normalization_version": bar.NormalizationVersion,
		"adjustment_basis":      bar.AdjustmentBasis, "volume_unit": bar.VolumeUnit,
		"amount_unit": bar.AmountUnit, "quality_status": bar.QualityStatus,
	} {
		if strings.TrimSpace(value) != "" {
			result[key] = value
		}
	}
	if len(bar.QualityReasons) > 0 {
		result["quality_reasons"] = append([]string(nil), bar.QualityReasons...)
	}
	if bar.IsPartial {
		result["is_partial_bar"] = true
		result["is_estimated"] = true
		result["estimated_fields"] = []string{"high", "low", "close", "volume", "amount", "pct_chg"}
	}
	return result
}

func analysisDate(input domain.AnalysisContext) string {
	if len(input.DailyBars) > 0 && strings.TrimSpace(input.DailyBars[len(input.DailyBars)-1].Date) != "" {
		return input.DailyBars[len(input.DailyBars)-1].Date
	}
	asOf := input.AsOf
	if asOf.IsZero() {
		asOf = time.Now()
	}
	if input.Market == "cn" || input.Market == "hk" || input.Market == "tw" {
		if loc, err := time.LoadLocation("Asia/Shanghai"); err == nil {
			asOf = asOf.In(loc)
		}
	}
	return asOf.Format("2006-01-02")
}

func putFloat(target map[string]any, key string, value *float64) {
	if value != nil {
		target[key] = *value
	}
}

func FormatNews(news []domain.NewsItem) string {
	if len(news) == 0 {
		return ""
	}
	var b strings.Builder
	for i, item := range news {
		fmt.Fprintf(&b, "%d. %s\n", i+1, strings.TrimSpace(item.Title))
		if item.Source != "" {
			fmt.Fprintf(&b, "来源：%s\n", item.Source)
		}
		if item.PublishedAt != "" {
			fmt.Fprintf(&b, "发布时间：%s\n", item.PublishedAt)
		}
		if item.URL != "" {
			fmt.Fprintf(&b, "链接：%s\n", item.URL)
		}
		if item.Summary != "" {
			fmt.Fprintf(&b, "%s\n", item.Summary)
		}
	}
	return b.String()
}

func QualitySummary(q domain.DataQuality) string {
	return fmt.Sprintf("数据质量：%s（%d/100）；缺失：%s；警告：%s。任何缺失字段均视为未知，不得推断为零或编造。", q.Level, q.Score, strings.Join(q.Missing, ", "), strings.Join(q.Warnings, "; "))
}

func inferMarket(code string) string {
	c := strings.ToUpper(strings.TrimSpace(code))
	if strings.HasPrefix(c, "HK") || strings.HasSuffix(c, ".HK") {
		return "hk"
	}
	if strings.HasSuffix(c, ".TWO") {
		return "tw"
	}
	if strings.HasSuffix(c, ".TW") {
		return "tw"
	}
	if strings.HasSuffix(c, ".KS") || strings.HasSuffix(c, ".KQ") {
		return "kr"
	}
	if strings.HasSuffix(c, ".T") {
		return "jp"
	}
	if strings.HasSuffix(c, ".US") {
		return "us"
	}
	for _, suffix := range []string{".SH", ".SZ", ".BJ", ".CN"} {
		if strings.HasSuffix(c, suffix) {
			c = strings.TrimSuffix(c, suffix)
			break
		}
	}
	for _, prefix := range []string{"SH", "SZ", "BJ"} {
		if strings.HasPrefix(c, prefix) {
			c = strings.TrimPrefix(c, prefix)
			break
		}
	}
	if c != "" {
		allDigits := true
		for _, r := range c {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return "cn"
		}
	}
	// Remaining letter tickers are treated as US symbols only after known
	// exchange prefixes/suffixes have been checked.
	return "us"
}
