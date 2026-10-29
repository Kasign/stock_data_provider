package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const eastmoneyVersion = "eastmoney-stock-api-v1"

// Bound implicit historical requests so old out-of-window rows do not pollute
// quality warnings when callers only want the most recent N trading bars.
const eastmoneyLookbackDaysPerBar = 4
const eastmoneyLookbackBufferDays = 30

// Eastmoney limit-pool responses can be shorter than the source total; fetch small pages until the requested coverage is reached.
const eastmoneyLimitPoolPageSize = 30

// defaultEastmoneyUT is the public web token commonly used by Eastmoney front-end clients.
// It is not a user credential or private API key; deployments can override it if Eastmoney rotates it.
const defaultEastmoneyUT = "7eea3edcaed734bea9cbfc24409ed989"
const defaultEastmoneyStockBasicURL = "https://datacenter.eastmoney.com/securities/api/data/v1/get"
const defaultEastmoneySectorRankingURL = "https://push2.eastmoney.com/api/qt/clist/get"
const defaultEastmoneyIndexURL = "https://push2.eastmoney.com/api/qt/ulist.np/get"
const defaultEastmoneyStockBoardsURL = "https://push2.eastmoney.com/api/qt/slist/get"
const defaultEastmoneyCapitalFlowURL = "https://push2.eastmoney.com/api/qt/stock/fflow/kline/get"

type Eastmoney struct {
	client           *http.Client
	quoteURL         string
	klineURL         string
	stockBasicURL    string
	sectorRankingURL string
	stockListURL     string
	indexURL         string
	marketBreadthURL string
	limitPoolBaseURL string
	stockBoardsURL   string
	capitalFlowURL   string
	dragonTigerURL   string
	announcementURL  string
	hotRankingURL    string
	hotSurgeURL      string
	ut               string

	stockSearchMu         sync.Mutex
	stockSearchIndex      []model.StockSearchItem
	stockSearchFetchedAt  time.Time
	stockSearchQuality    model.QualityStatus
	stockSearchReasons    []string
	stockSearchLoading    chan struct{}
	stockSearchRetryAfter time.Time
	stockSearchLastError  string
}

func NewEastmoney(timeout time.Duration, quoteURL, klineURL string) *Eastmoney {
	return NewEastmoneyConfigured(timeout, quoteURL, klineURL, "")
}

// NewEastmoneyConfigured allows the public Eastmoney front-end token to be overridden without changing endpoint URLs.
func NewEastmoneyConfigured(timeout time.Duration, quoteURL, klineURL, ut string) *Eastmoney {
	return NewEastmoneyConfiguredWithStockBasic(timeout, quoteURL, klineURL, "", ut)
}

// NewEastmoneyConfiguredWithStockBasic configures the independent F10 security-identity endpoint.
func NewEastmoneyConfiguredWithStockBasic(timeout time.Duration, quoteURL, klineURL, stockBasicURL, ut string) *Eastmoney {
	return NewEastmoneyConfiguredWithMarketData(timeout, quoteURL, klineURL, stockBasicURL, "", ut)
}

// NewEastmoneyConfiguredWithMarketData configures independent F10 and sector-ranking endpoints.
func NewEastmoneyConfiguredWithMarketData(timeout time.Duration, quoteURL, klineURL, stockBasicURL, sectorRankingURL, ut string) *Eastmoney {
	return NewEastmoneyConfiguredWithIndices(timeout, quoteURL, klineURL, stockBasicURL, sectorRankingURL, "", ut)
}

// NewEastmoneyConfiguredWithIndices configures independent F10, board ranking and index endpoints.
func NewEastmoneyConfiguredWithIndices(timeout time.Duration, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, ut string) *Eastmoney {
	return NewEastmoneyConfiguredWithDomesticMarketData(timeout, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, "", "", ut)
}

func NewEastmoneyConfiguredWithDomesticMarketData(timeout time.Duration, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, marketBreadthURL, limitPoolBaseURL, ut string) *Eastmoney {
	return NewEastmoneyConfiguredWithStockBoards(timeout, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, marketBreadthURL, limitPoolBaseURL, "", ut)
}

// NewEastmoneyConfiguredWithStockBoards configures the per-security board-membership endpoint independently.
func NewEastmoneyConfiguredWithStockBoards(timeout time.Duration, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, marketBreadthURL, limitPoolBaseURL, stockBoardsURL, ut string) *Eastmoney {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if quoteURL == "" {
		quoteURL = "https://push2.eastmoney.com/api/qt/stock/get"
	}
	if klineURL == "" {
		klineURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	}
	if stockBasicURL == "" {
		stockBasicURL = defaultEastmoneyStockBasicURL
	}
	if sectorRankingURL == "" {
		sectorRankingURL = defaultEastmoneySectorRankingURL
	}
	if indexURL == "" {
		indexURL = defaultEastmoneyIndexURL
	}
	if marketBreadthURL == "" {
		// Use the delay host directly for broad pagination; the primary CLIST host redirects here
		// anyway, and extra failed redirect/fallback attempts can aggravate transient rate limiting.
		marketBreadthURL = "https://push2ex.eastmoney.com/getTopicZDFenBu"
	}
	if limitPoolBaseURL == "" {
		limitPoolBaseURL = "https://push2ex.eastmoney.com"
	}
	if stockBoardsURL == "" {
		stockBoardsURL = defaultEastmoneyStockBoardsURL
	}
	ut = strings.TrimSpace(ut)
	if ut == "" {
		ut = defaultEastmoneyUT
	}
	return &Eastmoney{
		client:   &http.Client{Timeout: timeout, CheckRedirect: eastmoneyRedirectPolicy},
		quoteURL: quoteURL, klineURL: klineURL, stockBasicURL: stockBasicURL, sectorRankingURL: sectorRankingURL, stockListURL: sectorRankingURL, indexURL: indexURL, marketBreadthURL: marketBreadthURL, limitPoolBaseURL: strings.TrimRight(limitPoolBaseURL, "/"), stockBoardsURL: stockBoardsURL, capitalFlowURL: defaultEastmoneyCapitalFlowURL, dragonTigerURL: defaultEastmoneyDragonTigerURL, announcementURL: defaultEastmoneyAnnouncementURL, hotRankingURL: eastmoneyHotRankURL, hotSurgeURL: eastmoneyHotSurgeURL, ut: ut,
	}
}

// NewEastmoneyConfiguredWithAnnouncements adds an independently configurable A-share announcement endpoint.
func NewEastmoneyConfiguredWithAnnouncements(timeout time.Duration, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, marketBreadthURL, limitPoolBaseURL, stockBoardsURL, dragonTigerURL, announcementURL, ut string) *Eastmoney {
	p := NewEastmoneyConfiguredWithDragonTiger(timeout, quoteURL, klineURL, stockBasicURL, sectorRankingURL, indexURL, marketBreadthURL, limitPoolBaseURL, stockBoardsURL, dragonTigerURL, ut)
	if strings.TrimSpace(announcementURL) != "" {
		p.announcementURL = strings.TrimSpace(announcementURL)
	}
	return p
}

// WithHotRankingURLs 覆盖人气榜与飙升榜端点；传入空白值时保留原有官方默认地址。
func (p *Eastmoney) WithHotRankingURLs(popularURL, surgeURL string) *Eastmoney {
	if p == nil {
		return nil
	}
	if value := strings.TrimSpace(popularURL); value != "" {
		p.hotRankingURL = value
	}
	if value := strings.TrimSpace(surgeURL); value != "" {
		p.hotSurgeURL = value
	}
	return p
}

func (p *Eastmoney) Name() string { return "eastmoney" }

// WithCapitalFlowURL 覆盖东方财富个股资金流端点；空值保留默认公开地址。
func (p *Eastmoney) WithCapitalFlowURL(endpoint string) *Eastmoney {
	if p == nil {
		return nil
	}
	if value := strings.TrimSpace(endpoint); value != "" {
		p.capitalFlowURL = value
	}
	if strings.TrimSpace(p.capitalFlowURL) == "" {
		p.capitalFlowURL = defaultEastmoneyCapitalFlowURL
	}
	return p
}

