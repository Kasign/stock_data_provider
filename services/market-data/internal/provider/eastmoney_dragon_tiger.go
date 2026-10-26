package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const defaultEastmoneyDragonTigerURL = "https://datacenter-web.eastmoney.com/api/data/v1/get"

// NewEastmoneyConfiguredWithDragonTiger 为龙虎榜配置独立数据中心端点。
func NewEastmoneyConfiguredWithDragonTiger(timeout time.Duration, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, marketBreadthURL, limitPoolBaseURL, stockBoardsURL, dragonTigerURL, ut string) *Eastmoney {
	p := NewEastmoneyConfiguredWithStockBoards(timeout, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, marketBreadthURL, limitPoolBaseURL, stockBoardsURL, ut)
	if strings.TrimSpace(dragonTigerURL) != "" {
		p.dragonTigerURL = strings.TrimSpace(dragonTigerURL)
	}
	return p
}

// FetchDragonTiger 拉取指定交易日龙虎榜。源端只提供交易日，不提供精确快照时间。
func (p *Eastmoney) FetchDragonTiger(ctx context.Context, tradeDate string, page, pageSize int) (*model.DragonTigerSnapshot, error) {
	rawDate := strings.TrimSpace(tradeDate)
	parsedDate, err := time.Parse("2006-01-02", rawDate)
	if err != nil || parsedDate.Format("2006-01-02") != rawDate {
		return nil, fmt.Errorf("invalid dragon-tiger trade date %q: expected YYYY-MM-DD", tradeDate)
	}
	tradeDate = parsedDate.Format("2006-01-02")
	if page < 1 || page > 1000 {
		return nil, errors.New("dragon-tiger page must be between 1 and 1000")
	}
	if pageSize < 1 || pageSize > 500 {
		return nil, errors.New("dragon-tiger page size must be between 1 and 500")
	}
	if strings.TrimSpace(p.dragonTigerURL) == "" {
		return nil, errors.New("Eastmoney dragon-tiger endpoint is not configured")
	}
	params := url.Values{
		"reportName": {"RPT_DAILYBILLBOARD_DETAILSNEW"},
		"columns":    {"SECURITY_CODE,SECUCODE,SECURITY_NAME_ABBR,TRADE_DATE,EXPLAIN,EXPLANATION,CLOSE_PRICE,CHANGE_RATE,BILLBOARD_NET_AMT,BILLBOARD_BUY_AMT,BILLBOARD_SELL_AMT,BILLBOARD_DEAL_AMT,DEAL_AMOUNT_RATIO,SECURITY_TYPE_CODE"},
		"filter":     {"(TRADE_DATE='" + tradeDate + "')"},
		"pageNumber": {strconv.Itoa(page)}, "pageSize": {strconv.Itoa(pageSize)},
		"sortColumns": {"BILLBOARD_NET_AMT"}, "sortTypes": {"-1"}, "source": {"WEB"}, "client": {"WEB"},
	}
	var payload struct {
		Success *bool  `json:"success"`
		Message string `json:"message"`
		Code    int    `json:"code"`
		Result  *struct {
			Pages *int                         `json:"pages"`
			Count *int                         `json:"count"`
			Data  []map[string]json.RawMessage `json:"data"`
		} `json:"result"`
	}
	if err := p.getJSONWithRetry(ctx, p.dragonTigerURL, params, &payload, 8*time.Second); err != nil {
		return nil, fmt.Errorf("Eastmoney dragon-tiger request: %w", err)
	}
	if payload.Result == nil {
		// 东方财富对没有匹配记录的日期可能返回 code=9201，而不是 result.count=0。
		// 该响应表示“当前没有可用报表”，不能伪造成经过确认的零上榜。
		if payload.Code == 9201 && strings.TrimSpace(payload.Message) == "返回数据为空" {
			return &model.DragonTigerSnapshot{
				TradeDate: tradeDate, Page: page, PageSize: pageSize,
				Items: []model.DragonTigerItem{}, Source: p.Name(),
				SourceVersion: "eastmoney-datacenter-dragon-tiger-v1", FetchedAt: time.Now().UTC(),
				QualityStatus: model.QualityUnknown,
				QualityReasons: []string{"provider_returned_empty_report_cannot_distinguish_no_events_from_unavailable_or_not_yet_published", "provider_empty_response_code_9201"},
			}, nil
		}
		return nil, fmt.Errorf("Eastmoney dragon-tiger response is missing result/count (code=%d message=%q success=%v)", payload.Code, payload.Message, payload.Success)
	}
	if payload.Result.Count == nil {
		return nil, errors.New("Eastmoney dragon-tiger result is missing count")
	}
	total := *payload.Result.Count
	if total < 0 || total < len(payload.Result.Data) {
		return nil, fmt.Errorf("Eastmoney dragon-tiger count %d is inconsistent with %d rows", total, len(payload.Result.Data))
	}
	pages := 0
	reasons := []string{"provider_snapshot_timestamp_not_exposed"}
	if payload.Result.Pages != nil && *payload.Result.Pages >= 0 {
		pages = *payload.Result.Pages
	}
	if total == 0 {
		if len(payload.Result.Data) != 0 {
			return nil, errors.New("Eastmoney dragon-tiger reported zero total with non-empty rows")
		}
		return &model.DragonTigerSnapshot{
			TradeDate: tradeDate, Page: page, PageSize: pageSize, Items: []model.DragonTigerItem{},
			Source: p.Name(), SourceVersion: "eastmoney-datacenter-dragon-tiger-v1", FetchedAt: time.Now().UTC(),
			QualityStatus:  model.QualityUnknown,
			QualityReasons: []string{"provider_returned_empty_report_cannot_distinguish_no_events_from_unavailable_or_not_yet_published"},
		}, nil
	}
	if len(payload.Result.Data) == 0 {
		return nil, fmt.Errorf("Eastmoney dragon-tiger count is %d but page %d returned no rows", total, page)
	}
	if pages <= 0 {
		pages = (total + pageSize - 1) / pageSize
		reasons = append(reasons, "provider_pages_field_missing_or_invalid")
	}
	if page > pages {
		return nil, fmt.Errorf("requested dragon-tiger page %d exceeds provider page count %d", page, pages)
	}
	if len(payload.Result.Data) > pageSize {
		return nil, fmt.Errorf("Eastmoney dragon-tiger returned %d rows above page size %d", len(payload.Result.Data), pageSize)
	}
	fetchedAt := time.Now().UTC()
	snapshot := &model.DragonTigerSnapshot{
		TradeDate: tradeDate, ProviderReportDate: tradeDate,
		Page: page, PageSize: pageSize, Total: total, Pages: pages, Returned: len(payload.Result.Data),
		HasData: true, HasMore: page < pages, Items: make([]model.DragonTigerItem, 0, len(payload.Result.Data)),
		Source: p.Name(), SourceVersion: "eastmoney-datacenter-dragon-tiger-v1",
		FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded, QualityReasons: reasons,
	}
	for index, raw := range payload.Result.Data {
		code := strings.TrimSpace(parseEastmoneyBoardString(raw, "SECURITY_CODE"))
		symbol, normalizedCode, symbolErr := eastmoneyDragonTigerSymbol(raw, code)
		date := normalizeDragonTigerDate(parseEastmoneyBoardString(raw, "TRADE_DATE"))
		name := strings.TrimSpace(parseEastmoneyBoardString(raw, "SECURITY_NAME_ABBR"))
		if symbolErr != nil || code == "" || normalizedCode == "" || date != tradeDate || name == "" {
			return nil, fmt.Errorf("Eastmoney dragon-tiger row %d failed symbol/name/trade-date contract", index)
		}
		itemReasons := []string{"provider_snapshot_timestamp_not_exposed"}
		netBuy := parseEastmoneyBoardNumber(raw, "BILLBOARD_NET_AMT")
		buy := parseEastmoneyBoardNumber(raw, "BILLBOARD_BUY_AMT")
		sell := parseEastmoneyBoardNumber(raw, "BILLBOARD_SELL_AMT")
		deal := parseEastmoneyBoardNumber(raw, "BILLBOARD_DEAL_AMT")
		if netBuy == nil || buy == nil || sell == nil {
			itemReasons = append(itemReasons, "provider_billboard_amount_fields_incomplete")
		}
		reason := strings.TrimSpace(parseEastmoneyBoardString(raw, "EXPLAIN"))
		explanation := strings.TrimSpace(parseEastmoneyBoardString(raw, "EXPLANATION"))
		if reason == "" && explanation == "" {
			itemReasons = append(itemReasons, "provider_list_reason_missing")
		}
		for _, amount := range []*float64{buy, sell, deal} {
			if amount != nil && (!finiteNumber(*amount) || *amount < 0) {
				return nil, fmt.Errorf("Eastmoney dragon-tiger row %d has an invalid non-negative amount", index)
			}
		}
		if netBuy != nil && !finiteNumber(*netBuy) {
			return nil, fmt.Errorf("Eastmoney dragon-tiger row %d has an invalid net-buy amount", index)
		}
		changePercent := parseEastmoneyBoardNumber(raw, "CHANGE_RATE")
		closePrice := parseEastmoneyBoardNumber(raw, "CLOSE_PRICE")
		dealRatio := parseEastmoneyBoardNumber(raw, "DEAL_AMOUNT_RATIO")
		if closePrice != nil && (!finiteNumber(*closePrice) || *closePrice <= 0) {
			return nil, fmt.Errorf("Eastmoney dragon-tiger row %d has an invalid close price", index)
		}
		item := model.DragonTigerItem{
			Symbol: symbol, Code: normalizedCode, Name: name, TradeDate: date,
			ChangePercent: changePercent, ClosePrice: closePrice, NetBuyAmountCNY: netBuy,
			BuyAmountCNY: buy, SellAmountCNY: sell, DealAmountCNY: deal, DealAmountRatio: dealRatio,
			Reason: reason, Explanation: explanation, SecurityTypeCode: strings.TrimSpace(parseEastmoneyBoardString(raw, "SECURITY_TYPE_CODE")),
			Source: p.Name(), SourceVersion: snapshot.SourceVersion, FetchedAt: fetchedAt,
			QualityStatus: model.QualityDegraded, QualityReasons: itemReasons,
		}
		for _, reason := range itemReasons {
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, reason)
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if snapshot.Returned != len(snapshot.Items) {
		return nil, errors.New("Eastmoney dragon-tiger decoded row count mismatch")
	}
	return snapshot, nil
}

func normalizeDragonTigerDate(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 10 && raw[4] == '-' && raw[7] == '-' {
		return raw[:10]
	}
	return normalizeEastmoneyCompactDate(raw)
}

func eastmoneyDragonTigerSymbol(raw map[string]json.RawMessage, rawCode string) (string, string, error) {
	code := strings.TrimSpace(rawCode)
	securityCode := strings.TrimSpace(parseEastmoneyBoardString(raw, "SECUCODE"))
	if code == "" && securityCode != "" {
		code = strings.Split(securityCode, ".")[0]
	}
	if len(code) != 6 {
		return "", "", fmt.Errorf("invalid Eastmoney security code %q", code)
	}
	if _, err := strconv.Atoi(code); err != nil {
		return "", "", fmt.Errorf("invalid Eastmoney security code %q", code)
	}
	candidate := securityCode
	if !strings.Contains(candidate, ".") {
		suffix := ""
		switch code[0] {
		case '6', '9':
			suffix = ".SH"
		case '0', '2', '3':
			suffix = ".SZ"
		case '4', '8':
			suffix = ".BJ"
		default:
			return "", "", fmt.Errorf("cannot infer exchange for Eastmoney security code %q", code)
		}
		candidate = code + suffix
	}
	symbol, err := normalize.NormalizeSymbol(candidate)
	if err != nil {
		return "", "", err
	}
	if strings.Split(symbol, ".")[0] != code {
		return "", "", fmt.Errorf("Eastmoney security identity mismatch for %q", code)
	}
	return symbol, code, nil
}
