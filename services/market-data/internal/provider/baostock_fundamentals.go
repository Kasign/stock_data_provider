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

type baostockQuarterlyFinancialClient interface {
	QueryQuarterlyFinancialData(ctx context.Context, code, category string, year, quarter int) (*baostock.QuarterlyFinancialRecord, error)
}

type fundamentalFieldMapping struct {
	Key, Name, SourceField, Unit string
}
type fundamentalSectionMapping struct {
	Category string
	Fields   []fundamentalFieldMapping
}

var quarterlyFundamentalMappings = []fundamentalSectionMapping{
	{Category: "profitability", Fields: []fundamentalFieldMapping{
		{"roe_avg", "平均净资产收益率", "roeAvg", "ratio"},
		{"net_profit_margin", "销售净利率", "npMargin", "ratio"},
		{"gross_profit_margin", "销售毛利率", "gpMargin", "ratio"},
		{"net_profit", "净利润", "netProfit", "CNY"},
		{"eps_ttm", "每股收益TTM", "epsTTM", "CNY/share"},
		{"main_business_revenue", "主营营业收入", "MBRevenue", "CNY"},
		{"total_shares", "总股本", "totalShare", "shares"},
		{"float_shares", "流通股本", "liqaShare", "shares"},
	}},
	{Category: "operating", Fields: []fundamentalFieldMapping{
		{"receivables_turnover_ratio", "应收账款周转率", "NRTurnRatio", "times"},
		{"receivables_turnover_days", "应收账款周转天数", "NRTurnDays", "days"},
		{"inventory_turnover_ratio", "存货周转率", "INVTurnRatio", "times"},
		{"inventory_turnover_days", "存货周转天数", "INVTurnDays", "days"},
		{"current_assets_turnover_ratio", "流动资产周转率", "CATurnRatio", "times"},
		{"total_assets_turnover_ratio", "总资产周转率", "AssetTurnRatio", "times"},
	}},
	{Category: "growth", Fields: []fundamentalFieldMapping{
		{"equity_yoy", "净资产同比变化", "YOYEquity", "ratio"},
		{"asset_yoy", "总资产同比变化", "YOYAsset", "ratio"},
		{"net_income_yoy", "净利润同比变化", "YOYNI", "ratio"},
		{"basic_eps_yoy", "基本每股收益同比变化", "YOYEPSBasic", "ratio"},
		{"parent_net_income_yoy", "归母净利润同比变化", "YOYPNI", "ratio"},
	}},
	{Category: "solvency", Fields: []fundamentalFieldMapping{
		{"current_ratio", "流动比率", "currentRatio", "ratio"},
		{"quick_ratio", "速动比率", "quickRatio", "ratio"},
		{"cash_ratio", "现金比率", "cashRatio", "ratio"},
		{"liability_yoy", "总负债同比变化", "YOYLiability", "ratio"},
		{"liability_to_asset", "资产负债率", "liabilityToAsset", "ratio"},
		{"asset_to_equity", "权益乘数", "assetToEquity", "ratio"},
	}},
	{Category: "cash_flow", Fields: []fundamentalFieldMapping{
		{"current_asset_to_asset", "流动资产占比", "CAToAsset", "ratio"},
		{"noncurrent_asset_to_asset", "非流动资产占比", "NCAToAsset", "ratio"},
		{"tangible_asset_to_asset", "有形资产占比", "tangibleAssetToAsset", "ratio"},
		{"ebit_to_interest", "已获利息倍数", "ebitToInterest", "times"},
		{"operating_cash_to_revenue", "经营现金流/营业收入", "CFOToOR", "ratio"},
		{"operating_cash_to_net_profit", "经营现金流/净利润", "CFOToNP", "ratio"},
		{"operating_cash_to_total_revenue", "经营现金流/营业总收入", "CFOToGr", "ratio"},
	}},
	{Category: "dupont", Fields: []fundamentalFieldMapping{
		{"dupont_roe", "杜邦净资产收益率", "dupontROE", "ratio"},
		{"dupont_asset_to_equity", "杜邦权益乘数", "dupontAssetStoEquity", "times"},
		{"dupont_asset_turnover", "杜邦总资产周转率", "dupontAssetTurn", "times"},
		{"dupont_parent_net_to_net_profit", "归母净利润/净利润", "dupontPnitoni", "ratio"},
		{"dupont_net_margin", "杜邦销售净利率", "dupontNitogr", "ratio"},
		{"dupont_tax_burden", "税收负担率", "dupontTaxBurden", "ratio"},
		{"dupont_interest_burden", "利息负担率", "dupontIntburden", "ratio"},
		{"dupont_ebit_to_revenue", "息税前利润/营业总收入", "dupontEbittogr", "ratio"},
	}},
}

