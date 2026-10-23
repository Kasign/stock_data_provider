package provider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const baostockVersion = "baostock-public-tcp-v1"

type baostockDailyClient interface {
	QueryDailyKLines(ctx context.Context, code, startDate, endDate, adjustFlag string) ([]baostock.DailyKLine, error)
}

// Baostock implements only historical daily bars; it is not a real-time quote source.
type Baostock struct {
	client baostockDailyClient
}

func NewBaostock(client baostockDailyClient) *Baostock {
	return &Baostock{client: client}
}

func (p *Baostock) Name() string { return "baostock" }

func (p *Baostock) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("BaoStock client is not configured")
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	code, err := baostockSecurityCode(symbol)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	start, end, err := baostockDateRange(startDate, endDate, limit, time.Now())
	if err != nil {
		return nil, err
	}
	adjustFlag, adjustmentMethod, err := baostockAdjustFlag(adjustment)
	if err != nil {
		return nil, err
	}
	rows, err := p.client.QueryDailyKLines(ctx, code, start, end, adjustFlag)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("BaoStock returned no daily K-line rows")
	}

	fetchedAt := time.Now().UTC()
	out := make([]model.DailyBar, 0, len(rows))
	seenDates := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		tradeDate := strings.TrimSpace(row.Date)
		if _, err := time.Parse("2006-01-02", tradeDate); err != nil {
			return nil, fmt.Errorf("BaoStock row %d has invalid trade date %q", index, tradeDate)
		}
		if tradeDate < start || tradeDate > end {
			return nil, fmt.Errorf("BaoStock row %d date %s is outside requested range %s..%s", index, tradeDate, start, end)
		}
		// 同一交易日只允许一根日 K；重复记录通常意味着上游分页或协议数据异常。
		if _, exists := seenDates[tradeDate]; exists {
			return nil, fmt.Errorf("BaoStock returned duplicate trade date %s", tradeDate)
		}
		seenDates[tradeDate] = struct{}{}
		returnedSymbol, err := baostockReturnedSymbol(row.Code)
		if err != nil {
			return nil, fmt.Errorf("BaoStock row %d has invalid code %q: %w", index, row.Code, err)
		}
		if returnedSymbol != symbol {
			return nil, fmt.Errorf("BaoStock row %d symbol mismatch: requested %s, got %s", index, symbol, returnedSymbol)
		}
		if strings.TrimSpace(row.AdjustFlag) != adjustFlag {
			return nil, fmt.Errorf("BaoStock adjustment mismatch on %s: requested flag %s, got %q", tradeDate, adjustFlag, row.AdjustFlag)
		}

		open, err := parseBaoStockNumber(row.Open, "open", tradeDate, true)
		if err != nil {
			return nil, err
		}
		high, err := parseBaoStockNumber(row.High, "high", tradeDate, true)
		if err != nil {
			return nil, err
		}
		low, err := parseBaoStockNumber(row.Low, "low", tradeDate, true)
		if err != nil {
			return nil, err
		}
		closePrice, err := parseBaoStockNumber(row.Close, "close", tradeDate, true)
		if err != nil {
			return nil, err
		}
		// OHLC 必须满足最低价不高于最高价，开盘价和收盘价都落在当日区间内。
		// 检查失败时整批拒绝，避免把异常报价标记为 valid 或混入其他来源的数据。
		if low > high || open < low || open > high || closePrice < low || closePrice > high {
			return nil, fmt.Errorf("BaoStock OHLC invariant failed on %s: open=%g high=%g low=%g close=%g", tradeDate, open, high, low, closePrice)
		}
		rawVolumeValue, err := parseBaoStockNumber(row.Volume, "volume", tradeDate, false)
		if err != nil {
			return nil, err
		}
		if math.Abs(rawVolumeValue-math.Round(rawVolumeValue)) > 1e-6 {
			return nil, fmt.Errorf("BaoStock volume on %s is not a whole number of shares: %q", tradeDate, row.Volume)
		}
		volume, err := normalize.RawSharesToInt64(rawVolumeValue)
		if err != nil {
			return nil, fmt.Errorf("normalize BaoStock volume on %s: %w", tradeDate, err)
		}

		var amount *float64
		var rawAmount *float64
		rawAmountUnit := ""
		rawAmountField := ""
		quality := model.QualityValid
		var reasons []string
		if strings.TrimSpace(row.Amount) == "" {
			quality = model.QualityDegraded
			reasons = append(reasons, "amount_missing_from_provider", "amount_unit_unknown_because_amount_missing")
		} else {
			value, amountErr := parseBaoStockNumber(row.Amount, "amount", tradeDate, false)
			if amountErr != nil {
				return nil, amountErr
			}
			amountCopy := value
			rawAmountCopy := value
			amount = &amountCopy
			rawAmount = &rawAmountCopy
			rawAmountUnit = "CNY"
			rawAmountField = "amount"
		}
		rawVolume := rawVolumeValue
		amountUnit := "CNY"
		if amount == nil {
			amountUnit = "unknown"
		}
		changePercent, err := parseBaoStockOptionalNumber(row.PctChg, "pctChg", tradeDate)
		if err != nil {
			return nil, err
		}
		out = append(out, model.DailyBar{
			Symbol: symbol, TradeDate: tradeDate, Period: "1d",
			Open: open, High: high, Low: low, Close: closePrice,
			Volume: &volume, Amount: amount, ChangePercent: changePercent,
			AdjustmentBasis: adjustment, AdjustmentMethod: adjustmentMethod,
			Source: p.Name(), SourceVersion: baostockVersion,
			FetchedAt: fetchedAt, NormalizedAt: time.Now().UTC(),
			RawVolume: &rawVolume, RawVolumeUnit: "shares",
			RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: rawAmountField,
			VolumeUnit: "shares", AmountUnit: amountUnit,
			QualityStatus: quality, QualityReasons: reasons, NormalizationVer: normalize.Version,
		})
	}
	// 所有 Provider 都遵守 limit 上限；显式日期区间不能绕过返回条数限制。
	return normalizeProviderDailyBars(symbol, start, end, limit, out, 0)
}

