package service

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const (
	mainBoardRiskWarningChangeDate = "2026-07-06"
	chiNextTwentyPercentDate       = "2020-08-24"
	mainBoardIPOReformDate         = "2023-04-10"
	starMarketStartDate            = "2019-07-22"
	beijingExchangeStartDate       = "2021-11-15"
)

const (
	sseTradingRuleURL        = "https://www.sse.com.cn/lawandrules/sselawsrules2025/stocks/exchange/c/c_20260424_10816482.shtml"
	szseTradingRuleURL       = "https://docs.static.szse.cn/www/lawrules/rule/trade/current/W020260424690713155663.pdf"
	szseRiskWarningChangeURL = "https://www.szse.cn/aboutus/trends/news/t20250627_614643.html"
	beijingTradingRuleURL    = "https://www.bse.cn/qt/200025234.html"
)

// ResolvePriceLimitRule resolves the effective percentage rule for an A-share symbol and trade date.
// Risk-warning, listing-window and special no-limit inputs must be sourced independently for that
// exact date. Missing inputs produce QualityUnknown instead of silently assuming a normal stock.
func ResolvePriceLimitRule(
	rawSymbol, tradeDate string,
	riskStatus model.RiskWarningStatus,
	listingTradingDayNumber int,
	listingStatusKnown bool,
	specialException model.PriceLimitException,
) (*model.PriceLimitRuleSnapshot, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	parsedDate, err := time.Parse("2006-01-02", tradeDate)
	if err != nil || parsedDate.Format("2006-01-02") != tradeDate {
		return nil, fmt.Errorf("invalid trade date %q", tradeDate)
	}
	date := parsedDate.Format("2006-01-02")
	if riskStatus == "" {
		riskStatus = model.RiskWarningUnknown
	}
	switch riskStatus {
	case model.RiskWarningUnknown, model.RiskWarningNormal, model.RiskWarningApplied:
	default:
		return nil, fmt.Errorf("unsupported risk-warning status %q", riskStatus)
	}
	if specialException == "" {
		specialException = model.PriceLimitExceptionUnknown
	}
	switch specialException {
	case model.PriceLimitExceptionUnknown, model.PriceLimitExceptionNone,
		model.PriceLimitExceptionRelistingFirst, model.PriceLimitExceptionDelistingFirst,
		model.PriceLimitExceptionExchangeSpecial:
	default:
		return nil, fmt.Errorf("unsupported no-limit exception status %q", specialException)
	}
	if listingStatusKnown && listingTradingDayNumber < 1 {
		return nil, errors.New("listing trading-day number must be positive when listing status is known")
	}

	market, board, boardStart, ruleURL, err := identifyAshareBoard(symbol)
	if err != nil {
		return nil, err
	}
	if date < boardStart {
		return nil, fmt.Errorf("trade date %s predates %s board start %s", date, board, boardStart)
	}
	result := &model.PriceLimitRuleSnapshot{
		Symbol: symbol, Market: market, Board: board, TradeDate: date,
		RiskWarningStatus: riskStatus, ListingTradingDayNumber: listingTradingDayNumber,
		ListingStatusKnown: listingStatusKnown, SpecialExceptionStatus: specialException,
		RuleSourceURL: ruleURL, RuleSource: market + " official trading rules",
		Status: model.PriceLimitStatusUnknown, QualityStatus: model.QualityUnknown,
		QualityReasons: []string{},
	}

	if specialException != model.PriceLimitExceptionUnknown &&
		specialException != model.PriceLimitExceptionNone {
		result.Status = model.PriceLimitStatusNoLimit
		result.NoLimitReason = string(specialException)
		result.RuleVersion = board + "-explicit-no-limit-event-v1"
		result.QualityStatus = model.QualityValid
		return result, nil
	}
	if specialException == model.PriceLimitExceptionUnknown {
		appendReasonOnce(&result.QualityReasons, "price_limit_special_exception_status_unverified")
	}
	if !listingStatusKnown {
		appendReasonOnce(&result.QualityReasons, "listing_trading_day_number_unverified_ipo_limit_exception_not_evaluated")
	} else if specialException == model.PriceLimitExceptionNone {
		if threshold, version, effectiveFrom, effectiveTo := ipoNoLimitWindow(board, date); listingTradingDayNumber <= threshold {
			result.Status = model.PriceLimitStatusNoLimit
			result.NoLimitReason = "ipo_listing_day_without_price_limit"
			result.RuleVersion = version
			result.RuleEffectiveFrom = effectiveFrom
			result.RuleEffectiveTo = effectiveTo
			result.QualityStatus = model.QualityValid
			return result, nil
		}
	}

	percent, version, effectiveFrom, effectiveTo, riskUnknown := boardPriceLimit(board, date, riskStatus)
	result.RuleVersion = version
	result.RuleEffectiveFrom = effectiveFrom
	result.RuleEffectiveTo = effectiveTo
	if riskUnknown {
		appendReasonOnce(&result.QualityReasons, "risk_warning_status_required_for_historical_mainboard_price_limit")
	}
	if specialException == model.PriceLimitExceptionUnknown || !listingStatusKnown || riskUnknown {
		// The nominal percentage is deliberately not emitted as usable when an unverified
		// exception or listing window may supersede it.
		return result, nil
	}
	result.Status = model.PriceLimitStatusLimited
	result.LimitPercent = &percent
	result.QualityStatus = model.QualityValid
	return result, nil
}