// FetchStockBoards returns the board memberships associated with one security via Eastmoney's slist endpoint.
func (p *Eastmoney) FetchStockBoards(ctx context.Context, rawSymbol string) (*model.StockBoardSnapshot, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Eastmoney stock boards only support common A-share equities, got %q", rawSymbol)
	}
	secID, err := eastmoneySecID(symbol)
	if err != nil {
		return nil, err
	}
	params := url.Values{
		"forcect": {"1"},
		"spt":     {"3"},
		"fields":  {"f1,f12,f152,f3,f14,f128,f136"},
		"pi":      {"0"},
		"pz":      {"1000"},
		"po":      {"1"},
		"fid":     {"f3"},
		"fid0":    {"f4003"},
		"invt":    {"2"},
		"secid":   {secID},
		"ut":      {p.ut},
		"_":       {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Total int             `json:"total"`
			Diff  json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if err := p.getJSONWithRetry(ctx, p.stockBoardsURL, params, &payload, 6*time.Second); err != nil {
		return nil, fmt.Errorf("Eastmoney stock boards request: %w", err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney stock boards provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil {
		return nil, errors.New("Eastmoney stock boards response has no data object")
	}
	if payload.Data.Total < 0 || payload.Data.Total > 1000 {
		return nil, fmt.Errorf("Eastmoney stock boards returned unsupported total %d", payload.Data.Total)
	}
	rows, err := decodeEastmoneyBoardRows(payload.Data.Diff)
	if err != nil {
		return nil, fmt.Errorf("decode Eastmoney stock-board rows: %w", err)
	}
	if payload.Data.Total != len(rows) {
		return nil, fmt.Errorf("Eastmoney stock boards coverage incomplete: received %d of %d rows", len(rows), payload.Data.Total)
	}
	now := time.Now().UTC()
	snapshot := &model.StockBoardSnapshot{
		Symbol: symbol, Total: payload.Data.Total, HasData: payload.Data.Total > 0,
		Items:  make([]model.StockBoard, 0, len(rows)),
		Source: p.Name(), SourceVersion: "eastmoney-slist-stock-boards-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
	if !snapshot.HasData {
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = []string{"provider_returned_empty_board_list_cannot_distinguish_no_membership_from_unavailable_data"}
		return snapshot, nil
	}
	seen := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		code, name := parseEastmoneyBoardString(row, "f12"), parseEastmoneyBoardString(row, "f14")
		if code == "" || name == "" {
			return nil, fmt.Errorf("Eastmoney stock-board row %d has an empty code or name", index)
		}
		if _, exists := seen[code]; exists {
			return nil, fmt.Errorf("Eastmoney stock boards contains duplicate board code %s", code)
		}
		seen[code] = struct{}{}
		change := parseEastmoneyBoardNumber(row, "f3")
		reasons := []string{"provider_snapshot_timestamp_not_exposed"}
		if change == nil {
			reasons = append(reasons, "provider_board_change_percent_missing")
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_board_change_percent_missing")
		}
		snapshot.Items = append(snapshot.Items, model.StockBoard{
			Code: code, Name: name, ChangePercent: change, Source: p.Name(),
			SourceVersion: snapshot.SourceVersion, FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: reasons,
		})
	}
	return snapshot, nil
}

// FetchStockBasic retrieves low-frequency A-share identity and listing metadata from Eastmoney F10.
func (p *Eastmoney) FetchStockBasic(ctx context.Context, rawSymbol string) (*model.StockBasic, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Eastmoney stock-basic only supports common A-share equities, got %q", rawSymbol)
	}

	fetchRow := func(reportName string) (map[string]json.RawMessage, error) {
		params := url.Values{
			"reportName":   {reportName},
			"columns":      {"ALL"},
			"quoteColumns": {""},
			"filter":       {fmt.Sprintf(`(SECUCODE="%s")`, symbol)},
			"pageNumber":   {"1"},
			"pageSize":     {"1"},
			"sortTypes":    {""},
			"sortColumns":  {""},
			"source":       {"HSF10"},
			"client":       {"PC"},
			"v":            {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}
		var payload struct {
			Result *struct {
				Data []map[string]json.RawMessage `json:"data"`
			} `json:"result"`
		}
		u, err := url.Parse(p.stockBasicURL)
		if err != nil {
			return nil, fmt.Errorf("eastmoney stock-basic endpoint URL: %w", err)
		}
		u.RawQuery = params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://emweb.securities.eastmoney.com/")
		req.Header.Set("Origin", "https://emweb.securities.eastmoney.com")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("eastmoney stock-basic request: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("eastmoney stock-basic HTTP status %d", resp.StatusCode)
		}
		body, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("read Eastmoney stock-basic payload: %w", err)
		}
		if err := decodeSingleJSON(body, &payload); err != nil {
			return nil, fmt.Errorf("decode Eastmoney stock-basic JSON: %w", err)
		}
		if payload.Result == nil || len(payload.Result.Data) == 0 {
			return nil, fmt.Errorf("eastmoney stock-basic report %s returned no row for %s", reportName, symbol)
		}
		if len(payload.Result.Data) != 1 {
			return nil, fmt.Errorf("eastmoney stock-basic report %s expected exactly one row for %s, got %d", reportName, symbol, len(payload.Result.Data))
		}
		row := payload.Result.Data[0]
		field := func(name string) string {
			raw, ok := row[name]
			if !ok || len(raw) == 0 || string(raw) == "null" {
				return ""
			}
			var value string
			if json.Unmarshal(raw, &value) == nil {
				return strings.TrimSpace(value)
			}
			return strings.TrimSpace(strings.Trim(string(raw), `"`))
		}
		providerSymbol := field("SECUCODE")
		if providerSymbol == "" {
			providerSymbol = field("SECURITY_CODE")
			if len(providerSymbol) == 6 {
				if strings.HasPrefix(providerSymbol, "6") {
					providerSymbol += ".SH"
				} else if strings.HasPrefix(providerSymbol, "0") || strings.HasPrefix(providerSymbol, "3") {
					providerSymbol += ".SZ"
				}
			}
		}
		canonical, symbolErr := normalize.NormalizeSymbol(providerSymbol)
		if symbolErr != nil || canonical != symbol {
			return nil, fmt.Errorf("eastmoney stock-basic symbol mismatch in %s: requested %s, returned %q", reportName, symbol, providerSymbol)
		}
		return row, nil
	}

	readField := func(row map[string]json.RawMessage, name string) string {
		raw, ok := row[name]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			return ""
		}
		var value string
		if json.Unmarshal(raw, &value) == nil {
			return strings.TrimSpace(value)
		}
		return strings.TrimSpace(strings.Trim(string(raw), `"`))
	}

	// 发行信息表有上市日期，但没有稳定的 A 股简称字段。
	issueRow, err := fetchRow("RPT_PCF10_ORG_ISSUEINFO")
	if err != nil {
		return nil, err
	}
	name := readField(issueRow, "SECURITY_NAME_ABBR")
	if name == "" {
		// 公司概况表提供 STR_NAMEA/ORG_NAME；独立查询避免把股票代码误当成名称。
		orgRow, orgErr := fetchRow("RPT_F10_BASIC_ORGINFO")
		if orgErr != nil {
			return nil, orgErr
		}
		for _, key := range []string{"STR_NAMEA", "SECURITY_NAME_ABBR", "ORG_NAME"} {
			if name = readField(orgRow, key); name != "" {
				break
			}
		}
	}
	if name == "" {
		return nil, fmt.Errorf("eastmoney stock-basic name is empty for %s", symbol)
	}
	ipoDate, err := parseEastmoneyProfileDate(readField(issueRow, "LISTING_DATE"))
	if err != nil {
		return nil, err
	}
	outDate, err := parseEastmoneyProfileDate(readField(issueRow, "OUT_DATE"))
	if err != nil {
		return nil, err
	}
	status, statusLabel := 1, "listed"
	if outDate != "" {
		status, statusLabel = 0, "delisted"
	}
	item := &model.StockBasic{
		Symbol: symbol, Name: name, IPODate: ipoDate, OutDate: outDate,
		SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: status, ListingStatusLabel: statusLabel,
		Source: p.Name(), SourceVersion: "eastmoney-f10-market-profile-v3",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
		QualityReasons:   []string{"market_profile_timestamp_not_exposed"},
		NormalizationVer: normalize.Version,
	}

	// Efinance get_base_info() uses the same Eastmoney quote endpoint and field mapping.
	// The response has no independently verifiable snapshot timestamp; do not mark this profile as fully valid.
	secid, secidErr := eastmoneySecID(symbol)
	if secidErr != nil {
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_symbol_not_supported_by_quote_endpoint")
		return item, nil
	}
	quoteParams := url.Values{
		"secid":  []string{secid},
		"fields": []string{"f57,f58,f162,f167,f127,f116,f117,f84,f85,f198,f173,f187,f105,f186"},
		"fltt":   []string{"2"},
		"invt":   []string{"2"},
		"ut":     []string{p.ut},
	}
	var quotePayload struct {
		RC   *int                       `json:"rc"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if quoteErr := p.getJSONWithRetry(ctx, p.quoteURL, quoteParams, &quotePayload, 8*time.Second); quoteErr != nil {
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_request_failed")
		return item, nil
	}
	if quotePayload.RC != nil && *quotePayload.RC != 0 {
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_provider_error")
		return item, nil
	}
	if len(quotePayload.Data) == 0 {
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_data_missing")
		return item, nil
	}
	sourceCode := readEastmoneyField(quotePayload.Data, "f57")
	if sourceCode != "" && sourceCode != strings.Split(symbol, ".")[0] {
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_security_code_mismatch")
		return item, nil
	}
	itemName := readEastmoneyField(quotePayload.Data, "f58")
	if itemName != "" && itemName != name {
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_name_differs_from_f10_identity")
	}
	item.DynamicPE = parseEastmoneyProfileNumber(quotePayload.Data["f162"])
	item.PB = parseEastmoneyProfileNumber(quotePayload.Data["f167"])
	item.Industry = readEastmoneyField(quotePayload.Data, "f127")
	item.MarketCapCNY = parseEastmoneyProfileNumber(quotePayload.Data["f116"])
	item.FloatMarketCapCNY = parseEastmoneyProfileNumber(quotePayload.Data["f117"])
	item.TotalShares = parseEastmoneyProfileNumber(quotePayload.Data["f84"])
	item.FloatShares = parseEastmoneyProfileNumber(quotePayload.Data["f85"])
	item.BoardCode = readEastmoneyField(quotePayload.Data, "f198")
	item.ROEPercent = parseEastmoneyProfileNumber(quotePayload.Data["f173"])
	item.NetProfitMarginPercent = parseEastmoneyProfileNumber(quotePayload.Data["f187"])
	item.NetProfitCNY = parseEastmoneyProfileNumber(quotePayload.Data["f105"])
	item.GrossMarginPercent = parseEastmoneyProfileNumber(quotePayload.Data["f186"])

	for key, value := range map[string]*float64{
		"pe_dynamic": item.DynamicPE, "pb": item.PB, "market_cap_cny": item.MarketCapCNY,
		"float_market_cap_cny": item.FloatMarketCapCNY,
		"total_shares": item.TotalShares, "float_shares": item.FloatShares,
		"roe_percent": item.ROEPercent,
		"net_profit_margin_percent": item.NetProfitMarginPercent, "net_profit_cny": item.NetProfitCNY,
		"gross_margin_percent": item.GrossMarginPercent,
	} {
		if value == nil {
			item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_"+key+"_missing_or_invalid")
		}
	}
	if item.MarketCapCNY != nil && *item.MarketCapCNY < 0 {
		item.MarketCapCNY = nil
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_market_cap_cny_negative")
	}
	if item.FloatMarketCapCNY != nil && *item.FloatMarketCapCNY < 0 {
		item.FloatMarketCapCNY = nil
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_float_market_cap_cny_negative")
	}
	if item.TotalShares != nil && *item.TotalShares <= 0 {
		item.TotalShares = nil
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_total_shares_non_positive")
	}
	if item.FloatShares != nil && *item.FloatShares < 0 {
		item.FloatShares = nil
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_float_shares_negative")
	}
	if item.TotalShares != nil && item.FloatShares != nil && *item.FloatShares > *item.TotalShares {
		item.FloatShares = nil
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_float_shares_exceed_total_shares")
	}
	if item.Industry == "" {
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_industry_missing")
	}
	if item.BoardCode == "" {
		item.QualityReasons = appendUniqueEastmoneyReason(item.QualityReasons, "market_profile_board_code_missing")
	}
	return item, nil
}

func readEastmoneyField(row map[string]json.RawMessage, name string) string {
	raw, ok := row[name]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(strings.Trim(string(raw), `"`))
}

func parseEastmoneyProfileNumber(raw json.RawMessage) *float64 {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	value := strings.TrimSpace(strings.Trim(string(raw), `"`))
	if value == "" || value == "-" || strings.EqualFold(value, "null") {
		return nil
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		return nil
	}
	result := n
	return &result
}

func parseEastmoneyProfileDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" || strings.EqualFold(value, "null") {
		return "", nil
	}
	for _, layout := range []string{
		"2006-01-02", "2006-01-02 15:04:05", "2006-01-02T15:04:05",
		"2006/01/02", "20060102", time.RFC3339,
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("eastmoney stock-basic date %q has an unsupported format", value)
}

func (p *Eastmoney) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Eastmoney stock quote only supports common A-share equities, got %q", rawSymbol)
	}
	secid, err := eastmoneySecID(symbol)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("secid", secid)
	q.Set("fields", "f43,f44,f45,f46,f47,f48,f50,f57,f58,f60,f86,f169,f170,f168")
	q.Set("fltt", "2") // request formatted prices in yuan and percentages in percentage points
	q.Set("invt", "2")
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	var payload struct {
		RC   *int                       `json:"rc"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := p.getJSON(ctx, p.quoteURL, q, &payload); err != nil {
		return nil, err
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney quote provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil {
		return nil, errors.New("eastmoney returned an empty quote")
	}
	getString := func(key string) string {
		var s string
		if json.Unmarshal(payload.Data[key], &s) == nil {
			return s
		}
		var n json.Number
		dec := json.NewDecoder(strings.NewReader(string(payload.Data[key])))
		dec.UseNumber()
		if dec.Decode(&n) == nil {
			return n.String()
		}
		return ""
	}
	reportedSymbol := strings.TrimSpace(getString("f57"))
	if reportedSymbol != "" {
		canonical, canonicalErr := normalize.NormalizeSymbol(reportedSymbol)
		if canonicalErr != nil || canonical != symbol {
			return nil, fmt.Errorf("Eastmoney quote symbol mismatch: requested %s got %q", symbol, reportedSymbol)
		}
	}
	getNumber := func(key string) (*float64, error) {
		s := getString(key)
		if s == "" || s == "-" || s == "null" {
			return nil, nil
		}
		v, e := strconv.ParseFloat(s, 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("invalid Eastmoney numeric field %s", key)
		}
		return &v, nil
	}
	price, err := getNumber("f43")
	if err != nil {
		return nil, err
	}
	high, err := getNumber("f44")
	if err != nil {
		return nil, err
	}
	low, err := getNumber("f45")
	if err != nil {
		return nil, err
	}
	open, err := getNumber("f46")
	if err != nil {
		return nil, err
	}
	prev, err := getNumber("f60")
	if err != nil {
		return nil, err
	}
	change, err := getNumber("f169")
	if err != nil {
		return nil, err
	}
	pct, err := getNumber("f170")
	if err != nil {
		return nil, err
	}
	rawVol, err := getNumber("f47")
	if err != nil {
		return nil, err
	}
	var volume *int64
	var volumePtr *float64
	if rawVol != nil {
		shares, conversionErr := normalize.HandsToShares(*rawVol)
		if conversionErr != nil {
			return nil, fmt.Errorf("normalize Eastmoney quote volume: %w", conversionErr)
		}
		volume = &shares
		volumePtr = rawVol
	}
	rawAmount, err := getNumber("f48")
	if err != nil {
		return nil, err
	}
	amount := rawAmount // Eastmoney quote amount is CNY
	volumeRatio, err := getNumber("f50")
	if err != nil {
		return nil, err
	}
	turnoverRate, err := getNumber("f168")
	if err != nil {
		return nil, err
	}
	name := getString("f58")
	fetchedAt := time.Now().UTC()
	providerTime := validProviderSnapshotTime(parseEastmoneyTime(getString("f86")), fetchedAt)
	quote := &model.Quote{Symbol: symbol, Name: name, CurrentPrice: price, Change: change, ChangePercent: pct, Open: open, High: high, Low: low, PrevClose: prev, Volume: volume, Amount: amount, VolumeRatio: volumeRatio, TurnoverRate: turnoverRate, QuoteTime: providerTime, QuoteTimeSemantics: "provider_snapshot_time", FetchedAt: fetchedAt, NormalizedAt: fetchedAt, Source: p.Name(), SourceVersion: eastmoneyVersion, RawVolume: volumePtr, RawVolumeUnit: "hands", RawAmount: rawAmount, RawAmountUnit: optionalUnit(rawAmount, "CNY"), RawAmountField: optionalUnitName(rawAmount, "f48"), VolumeUnit: "shares", AmountUnit: optionalUnit(amount, "CNY"), AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}
	if reportedSymbol == "" {
		quote.QualityStatus = model.QualityDegraded
		quote.QualityReasons = append(quote.QualityReasons, "provider_symbol_missing_assumed_from_request")
	}
	if providerTime == nil {
		quote.QualityStatus = model.QualityDegraded
		quote.QualityReasons = append(quote.QualityReasons, "provider_timestamp_missing")
	}
	if name == "" {
		quote.QualityReasons = append(quote.QualityReasons, "name_missing")
		quote.QualityStatus = model.QualityDegraded
	}
	if err := normalize.ValidateQuote(quote); err != nil {
		return nil, fmt.Errorf("Eastmoney quote data contract: %w", err)
	}
	// 源端涨跌字段已通过容差校验；输出值统一以最新价和昨收价计算。
	canonicalChange := *quote.CurrentPrice - *quote.PrevClose
	canonicalChangePercent := canonicalChange / *quote.PrevClose * 100
	quote.Change = &canonicalChange
	quote.ChangePercent = &canonicalChangePercent
	return quote, nil
}

type eastmoneyChipKline struct {
	date         string
	open         float64
	close        float64
	high         float64
	low          float64
	turnoverRate float64
}

// FetchChipDistribution 在 Go 中复刻 AkShare 的 CYQCalculator 核心指标计算。
// 东方财富未提供筹码分布的独立快照时间，因此返回值保守标记为 degraded。
const (
	eastmoneyHotRankURL  = "https://emappdata.eastmoney.com/stockrank/getAllCurrentList"
	eastmoneyHotSurgeURL = "https://emappdata.eastmoney.com/stockrank/getAllHisRcList"
	eastmoneyHotAppID    = "appId01"
	eastmoneyHotGlobalID = "786e4c21-70dc-435a-93bb-38"
)

// FetchHotStocks maps the domestic Eastmoney popularity/surge ranking into a bounded, typed snapshot.
// The ranking API exposes no trustworthy snapshot timestamp, so every successful result remains degraded.
func (p *Eastmoney) FetchHotStocks(ctx context.Context, rankingType string, limit int) (*model.HotRankingSnapshot, error) {
	rankingType = strings.ToLower(strings.TrimSpace(rankingType))
	if rankingType == "" {
		rankingType = "popular"
	}
	endpoint := ""
	switch rankingType {
	case "popular":
		endpoint = strings.TrimSpace(p.hotRankingURL)
		if endpoint == "" {
			endpoint = eastmoneyHotRankURL
		}
	case "surge":
		endpoint = strings.TrimSpace(p.hotSurgeURL)
		if endpoint == "" {
			endpoint = eastmoneyHotSurgeURL
		}
	default:
		return nil, fmt.Errorf("unsupported hot-ranking type %q; want popular or surge", rankingType)
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		return nil, errors.New("Eastmoney hot-ranking limit must be between 1 and 100")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rankRows, err := p.fetchHotRankingRows(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if len(rankRows) == 0 {
		return nil, errors.New("Eastmoney hot-ranking endpoint returned an empty list")
	}
	if len(rankRows) > 1000 {
		return nil, errors.New("Eastmoney hot-ranking response exceeds 1000 rows")
	}

	type rankedCode struct {
		symbol     string
		code       string
		secid      string
		rank       int
		rankChange *int
	}
	ranked := make([]rankedCode, 0, min(limit, len(rankRows)))
	seen := make(map[string]struct{}, len(rankRows))
	for index, row := range rankRows {
		if len(ranked) >= limit {
			break
		}
		rawCode := eastmoneyHotRowString(row, "sc")
		symbol, code, secid, symbolErr := eastmoneyHotRankingSymbol(rawCode)
		if symbolErr != nil {
			return nil, fmt.Errorf("Eastmoney hot-ranking row %d has invalid security code: %w", index, symbolErr)
		}
		if _, duplicate := seen[symbol]; duplicate {
			return nil, fmt.Errorf("Eastmoney hot-ranking response contains duplicate symbol %s", symbol)
		}
		seen[symbol] = struct{}{}
		rank, ok := eastmoneyHotRowInt(row, "rk")
		if !ok || rank < 1 {
			return nil, fmt.Errorf("Eastmoney hot-ranking row %d has invalid rank", index)
		}
		var rankChange *int
		if rankingType == "surge" {
			if value, valid := eastmoneyHotRowInt(row, "hrc"); valid {
				rankChange = &value
			}
		}
		ranked = append(ranked, rankedCode{symbol: symbol, code: code, secid: secid, rank: rank, rankChange: rankChange})
	}
	if len(ranked) == 0 {
		return nil, errors.New("Eastmoney hot-ranking response contains no supported A-share symbols")
	}

	secids := make([]string, 0, len(ranked))
	for _, item := range ranked {
		secids = append(secids, item.secid)
	}
	q := url.Values{
		"ut":     []string{p.ut},
		"fltt":   []string{"2"},
		"invt":   []string{"2"},
		"fields": []string{"f14,f3,f12,f2"},
		"secids": []string{strings.Join(secids, ",")},
	}
	var quotePayload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Diff []map[string]json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if err := p.getJSONWithRetry(ctx, p.indexURL, q, &quotePayload, 8*time.Second); err != nil {
		return nil, fmt.Errorf("Eastmoney hot-ranking quote enrichment failed: %w", err)
	}
	if quotePayload.RC != nil && *quotePayload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney hot-ranking quote enrichment provider error rc=%d", *quotePayload.RC)
	}
	if quotePayload.Data == nil {
		return nil, errors.New("Eastmoney hot-ranking quote enrichment returned no data")
	}
	requestedQuoteCodes := make(map[string]struct{}, len(ranked))
	for _, item := range ranked {
		requestedQuoteCodes[item.code] = struct{}{}
	}
	quotes := make(map[string]map[string]json.RawMessage, len(quotePayload.Data.Diff))
	for index, row := range quotePayload.Data.Diff {
		code := eastmoneyHotRowString(row, "f12")
		if code == "" {
			return nil, fmt.Errorf("Eastmoney hot-ranking quote enrichment row %d is missing security code f12", index)
		}
		if _, requested := requestedQuoteCodes[code]; !requested {
			return nil, fmt.Errorf("Eastmoney hot-ranking quote enrichment returned unrequested code %s", code)
		}
		if _, duplicate := quotes[code]; duplicate {
			return nil, fmt.Errorf("Eastmoney hot-ranking quote enrichment contains duplicate code %s", code)
		}
		quotes[code] = row
	}
	now := time.Now().UTC()
	sourceVersion := eastmoneyVersion + "-hot-ranking-v1"
	snapshot := &model.HotRankingSnapshot{
		RankingType: rankingType, Items: make([]model.HotRankingItem, 0, len(ranked)),
		ProviderReturned: len(rankRows), TotalIsLowerBound: true,
		Source: p.Name(), SourceVersion: sourceVersion, FetchedAt: now,
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_ranking_snapshot_timestamp_not_exposed", "provider_total_missing_using_returned_count"},
	}
	for _, ranking := range ranked {
		item := model.HotRankingItem{
			Rank: ranking.rank, Symbol: ranking.symbol, Code: ranking.code, RankChange: ranking.rankChange,
			Source: p.Name(), SourceVersion: sourceVersion, FetchedAt: now,
			QualityStatus:  model.QualityDegraded,
			QualityReasons: []string{"provider_ranking_snapshot_timestamp_not_exposed"},
		}
		quote, exists := quotes[ranking.code]
		if !exists {
			item.QualityReasons = append(item.QualityReasons, "quote_enrichment_missing")
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "one_or_more_quote_enrichment_rows_missing")
		} else {
			item.Name = eastmoneyHotRowString(quote, "f14")
			if value, ok := eastmoneyHotRowFloat(quote, "f2"); ok && value > 0 {
				item.LatestPrice = &value
			}
			if value, ok := eastmoneyHotRowFloat(quote, "f3"); ok {
				item.ChangePercent = &value
			}
			if item.LatestPrice != nil && item.ChangePercent != nil {
				change := *item.LatestPrice * *item.ChangePercent / 100
				item.Change = &change
			}
			if item.Name == "" || item.LatestPrice == nil || item.ChangePercent == nil {
				item.QualityReasons = append(item.QualityReasons, "quote_name_or_price_fields_missing")
				snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "one_or_more_quote_fields_missing")
			}
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	snapshot.Total = len(snapshot.Items)
	return snapshot, nil
}

func (p *Eastmoney) fetchHotRankingRows(ctx context.Context, endpoint string) ([]map[string]json.RawMessage, error) {
	payload := map[string]any{
		"appId":      eastmoneyHotAppID,
		"globalId":   eastmoneyHotGlobalID,
		"marketType": "",
		"pageNo":     1,
		"pageSize":   100,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Eastmoney hot-ranking request: %w", err)
	}
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
		req, reqErr := http.NewRequestWithContext(requestCtx, http.MethodPost, endpoint, bytes.NewReader(body))
		if reqErr == nil {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Accept", "application/json, text/plain, */*")
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
			req.Header.Set("Referer", "https://guba.eastmoney.com/rank/")
			var resp *http.Response
			resp, reqErr = p.client.Do(req)
			if reqErr == nil {
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
					reqErr = fmt.Errorf("Eastmoney hot-ranking HTTP status %d", resp.StatusCode)
				} else {
					raw, readErr := readBoundedBody(resp.Body, maxProviderPayloadBytes)
					if readErr != nil {
						reqErr = fmt.Errorf("read Eastmoney hot-ranking response: %w", readErr)
					} else {
						var envelope struct {
							Code    *int                         `json:"code"`
							Message string                       `json:"message"`
							Data    []map[string]json.RawMessage `json:"data"`
						}
						if decodeErr := decodeSingleJSON(raw, &envelope); decodeErr != nil {
							reqErr = fmt.Errorf("decode Eastmoney hot-ranking response: %w", decodeErr)
						} else if envelope.Code != nil && *envelope.Code != 0 {
							reqErr = fmt.Errorf("Eastmoney hot-ranking response code=%d: %s", *envelope.Code, strings.TrimSpace(envelope.Message))
						} else if envelope.Data == nil {
							reqErr = errors.New("Eastmoney hot-ranking response is missing data")
						} else {
							_ = resp.Body.Close()
							cancel()
							return envelope.Data, nil
						}
					}
				}
				_ = resp.Body.Close()
			}
		}
		cancel()
		lastErr = reqErr
		if attempt == 3 || ctx.Err() != nil || (reqErr != nil && !isRetryableEastmoneyRequestError(reqErr)) {
			return nil, fmt.Errorf("Eastmoney hot-ranking request failed: %w", lastErr)
		}
		timer := time.NewTimer(time.Duration(attempt) * 500 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	return nil, lastErr
}

func eastmoneyHotRankingSymbol(raw string) (symbol, code, secid string, err error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if len(value) != 8 {
		return "", "", "", fmt.Errorf("unexpected stock code %q", raw)
	}
	exchange, bare := value[:2], value[2:]
	if exchange != "SH" && exchange != "SZ" {
		return "", "", "", fmt.Errorf("unsupported market prefix in %q", raw)
	}
	for _, digit := range bare {
		if digit < '0' || digit > '9' {
			return "", "", "", fmt.Errorf("non-numeric stock code %q", raw)
		}
	}
	// 热度榜契约是普通 A 股股票；仅识别交易所前缀会把 ETF、指数和可转债误作个股。
	stockCode := false
	prefixes := []string{"600", "601", "603", "605", "688", "689"}
	if exchange == "SZ" {
		prefixes = []string{"000", "001", "002", "003", "300", "301"}
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(bare, prefix) {
			stockCode = true
			break
		}
	}
	if !stockCode {
		return "", "", "", fmt.Errorf("unsupported non-equity or unrecognized stock code %q", raw)
	}
	market := "1"
	if exchange == "SZ" {
		market = "0"
	}
	return bare + "." + exchange, bare, market + "." + bare, nil
}

func eastmoneyHotRowString(row map[string]json.RawMessage, key string) string {
	raw, ok := row[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(strings.Trim(string(raw), `"`))
}