func baostockSecurityCode(symbol string) (string, error) {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("BaoStock requires a canonical symbol with exchange suffix: %q", symbol)
	}
	switch parts[1] {
	case "SH":
		return "sh." + parts[0], nil
	case "SZ":
		return "sz." + parts[0], nil
	default:
		return "", fmt.Errorf("BaoStock daily K-lines currently support Shanghai/Shenzhen only, got %s", symbol)
	}
}

func baostockReturnedSymbol(raw string) (string, error) {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(raw)), ".")
	if len(parts) != 2 || len(parts[1]) != 6 {
		return "", errors.New("expected sh.###### or sz.###### format")
	}
	switch parts[0] {
	case "sh":
		return normalize.NormalizeSymbol(parts[1] + ".SH")
	case "sz":
		return normalize.NormalizeSymbol(parts[1] + ".SZ")
	default:
		return "", fmt.Errorf("unsupported BaoStock exchange code %q", parts[0])
	}
}

func baostockAdjustFlag(adjustment model.AdjustmentBasis) (string, string, error) {
	switch adjustment {
	case model.AdjustmentForward:
		return "2", "baostock_pct_change", nil
	case model.AdjustmentBackward:
		return "1", "baostock_pct_change", nil
	case model.AdjustmentNone:
		return "3", "none", nil
	default:
		return "", "", fmt.Errorf("unsupported BaoStock adjustment basis %q", adjustment)
	}
}

// Explicit start/end dates are preserved. An implicit start uses a bounded
// lookback so an unbounded all-history scan does not flood the public endpoint.
func baostockDateRange(startDate, endDate string, limit int, now time.Time) (string, string, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	end := now.In(loc)
	if strings.TrimSpace(endDate) != "" {
		end, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(endDate), loc)
		if err != nil {
			return "", "", fmt.Errorf("invalid BaoStock end date: %w", err)
		}
	}
	today := now.In(loc).Format("2006-01-02")
	endDateText := end.Format("2006-01-02")
	if endDateText > today {
		return "", "", errors.New("BaoStock end date must not be after today in Asia/Shanghai")
	}

	start := end.AddDate(0, 0, -(limit*4 + 30))
	if strings.TrimSpace(startDate) != "" {
		start, err = time.ParseInLocation("2006-01-02", strings.TrimSpace(startDate), loc)
		if err != nil {
			return "", "", fmt.Errorf("invalid BaoStock start date: %w", err)
		}
		if start.After(end) {
			return "", "", errors.New("BaoStock start date must not be after end date")
		}
	}
	return start.Format("2006-01-02"), endDateText, nil
}

func parseBaoStockNumber(raw, field, date string, positive bool) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || (positive && value <= 0) || (!positive && value < 0) {
		return 0, fmt.Errorf("invalid BaoStock %s on %s: %q", field, date, raw)
	}
	return value, nil
}

func parseBaoStockOptionalNumber(raw, field, date string) (*float64, error) {
	valueText := strings.TrimSpace(raw)
	if valueText == "" || strings.EqualFold(valueText, "null") {
		return nil, nil
	}
	value, err := strconv.ParseFloat(valueText, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil, fmt.Errorf("invalid BaoStock %s on %s: %q", field, date, raw)
	}
	return &value, nil
}