func fundamentalQuarterEnd(year, quarter int) string {
	return time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
}

// FetchFundamentals queries the native BaoStock TCP API. Raw ratio values are not rescaled.
func (p *Baostock) FetchFundamentals(ctx context.Context, rawSymbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("BaoStock client is not configured")
	}
	client, ok := p.client.(baostockQuarterlyFinancialClient)
	if !ok {
		return nil, errors.New("BaoStock client does not implement quarterly financial queries")
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	code, err := baostockSecurityCode(symbol)
	if err != nil {
		return nil, err
	}
	if year < 2007 || year > time.Now().Year() {
		return nil, fmt.Errorf("fundamental year must be between 2007 and %d", time.Now().Year())
	}
	if quarter < 1 || quarter > 4 {
		return nil, errors.New("fundamental quarter must be between 1 and 4")
	}
	snapshot := &model.FundamentalsSnapshot{
		Symbol: symbol, Year: year, Quarter: quarter,
		Sections: make([]model.FundamentalSection, 0, len(quarterlyFundamentalMappings)),
		Source:   p.Name(), SourceVersion: "baostock-quarterly-financial-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityUnknown,
	}
	queryErrors := 0
	var queryErrorMessages []string
	for _, mapping := range quarterlyFundamentalMappings {
		section := model.FundamentalSection{
			Category: mapping.Category, Metrics: make([]model.FundamentalMetric, 0, len(mapping.Fields)),
			QualityStatus: model.QualityUnknown,
		}
		record, queryErr := client.QueryQuarterlyFinancialData(ctx, code, mapping.Category, year, quarter)
		if queryErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			queryErrors++
			queryErrorMessages = append(queryErrorMessages, mapping.Category+": "+queryErr.Error())
			section.QualityStatus = model.QualityDegraded
			section.QualityReasons = []string{"provider_query_failed"}
			snapshot.Sections = append(snapshot.Sections, section)
			continue
		}
		if record == nil {
			section.QualityReasons = []string{"no_record_for_requested_report_period"}
			snapshot.Sections = append(snapshot.Sections, section)
			continue
		}
		if record.ReportDate != fundamentalQuarterEnd(year, quarter) {
			return nil, fmt.Errorf("BaoStock %s report date %s differs from requested quarter", mapping.Category, record.ReportDate)
		}
		section.ReportDate, section.PublicationDate = record.ReportDate, record.PublicationDate
		degraded := false
		if _, dateErr := time.Parse("2006-01-02", record.PublicationDate); dateErr != nil {
			degraded = true
			section.QualityReasons = append(section.QualityReasons, "publication_date_missing_or_invalid")
		}
		for _, metric := range mapping.Fields {
			raw := strings.TrimSpace(record.Fields[metric.SourceField])
			if raw == "" {
				degraded = true
				section.QualityReasons = appendFundamentalReason(section.QualityReasons, "one_or_more_metrics_missing")
				continue
			}
			value, parseErr := strconv.ParseFloat(raw, 64)
			if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				degraded = true
				section.QualityReasons = appendFundamentalReason(section.QualityReasons, "one_or_more_metrics_invalid")
				continue
			}
			section.Metrics = append(section.Metrics, model.FundamentalMetric{
				Key: metric.Key, Name: metric.Name, Value: value, Unit: metric.Unit, SourceField: metric.SourceField,
			})
		}
		section.HasData = len(section.Metrics) > 0
		if !section.HasData {
			degraded = true
			section.QualityReasons = appendFundamentalReason(section.QualityReasons, "no_numeric_metrics_available")
		} else {
			snapshot.HasData = true
		}
		if degraded {
			section.QualityStatus = model.QualityDegraded
		} else {
			section.QualityStatus = model.QualityValid
		}
		snapshot.Sections = append(snapshot.Sections, section)
	}
	if queryErrors == len(quarterlyFundamentalMappings) {
		return nil, fmt.Errorf("all BaoStock quarterly financial queries failed: %s", strings.Join(queryErrorMessages, "; "))
	}
	if snapshot.HasData {
		snapshot.QualityStatus = model.QualityValid
		for _, section := range snapshot.Sections {
			if !section.HasData || section.QualityStatus != model.QualityValid {
				snapshot.QualityStatus = model.QualityDegraded
				break
			}
		}
		if queryErrors > 0 {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = append(snapshot.QualityReasons, "one_or_more_financial_sections_failed")
		}
	} else if queryErrors > 0 {
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.QualityReasons = []string{"financial_sections_partially_unavailable"}
	} else {
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = []string{"no_financial_records_for_requested_period"}
	}
	return snapshot, nil
}

// appendFundamentalReason appends a quality reason only once.
func appendFundamentalReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