// ResolvePriceLimitRuleWithEvidence is the source-validated entry point for special-event
// rules. ResolvePriceLimitRule remains a pure policy helper and must not be fed guessed events.
func ResolvePriceLimitRuleWithEvidence(
	rawSymbol, tradeDate string,
	riskStatus model.RiskWarningStatus,
	listingTradingDayNumber int,
	listingStatusKnown bool,
	evidence *model.PriceLimitEventEvidence,
) (*model.PriceLimitRuleSnapshot, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	parsedDate, err := time.Parse("2006-01-02", tradeDate)
	if err != nil || parsedDate.Format("2006-01-02") != tradeDate {
		return nil, fmt.Errorf("invalid trade date %q", tradeDate)
	}

	status := model.PriceLimitExceptionUnknown
	incompleteReason := ""
	if evidence != nil {
		evidenceSymbol, symbolErr := normalize.NormalizeSymbol(evidence.Symbol)
		if symbolErr != nil || evidenceSymbol != symbol {
			return nil, fmt.Errorf("price-limit event evidence symbol does not match request %q", symbol)
		}
		switch evidence.Status {
		case model.PriceLimitExceptionUnknown, model.PriceLimitExceptionNone,
			model.PriceLimitExceptionRelistingFirst, model.PriceLimitExceptionDelistingFirst,
			model.PriceLimitExceptionExchangeSpecial:
		default:
			return nil, fmt.Errorf("unsupported event evidence status %q", evidence.Status)
		}
		sourceURL, urlErr := url.Parse(strings.TrimSpace(evidence.SourceURL))
		if strings.TrimSpace(evidence.Source) == "" || urlErr != nil ||
			sourceURL.Host == "" || (sourceURL.Scheme != "http" && sourceURL.Scheme != "https") ||
			evidence.ObservedAt.IsZero() {
			incompleteReason = "price_limit_event_evidence_source_or_observation_missing"
		} else if evidence.Status == model.PriceLimitExceptionNone {
			if !evidence.CoverageConfirmed {
				incompleteReason = "price_limit_no_event_not_confirmed_by_complete_source"
			} else if !priceLimitEvidenceCoversDate(evidence, parsedDate) {
				incompleteReason = "price_limit_no_event_evidence_coverage_range_missing_or_out_of_range"
			} else {
				status = model.PriceLimitExceptionNone
			}
		} else if evidence.Status == model.PriceLimitExceptionUnknown {
			incompleteReason = "price_limit_special_exception_status_unverified"
		} else {
			if !priceLimitEvidenceCoversDate(evidence, parsedDate) ||
				strings.TrimSpace(evidence.AnnouncementID) == "" {
				incompleteReason = "price_limit_special_event_evidence_incomplete_or_out_of_range"
			} else {
				status = evidence.Status
			}
		}
	} else {
		incompleteReason = "price_limit_event_evidence_missing"
	}

	result, err := ResolvePriceLimitRule(symbol, tradeDate, riskStatus, listingTradingDayNumber, listingStatusKnown, status)
	if err != nil {
		return nil, err
	}
	result.SpecialExceptionEvidence = evidence
	if incompleteReason != "" {
		appendReasonOnce(&result.QualityReasons, incompleteReason)
		result.Status = model.PriceLimitStatusUnknown
		result.LimitPercent = nil
		result.NoLimitReason = ""
		result.QualityStatus = model.QualityUnknown
	}
	return result, nil
}

