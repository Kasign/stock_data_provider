package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const tushareSecurityTradingStatusVersion = "tushare-namechange-suspend-d-trade-cal-v1"

// FetchSecurityTradingStatus reconstructs date-effective security status using source records.
// It intentionally does not substitute the current stock name for a missing historical name row.
func (p *Tushare) FetchSecurityTradingStatus(ctx context.Context, rawSymbol, rawTradeDate string) (*model.SecurityTradingStatusSnapshot, error) {
	symbol, err := tushareExchangeCode(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tushare historical security status only supports common A-share equities, got %q", rawSymbol)
	}
	compactDate, err := tushareCompactDate(rawTradeDate)
	if err != nil {
		return nil, err
	}
	tradeDate, err := time.Parse("20060102", compactDate)
	if err != nil {
		return nil, err
	}
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		shanghai = time.FixedZone("CST", 8*60*60)
	}
	date := tradeDate.Format("2006-01-02")
	if date > time.Now().In(shanghai).Format("2006-01-02") {
		return nil, errors.New("Tushare security-status trade date must not be in the future")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	basic, err := p.FetchStockBasic(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("Tushare stock_basic for listing date: %w", err)
	}
	exchange, err := tushareCalendarExchange(symbol)
	if err != nil {
		return nil, err
	}
	tradingDay, err := p.fetchSecurityTradingDay(ctx, exchange, compactDate)
	if err != nil {
		return nil, fmt.Errorf("Tushare trade_cal for %s: %w", date, err)
	}
	name, nameErr := p.fetchEffectiveSecurityName(ctx, symbol, compactDate)
	suspension, suspensionTiming, suspensionType, suspensionReasons, suspensionErr := p.fetchSuspensionOnDate(ctx, symbol, compactDate, tradingDay)

	result := &model.SecurityTradingStatusSnapshot{
		Symbol: symbol,
		TradeDate: date,
		ListingDate: strings.TrimSpace(basic.IPODate),
		IsTradingDay: tradingDay,
		TradingDayKnown: true,
		SuspensionStatus: suspension,
		SuspensionTiming: suspensionTiming,
		SuspensionType: suspensionType,
		Source: p.Name(),
		SourceVersion: tushareSecurityTradingStatusVersion,
		FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_exact_snapshot_time_not_exposed"},
	}

	if nameErr != nil {
		result.RiskWarningStatus = model.RiskWarningUnknown
		result.QualityStatus = model.QualityUnknown
		result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "historical_security_name_unavailable")
	} else {
		result.SecurityName = name.Name
		result.NameEffectiveFrom = name.StartDate
		result.NameEffectiveTo = name.EndDate
		result.RiskWarningStatus = riskWarningStatusFromHistoricalName(name.Name)
		if result.RiskWarningStatus == model.RiskWarningUnknown {
			result.QualityStatus = model.QualityUnknown
			result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "historical_security_name_does_not_prove_risk_warning_status")
		}
	}
	if strings.TrimSpace(basic.IPODate) == "" {
		result.ListingStatusKnown = false
		result.QualityStatus = model.QualityUnknown
		result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "listing_date_missing")
	} else {
		listingDate, parseErr := time.Parse("2006-01-02", basic.IPODate)
		if parseErr != nil || listingDate.Format("2006-01-02") != basic.IPODate || listingDate.After(tradeDate) {
			result.ListingStatusKnown = false
			result.QualityStatus = model.QualityUnknown
			result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "listing_date_invalid_or_after_requested_trade_date")
		} else if exchange == "BSE" {
			// Tushare trade_cal documents SSE/SZSE calendars, but does not provide a
			// sufficiently verified BSE listing-session count. Do not approximate it.
			result.ListingStatusKnown = false
			result.QualityStatus = model.QualityUnknown
			result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "listing_trading_day_number_not_verified_for_beijing_exchange")
		} else {
			count, countErr := p.countListingTradingDays(ctx, exchange, listingDate, tradeDate)
			if countErr != nil || count < 1 {
				result.ListingStatusKnown = false
				result.QualityStatus = model.QualityUnknown
				result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "listing_trading_day_number_unavailable")
			} else {
				result.ListingTradingDayNumber = count
				result.ListingStatusKnown = true
			}
		}
	}
	if suspensionErr != nil {
		result.SuspensionStatus = model.SuspensionStatusUnknown
		result.SuspensionTiming = ""
		result.SuspensionType = ""
		result.QualityStatus = model.QualityUnknown
		result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "historical_suspension_status_unavailable")
	} else {
		result.QualityReasons = append(result.QualityReasons, suspensionReasons...)
		if suspension == model.SuspensionStatusUnknown {
			result.QualityStatus = model.QualityUnknown
		}
	}
	result.QualityReasons = dedupeSecurityStatusReasons(result.QualityReasons)
	return result, nil
}