func eastmoneyHotRowFloat(row map[string]json.RawMessage, key string) (float64, bool) {
	raw := eastmoneyHotRowString(row, key)
	if raw == "" || raw == "-" {
		return 0, false
	}
	value, err := strconv.ParseFloat(raw, 64)
	return value, err == nil && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func eastmoneyHotRowInt(row map[string]json.RawMessage, key string) (int, bool) {
	value, ok := eastmoneyHotRowFloat(row, key)
	if !ok || math.Trunc(value) != value || value < -1000000 || value > 1000000 {
		return 0, false
	}
	return int(value), true
}

func appendUniqueEastmoneyReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func (p *Eastmoney) FetchChipDistribution(ctx context.Context, rawSymbol string) (*model.ChipDistribution, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Eastmoney chip distribution only supports common A-share equities, got %q", rawSymbol)
	}
	secid, err := eastmoneySecID(symbol)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := url.Values{
		"secid":   []string{secid},
		"fields1": []string{"f1,f2,f3,f4,f5,f6"},
		"fields2": []string{"f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61"},
		"klt":     []string{"101"},
		"fqt":     []string{"0"},
		"beg":     []string{"0"},
		"end":     []string{time.Now().Format("20060102")},
		"lmt":     []string{"210"},
		"ut":      []string{p.ut},
		"_":       []string{strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Code   string   `json:"code"`
			KLines []string `json:"klines"`
		} `json:"data"`
	}
	if err := p.getJSONWithRetry(ctx, p.klineURL, query, &payload, 8*time.Second); err != nil {
		return nil, fmt.Errorf("Eastmoney chip-distribution daily bars request: %w", err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney chip-distribution provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil || len(payload.Data.KLines) == 0 {
		return nil, errors.New("Eastmoney chip-distribution response has no daily bars")
	}
	if reported := strings.TrimSpace(payload.Data.Code); reported != "" {
		reportedSymbol, normalizeErr := normalize.NormalizeSymbol(reported)
		if normalizeErr != nil || reportedSymbol != symbol {
			return nil, fmt.Errorf("Eastmoney chip-distribution symbol mismatch: requested %s got %q", symbol, reported)
		}
	}
	rows := make([]eastmoneyChipKline, 0, len(payload.Data.KLines))
	seen := make(map[string]struct{}, len(payload.Data.KLines))
	for index, line := range payload.Data.KLines {
		parts := strings.Split(line, ",")
		if len(parts) < 11 {
			return nil, fmt.Errorf("Eastmoney chip-distribution row %d lacks turnover-rate field", index)
		}
		date, dateErr := time.Parse("2006-01-02", strings.TrimSpace(parts[0]))
		if dateErr != nil {
			return nil, fmt.Errorf("Eastmoney chip-distribution row %d has invalid date", index)
		}
		dateText := date.Format("2006-01-02")
		if _, exists := seen[dateText]; exists {
			return nil, fmt.Errorf("Eastmoney chip-distribution has duplicate date %s", dateText)
		}
		seen[dateText] = struct{}{}
		values := make([]float64, 0, 5)
		for _, field := range []string{parts[1], parts[2], parts[3], parts[4], parts[10]} {
			value, parseErr := strconv.ParseFloat(strings.TrimSpace(field), 64)
			if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("Eastmoney chip-distribution row %s has invalid price/turnover data", dateText)
			}
			values = append(values, value)
		}
		open, closeValue, high, low, turnoverRate := values[0], values[1], values[2], values[3], values[4]
		if open <= 0 || closeValue <= 0 || high <= 0 || low <= 0 || low > high ||
			open < low || open > high || closeValue < low || closeValue > high || turnoverRate < 0 {
			return nil, fmt.Errorf("Eastmoney chip-distribution row %s violates OHLC/turnover constraints", dateText)
		}
		rows = append(rows, eastmoneyChipKline{date: dateText, open: open, close: closeValue, high: high, low: low, turnoverRate: turnoverRate})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].date < rows[j].date })
	if len(rows) > 120 {
		rows = rows[len(rows)-120:]
	}
	metrics, err := calculateEastmoneyChipMetrics(rows)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &model.ChipDistribution{
		Symbol: symbol, Date: rows[len(rows)-1].date, ProviderDate: rows[len(rows)-1].date,
		AverageCost: metrics.averageCost, ProfitRatio: &metrics.profitRatio,
		Cost90Low: &metrics.cost90Low, Cost90High: &metrics.cost90High,
		Cost70Low: &metrics.cost70Low, Cost70High: &metrics.cost70High,
		Concentration90: &metrics.concentration90, Concentration70: &metrics.concentration70,
		Source: p.Name(), SourceVersion: "eastmoney-cyq-go-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_chip_snapshot_timestamp_not_exposed", "chip_metrics_computed_from_latest_120_daily_bars"},
	}, nil
}

