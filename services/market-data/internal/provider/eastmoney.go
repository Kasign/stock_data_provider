package provider

import (
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
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const eastmoneyVersion = "eastmoney-stock-api-v1"

// Bound implicit historical requests so old out-of-window rows do not pollute
// quality warnings when callers only want the most recent N trading bars.
const eastmoneyLookbackDaysPerBar = 4
const eastmoneyLookbackBufferDays = 30

// defaultEastmoneyUT is the public web token commonly used by Eastmoney front-end clients.
// It is not a user credential or private API key; deployments can override it if Eastmoney rotates it.
const defaultEastmoneyUT = "7eea3edcaed734bea9cbfc24409ed989"
const defaultEastmoneyStockBasicURL = "https://datacenter.eastmoney.com/securities/api/data/v1/get"
const defaultEastmoneySectorRankingURL = "https://push2.eastmoney.com/api/qt/clist/get"
const defaultEastmoneyIndexURL = "https://push2.eastmoney.com/api/qt/ulist.np/get"
const defaultEastmoneyStockBoardsURL = "https://push2.eastmoney.com/api/qt/slist/get"

type Eastmoney struct {
	client           *http.Client
	quoteURL         string
	klineURL         string
	stockBasicURL    string
	sectorRankingURL string
	indexURL         string
	marketBreadthURL string
	limitPoolBaseURL string
	stockBoardsURL   string
	ut               string
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
		quoteURL: quoteURL, klineURL: klineURL, stockBasicURL: stockBasicURL, sectorRankingURL: sectorRankingURL, indexURL: indexURL, marketBreadthURL: marketBreadthURL, limitPoolBaseURL: strings.TrimRight(limitPoolBaseURL, "/"), stockBoardsURL: stockBoardsURL, ut: ut,
	}
}
func (p *Eastmoney) Name() string { return "eastmoney" }

// FetchStockBoards returns the board memberships associated with one security via Eastmoney's slist endpoint.
func (p *Eastmoney) FetchStockBoards(ctx context.Context, rawSymbol string) (*model.StockBoardSnapshot, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
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
	return &model.StockBasic{
		Symbol: symbol, Name: name, IPODate: ipoDate, OutDate: outDate,
		SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: status, ListingStatusLabel: statusLabel,
		Source: p.Name(), SourceVersion: "eastmoney-f10-issueinfo-orginfo-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid,
		NormalizationVer: normalize.Version,
	}, nil
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
	providerTime := parseEastmoneyTime(getString("f86"))
	quote := &model.Quote{Symbol: symbol, Name: name, CurrentPrice: price, Change: change, ChangePercent: pct, Open: open, High: high, Low: low, PrevClose: prev, Volume: volume, Amount: amount, VolumeRatio: volumeRatio, TurnoverRate: turnoverRate, QuoteTime: providerTime, QuoteTimeSemantics: "provider_snapshot_time", FetchedAt: time.Now().UTC(), NormalizedAt: time.Now().UTC(), Source: p.Name(), SourceVersion: eastmoneyVersion, RawVolume: volumePtr, RawVolumeUnit: "hands", RawAmount: rawAmount, RawAmountUnit: optionalUnit(rawAmount, "CNY"), RawAmountField: optionalUnitName(rawAmount, "f48"), VolumeUnit: "shares", AmountUnit: optionalUnit(amount, "CNY"), AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}
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
	return quote, nil
}