type effectiveTushareSecurityName struct {
	Name      string
	StartDate string
	EndDate   string
}

func (p *Tushare) fetchEffectiveSecurityName(ctx context.Context, symbol, compactDate string) (effectiveTushareSecurityName, error) {
	table, err := p.fetchCapabilityTable(ctx, "namechange", map[string]string{"ts_code": symbol},
		"ts_code,name,start_date,end_date,ann_date,change_reason")
	if err != nil {
		return effectiveTushareSecurityName{}, err
	}
	if len(table.rows) == 0 {
		return effectiveTushareSecurityName{}, errors.New("Tushare namechange returned no historical name rows")
	}
	var selected *effectiveTushareSecurityName
	for index, row := range table.rows {
		code, err := table.stringValue(row, "ts_code")
		if err != nil {
			return effectiveTushareSecurityName{}, err
		}
		canonical, err := normalize.NormalizeSymbol(code)
		if err != nil || canonical != symbol {
			return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d returned a different or invalid symbol %q", index, code)
		}
		name, err := table.stringValue(row, "name")
		if err != nil {
			return effectiveTushareSecurityName{}, err
		}
		if strings.TrimSpace(name) == "" {
			return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d has an empty name", index)
		}
		startRaw, err := table.stringValue(row, "start_date")
		if err != nil {
			return effectiveTushareSecurityName{}, err
		}
		startDate, err := tushareISODate(startRaw)
		if err != nil {
			return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d has invalid start_date: %w", index, err)
		}
		endRaw, err := table.stringValue(row, "end_date")
		if err != nil {
			return effectiveTushareSecurityName{}, err
		}
		endDate := ""
		if strings.TrimSpace(endRaw) != "" {
			endDate, err = tushareISODate(endRaw)
			if err != nil {
				return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d has invalid end_date: %w", index, err)
			}
			if endDate < startDate {
				return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d ends before it starts", index)
			}
		}
		if startDate > compactDate || (endDate != "" && endDate < compactDate) {
			continue
		}
		candidate := &effectiveTushareSecurityName{Name: strings.TrimSpace(name), StartDate: startDate, EndDate: endDate}
		if selected != nil {
			return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange has overlapping names effective on %s for %s", compactDate, symbol)
		}
		selected = candidate
	}
	if selected == nil {
		return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange has no name effective on %s for %s", compactDate, symbol)
	}
	return *selected, nil
}

func riskWarningStatusFromHistoricalName(name string) model.RiskWarningStatus {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	for _, prefix := range []string{"*ST", "S*ST", "SST", "ST"} {
		if strings.HasPrefix(normalized, prefix) {
			return model.RiskWarningApplied
		}
	}
	if normalized == "" || strings.HasPrefix(normalized, "PT") {
		return model.RiskWarningUnknown
	}
	return model.RiskWarningNormal
}