type eastmoneyChipMetrics struct {
	averageCost     float64
	profitRatio     float64
	cost90Low       float64
	cost90High      float64
	concentration90 float64
	cost70Low       float64
	cost70High      float64
	concentration70 float64
}

func calculateEastmoneyChipMetrics(rows []eastmoneyChipKline) (eastmoneyChipMetrics, error) {
	if len(rows) == 0 {
		return eastmoneyChipMetrics{}, errors.New("cannot calculate chip distribution from an empty series")
	}
	minPrice, maxPrice := rows[0].low, rows[0].high
	for _, row := range rows[1:] {
		if row.low < minPrice {
			minPrice = row.low
		}
		if row.high > maxPrice {
			maxPrice = row.high
		}
	}
	const factor = 150
	accuracy := math.Max(0.01, (maxPrice-minPrice)/float64(factor-1))
	chips := make([]float64, factor)
	for _, row := range rows {
		averagePrice := (row.open + row.close + row.high + row.low) / 4
		turnover := math.Min(1, row.turnoverRate/100)
		if turnover < 0 || math.IsNaN(turnover) || math.IsInf(turnover, 0) {
			return eastmoneyChipMetrics{}, errors.New("invalid turnover ratio while calculating chip distribution")
		}
		for i := range chips {
			chips[i] *= 1 - turnover
		}
		if row.high == row.low {
			index := int(math.Floor((averagePrice - minPrice) / accuracy))
			if index >= 0 && index < factor {
				chips[index] += float64(factor-1) * turnover / 2
			}
			continue
		}
		highIndex := int(math.Floor((row.high - minPrice) / accuracy))
		lowIndex := int(math.Ceil((row.low - minPrice) / accuracy))
		groupFactor := 2 / (row.high - row.low)
		for j := lowIndex; j <= highIndex; j++ {
			if j < 0 || j >= factor {
				continue
			}
			currentPrice := minPrice + accuracy*float64(j)
			if currentPrice <= averagePrice {
				if math.Abs(averagePrice-row.low) < 1e-8 {
					chips[j] += groupFactor * turnover
				} else {
					chips[j] += (currentPrice - row.low) / (averagePrice - row.low) * groupFactor * turnover
				}
			} else {
				if math.Abs(row.high-averagePrice) < 1e-8 {
					chips[j] += groupFactor * turnover
				} else {
					chips[j] += (row.high - currentPrice) / (row.high - averagePrice) * groupFactor * turnover
				}
			}
		}
	}
	total := 0.0
	for i := range chips {
		if chips[i] < 0 || math.IsNaN(chips[i]) || math.IsInf(chips[i], 0) {
			return eastmoneyChipMetrics{}, errors.New("chip distribution produced an invalid density")
		}
		total += chips[i]
	}
	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return eastmoneyChipMetrics{}, errors.New("chip distribution has no accumulated turnover density")
	}
	costFor := func(target float64) float64 {
		accumulated := 0.0
		for i, value := range chips {
			if accumulated+value > target {
				return minPrice + float64(i)*accuracy
			}
			accumulated += value
		}
		return 0
	}
	out := eastmoneyChipMetrics{averageCost: costFor(total * 0.5)}
	if out.averageCost <= 0 || math.IsNaN(out.averageCost) || math.IsInf(out.averageCost, 0) {
		return eastmoneyChipMetrics{}, errors.New("chip distribution could not determine average cost")
	}
	below := 0.0
	currentPrice := rows[len(rows)-1].close
	for i, value := range chips {
		if currentPrice >= minPrice+float64(i)*accuracy {
			below += value
		}
	}
	out.profitRatio = below / total
	out.cost90Low = costFor(total * 0.05)
	out.cost90High = costFor(total * 0.95)
	out.cost70Low = costFor(total * 0.15)
	out.cost70High = costFor(total * 0.85)
	if out.cost90Low <= 0 || out.cost90High <= 0 || out.cost70Low <= 0 || out.cost70High <= 0 {
		return eastmoneyChipMetrics{}, errors.New("chip distribution could not determine cost percentile bounds")
	}
	if denominator := out.cost90High + out.cost90Low; denominator > 0 {
		out.concentration90 = (out.cost90High - out.cost90Low) / denominator
	}
	if denominator := out.cost70High + out.cost70Low; denominator > 0 {
		out.concentration70 = (out.cost70High - out.cost70Low) / denominator
	}
	if out.profitRatio < 0 || out.profitRatio > 1 || out.concentration90 < 0 || out.concentration90 > 1 || out.concentration70 < 0 || out.concentration70 > 1 {
		return eastmoneyChipMetrics{}, errors.New("chip distribution produced ratio outside [0,1]")
	}
	return out, nil
}

func (p *Eastmoney) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Eastmoney daily bars only support common A-share equities, got %q", rawSymbol)
	}
	secid, err := eastmoneySecID(symbol)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	beg, end, err := eastmoneyDateRange(startDate, endDate, limit, time.Now())
	if err != nil {
		return nil, err
	}
	fqt := "1"
	basis := adjustment
	switch adjustment {
	case model.AdjustmentForward:
		fqt = "1"
	case model.AdjustmentBackward:
		fqt = "2"
	case model.AdjustmentNone:
		fqt = "0"
	default:
		return nil, fmt.Errorf("unsupported adjustment basis %q", adjustment)
	}
	q := url.Values{}
	q.Set("secid", secid)
	q.Set("fields1", "f1,f2,f3,f4,f5,f6")
	q.Set("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61")
	q.Set("klt", "101")
	q.Set("fqt", fqt)
	q.Set("beg", beg)
	q.Set("end", end)
	q.Set("lmt", strconv.Itoa(limit))
	q.Set("ut", p.ut)
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Code   string   `json:"code"`
			Name   string   `json:"name"`
			KLines []string `json:"klines"`
		} `json:"data"`
	}
	if err := p.getJSON(ctx, p.klineURL, q, &payload); err != nil {
		return nil, err
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney daily-bars provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil {
		return nil, errors.New("eastmoney returned empty daily bars")
	}
	reported := strings.TrimSpace(payload.Data.Code)
	reportedSymbolMissing := reported == ""
	if !reportedSymbolMissing {
		reportedSymbol, normalizeErr := normalize.NormalizeSymbol(reported)
		if normalizeErr != nil || reportedSymbol != symbol {
			return nil, fmt.Errorf("Eastmoney daily-bars symbol mismatch: requested %s got %q", symbol, reported)
		}
	}
	out := make([]model.DailyBar, 0, len(payload.Data.KLines))
	rejectedRows := 0
	for _, line := range payload.Data.KLines {
		parts := strings.Split(line, ",")
		if len(parts) < 7 {
			rejectedRows++
			continue
		}
		open, e1 := strconv.ParseFloat(parts[1], 64)
		closeV, e2 := strconv.ParseFloat(parts[2], 64)
		high, e3 := strconv.ParseFloat(parts[3], 64)
		low, e4 := strconv.ParseFloat(parts[4], 64)
		rawVol, e5 := strconv.ParseFloat(parts[5], 64)
		amount, e6 := strconv.ParseFloat(parts[6], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil ||
			!finiteNumber(open) || !finiteNumber(closeV) || !finiteNumber(high) || !finiteNumber(low) || !finiteNumber(amount) || amount < 0 {
			rejectedRows++
			continue
		}
		shares, conversionErr := normalize.HandsToShares(rawVol)
		if conversionErr != nil {
			rejectedRows++
			continue
		}
		amountValue := amount
		rawVolume := rawVol
		rawAmount := amount
		bar := model.DailyBar{Symbol: symbol, TradeDate: parts[0], Period: "1d", Open: open, High: high, Low: low, Close: closeV, Volume: &shares, Amount: &amountValue, AdjustmentBasis: basis, Source: p.Name(), SourceVersion: eastmoneyVersion, FetchedAt: time.Now().UTC(), NormalizedAt: time.Now().UTC(), RawVolume: &rawVolume, RawVolumeUnit: "hands", RawAmount: &rawAmount, RawAmountUnit: "CNY", RawAmountField: "kline_field_6", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}
		if len(parts) > 8 {
			if pct, e := strconv.ParseFloat(parts[8], 64); e == nil {
				bar.ChangePercent = &pct
			}
		}
		out = append(out, bar)
	}
	// 使用实际发给上游的查询区间（包括自动计算的回溯起始日）过滤结果。
	startISO := beg[:4] + "-" + beg[4:6] + "-" + beg[6:]
	endISO := end[:4] + "-" + end[4:6] + "-" + end[6:]
	bars, normalizeErr := normalizeProviderDailyBars(symbol, startISO, endISO, limit, out, rejectedRows)
	if normalizeErr != nil {
		return nil, normalizeErr
	}
	if reportedSymbolMissing {
		for index := range bars {
			if bars[index].QualityStatus == model.QualityValid || bars[index].QualityStatus == model.QualityUnknown || bars[index].QualityStatus == "" {
				bars[index].QualityStatus = model.QualityDegraded
			}
			bars[index].QualityReasons = appendProviderQualityReason(bars[index].QualityReasons, "provider_symbol_missing_assumed_from_request")
		}
	}
	return bars, nil
}

// eastmoneyDateRange preserves explicit caller ranges. When no start date is
// specified, it asks for a bounded lookback large enough to normally cover the
// requested trading-bar limit, while avoiding an unnecessary all-history scan.
// Four calendar days per requested bar plus a 30-day buffer accommodates
// weekends, exchange holidays, and short suspensions without relying on lmt alone.
func eastmoneyDateRange(startDate, endDate string, limit int, now time.Time) (string, string, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}

	end := now.In(loc)
	if endDate != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02", endDate, loc)
		if parseErr != nil {
			return "", "", fmt.Errorf("invalid end date: %w", parseErr)
		}
		end = parsed
	}

	start := end.AddDate(0, 0, -(limit*eastmoneyLookbackDaysPerBar + eastmoneyLookbackBufferDays))
	if startDate != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02", startDate, loc)
		if parseErr != nil {
			return "", "", fmt.Errorf("invalid start date: %w", parseErr)
		}
		start = parsed
	}
	if start.After(end) {
		return "", "", errors.New("start date must not be after end date")
	}
	return start.Format("20060102"), end.Format("20060102"), nil
}

func (p *Eastmoney) getJSON(ctx context.Context, base string, params url.Values, out any) error {
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://quote.eastmoney.com/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("eastmoney request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("eastmoney HTTP status %d", resp.StatusCode)
	}
	body, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil {
		return fmt.Errorf("read Eastmoney payload: %w", err)
	}
	if err := decodeSingleJSON(body, out); err != nil {
		return fmt.Errorf("decode Eastmoney JSON: %w", err)
	}
	return nil
}

func (p *Eastmoney) getJSONWithRetry(ctx context.Context, base string, params url.Values, out any, attemptTimeout time.Duration) error {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		requestCtx, cancel := context.WithTimeout(ctx, attemptTimeout)
		err := p.getJSON(requestCtx, base, params, out)
		cancel()
		if err == nil {
			return nil
		}
		lastErr = err
		if attempt == maxAttempts || ctx.Err() != nil || !isRetryableEastmoneyRequestError(err) {
			return err
		}
		// Public quote gateways can briefly rate-limit large market-list pages. Spread retries
		// out rather than issuing a burst of requests to the same redirected host.
		timer := time.NewTimer(time.Duration(attempt) * 700 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return lastErr
}

func isRetryableEastmoneyRequestError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, fragment := range []string{"http status 429", "http status 500", "http status 502", "http status 503", "http status 504", "timeout", "timed out", "deadline exceeded", "unexpected eof", "connection reset", "connection refused", "server closed", "temporary"} {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

func eastmoneySecID(symbol string) (string, error) {
	p := strings.Split(symbol, ".")
	code := p[0]
	exchange := ""
	if len(p) == 2 {
		exchange = p[1]
	}
	if exchange == "" {
		symbol2, err := normalize.NormalizeSymbol(code)
		if err != nil {
			return "", err
		}
		exchange = strings.Split(symbol2, ".")[1]
	}
	switch exchange {
	case "SH":
		return "1." + code, nil
	case "SZ":
		return "0." + code, nil
	case "BJ":
		return "0." + code, nil
	default:
		return "", fmt.Errorf("unsupported A-share exchange %q", exchange)
	}
}

func parseEastmoneyTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	// 常见响应将 f86 编码为上海本地时间；遇到数字型 Unix 时间戳时再按秒/毫秒处理。
	for _, layout := range []string{"20060102150405", "200601021504", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, parseErr := time.ParseInLocation(layout, s, loc); parseErr == nil {
			t = t.UTC()
			return &t
		}
	}
	if epoch, parseErr := strconv.ParseInt(s, 10, 64); parseErr == nil {
		var t time.Time
		switch len(s) {
		case 10:
			t = time.Unix(epoch, 0).UTC()
		case 13:
			t = time.UnixMilli(epoch).UTC()
		default:
			return nil
		}
		// 避免把任意十位数字或明显错误的时间值当成有效报价时间。
		if t.Year() < 2000 || t.After(time.Now().Add(24*time.Hour)) {
			return nil
		}
		return &t
	}
	return nil
}

func finiteNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// eastmoneyRedirectPolicy 只跟随 HTTPS 下的东方财富自有域名重定向，兼容 push2 -> push2delay 的行情调度。
func eastmoneyRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return errors.New("Eastmoney redirect limit exceeded")
	}
	if req.URL.Scheme == "https" && isEastmoneyHostname(req.URL.Hostname()) {
		return nil
	}
	// 对外部域名和 HTTPS 降级到 HTTP 的跳转，不发送第二次请求。
	return http.ErrUseLastResponse
}

func isEastmoneyHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return host == "eastmoney.com" || strings.HasSuffix(host, ".eastmoney.com")
}

const (
	eastmoneyStockSearchIndexTTL = 6 * time.Hour
	// Live provider testing shows the CLIST endpoint returns at most 100 rows per page,
	// even when a much larger pz is requested. Walk every source page within a safety cap.
	eastmoneyStockSearchPageSize   = 100
	eastmoneyStockSearchMaxPages   = 100
	eastmoneyStockSearchRetryDelay = 30 * time.Second
)

// SearchStocks searches Eastmoney's real A-share security directory by Chinese name or code.
// The directory is cached for six hours so autocomplete does not trigger a full-market request
// on every keystroke. The response intentionally contains identity only, not quote data.
func (p *Eastmoney) SearchStocks(ctx context.Context, rawQuery string, limit int) (*model.StockSearchSnapshot, error) {
	query := strings.TrimSpace(rawQuery)
	if query == "" {
		return nil, errors.New("stock-search query is required")
	}
	if len([]rune(query)) > 64 {
		return nil, errors.New("stock-search query must not exceed 64 characters")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		return nil, errors.New("stock-search limit must not exceed 50")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	items, fetchedAt, quality, reasons, stale, err := p.loadStockSearchIndex(ctx)
	if err != nil {
		return nil, fmt.Errorf("eastmoney stock-search directory: %w", err)
	}
	needle := strings.ToLower(query)
	type rankedItem struct {
		item model.StockSearchItem
		rank int
	}
	matches := make([]rankedItem, 0)
	for _, item := range items {
		name := strings.ToLower(item.Name)
		code := strings.ToLower(item.Code)
		symbol := strings.ToLower(item.Symbol)
		rank := -1
		switch {
		case name == needle || code == needle || symbol == needle:
			rank = 0
		case strings.HasPrefix(name, needle) || strings.HasPrefix(code, needle) || strings.HasPrefix(symbol, needle):
			rank = 1
		case strings.Contains(name, needle) || strings.Contains(code, needle) || strings.Contains(symbol, needle):
			rank = 2
		}
		if rank >= 0 {
			matches = append(matches, rankedItem{item: item, rank: rank})
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].rank != matches[j].rank {
			return matches[i].rank < matches[j].rank
		}
		if matches[i].item.Name != matches[j].item.Name {
			return matches[i].item.Name < matches[j].item.Name
		}
		return matches[i].item.Symbol < matches[j].item.Symbol
	})
	if limit > len(matches) {
		limit = len(matches)
	}
	results := make([]model.StockSearchItem, 0, limit)
	for _, match := range matches[:limit] {
		results = append(results, match.item)
	}
	if stale {
		quality = model.QualityStale
		reasons = appendUniqueEastmoneyReason(reasons, "provider_refresh_failed_cached_directory_used")
	}
	if quality == "" {
		quality = model.QualityUnknown
	}
	return &model.StockSearchSnapshot{
		Query: query, Total: len(matches), Items: results,
		Source: p.Name(), SourceVersion: "eastmoney-stock-search-clist-v1",
		FetchedAt: fetchedAt, QualityStatus: quality, QualityReasons: reasons,
	}, nil
}

func (p *Eastmoney) loadStockSearchIndex(ctx context.Context) ([]model.StockSearchItem, time.Time, model.QualityStatus, []string, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, time.Time{}, model.QualityUnknown, nil, false, err
		}
		p.stockSearchMu.Lock()
		cached := append([]model.StockSearchItem(nil), p.stockSearchIndex...)
		fetchedAt := p.stockSearchFetchedAt
		quality := p.stockSearchQuality
		reasons := append([]string(nil), p.stockSearchReasons...)
		age := time.Since(fetchedAt)
		fresh := len(cached) > 0 && !fetchedAt.IsZero() && age >= 0 && age < eastmoneyStockSearchIndexTTL
		if fresh {
			p.stockSearchMu.Unlock()
			return cached, fetchedAt, quality, reasons, false, nil
		}
		if loading := p.stockSearchLoading; loading != nil {
			p.stockSearchMu.Unlock()
			select {
			case <-ctx.Done():
				return nil, time.Time{}, model.QualityUnknown, nil, false, ctx.Err()
			case <-loading:
				continue
			}
		}
		if time.Now().Before(p.stockSearchRetryAfter) {
			lastError := p.stockSearchLastError
			p.stockSearchMu.Unlock()
			if len(cached) > 0 {
				return cached, fetchedAt, model.QualityStale, appendUniqueEastmoneyReason(reasons, "provider_refresh_failed_cached_directory_used"), true, nil
			}
			if lastError == "" {
				lastError = "stock-search directory refresh is cooling down"
			}
			return nil, time.Time{}, model.QualityUnknown, nil, false, errors.New(lastError)
		}
		loading := make(chan struct{})
		p.stockSearchLoading = loading
		p.stockSearchMu.Unlock()

		items, loadedQuality, loadedReasons, err := p.fetchStockSearchIndex(ctx)
		now := time.Now().UTC()

		p.stockSearchMu.Lock()
		if err == nil {
			p.stockSearchIndex = append([]model.StockSearchItem(nil), items...)
			p.stockSearchFetchedAt = now
			p.stockSearchQuality = loadedQuality
			p.stockSearchReasons = append([]string(nil), loadedReasons...)
			p.stockSearchRetryAfter = time.Time{}
			p.stockSearchLastError = ""
		} else {
			p.stockSearchRetryAfter = now.Add(eastmoneyStockSearchRetryDelay)
			p.stockSearchLastError = err.Error()
		}
		cached = append([]model.StockSearchItem(nil), p.stockSearchIndex...)
		fetchedAt = p.stockSearchFetchedAt
		quality = p.stockSearchQuality
		reasons = append([]string(nil), p.stockSearchReasons...)
		p.stockSearchLoading = nil
		close(loading)
		p.stockSearchMu.Unlock()

		if err != nil {
			if len(cached) > 0 && !fetchedAt.IsZero() {
				return cached, fetchedAt, model.QualityStale, appendUniqueEastmoneyReason(reasons, "provider_refresh_failed_cached_directory_used"), true, nil
			}
			return nil, time.Time{}, model.QualityUnknown, nil, false, err
		}
		return items, now, quality, loadedReasons, false, nil
	}
}

func (p *Eastmoney) fetchStockSearchIndex(ctx context.Context) ([]model.StockSearchItem, model.QualityStatus, []string, error) {
	baseURL := strings.TrimSpace(p.stockListURL)
	if baseURL == "" {
		baseURL = defaultEastmoneySectorRankingURL
	}
	items := make([]model.StockSearchItem, 0, 6000)
	seen := make(map[string]struct{}, 6000)
	reasons := make([]string, 0)
	quality := model.QualityValid
	total := 0
	for page := 1; page <= eastmoneyStockSearchMaxPages; page++ {
		params := url.Values{
			"pn": {strconv.Itoa(page)}, "pz": {strconv.Itoa(eastmoneyStockSearchPageSize)},
			"po": {"1"}, "np": {"1"}, "ut": {p.ut}, "fltt": {"2"}, "invt": {"2"},
			"fid": {"f3"}, "fs": {"m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048"},
			"fields": {"f12,f13,f14"}, "_": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}
		var payload struct {
			RC   *int `json:"rc"`
			Data *struct {
				Total int             `json:"total"`
				Diff  json.RawMessage `json:"diff"`
			} `json:"data"`
		}
		if err := p.getJSONWithRetry(ctx, baseURL, params, &payload, 6*time.Second); err != nil {
			return nil, model.QualityUnknown, nil, err
		}
		if payload.RC != nil && *payload.RC != 0 {
			return nil, model.QualityUnknown, nil, fmt.Errorf("Eastmoney stock-search provider error rc=%d", *payload.RC)
		}
		if payload.Data == nil {
			return nil, model.QualityUnknown, nil, errors.New("Eastmoney stock-search response has no data")
		}
		if page == 1 {
			total = payload.Data.Total
			if total <= 0 {
				return nil, model.QualityUnknown, nil, errors.New("Eastmoney stock-search directory is empty")
			}
			if total > eastmoneyStockSearchPageSize*eastmoneyStockSearchMaxPages {
				return nil, model.QualityUnknown, nil, fmt.Errorf("Eastmoney stock-search directory total %d exceeds safety limit", total)
			}
		} else if payload.Data.Total != total {
			// 全量目录没有源端快照令牌；分页总数漂移表示无法证明各页属于同一完整目录。
			return nil, model.QualityUnknown, nil, fmt.Errorf(
				"Eastmoney stock-search directory total changed across pages: first=%d page=%d got=%d",
				total, page, payload.Data.Total,
			)
		}
		expectedRows := total - (page-1)*eastmoneyStockSearchPageSize
		if expectedRows > eastmoneyStockSearchPageSize {
			expectedRows = eastmoneyStockSearchPageSize
		}
		if expectedRows <= 0 {
			return nil, model.QualityUnknown, nil, fmt.Errorf(
				"Eastmoney stock-search page %d exceeds the declared directory total %d", page, total,
			)
		}
		rows, err := decodeEastmoneyBoardRows(payload.Data.Diff)
		if err != nil {
			return nil, model.QualityUnknown, nil, fmt.Errorf("decode Eastmoney stock-search rows: %w", err)
		}
		if len(rows) > expectedRows {
			return nil, model.QualityUnknown, nil, fmt.Errorf(
				"Eastmoney stock-search page %d returned %d rows, exceeding %d rows implied by total %d",
				page, len(rows), expectedRows, total,
			)
		}
		if len(rows) == 0 {
			if page == 1 {
				return nil, model.QualityUnknown, nil, errors.New("Eastmoney stock-search directory returned no rows")
			}
			quality = model.QualityDegraded
			reasons = appendUniqueEastmoneyReason(reasons, "provider_directory_page_missing")
			break
		}
		for _, row := range rows {
			code := parseEastmoneyBoardString(row, "f12")
			name := parseEastmoneyBoardString(row, "f14")
			market := parseEastmoneyBoardString(row, "f13")
			if code == "" || name == "" || (market != "0" && market != "1") {
				quality = model.QualityDegraded
				reasons = appendUniqueEastmoneyReason(reasons, "provider_rows_dropped_after_validation")
				continue
			}
			suffix := ".SZ"
			if market == "1" {
				suffix = ".SH"
			}
			if strings.HasPrefix(code, "8") || strings.HasPrefix(code, "4") || strings.HasPrefix(code, "920") {
				suffix = ".BJ"
			}
			symbol, normalizeErr := normalize.NormalizeSymbol(code + suffix)
			if normalizeErr != nil {
				quality = model.QualityDegraded
				reasons = appendUniqueEastmoneyReason(reasons, "provider_rows_dropped_after_validation")
				continue
			}
			parts := strings.Split(symbol, ".")
			if len(parts) != 2 {
				quality = model.QualityDegraded
				reasons = appendUniqueEastmoneyReason(reasons, "provider_rows_dropped_after_validation")
				continue
			}
			if !isTushareAShareSymbol(symbol) {
				quality = model.QualityDegraded
				reasons = appendUniqueEastmoneyReason(reasons, "provider_non_a_share_security_rows_dropped")
				continue
			}
			if _, exists := seen[symbol]; exists {
				quality = model.QualityDegraded
				reasons = appendUniqueEastmoneyReason(reasons, "provider_duplicate_symbols_dropped")
				continue
			}
			seen[symbol] = struct{}{}
			items = append(items, model.StockSearchItem{Symbol: symbol, Code: parts[0], Name: name, Exchange: parts[1]})
		}
		if page*eastmoneyStockSearchPageSize >= total {
			break
		}
		if len(rows) < eastmoneyStockSearchPageSize {
			quality = model.QualityDegraded
			reasons = appendUniqueEastmoneyReason(reasons, "provider_directory_incomplete")
			break
		}
	}
	if len(items) == 0 {
		return nil, model.QualityUnknown, nil, errors.New("Eastmoney stock-search directory contained no valid A-share rows")
	}
	if len(items) < total {
		quality = model.QualityDegraded
		reasons = appendUniqueEastmoneyReason(reasons, "provider_rows_dropped_or_directory_incomplete")
	}
	return items, quality, reasons, nil
}

func (p *Eastmoney) FetchSectorRankings(ctx context.Context, rawCategory string, limit int) (*model.SectorRankingSnapshot, error) {
	return p.FetchSectorRankingsDirection(ctx, rawCategory, "up", limit)
}