func priceLimitEvidenceCoversDate(evidence *model.PriceLimitEventEvidence, date time.Time) bool {
	from, err := time.Parse("2006-01-02", evidence.EffectiveFrom)
	if err != nil || from.Format("2006-01-02") != evidence.EffectiveFrom {
		return false
	}
	to := from
	if evidence.EffectiveTo != "" {
		to, err = time.Parse("2006-01-02", evidence.EffectiveTo)
		if err != nil || to.Format("2006-01-02") != evidence.EffectiveTo {
			return false
		}
	}
	return !to.Before(from) && !date.Before(from) && !date.After(to)
}

func identifyAshareBoard(symbol string) (market, board, boardStart, ruleURL string, err error) {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 {
		return "", "", "", "", fmt.Errorf("symbol %q has no canonical exchange suffix", symbol)
	}
	code, exchange := parts[0], parts[1]
	switch exchange {
	case "SH":
		switch {
		case strings.HasPrefix(code, "688"), strings.HasPrefix(code, "689"):
			return "sse", "star_market", starMarketStartDate, sseTradingRuleURL, nil
		case strings.HasPrefix(code, "600"), strings.HasPrefix(code, "601"),
			strings.HasPrefix(code, "603"), strings.HasPrefix(code, "605"):
			return "sse", "main_board", "1990-12-19", sseTradingRuleURL, nil
		default:
			return "", "", "", "", fmt.Errorf("symbol %s is not a supported Shanghai A-share stock board", symbol)
		}
	case "SZ":
		switch {
		case strings.HasPrefix(code, "300"), strings.HasPrefix(code, "301"):
			return "szse", "chinext", "2009-10-30", szseTradingRuleURL, nil
		case strings.HasPrefix(code, "000"), strings.HasPrefix(code, "001"),
			strings.HasPrefix(code, "002"), strings.HasPrefix(code, "003"):
			return "szse", "main_board", "1991-04-03", szseTradingRuleURL, nil
		default:
			return "", "", "", "", fmt.Errorf("symbol %s is not a supported Shenzhen A-share stock board", symbol)
		}
	case "BJ":
		return "bse", "beijing_exchange", beijingExchangeStartDate, beijingTradingRuleURL, nil
	default:
		return "", "", "", "", fmt.Errorf("symbol %s is not a supported mainland A-share exchange", symbol)
	}
}

func boardPriceLimit(board, tradeDate string, riskStatus model.RiskWarningStatus) (percent float64, version, effectiveFrom, effectiveTo string, riskUnknown bool) {
	switch board {
	case "main_board":
		if riskStatus == model.RiskWarningApplied {
			if tradeDate < mainBoardRiskWarningChangeDate {
				return 5, "cn-mainboard-risk-warning-5-percent-before-2026-07-06", "", "2026-07-05", false
			}
			return 10, "cn-mainboard-risk-warning-10-percent-2026-07-06", mainBoardRiskWarningChangeDate, "", false
		}
		if riskStatus == model.RiskWarningUnknown && tradeDate < mainBoardRiskWarningChangeDate {
			return 0, "cn-mainboard-risk-warning-status-required-before-2026-07-06", "", "2026-07-05", true
		}
		if tradeDate >= mainBoardRiskWarningChangeDate {
			return 10, "cn-mainboard-10-percent-current-v2", mainBoardRiskWarningChangeDate, "", false
		}
		return 10, "cn-mainboard-10-percent-v1", "", "", false
	case "chinext":
		if tradeDate >= chiNextTwentyPercentDate {
			return 20, "cn-chinext-20-percent-2020-08-24", chiNextTwentyPercentDate, "", false
		}
		return 10, "cn-chinext-10-percent-before-2020-08-24", "", "2020-08-23", false
	case "star_market":
		return 20, "cn-star-market-20-percent-v1", starMarketStartDate, "", false
	case "beijing_exchange":
		return 30, "cn-beijing-exchange-30-percent-v1", beijingExchangeStartDate, "", false
	default:
		return 0, "unknown", "", "", true
	}
}

