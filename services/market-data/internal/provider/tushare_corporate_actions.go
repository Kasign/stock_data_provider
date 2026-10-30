package provider

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

const tushareDividendVersion = "tushare-dividend-v1"

const tushareDividendFields = "ts_code,end_date,ann_date,div_proc,stk_bo_rate,stk_co_rate,cash_div,cash_div_tax,record_date,ex_date,pay_date,imp_ann_date"

// FetchCorporateActions 使用 Tushare dividend 接口读取分红送转事件。
// 接口只有公告日期，没有精确发布时间，因此 AvailableAt 必须保持 unknown。
func (p *Tushare) FetchCorporateActions(ctx context.Context, rawSymbol, startDate, endDate string) ([]model.CorporateAction, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("Tushare HTTP client is not configured")
	}
	if p.token == "" {
		return nil, fmt.Errorf("Tushare token is not configured")
	}
	symbol, err := tushareExchangeCode(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tushare dividend only supports common A-share equities, got %q", rawSymbol)
	}
	startCompact, err := tushareCompactDate(startDate)
	if err != nil {
		return nil, fmt.Errorf("Tushare dividend start date: %w", err)
	}
	endCompact, err := tushareCompactDate(endDate)
	if err != nil {
		return nil, fmt.Errorf("Tushare dividend end date: %w", err)
	}
	startISO, err := tushareISODate(startCompact)
	if err != nil {
		return nil, err
	}
	endISO, err := tushareISODate(endCompact)
	if err != nil {
		return nil, err
	}
	if startISO > endISO {
		return nil, fmt.Errorf("corporate-action start date must not be after end date")
	}

	const pageSize = 2000
	const maxPages = 10
	out := make([]model.CorporateAction, 0)
	fetchedAt := time.Now().UTC()
	for page := 0; page < maxPages; page++ {
		table, fetchErr := p.fetchCapabilityTable(ctx, "dividend", map[string]string{
			"ts_code": symbol, "limit": fmt.Sprintf("%d", pageSize), "offset": fmt.Sprintf("%d", page*pageSize),
		}, tushareDividendFields)
		if fetchErr != nil {
			return nil, fmt.Errorf("fetch Tushare dividend page %d: %w", page+1, fetchErr)
		}
		if len(table.rows) == 0 {
			break
		}
		for rowIndex, row := range table.rows {
			code, rowErr := table.stringValue(row, "ts_code")
			if rowErr != nil || strings.ToUpper(strings.TrimSpace(code)) != symbol {
				return nil, fmt.Errorf("Tushare dividend row %d symbol mismatch: requested %s, returned %q", rowIndex, symbol, code)
			}
			readDate := func(field string) (string, error) {
				raw, readErr := table.stringValue(row, field)
				if readErr != nil || raw == "" {
					return "", readErr
				}
				return tushareISODate(raw)
			}
			reportDate, rowErr := readDate("end_date")
			if rowErr != nil {
				return nil, fmt.Errorf("Tushare dividend row %d report date: %w", rowIndex, rowErr)
			}
			announcementDate, rowErr := readDate("ann_date")
			if rowErr != nil {
				return nil, fmt.Errorf("Tushare dividend row %d announcement date: %w", rowIndex, rowErr)
			}
			implementationDate, rowErr := readDate("imp_ann_date")
			if rowErr != nil {
				return nil, fmt.Errorf("Tushare dividend row %d implementation announcement date: %w", rowIndex, rowErr)
			}
			recordDate, rowErr := readDate("record_date")
			if rowErr != nil {
				return nil, fmt.Errorf("Tushare dividend row %d record date: %w", rowIndex, rowErr)
			}
			exDate, rowErr := readDate("ex_date")
			if rowErr != nil {
				return nil, fmt.Errorf("Tushare dividend row %d ex-date: %w", rowIndex, rowErr)
			}
			payDate, rowErr := readDate("pay_date")
			if rowErr != nil {
				return nil, fmt.Errorf("Tushare dividend row %d pay date: %w", rowIndex, rowErr)
			}
			inRange := func(date string) bool {
				return date != "" && date >= startISO && date <= endISO
			}
			eventDate := exDate
			if eventDate == "" {
				eventDate = announcementDate
			}
			if !inRange(eventDate) {
				continue
			}

			status, rowErr := table.stringValue(row, "div_proc")
			if rowErr != nil {
				return nil, rowErr
			}
			bonus, rowErr := table.optionalFloat(row, "stk_bo_rate")
			if rowErr != nil {
				return nil, rowErr
			}
			transfer, rowErr := table.optionalFloat(row, "stk_co_rate")
			if rowErr != nil {
				return nil, rowErr
			}
			cashAfterTax, rowErr := table.optionalFloat(row, "cash_div")
			if rowErr != nil {
				return nil, rowErr
			}
			cashBeforeTax, rowErr := table.optionalFloat(row, "cash_div_tax")
			if rowErr != nil {
				return nil, rowErr
			}
			eventID := strings.Join([]string{symbol, reportDate, announcementDate, implementationDate}, "|")
			if reportDate == "" || announcementDate == "" {
				return nil, fmt.Errorf("Tushare dividend row %d is missing stable event identity fields", rowIndex)
			}
			out = append(out, model.CorporateAction{
				Symbol: symbol, ProviderEventID: eventID, EventType: "dividend",
				EventStatus: status, AnnouncementDate: announcementDate,
				ImplementationAnnouncementDate: implementationDate, RecordDate: recordDate,
				ExDate: exDate, PayDate: payDate,
				CashDividendCNY: cashAfterTax, CashDividendBeforeTaxCNY: cashBeforeTax,
				BonusShareRatio: bonus, TransferShareRatio: transfer,
				Source: p.Name(), SourceVersion: tushareDividendVersion, FetchedAt: fetchedAt,
				QualityStatus:  model.QualityDegraded,
				QualityReasons: []string{"provider_available_at_timestamp_not_exposed_by_dividend"},
			})
		}
		if len(table.rows) < pageSize {
			break
		}
		if page == maxPages-1 {
			return nil, fmt.Errorf("Tushare dividend response reached safety limit of %d rows; refusing a potentially truncated history", pageSize*maxPages)
		}
	}
	return out, nil
}