func (p *Eastmoney) FetchSectorRankingsDirection(ctx context.Context, rawCategory, rawDirection string, limit int) (*model.SectorRankingSnapshot, error) {
	category := strings.ToLower(strings.TrimSpace(rawCategory))
	filters := map[string]string{
		"industry": "m:90+t:2+f:!50",
		"concept":  "m:90+t:3+f:!50",
	}
	filter, ok := filters[category]
	if !ok {
		return nil, fmt.Errorf("unsupported sector category %q; want industry or concept", rawCategory)
	}
	direction := strings.ToLower(strings.TrimSpace(rawDirection))
	if direction == "" {
		direction = "up"
	}
	if direction != "up" && direction != "down" {
		return nil, fmt.Errorf("unsupported sector ranking direction %q; want up or down", rawDirection)
	}
	sortOrder := "1"
	if direction == "down" {
		sortOrder = "0"
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		return nil, errors.New("sector ranking limit must not exceed 100")
	}
	params := url.Values{
		"pn": {"1"}, "pz": {strconv.Itoa(limit)}, "po": {sortOrder}, "np": {"1"},
		"ut": {p.ut}, "fltt": {"2"}, "invt": {"2"}, "fid": {"f3"},
		"fs": {filter}, "fields": {"f2,f3,f4,f6,f12,f14,f104,f105,f128,f136"},
		"_": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Total *int `json:"total"`
			Diff  []map[string]json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if err := p.getJSONWithRetry(ctx, p.sectorRankingURL, params, &payload, 6*time.Second); err != nil {
		return nil, fmt.Errorf("eastmoney sector ranking request: %w", err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney sector ranking provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil || len(payload.Data.Diff) == 0 {
		return nil, errors.New("Eastmoney sector ranking returned no rows")
	}
	reportedTotal := 0
	sourceTotalKnown := payload.Data.Total != nil
	if sourceTotalKnown {
		reportedTotal = *payload.Data.Total
		if reportedTotal < 0 {
			return nil, fmt.Errorf("Eastmoney sector ranking total is negative: %d", reportedTotal)
		}
		if reportedTotal == 0 {
			return nil, errors.New("Eastmoney sector ranking reports total=0 with non-empty rows")
		}
		expectedRows := limit
		if reportedTotal < expectedRows {
			expectedRows = reportedTotal
		}
		if len(payload.Data.Diff) != expectedRows {
			return nil, fmt.Errorf(
				"Eastmoney sector ranking returned %d rows, expected %d rows from total=%d and limit=%d",
				len(payload.Data.Diff), expectedRows, reportedTotal, limit,
			)
		}
	}
	fetchedAt := time.Now().UTC()
	snapshot := &model.SectorRankingSnapshot{
		Category: category, Direction: direction, Total: reportedTotal,
		ProviderReturned: len(payload.Data.Diff), TotalIsLowerBound: !sourceTotalKnown,
		Source: p.Name(), SourceVersion: "eastmoney-sector-clist-v1",
		FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		Items: make([]model.SectorRanking, 0, len(payload.Data.Diff)),
	}
	for _, raw := range payload.Data.Diff {
		code := parseEastmoneyBoardString(raw, "f12")
		name := parseEastmoneyBoardString(raw, "f14")
		changePercent := parseEastmoneyBoardNumber(raw, "f3")
		if code == "" || name == "" || changePercent == nil {
			snapshot.ExcludedRows++
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_rows_failed_required_field_validation")
			continue
		}
		reasons := []string{"provider_snapshot_timestamp_not_exposed"}
		amount := parseEastmoneyBoardNumber(raw, "f6")
		amountUnit := "CNY"
		if amount == nil {
			amountUnit = "unknown"
			reasons = append(reasons, "provider_amount_missing")
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_amount_missing")
		}
		snapshot.Items = append(snapshot.Items, model.SectorRanking{
			Rank: len(snapshot.Items) + 1, Code: code, Name: name, Category: category,
			LatestPrice: parseEastmoneyBoardNumber(raw, "f2"), Change: parseEastmoneyBoardNumber(raw, "f4"),
			ChangePercent: changePercent, Amount: amount, AmountUnit: amountUnit,
			AdvancingCount: parseEastmoneyBoardInteger(raw, "f104"), DecliningCount: parseEastmoneyBoardInteger(raw, "f105"),
			LeadingStock: parseEastmoneyBoardString(raw, "f128"), LeaderChangePercent: parseEastmoneyBoardNumber(raw, "f136"),
			Source: p.Name(), SourceVersion: snapshot.SourceVersion, FetchedAt: fetchedAt,
			QualityStatus: model.QualityDegraded, QualityReasons: reasons,
		})
	}
	if len(snapshot.Items) == 0 {
		return nil, errors.New("Eastmoney sector ranking rows failed code/name/change-percent validation")
	}
	if snapshot.Total == 0 {
		// 某些响应可能缺少总数；用已解析条数作为下界，但明确标记总量未知。
		snapshot.Total = len(snapshot.Items)
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_total_missing_using_returned_count")
	}
	if len(snapshot.Items) < len(payload.Data.Diff) {
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_rows_dropped_after_validation")
	}
	if err := sortSectorRankingSnapshot(snapshot, direction); err != nil {
		return nil, err
	}
	return snapshot, nil
}
// decodeEastmoneyBoardRows accepts array payloads and Eastmoney's indexed-object shape
// ({"0": {...}, "1": {...}}). Numeric index ordering is preserved explicitly because
// Go map iteration order is intentionally unspecified.
func decodeEastmoneyBoardRows(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return nil, nil
	}
	switch value[0] {
	case '[':
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &rows); err != nil {
			return nil, err
		}
		return rows, nil
	case '{':
		var indexed map[string]json.RawMessage
		if err := json.Unmarshal(raw, &indexed); err != nil {
			return nil, err
		}
		keys := make([]string, 0, len(indexed))
		for key := range indexed {
			if _, err := strconv.Atoi(key); err != nil {
				return nil, fmt.Errorf("unexpected non-numeric row key %q", key)
			}
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			a, _ := strconv.Atoi(keys[i])
			b, _ := strconv.Atoi(keys[j])
			return a < b
		})
		rows := make([]map[string]json.RawMessage, 0, len(keys))
		for _, key := range keys {
			var row map[string]json.RawMessage
			if err := json.Unmarshal(indexed[key], &row); err != nil {
				return nil, fmt.Errorf("row %s: %w", key, err)
			}
			if row == nil {
				return nil, fmt.Errorf("row %s is not a JSON object", key)
			}
			rows = append(rows, row)
		}
		return rows, nil
	default:
		return nil, fmt.Errorf("expected JSON array or indexed object")
	}
}