func ipoNoLimitWindow(board, tradeDate string) (threshold int, version, effectiveFrom, effectiveTo string) {
	switch board {
	case "main_board":
		if tradeDate >= mainBoardIPOReformDate {
			return 5, "cn-mainboard-ipo-first-five-trading-days-no-limit-2023-04-10", mainBoardIPOReformDate, ""
		}
		return 1, "cn-mainboard-ipo-first-day-no-limit-before-2023-04-10", "", "2023-04-09"
	case "chinext":
		if tradeDate >= chiNextTwentyPercentDate {
			return 5, "cn-chinext-ipo-first-five-trading-days-no-limit-2020-08-24", chiNextTwentyPercentDate, ""
		}
		return 1, "cn-chinext-ipo-first-day-no-limit-before-2020-08-24", "", "2020-08-23"
	case "star_market":
		return 5, "cn-star-market-ipo-first-five-trading-days-no-limit-v1", starMarketStartDate, ""
	case "beijing_exchange":
		return 1, "cn-beijing-exchange-ipo-first-day-no-limit-v1", beijingExchangeStartDate, ""
	default:
		return 0, "unknown", "", ""
	}
}

// ResolvePriceLimitRuleFromTradingStatus joins the historical security-status snapshot with
// separately sourced special-event evidence. It never treats an unknown/non-trading date as
// evidence that a special exception did not occur.
func ResolvePriceLimitRuleFromTradingStatus(
	rawSymbol, tradeDate string,
	tradingStatus *model.SecurityTradingStatusSnapshot,
	evidence *model.PriceLimitEventEvidence,
) (*model.PriceLimitRuleSnapshot, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	parsedDate, err := time.Parse("2006-01-02", tradeDate)
	if err != nil || parsedDate.Format("2006-01-02") != tradeDate {
		return nil, fmt.Errorf("invalid trade date %q", tradeDate)
	}

	if tradingStatus == nil {
		result, resolveErr := ResolvePriceLimitRuleWithEvidence(
			symbol, tradeDate, model.RiskWarningUnknown, 0, false, nil,
		)
		if resolveErr != nil {
			return nil, resolveErr
		}
		appendReasonOnce(&result.QualityReasons, "security_trading_status_evidence_missing")
		return result, nil
	}

	statusSymbol, symbolErr := normalize.NormalizeSymbol(tradingStatus.Symbol)
	if symbolErr != nil || statusSymbol != symbol {
		return nil, fmt.Errorf("security trading-status symbol does not match request %q", symbol)
	}
	if tradingStatus.TradeDate != tradeDate {
		return nil, fmt.Errorf("security trading-status date %q does not match request %q", tradingStatus.TradeDate, tradeDate)
	}
	if !tradingStatus.TradingDayKnown || !tradingStatus.IsTradingDay {
		result, resolveErr := ResolvePriceLimitRuleWithEvidence(
			symbol, tradeDate, model.RiskWarningUnknown, 0, false, nil,
		)
		if resolveErr != nil {
			return nil, resolveErr
		}
		if !tradingStatus.TradingDayKnown {
			appendReasonOnce(&result.QualityReasons, "trading_day_status_unverified")
		} else {
			appendReasonOnce(&result.QualityReasons, "price_limit_rule_not_applicable_non_trading_day")
		}
		return result, nil
	}

	result, err := ResolvePriceLimitRuleWithEvidence(
		symbol,
		tradeDate,
		tradingStatus.RiskWarningStatus,
		tradingStatus.ListingTradingDayNumber,
		tradingStatus.ListingStatusKnown,
		evidence,
	)
	if err != nil {
		return nil, err
	}

	// 退市状态未知或处于退市整理期时，不能套用普通板块涨跌幅规则。
	// 只有与请求日期匹配、已通过来源契约校验的明确特殊事件，才能独立决定无涨跌幅限制。
	delistingStatusNeedsReview := tradingStatus.DelistingStatus == model.DelistingStatusUnknown ||
		tradingStatus.DelistingStatus == model.DelistingStatusDelistingPeriod
	explicitSpecialEvent := result.SpecialExceptionStatus == model.PriceLimitExceptionRelistingFirst ||
		result.SpecialExceptionStatus == model.PriceLimitExceptionDelistingFirst ||
		result.SpecialExceptionStatus == model.PriceLimitExceptionExchangeSpecial
	if delistingStatusNeedsReview && !(explicitSpecialEvent && result.Status == model.PriceLimitStatusNoLimit) {
		if tradingStatus.DelistingStatus == model.DelistingStatusUnknown {
			appendReasonOnce(&result.QualityReasons, "delisting_status_unverified_for_price_limit_rule")
		} else {
			appendReasonOnce(&result.QualityReasons, "delisting_period_requires_exchange_specific_price_limit_rule")
		}
		result.Status = model.PriceLimitStatusUnknown
		result.LimitPercent = nil
		result.NoLimitReason = ""
		result.QualityStatus = model.QualityUnknown
	}
	return result, nil
}