func (p *Tushare) fetchSecurityTradingDay(ctx context.Context, exchange, compactDate string) (bool, error) {
	table, err := p.fetchCapabilityTable(ctx, "trade_cal", map[string]string{
		"exchange": exchange, "start_date": compactDate, "end_date": compactDate,
	}, "exchange,cal_date,is_open")
	if err != nil {
		return false, err
	}
	if len(table.rows) != 1 {
		return false, fmt.Errorf("expected one calendar row for %s/%s, received %d", exchange, compactDate, len(table.rows))
	}
	exchangeValue, err := table.stringValue(table.rows[0], "exchange")
	if err != nil || !strings.EqualFold(strings.TrimSpace(exchangeValue), exchange) {
		return false, fmt.Errorf("calendar exchange does not match requested exchange %s", exchange)
	}
	date, err := table.stringValue(table.rows[0], "cal_date")
	if err != nil {
		return false, err
	}
	normalizedDate, err := tushareCompactDate(date)
	if err != nil || normalizedDate != compactDate {
		return false, fmt.Errorf("calendar row date does not match requested date %s", compactDate)
	}
	open, err := table.stringValue(table.rows[0], "is_open")
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(open) {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("calendar is_open has invalid value %q", open)
	}
}

func (p *Tushare) countListingTradingDays(ctx context.Context, exchange string, listingDate, tradeDate time.Time) (int, error) {
	if listingDate.After(tradeDate) {
		return 0, errors.New("listing date is after requested trade date")
	}
	const chunkDays = 3500
	count := 0
	for start := listingDate; !start.After(tradeDate); {
		end := start.AddDate(0, 0, chunkDays-1)
		if end.After(tradeDate) {
			end = tradeDate
		}
		startCompact := start.Format("20060102")
		endCompact := end.Format("20060102")
		table, err := p.fetchCapabilityTable(ctx, "trade_cal", map[string]string{
			"exchange": exchange, "start_date": startCompact, "end_date": endCompact,
		}, "exchange,cal_date,is_open")
		if err != nil {
			return 0, err
		}
		expectedDays := int(end.Sub(start)/(24*time.Hour)) + 1
		if len(table.rows) != expectedDays {
			return 0, fmt.Errorf("listing calendar coverage mismatch for %s..%s: expected %d dates, received %d", startCompact, endCompact, expectedDays, len(table.rows))
		}
		seen := make(map[string]struct{}, len(table.rows))
		for index, row := range table.rows {
			exchangeValue, err := table.stringValue(row, "exchange")
			if err != nil || !strings.EqualFold(strings.TrimSpace(exchangeValue), exchange) {
				return 0, fmt.Errorf("listing calendar row %d has mismatched exchange %q", index, exchangeValue)
			}
			dateRaw, err := table.stringValue(row, "cal_date")
			if err != nil {
				return 0, fmt.Errorf("listing calendar row %d: %w", index, err)
			}
			date, err := tushareISODate(dateRaw)
			if err != nil {
				return 0, fmt.Errorf("listing calendar row %d: %w", index, err)
			}
			if date < start.Format("2006-01-02") || date > end.Format("2006-01-02") {
				return 0, fmt.Errorf("listing calendar returned out-of-range date %s", date)
			}
			if _, duplicate := seen[date]; duplicate {
				return 0, fmt.Errorf("listing calendar returned duplicate date %s", date)
			}
			seen[date] = struct{}{}
			open, err := table.stringValue(row, "is_open")
			if err != nil {
				return 0, fmt.Errorf("listing calendar row %s has no is_open value", date)
			}
			switch strings.TrimSpace(open) {
			case "1":
				count++
			case "0":
			default:
				return 0, fmt.Errorf("listing calendar row %s has invalid is_open value %q", date, open)
			}
		}
		if len(seen) != expectedDays {
			return 0, fmt.Errorf("listing calendar has incomplete date coverage for %s..%s", startCompact, endCompact)
		}
		start = end.AddDate(0, 0, 1)
	}
	return count, nil
}

func tushareCalendarExchange(symbol string) (string, error) {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid canonical A-share symbol %q", symbol)
	}
	switch parts[1] {
	case "SH":
		return "SSE", nil
	case "SZ":
		return "SZSE", nil
	case "BJ":
		return "BSE", nil
	default:
		return "", fmt.Errorf("unsupported exchange for calendar query: %s", symbol)
	}
}