func parseEastmoneyBoardString(row map[string]json.RawMessage, key string) string {
	raw, ok := row[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(strings.Trim(string(raw), `"`))
}

func parseEastmoneyBoardNumber(row map[string]json.RawMessage, key string) *float64 {
	raw, ok := row[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err == nil {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil
		}
		return &value
	}
	var textValue string
	if err := json.Unmarshal(raw, &textValue); err != nil {
		textValue = strings.TrimSpace(strings.Trim(string(raw), `"`))
	}
	textValue = strings.TrimSpace(textValue)
	if textValue == "" || textValue == "-" || strings.EqualFold(textValue, "null") {
		return nil
	}
	value, err := strconv.ParseFloat(textValue, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func parseEastmoneyBoardInteger(row map[string]json.RawMessage, key string) *int {
	value := parseEastmoneyBoardNumber(row, key)
	if value == nil || *value < 0 || math.Trunc(*value) != *value {
		return nil
	}
	// 计数字段必须是非负整数；同时限制在浮点数可精确表示的整数范围内，
	// 避免小数被截断或超大值转换为 int 时发生精度/溢出问题。
	const maxExactInteger = 1 << 53
	maxInt := int(^uint(0) >> 1)
	if *value >= maxExactInteger || *value > float64(maxInt) {
		return nil
	}
	integer := int(*value)
	return &integer
}

type eastmoneyIndexDefinition struct {
	Symbol string
	Code   string
	SecID  string
	Market string
	Name   string
}

const eastmoneyIndexChangeTolerance = 0.05
const eastmoneyIndexChangePercentTolerancePP = 0.15

var eastmoneyCoreIndexDefinitions = []eastmoneyIndexDefinition{
	{Symbol: "000001.SH", Code: "000001", SecID: "1.000001", Market: "1", Name: "上证指数"},
	{Symbol: "399001.SZ", Code: "399001", SecID: "0.399001", Market: "0", Name: "深证成指"},
	{Symbol: "399006.SZ", Code: "399006", SecID: "0.399006", Market: "0", Name: "创业板指"},
	{Symbol: "000300.SH", Code: "000300", SecID: "1.000300", Market: "1", Name: "沪深300"},
	{Symbol: "000016.SH", Code: "000016", SecID: "1.000016", Market: "1", Name: "上证50"},
	{Symbol: "000905.SH", Code: "000905", SecID: "1.000905", Market: "1", Name: "中证500"},
	{Symbol: "000688.SH", Code: "000688", SecID: "1.000688", Market: "1", Name: "科创50"},
}

func (p *Eastmoney) FetchIndexQuotes(ctx context.Context) (*model.IndexQuoteSnapshot, error) {
	secids := make([]string, 0, len(eastmoneyCoreIndexDefinitions))
	definitions := make(map[string]eastmoneyIndexDefinition, len(eastmoneyCoreIndexDefinitions))
	for _, index := range eastmoneyCoreIndexDefinitions {
		secids = append(secids, index.SecID)
		definitions[index.Market+"."+index.Code] = index
	}
	params := url.Values{
		"secids": {strings.Join(secids, ",")},
		"fields": {"f2,f3,f4,f6,f12,f13,f14,f15,f16,f17,f18,f86"},
		"fltt":   {"2"}, "invt": {"2"}, "ut": {p.ut},
		"_": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Total *int                         `json:"total"`
			Diff  []map[string]json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if err := p.getJSON(ctx, p.indexURL, params, &payload); err != nil {
		return nil, fmt.Errorf("Eastmoney index quote request: %w", err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney index quote provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil {
		return nil, errors.New("Eastmoney index quote response has no data object")
	}
	if payload.Data.Total != nil {
		if *payload.Data.Total < 0 {
			return nil, fmt.Errorf("Eastmoney index quote total is negative: %d", *payload.Data.Total)
		}
		if *payload.Data.Total == 0 && len(payload.Data.Diff) > 0 {
			return nil, errors.New("Eastmoney index quote reports total=0 with non-empty rows")
		}
		if *payload.Data.Total < len(payload.Data.Diff) {
			return nil, fmt.Errorf("Eastmoney index quote reports total=%d with %d returned rows", *payload.Data.Total, len(payload.Data.Diff))
		}
	}
	if len(payload.Data.Diff) == 0 {
		return nil, errors.New("Eastmoney index quote returned no rows")
	}
	fetchedAt := time.Now().UTC()
	snapshot := &model.IndexQuoteSnapshot{Source: p.Name(), SourceVersion: "eastmoney-ulist-index-v1", FetchedAt: fetchedAt, QualityStatus: model.QualityValid, Items: make([]model.MarketIndexQuote, 0, len(eastmoneyCoreIndexDefinitions))}
	if payload.Data.Total != nil && *payload.Data.Total > len(payload.Data.Diff) {
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_total_exceeds_returned_rows")
	}
	seen := make(map[string]bool, len(eastmoneyCoreIndexDefinitions))
	for _, raw := range payload.Data.Diff {
		market, code := parseEastmoneyBoardString(raw, "f13"), parseEastmoneyBoardString(raw, "f12")
		definition, ok := definitions[market+"."+code]
		if !ok {
			continue
		}
		if seen[definition.Symbol] {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "duplicate_index_row_dropped")
			continue
		}
		name := parseEastmoneyBoardString(raw, "f14")
		if name == "" {
			name = definition.Name
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_index_name_missing_used_allowlisted_name")
		}
		price, changePercent := parseEastmoneyBoardNumber(raw, "f2"), parseEastmoneyBoardNumber(raw, "f3")
		change := parseEastmoneyBoardNumber(raw, "f4")
		if price == nil || *price <= 0 || changePercent == nil {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_index_required_price_field_missing_or_invalid")
			continue
		}
		openPrice := parseEastmoneyBoardNumber(raw, "f17")
		high := parseEastmoneyBoardNumber(raw, "f15")
		low := parseEastmoneyBoardNumber(raw, "f16")
		prevClose := parseEastmoneyBoardNumber(raw, "f18")
		invalidOptionalPrice := false
		for _, value := range []*float64{openPrice, high, low, prevClose} {
			if value != nil && *value <= 0 {
				invalidOptionalPrice = true
				break
			}
		}
		if invalidOptionalPrice || (high != nil && low != nil && (*low > *high || *price < *low || *price > *high || (openPrice != nil && (*openPrice < *low || *openPrice > *high)))) {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_index_ohlc_invariant_failed")
			continue
		}
		if prevClose != nil {
			priceChange := *price - *prevClose
			if (change != nil && math.Abs(*change-priceChange) > eastmoneyIndexChangeTolerance) ||
				math.Abs(*changePercent-priceChange / *prevClose * 100) > eastmoneyIndexChangePercentTolerancePP {
				snapshot.QualityStatus = model.QualityDegraded
				snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_index_change_fields_inconsistent")
				continue
			}
		}
		reasons := []string{}
		amount := parseEastmoneyBoardNumber(raw, "f6")
		amountUnit := "CNY"
		if amount != nil && *amount < 0 {
			amount = nil
			reasons = append(reasons, "provider_amount_negative_or_invalid")
		}
		if amount == nil {
			amountUnit = "unknown"
			reasons = append(reasons, "provider_amount_missing")
		}
		quoteTime := validProviderSnapshotTime(parseEastmoneyTime(parseEastmoneyBoardString(raw, "f86")), time.Now().UTC())
		quoteTimeSemantics := ""
		if quoteTime == nil {
			reasons = append(reasons, "provider_snapshot_timestamp_not_exposed")
		} else {
			quoteTimeSemantics = "provider_snapshot_time"
		}
		if len(reasons) > 0 {
			snapshot.QualityStatus = model.QualityDegraded
		}
		for _, reason := range reasons {
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, reason)
		}
		item := model.MarketIndexQuote{
			Symbol: definition.Symbol, Code: definition.Code, Name: name,
			LatestPrice: price, Change: change, ChangePercent: changePercent,
			Open: openPrice, High: high, Low: low, PrevClose: prevClose,
			Amount: amount, AmountUnit: amountUnit, QuoteTime: quoteTime, QuoteTimeSemantics: quoteTimeSemantics,
			Source: p.Name(), SourceVersion: snapshot.SourceVersion, FetchedAt: fetchedAt, QualityStatus: model.QualityValid, QualityReasons: reasons,
		}
		if len(reasons) > 0 {
			item.QualityStatus = model.QualityDegraded
		}
		snapshot.Items = append(snapshot.Items, item)
		seen[definition.Symbol] = true
	}
	if len(snapshot.Items) == 0 {
		return nil, errors.New("Eastmoney index quote rows failed index-code and price validation")
	}
	if len(snapshot.Items) != len(eastmoneyCoreIndexDefinitions) {
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_core_index_rows_missing")
	}
	bySymbol := make(map[string]model.MarketIndexQuote, len(snapshot.Items))
	for _, item := range snapshot.Items {
		bySymbol[item.Symbol] = item
	}
	ordered := make([]model.MarketIndexQuote, 0, len(snapshot.Items))
	for _, index := range eastmoneyCoreIndexDefinitions {
		if item, ok := bySymbol[index.Symbol]; ok {
			ordered = append(ordered, item)
		}
	}
	snapshot.Items = ordered
	return snapshot, nil
}

const eastmoneyMarketUniverseFilter = "m:0+t:6,m:0+t:80,m:1+t:2,m:1+t:23,m:0+t:81+s:2048"
const eastmoneyMarketBreadthPageSize = 20000
const eastmoneyMarketBreadthMaxSecurities = 12000
const eastmoneyMarketTurnoverPageSize = 100
const eastmoneyMarketTurnoverWorkers = 4
const eastmoneyMarketTurnoverTimeout = 60 * time.Second

type eastmoneyMarketTurnoverItem struct {
	securityKey string
	amountCNY   float64
}

// fetchMarketTurnover 从东方财富证券列表接口分页汇总全市场成交额。
// 只有分页完整、证券唯一、金额字段有效且报价日期与广度报告日一致时才返回总额。
func (p *Eastmoney) fetchMarketTurnover(ctx context.Context, reportDate string) (float64, int, string) {
	fail := func(reason string) (float64, int, string) {
		return 0, 0, reason
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return fail("market_amount_collection_cancelled")
	}
	if _, err := time.Parse("2006-01-02", reportDate); err != nil {
		return fail("market_amount_report_date_invalid")
	}

	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}

	fetchPage := func(requestCtx context.Context, page, expectedTotal int) (int, []eastmoneyMarketTurnoverItem, string) {
		baseURL := strings.TrimSpace(p.stockListURL)
		if baseURL == "" {
			baseURL = defaultEastmoneySectorRankingURL
		}

		params := url.Values{
			"pn":     {strconv.Itoa(page)},
			"pz":     {strconv.Itoa(eastmoneyMarketTurnoverPageSize)},
			"po":     {"1"},
			"np":     {"1"},
			"fid":    {"f12"},
			"fs":     {eastmoneyMarketUniverseFilter},
			"fields": {"f12,f13,f6,f86"},
			"fltt":   {"2"},
			"invt":   {"2"},
			"ut":     {p.ut},
			"_":      {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}
		var payload struct {
			RC   *int `json:"rc"`
			Data *struct {
				Total int             `json:"total"`
				Diff  json.RawMessage `json:"diff"`
			} `json:"data"`
		}
		if err := p.getJSONWithRetry(requestCtx, baseURL, params, &payload, 6*time.Second); err != nil {
			if requestCtx.Err() != nil {
				return 0, nil, "market_amount_collection_cancelled"
			}
			return 0, nil, "market_amount_provider_request_failed"
		}
		if payload.RC != nil && *payload.RC != 0 {
			return 0, nil, "market_amount_provider_rejected_request"
		}
		if payload.Data == nil || payload.Data.Total <= 0 {
			return 0, nil, "market_amount_provider_total_missing"
		}

		total := payload.Data.Total
		if total > eastmoneyMarketBreadthMaxSecurities {
			return 0, nil, "market_amount_universe_exceeds_safety_limit"
		}
		if expectedTotal > 0 && total != expectedTotal {
			return 0, nil, "market_amount_provider_total_changed_during_pagination"
		}

		rows, err := decodeEastmoneyBoardRows(payload.Data.Diff)
		if err != nil {
			return 0, nil, "market_amount_provider_payload_invalid"
		}
		expectedRows := eastmoneyMarketTurnoverPageSize
		remaining := total - (page-1)*eastmoneyMarketTurnoverPageSize
		if remaining < expectedRows {
			expectedRows = remaining
		}
		if expectedRows <= 0 || len(rows) != expectedRows {
			return 0, nil, "market_amount_provider_pagination_incomplete"
		}

		items := make([]eastmoneyMarketTurnoverItem, 0, len(rows))
		for _, row := range rows {
			code := parseEastmoneyBoardString(row, "f12")
			market := parseEastmoneyBoardString(row, "f13")
			if len(code) != 6 || (market != "0" && market != "1") {
				return 0, nil, "market_amount_provider_security_identity_invalid"
			}
			for _, digit := range code {
				if digit < '0' || digit > '9' {
					return 0, nil, "market_amount_provider_security_identity_invalid"
				}
			}

			amount := parseEastmoneyBoardNumber(row, "f6")
			if amount == nil || *amount < 0 || !finiteNumber(*amount) {
				return 0, nil, "market_amount_provider_amount_invalid"
			}
			quoteTime := parseEastmoneyTime(parseEastmoneyBoardString(row, "f86"))
			if quoteTime == nil || quoteTime.In(location).Format("2006-01-02") != reportDate {
				return 0, nil, "market_amount_quote_date_mismatch"
			}

			items = append(items, eastmoneyMarketTurnoverItem{
				securityKey: market + "." + code,
				amountCNY:   *amount,
			})
		}
		return total, items, ""
	}

	total, firstPage, reason := fetchPage(ctx, 1, 0)
	if reason != "" {
		return fail(reason)
	}
	pageCount := (total + eastmoneyMarketTurnoverPageSize - 1) / eastmoneyMarketTurnoverPageSize
	if pageCount <= 0 || pageCount > eastmoneyStockSearchMaxPages {
		return fail("market_amount_provider_page_count_invalid")
	}

	pages := make([][]eastmoneyMarketTurnoverItem, pageCount)
	pages[0] = firstPage
	if pageCount > 1 {
		type pageResult struct {
			page   int
			items  []eastmoneyMarketTurnoverItem
			reason string
		}
		workCtx, cancel := context.WithCancel(ctx)
		jobs := make(chan int, pageCount-1)
		results := make(chan pageResult, pageCount-1)
		for page := 2; page <= pageCount; page++ {
			jobs <- page
		}
		close(jobs)

		workerCount := eastmoneyMarketTurnoverWorkers
		if workerCount > pageCount-1 {
			workerCount = pageCount - 1
		}
		var workers sync.WaitGroup
		var failureMu sync.Mutex
		firstFailureReason := ""
		workers.Add(workerCount)
		for worker := 0; worker < workerCount; worker++ {
			go func() {
				defer workers.Done()
				for page := range jobs {
					if workCtx.Err() != nil {
						results <- pageResult{page: page, reason: "market_amount_collection_cancelled"}
						continue
					}
					_, items, pageReason := fetchPage(workCtx, page, total)
					if pageReason != "" {
						failureMu.Lock()
						if firstFailureReason == "" {
							firstFailureReason = pageReason
						}
						failureMu.Unlock()
						cancel()
					}
					results <- pageResult{page: page, items: items, reason: pageReason}
				}
			}()
		}

		pageReasons := make([]string, pageCount)
		for i := 0; i < pageCount-1; i++ {
			result := <-results
			pageReasons[result.page-1] = result.reason
			if result.reason == "" {
				pages[result.page-1] = result.items
			}
		}
		workers.Wait()
		cancel()
		if firstFailureReason != "" {
			return fail(firstFailureReason)
		}
		for _, pageReason := range pageReasons {
			if pageReason != "" {
				return fail(pageReason)
			}
		}
	}

	seen := make(map[string]struct{}, total)
	var totalAmount float64
	for _, page := range pages {
		for _, item := range page {
			if _, exists := seen[item.securityKey]; exists {
				return fail("market_amount_provider_duplicate_security")
			}
			seen[item.securityKey] = struct{}{}
			totalAmount += item.amountCNY
			if !finiteNumber(totalAmount) {
				return fail("market_amount_provider_total_invalid")
			}
		}
	}
	if len(seen) != total {
		return fail("market_amount_provider_pagination_incomplete")
	}
	return totalAmount, len(seen), ""
}

func eastmoneyMarketStocksFallbackURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Scheme != "https" {
		return ""
	}
	if !strings.HasSuffix(strings.TrimSuffix(u.Path, "/"), "/api/qt/clist/get") {
		return ""
	}
	switch strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")) {
	case "push2.eastmoney.com", "82.push2.eastmoney.com":
		// 82.push2 is the bulk-query candidate; use the delay hostname only as fallback.
	default:
		return ""
	}
	port := u.Port()
	u.Host = "push2delay.eastmoney.com"
	if port != "" {
		u.Host += ":" + port
	}
	return u.String()
}

func (p *Eastmoney) FetchMarketBreadth(ctx context.Context) (*model.MarketBreadthSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	params := url.Values{
		"dpt": {"wz.ztzt"},
		"ut":  {p.ut},
		"_":   {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			QDate json.RawMessage              `json:"qdate"`
			Fenbu []map[string]json.RawMessage `json:"fenbu"`
		} `json:"data"`
	}
	if err := p.getJSONWithRetry(ctx, p.marketBreadthURL, params, &payload, 8*time.Second); err != nil {
		return nil, fmt.Errorf("Eastmoney market-breadth distribution request: %w", err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney market-breadth distribution provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil || len(payload.Data.Fenbu) == 0 {
		return nil, errors.New("Eastmoney market-breadth distribution returned no fenbu buckets")
	}
	qdate := parseEastmoneyBoardString(map[string]json.RawMessage{"qdate": payload.Data.QDate}, "qdate")
	reportDate := ""
	if len(qdate) == 8 {
		if parsed, err := time.Parse("20060102", qdate); err == nil {
			reportDate = parsed.Format("2006-01-02")
		}
	}
	if reportDate == "" {
		return nil, errors.New("Eastmoney market-breadth distribution has an invalid or missing qdate")
	}

	snapshot := &model.MarketBreadthSnapshot{
		Market:             "cn_a_share",
		Source:             p.Name(),
		SourceVersion:      "eastmoney-push2ex-zdfenbu-v1",
		ProviderReportDate: reportDate,
		FetchedAt:          time.Now().UTC(),
		QualityStatus:      model.QualityDegraded,
		QualityReasons: []string{
			"provider_snapshot_timestamp_not_exposed",
			"provider_distribution_population_not_verified_against_security_master",
		},
	}
	seenBuckets := make(map[int]struct{}, len(payload.Data.Fenbu))
	total := 0
	for rowIndex, bucket := range payload.Data.Fenbu {
		if len(bucket) != 1 {
			return nil, fmt.Errorf("Eastmoney market-breadth bucket %d must contain exactly one range/count pair", rowIndex)
		}
		for rawKey, rawCount := range bucket {
			key, err := strconv.Atoi(rawKey)
			if err != nil || key < -11 || key > 11 {
				return nil, fmt.Errorf("Eastmoney market-breadth bucket %d has invalid range key %q", rowIndex, rawKey)
			}
			if _, exists := seenBuckets[key]; exists {
				return nil, fmt.Errorf("Eastmoney market-breadth distribution contains duplicate range %d", key)
			}
			seenBuckets[key] = struct{}{}
			value := parseEastmoneyBoardNumber(map[string]json.RawMessage{"count": rawCount}, "count")
			if value == nil || *value < 0 || math.Trunc(*value) != *value || *value > float64(eastmoneyMarketBreadthMaxSecurities) {
				return nil, fmt.Errorf("Eastmoney market-breadth bucket %d has invalid count", key)
			}
			count := int(*value)
			total += count
			switch {
			case key > 0:
				snapshot.AdvancingCount += count
			case key < 0:
				snapshot.DecliningCount += count
			default:
				snapshot.UnchangedCount += count
			}
		}
	}
	if total <= 0 {
		return nil, errors.New("Eastmoney market-breadth distribution count is not positive")
	}
	if total > eastmoneyMarketBreadthMaxSecurities {
		return nil, fmt.Errorf("Eastmoney market-breadth distribution count %d exceeds safety cap %d", total, eastmoneyMarketBreadthMaxSecurities)
	}
	snapshot.TotalSecurities = total
	snapshot.CoveredSecurities = total
	snapshot.UnclassifiedCount = 0
	snapshot.AmountCoverageCount = 0
	snapshot.TotalAmountCNY = nil

	// 全市场金额需要分页遍历证券列表。限制额外耗时，失败时保留广度计数，不阻断主接口。
	turnoverCtx, cancelTurnover := context.WithTimeout(ctx, eastmoneyMarketTurnoverTimeout)
	totalAmount, amountCoverage, amountReason := p.fetchMarketTurnover(turnoverCtx, reportDate)
	cancelTurnover()
	if amountReason != "" {
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, amountReason)
	} else if amountCoverage != snapshot.TotalSecurities {
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "market_amount_security_count_mismatch")
	} else {
		snapshot.TotalAmountCNY = &totalAmount
		snapshot.AmountCoverageCount = amountCoverage
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "market_amount_quotes_collected_non_atomically")
	}

	// Limit-pool totals are separate upstream queries. Only attach them when the
	// provider report date matches the breadth snapshot date; missing values stay nil.
	upCount, upReason, err := p.fetchMarketBreadthLimitCount(ctx, "up", reportDate)
	if err != nil {
		return nil, err
	}
	snapshot.LimitUpCount = upCount
	if upReason != "" {
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, upReason)
	}
	downCount, downReason, err := p.fetchMarketBreadthLimitCount(ctx, "down", reportDate)
	if err != nil {
		return nil, err
	}
	snapshot.LimitDownCount = downCount
	if downReason != "" {
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, downReason)
	}
	return snapshot, nil
}

