package service

import (
	"errors"
	"fmt"
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
	sseTradingRuleURL = "https://www.sse.com.cn/lawandrules/sselawsrules2025/stocks/exchange/c/c_20260424_10816482.shtml"
	szseTradingRuleURL = "https://docs.static.szse.cn/www/lawrules/rule/trade/current/W020260424690713155663.pdf"
	szseRiskWarningChangeURL = "https://www.szse.cn/aboutus/trends/news/t20250627_614643.html"
	beijingTradingRuleURL = "https://www.bse.cn/qt/200025234.html"
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