func suspensionStatusFromTushareRows(tradingDay bool, rows []struct {
	status string
	timing string
}) (model.SuspensionStatus, string, string, error) {
	if !tradingDay {
		if len(rows) > 0 {
			return model.SuspensionStatusUnknown, "", "", errors.New("Tushare returned suspension records for a non-trading date")
		}
		return model.SuspensionStatusNotTradingDay, "", "", nil
	}
	if len(rows) == 0 {
		return model.SuspensionStatusNoEvent, "", "", nil
	}
	seenS, seenR := false, false
	timings := []string{}
	for _, row := range rows {
		switch strings.TrimSpace(row.status) {
		case "S":
			seenS = true
		case "R":
			seenR = true
		default:
			return model.SuspensionStatusUnknown, "", "", fmt.Errorf("invalid Tushare suspend_type %q", row.status)
		}
		timing := strings.TrimSpace(row.timing)
		if timing != "" {
			timings = append(timings, timing)
		}
	}
	if seenS && seenR {
		if len(timings) > 0 {
			return model.SuspensionStatusIntraday, strings.Join(dedupeSecurityStatusReasons(timings), ","), "S,R", nil
		}
		return model.SuspensionStatusUnknown, "", "S,R", errors.New("conflicting all-day suspension and resumption records")
	}
	if seenS && len(timings) > 0 {
		return model.SuspensionStatusIntraday, strings.Join(dedupeSecurityStatusReasons(timings), ","), "S", nil
	}
	if seenS {
		return model.SuspensionStatusSuspended, "", "S", nil
	}
	if seenR {
		return model.SuspensionStatusResumed, strings.Join(dedupeSecurityStatusReasons(timings), ","), "R", nil
	}
	return model.SuspensionStatusUnknown, "", "", errors.New("Tushare suspend_d rows did not describe a known state")
}

func (p *Tushare) fetchSuspensionOnDate(ctx context.Context, symbol, compactDate string, tradingDay bool) (model.SuspensionStatus, string, string, []string, error) {
	table, err := p.fetchCapabilityTable(ctx, "suspend_d", map[string]string{
		"ts_code": symbol, "trade_date": compactDate,
	}, "ts_code,trade_date,suspend_timing,suspend_type")
	if err != nil {
		return model.SuspensionStatusUnknown, "", "", nil, err
	}
	rows := make([]struct {
		status string
		timing string
	}, 0, len(table.rows))
	for index, row := range table.rows {
		code, err := table.stringValue(row, "ts_code")
		if err != nil {
			return model.SuspensionStatusUnknown, "", "", nil, err
		}
		canonical, err := normalize.NormalizeSymbol(code)
		if err != nil || canonical != symbol {
			return model.SuspensionStatusUnknown, "", "", nil, fmt.Errorf("Tushare suspend_d row %d has a mismatched symbol %q", index, code)
		}
		dateRaw, err := table.stringValue(row, "trade_date")
		if err != nil {
			return model.SuspensionStatusUnknown, "", "", nil, err
		}
		date, err := tushareCompactDate(dateRaw)
		if err != nil || date != compactDate {
			return model.SuspensionStatusUnknown, "", "", nil, fmt.Errorf("Tushare suspend_d row %d has a mismatched date %q", index, dateRaw)
		}
		status, err := table.stringValue(row, "suspend_type")
		if err != nil {
			return model.SuspensionStatusUnknown, "", "", nil, err
		}
		timing, err := table.stringValue(row, "suspend_timing")
		if err != nil {
			return model.SuspensionStatusUnknown, "", "", nil, err
		}
		rows = append(rows, struct {
			status string
			timing string
		}{status: status, timing: timing})
	}
	status, timing, suspendType, stateErr := suspensionStatusFromTushareRows(tradingDay, rows)
	reasons := []string{}
	if stateErr != nil {
		reasons = append(reasons, "suspension_state_not_proven_by_provider_response")
	}
	return status, timing, suspendType, reasons, stateErr
}

func appendUniqueSecurityStatusReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func dedupeSecurityStatusReasons(reasons []string) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		out = appendUniqueSecurityStatusReason(out, reason)
	}
	return out
}