func (p *Eastmoney) fetchMarketBreadthLimitCount(ctx context.Context, poolType, reportDate string) (*int, string, error) {
	pool, err := p.FetchLimitPool(ctx, poolType, reportDate, 1)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, "", ctxErr
		}
		return nil, "market_breadth_limit_" + poolType + "_count_unavailable", nil
	}
	if pool == nil || pool.ProviderReportDate != reportDate {
		return nil, "market_breadth_limit_" + poolType + "_count_report_date_mismatch", nil
	}
	if !pool.HasData || pool.Total <= 0 {
		// A source's empty pool is not sufficient evidence of zero events.
		return nil, "market_breadth_limit_" + poolType + "_count_not_confirmed", nil
	}
	count := pool.Total
	return &count, "", nil
}

func (p *Eastmoney) FetchLimitPool(ctx context.Context, poolType, tradeDate string, limit int) (*model.LimitPoolSnapshot, error) {
	poolType = strings.ToLower(strings.TrimSpace(poolType))
	endpoints := map[string]string{"up": "getTopicZTPool", "down": "getTopicDTPool", "broken": "getTopicZBPool"}
	endpointPath, ok := endpoints[poolType]
	if !ok {
		return nil, fmt.Errorf("unsupported limit-pool type %q; want up, down or broken", poolType)
	}
	rawDate := strings.TrimSpace(tradeDate)
	date, err := time.Parse("2006-01-02", rawDate)
	if err != nil || date.Format("2006-01-02") != rawDate {
		return nil, fmt.Errorf("invalid limit-pool trade date %q: expected YYYY-MM-DD", tradeDate)
	}
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	if rawDate > today {
		return nil, fmt.Errorf("limit-pool trade date %s must not be in the future (China market date %s)", rawDate, today)
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 100 {
		return nil, errors.New("limit-pool limit must not exceed 100")
	}
	sortField := "fbt:asc"
	if poolType == "down" {
		// The DTPool endpoint has returned tc>0/pool=[] with fbt:asc; fund:asc returns its rows.
		sortField = "fund:asc"
	}
	pageSize := limit
	if pageSize > eastmoneyLimitPoolPageSize {
		pageSize = eastmoneyLimitPoolPageSize
	}
	requestPage := func(pageIndex int) (int, json.RawMessage, []map[string]json.RawMessage, error) {
		params := url.Values{
			"ut": {p.ut}, "dpt": {"wz.ztzt"}, "Pageindex": {strconv.Itoa(pageIndex)}, "pagesize": {strconv.Itoa(pageSize)},
			"sort": {sortField}, "date": {date.Format("20060102")},
			"_": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}
		var payload struct {
			RC   *int `json:"rc"`
			Data *struct {
				Total int                          `json:"tc"`
				QDate json.RawMessage              `json:"qdate"`
				Pool  []map[string]json.RawMessage `json:"pool"`
			} `json:"data"`
		}
		if err := p.getJSONWithRetry(ctx, p.limitPoolBaseURL+"/"+endpointPath, params, &payload, 6*time.Second); err != nil {
			return 0, nil, nil, fmt.Errorf("Eastmoney %s limit-pool page %d request: %w", poolType, pageIndex, err)
		}
		if payload.RC != nil && *payload.RC != 0 {
			return 0, nil, nil, fmt.Errorf("Eastmoney %s limit-pool provider error rc=%d on page %d", poolType, *payload.RC, pageIndex)
		}
		if payload.Data == nil {
			return 0, nil, nil, fmt.Errorf("Eastmoney limit-pool page %d has no data object", pageIndex)
		}
		if payload.Data.Total > 0 && len(payload.Data.Pool) == 0 {
			// push2ex can briefly report a non-empty total while omitting all pool rows (full=0).
			// Retry this semantic partial response once; never reinterpret tc>0 with no rows as an empty pool.
			firstTotal := payload.Data.Total
			timer := time.NewTimer(350 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return 0, nil, nil, ctx.Err()
			case <-timer.C:
			}
			payload.RC, payload.Data = nil, nil
			if retryErr := p.getJSONWithRetry(ctx, p.limitPoolBaseURL+"/"+endpointPath, params, &payload, 6*time.Second); retryErr != nil {
				return 0, nil, nil, fmt.Errorf("Eastmoney %s limit-pool page %d partial response (tc=%d, pool empty); retry failed: %w", poolType, pageIndex, firstTotal, retryErr)
			}
			if payload.RC != nil && *payload.RC != 0 {
				return 0, nil, nil, fmt.Errorf("Eastmoney %s limit-pool provider error after partial-response retry rc=%d", poolType, *payload.RC)
			}
			if payload.Data == nil {
				return 0, nil, nil, fmt.Errorf("Eastmoney limit-pool page %d retry returned no data object", pageIndex)
			}
			if payload.Data.Total > 0 && len(payload.Data.Pool) == 0 {
				return 0, nil, nil, fmt.Errorf("Eastmoney %s limit-pool page %d total/pool mismatch after retry (tc=%d, first tc=%d, pool empty)", poolType, pageIndex, payload.Data.Total, firstTotal)
			}
		}
		if payload.Data.Total < 0 {
			return 0, nil, nil, fmt.Errorf("Eastmoney %s limit-pool page %d returned negative total %d", poolType, pageIndex, payload.Data.Total)
		}
		if len(payload.Data.Pool) > pageSize {
			return 0, nil, nil, fmt.Errorf("Eastmoney %s limit-pool page %d returned %d rows above requested page size %d", poolType, pageIndex, len(payload.Data.Pool), pageSize)
		}
		// Validate raw rows against the source total before caller-limit truncation.
		// Trimming extra rows could otherwise hide a contradictory tc/pool response.
		expectedRows := payload.Data.Total - pageIndex*pageSize
		if expectedRows < 0 {
			expectedRows = 0
		}
		if expectedRows > pageSize {
			expectedRows = pageSize
		}
		if len(payload.Data.Pool) != expectedRows {
			return 0, nil, nil, fmt.Errorf("Eastmoney %s limit-pool page %d coverage mismatch: received %d rows, expected %d for source total %d", poolType, pageIndex, len(payload.Data.Pool), expectedRows, payload.Data.Total)
		}
		return payload.Data.Total, payload.Data.QDate, payload.Data.Pool, nil
	}

	var sourceTotal int
	var sourceQDate json.RawMessage
	sourceRowsReturned := 0
	poolRows := make([]map[string]json.RawMessage, 0, limit)
	for pageIndex := 0; pageIndex < 100; pageIndex++ {
		total, qdate, rows, requestErr := requestPage(pageIndex)
		if requestErr != nil {
			return nil, requestErr
		}
		sourceRowsReturned += len(rows)
		if pageIndex == 0 {
			sourceTotal, sourceQDate = total, qdate
			if total == 0 && len(rows) == 0 {
				break
			}
		} else {
			if total != sourceTotal {
				return nil, fmt.Errorf("Eastmoney %s limit-pool total changed across pages: first=%d page_%d=%d", poolType, sourceTotal, pageIndex, total)
			}
			if !bytes.Equal(bytes.TrimSpace(qdate), bytes.TrimSpace(sourceQDate)) {
				return nil, fmt.Errorf("Eastmoney %s limit-pool report date changed across pages", poolType)
			}
		}
		if total < 0 {
			return nil, fmt.Errorf("Eastmoney %s limit-pool returned negative total %d", poolType, total)
		}
		remaining := limit - len(poolRows)
		if remaining > len(rows) {
			remaining = len(rows)
		}
		if remaining > 0 {
			poolRows = append(poolRows, rows[:remaining]...)
		}
		targetRows := sourceTotal
		if targetRows > limit {
			targetRows = limit
		}
		if sourceTotal == 0 || len(poolRows) >= targetRows {
			break
		}
		if len(rows) == 0 {
			return nil, fmt.Errorf("Eastmoney %s limit-pool coverage ended at %d rows before target %d (source total %d)", poolType, len(poolRows), targetRows, sourceTotal)
		}
		if pageIndex == 99 {
			return nil, fmt.Errorf("Eastmoney %s limit-pool exceeded pagination safety limit", poolType)
		}
	}
	fetchedAt, requestedDate := time.Now().UTC(), date.Format("2006-01-02")
	snapshot := &model.LimitPoolSnapshot{
		PoolType: poolType, TradeDate: requestedDate, ProviderReportDate: normalizeEastmoneyCompactDate(strings.Trim(strings.TrimSpace(string(sourceQDate)), `"`)),
		Total: sourceTotal, ProviderReturned: sourceRowsReturned,
		HasData: sourceTotal > 0 && len(poolRows) > 0,
		Items: make([]model.LimitPoolItem, 0, len(poolRows)), Source: p.Name(),
		SourceVersion: "eastmoney-push2ex-limit-pool-v1", FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed", "requested_trade_date_is_authoritative"},
	}
	if sourceTotal > len(poolRows) {
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_total_exceeds_returned_rows")
	}
	if sourceTotal == 0 && len(poolRows) == 0 {
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = []string{"provider_returned_empty_pool_cannot_distinguish_zero_events_from_unavailable_date"}
		return snapshot, nil
	}
	if sourceTotal <= 0 || len(poolRows) == 0 {
		return nil, errors.New("Eastmoney limit-pool total/pool mismatch")
	}
	if len(poolRows) > limit {
		return nil, fmt.Errorf("Eastmoney limit-pool returned %d rows above requested limit %d", len(poolRows), limit)
	}
	seen := make(map[string]struct{}, len(poolRows))
	for index, raw := range poolRows {
		code, name, market := parseEastmoneyBoardString(raw, "c"), parseEastmoneyBoardString(raw, "n"), parseEastmoneyBoardString(raw, "m")
		change := parseEastmoneyBoardNumber(raw, "zdp")
		if code == "" || name == "" || (market != "0" && market != "1") || change == nil {
			return nil, fmt.Errorf("Eastmoney limit-pool row %d failed code/name/market/change validation", index)
		}
		suffix := ".SZ"
		if market == "1" {
			suffix = ".SH"
		}
		if strings.HasPrefix(code, "8") || strings.HasPrefix(code, "4") || strings.HasPrefix(code, "920") {
			suffix = ".BJ"
		}
		symbol := code + suffix
		if !isTushareAShareSymbol(symbol) {
			return nil, fmt.Errorf("Eastmoney limit-pool row %d is not a supported ordinary A-share security: %s", index, symbol)
		}
		if _, exists := seen[symbol]; exists {
			return nil, fmt.Errorf("Eastmoney limit-pool contains duplicate symbol %s", symbol)
		}
		seen[symbol] = struct{}{}
		firstTouchRaw, lastTouchRaw := parseEastmoneyBoardString(raw, "fbt"), parseEastmoneyBoardString(raw, "lbt")
		firstTouch, lastTouch := parseEastmoneyLimitTime(firstTouchRaw), parseEastmoneyLimitTime(lastTouchRaw)
		itemQualityReasons := []string{"provider_snapshot_timestamp_not_exposed"}
		for _, touchTime := range []struct{ raw, parsed string }{
			{raw: firstTouchRaw, parsed: firstTouch},
			{raw: lastTouchRaw, parsed: lastTouch},
		} {
			if strings.TrimSpace(touchTime.raw) != "" && touchTime.parsed == "" {
				itemQualityReasons = appendUniqueEastmoneyReason(itemQualityReasons, "provider_limit_pool_touch_time_invalid")
				snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_limit_pool_touch_time_invalid")
			}
		}
		snapshot.Items = append(snapshot.Items, model.LimitPoolItem{
			Symbol: symbol, Code: code, Name: name, ChangePercent: change,
			FirstTouchTime: firstTouch, LastTouchTime: lastTouch,
			ConsecutiveBoards: parseEastmoneyBoardInteger(raw, "lbc"), BrokenCount: parseEastmoneyBoardInteger(raw, "zbc"),
			Industry: parseEastmoneyBoardString(raw, "hybk"), Source: p.Name(), SourceVersion: snapshot.SourceVersion,
			FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded, QualityReasons: itemQualityReasons,
		})
	}
	return snapshot, nil
}

func parseEastmoneyLimitTime(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 6 {
		return ""
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return ""
		}
	}
	value = strings.Repeat("0", 6-len(value)) + value
	hour, hourErr := strconv.Atoi(value[:2])
	minute, minuteErr := strconv.Atoi(value[2:4])
	second, secondErr := strconv.Atoi(value[4:])
	if hourErr != nil || minuteErr != nil || secondErr != nil || hour > 23 || minute > 59 || second > 59 {
		return ""
	}
	return value[:2] + ":" + value[2:4] + ":" + value[4:]
}

func normalizeEastmoneyCompactDate(value string) string {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		return ""
	}
	if parsed, err := time.Parse("20060102", value); err == nil {
		return parsed.Format("2006-01-02")
	}
	return ""
}