func (p *Eastmoney) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
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
	if reported := strings.TrimSpace(payload.Data.Code); reported != "" {
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
	return normalizeProviderDailyBars(symbol, startISO, endISO, limit, out, rejectedRows)
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

func (p *Eastmoney) FetchSectorRankings(ctx context.Context, rawCategory string, limit int) (*model.SectorRankingSnapshot, error) {
	category := strings.ToLower(strings.TrimSpace(rawCategory))
	filters := map[string]string{
		"industry": "m:90+t:2+f:!50",
		"concept":  "m:90+t:3+f:!50",
	}
	filter, ok := filters[category]
	if !ok {
		return nil, fmt.Errorf("unsupported sector category %q; want industry or concept", rawCategory)
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		return nil, errors.New("sector ranking limit must not exceed 100")
	}
	params := url.Values{
		"pn": {"1"}, "pz": {strconv.Itoa(limit)}, "po": {"1"}, "np": {"1"},
		"ut": {p.ut}, "fltt": {"2"}, "invt": {"2"}, "fid": {"f3"},
		"fs": {filter}, "fields": {"f2,f3,f4,f6,f12,f14,f104,f105,f128,f136"},
		"_": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Total int                          `json:"total"`
			Diff  []map[string]json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if err := p.getJSON(ctx, p.sectorRankingURL, params, &payload); err != nil {
		return nil, fmt.Errorf("eastmoney sector ranking request: %w", err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney sector ranking provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil || len(payload.Data.Diff) == 0 {
		return nil, errors.New("Eastmoney sector ranking returned no rows")
	}
	fetchedAt := time.Now().UTC()
	snapshot := &model.SectorRankingSnapshot{
		Category: category, Total: payload.Data.Total, Source: p.Name(),
		SourceVersion:  "eastmoney-sector-clist-v1",
		FetchedAt:      fetchedAt,
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		Items:          make([]model.SectorRanking, 0, len(payload.Data.Diff)),
	}
	for _, raw := range payload.Data.Diff {
		code := parseEastmoneyBoardString(raw, "f12")
		name := parseEastmoneyBoardString(raw, "f14")
		changePercent := parseEastmoneyBoardNumber(raw, "f3")
		if code == "" || name == "" || changePercent == nil {
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
	if snapshot.Total <= 0 {
		snapshot.Total = len(snapshot.Items)
	}
	if len(snapshot.Items) < len(payload.Data.Diff) {
		snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_rows_dropped_after_validation")
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
	if value == nil {
		return nil
	}
	integer := int(*value)
	return &integer
}

func appendUniqueEastmoneyReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

type eastmoneyIndexDefinition struct {
	Symbol string
	Code   string
	SecID  string
	Market string
	Name   string
}

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
			Total int                          `json:"total"`
			Diff  []map[string]json.RawMessage `json:"diff"`
		} `json:"data"`
	}
	if err := p.getJSON(ctx, p.indexURL, params, &payload); err != nil {
		return nil, fmt.Errorf("Eastmoney index quote request: %w", err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney index quote provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil || len(payload.Data.Diff) == 0 {
		return nil, errors.New("Eastmoney index quote returned no rows")
	}
	fetchedAt := time.Now().UTC()
	snapshot := &model.IndexQuoteSnapshot{Source: p.Name(), SourceVersion: "eastmoney-ulist-index-v1", FetchedAt: fetchedAt, QualityStatus: model.QualityValid, Items: make([]model.MarketIndexQuote, 0, len(eastmoneyCoreIndexDefinitions))}
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
		if price == nil || changePercent == nil {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueEastmoneyReason(snapshot.QualityReasons, "provider_index_required_price_field_missing")
			continue
		}
		reasons := []string{}
		amount := parseEastmoneyBoardNumber(raw, "f6")
		amountUnit := "CNY"
		if amount == nil {
			amountUnit = "unknown"
			reasons = append(reasons, "provider_amount_missing")
		}
		quoteTime := parseEastmoneyTime(parseEastmoneyBoardString(raw, "f86"))
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
			LatestPrice: price, Change: parseEastmoneyBoardNumber(raw, "f4"), ChangePercent: changePercent,
			Open: parseEastmoneyBoardNumber(raw, "f17"), High: parseEastmoneyBoardNumber(raw, "f15"), Low: parseEastmoneyBoardNumber(raw, "f16"), PrevClose: parseEastmoneyBoardNumber(raw, "f18"),
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
			"provider_market_amount_unavailable_from_distribution_endpoint",
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
	return snapshot, nil
}

func (p *Eastmoney) FetchLimitPool(ctx context.Context, poolType, tradeDate string, limit int) (*model.LimitPoolSnapshot, error) {
	poolType = strings.ToLower(strings.TrimSpace(poolType))
	endpoints := map[string]string{"up": "getTopicZTPool", "down": "getTopicDTPool", "broken": "getTopicZBPool"}
	endpointPath, ok := endpoints[poolType]
	if !ok {
		return nil, fmt.Errorf("unsupported limit-pool type %q; want up, down or broken", poolType)
	}
	date, err := time.Parse("2006-01-02", tradeDate)
	if err != nil {
		return nil, fmt.Errorf("invalid limit-pool trade date %q: expected YYYY-MM-DD", tradeDate)
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		return nil, errors.New("limit-pool limit must not exceed 100")
	}
	sortField := "fbt:asc"
	if poolType == "down" {
		// The DTPool endpoint has returned tc>0/pool=[] with fbt:asc; fund:asc returns its rows.
		sortField = "fund:asc"
	}
	params := url.Values{
		"ut": {p.ut}, "dpt": {"wz.ztzt"}, "Pageindex": {"0"}, "pagesize": {strconv.Itoa(limit)},
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
		return nil, fmt.Errorf("Eastmoney %s limit-pool request: %w", poolType, err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney %s limit-pool provider error rc=%d", poolType, *payload.RC)
	}
	if payload.Data == nil {
		return nil, errors.New("Eastmoney limit-pool response has no data object")
	}
	// push2ex can briefly report a non-empty total while omitting all pool rows (full=0).
	// Retry this semantic partial response once; never reinterpret tc>0 with no rows as an empty pool.
	if payload.Data.Total > 0 && len(payload.Data.Pool) == 0 {
		firstTotal := payload.Data.Total
		timer := time.NewTimer(350 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
		payload.RC, payload.Data = nil, nil
		if retryErr := p.getJSONWithRetry(ctx, p.limitPoolBaseURL+"/"+endpointPath, params, &payload, 6*time.Second); retryErr != nil {
			return nil, fmt.Errorf("Eastmoney %s limit-pool partial response (tc=%d, pool empty); retry failed: %w", poolType, firstTotal, retryErr)
		}
		if payload.RC != nil && *payload.RC != 0 {
			return nil, fmt.Errorf("Eastmoney %s limit-pool provider error after partial-response retry rc=%d", poolType, *payload.RC)
		}
		if payload.Data == nil {
			return nil, errors.New("Eastmoney limit-pool retry returned no data object")
		}
		if payload.Data.Total > 0 && len(payload.Data.Pool) == 0 {
			return nil, fmt.Errorf("Eastmoney %s limit-pool total/pool mismatch after retry (tc=%d, first tc=%d, pool empty)", poolType, payload.Data.Total, firstTotal)
		}
	}
	fetchedAt, requestedDate := time.Now().UTC(), date.Format("2006-01-02")
	snapshot := &model.LimitPoolSnapshot{
		PoolType: poolType, TradeDate: requestedDate, ProviderReportDate: normalizeEastmoneyCompactDate(strings.Trim(strings.TrimSpace(string(payload.Data.QDate)), `"`)),
		Total: payload.Data.Total, HasData: payload.Data.Total > 0 && len(payload.Data.Pool) > 0,
		Items: make([]model.LimitPoolItem, 0, len(payload.Data.Pool)), Source: p.Name(),
		SourceVersion: "eastmoney-push2ex-limit-pool-v1", FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed", "requested_trade_date_is_authoritative"},
	}
	if payload.Data.Total == 0 && len(payload.Data.Pool) == 0 {
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = []string{"provider_returned_empty_pool_cannot_distinguish_zero_events_from_unavailable_date"}
		return snapshot, nil
	}
	if payload.Data.Total <= 0 || len(payload.Data.Pool) == 0 {
		return nil, errors.New("Eastmoney limit-pool total/pool mismatch")
	}
	if len(payload.Data.Pool) > limit {
		return nil, fmt.Errorf("Eastmoney limit-pool returned %d rows above requested limit %d", len(payload.Data.Pool), limit)
	}
	seen := make(map[string]struct{}, len(payload.Data.Pool))
	for index, raw := range payload.Data.Pool {
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
		if _, exists := seen[symbol]; exists {
			return nil, fmt.Errorf("Eastmoney limit-pool contains duplicate symbol %s", symbol)
		}
		seen[symbol] = struct{}{}
		snapshot.Items = append(snapshot.Items, model.LimitPoolItem{
			Symbol: symbol, Code: code, Name: name, ChangePercent: change,
			FirstTouchTime:    parseEastmoneyLimitTime(parseEastmoneyBoardString(raw, "fbt")),
			LastTouchTime:     parseEastmoneyLimitTime(parseEastmoneyBoardString(raw, "lbt")),
			ConsecutiveBoards: parseEastmoneyBoardInteger(raw, "lbc"), BrokenCount: parseEastmoneyBoardInteger(raw, "zbc"),
			Industry: parseEastmoneyBoardString(raw, "hybk"), Source: p.Name(), SourceVersion: snapshot.SourceVersion,
			FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
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
