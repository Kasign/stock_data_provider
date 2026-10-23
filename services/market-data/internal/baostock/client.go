package baostock

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHost      = "public-api.baostock.com"
	defaultPort      = 10030
	defaultUserID    = "anonymous"
	defaultPassword  = "123456"
	defaultPageSize  = 10000
	maxQueryRows     = 100000
	dailyKLineFields = "date,code,open,high,low,close,preclose,volume,amount,adjustflag,turn,tradestatus,pctChg,isST"
)

// Config controls the BaoStock TCP session. The anonymous credentials are the
// public protocol defaults, not a personal API key.
type Config struct {
	Host           string
	Port           int
	UserID         string
	Password       string
	ConnectTimeout time.Duration
	RequestTimeout time.Duration
}

// DailyKLine contains the source fields needed by the daily-bar adapter.
// Values are kept as strings until the adapter applies field-level validation.
type DailyKLine struct {
	Date        string
	Code        string
	Open        string
	High        string
	Low         string
	Close       string
	PreClose    string
	Volume      string
	Amount      string
	AdjustFlag  string
	Turn        string
	TradeStatus string
	PctChg      string
	IsST        string
}

// TradeDate is one calendar date and the provider's explicit trading-day flag.
type TradeDate struct {
	Date         string
	IsTradingDay bool
}

// QuarterlyFinancialRecord stores one BaoStock financial row without changing numeric scale.
type QuarterlyFinancialRecord struct {
	Code            string
	PublicationDate string
	ReportDate      string
	Fields          map[string]string
}

type financialQuerySpec struct {
	method       string
	requestType  string
	responseType string
	fields       []string
}

const quarterlyFinancialPageSize = 1000

var quarterlyFinancialSpecs = map[string]financialQuerySpec{
	"profitability": {
		method: "query_profit_data", requestType: messageProfitRequest, responseType: messageProfitResponse,
		fields: []string{"code", "pubDate", "statDate", "roeAvg", "npMargin", "gpMargin", "netProfit", "epsTTM", "MBRevenue", "totalShare", "liqaShare"},
	},
	"operating": {
		method: "query_operation_data", requestType: messageOperationRequest, responseType: messageOperationResponse,
		fields: []string{"code", "pubDate", "statDate", "NRTurnRatio", "NRTurnDays", "INVTurnRatio", "INVTurnDays", "CATurnRatio", "AssetTurnRatio"},
	},
	"growth": {
		method: "query_growth_data", requestType: messageGrowthRequest, responseType: messageGrowthResponse,
		fields: []string{"code", "pubDate", "statDate", "YOYEquity", "YOYAsset", "YOYNI", "YOYEPSBasic", "YOYPNI"},
	},
	"dupont": {
		method: "query_dupont_data", requestType: messageDupontRequest, responseType: messageDupontResponse,
		fields: []string{"code", "pubDate", "statDate", "dupontROE", "dupontAssetStoEquity", "dupontAssetTurn", "dupontPnitoni", "dupontNitogr", "dupontTaxBurden", "dupontIntburden", "dupontEbittogr"},
	},
	"solvency": {
		method: "query_balance_data", requestType: messageBalanceRequest, responseType: messageBalanceResponse,
		fields: []string{"code", "pubDate", "statDate", "currentRatio", "quickRatio", "cashRatio", "YOYLiability", "liabilityToAsset", "assetToEquity"},
	},
	"cash_flow": {
		method: "query_cash_flow_data", requestType: messageCashFlowRequest, responseType: messageCashFlowResponse,
		fields: []string{"code", "pubDate", "statDate", "CAToAsset", "NCAToAsset", "tangibleAssetToAsset", "ebitToInterest", "CFOToOR", "CFOToNP", "CFOToGr"},
	},
}

// QueryQuarterlyFinancialData retrieves one security/report-period/category from BaoStock.
// An empty response returns (nil, nil), never a synthetic zero-valued record.
func (c *Client) QueryQuarterlyFinancialData(ctx context.Context, code, category string, year, quarter int) (*QuarterlyFinancialRecord, error) {
	spec, ok := quarterlyFinancialSpecs[category]
	if !ok {
		return nil, fmt.Errorf("unsupported BaoStock financial category %q", category)
	}
	code = strings.TrimSpace(code)
	codeParts := strings.Split(code, ".")
	if len(codeParts) != 2 || (codeParts[0] != "sh" && codeParts[0] != "sz") || len(codeParts[1]) != 6 {
		return nil, fmt.Errorf("invalid BaoStock financial security code %q", code)
	}
	if year < 2007 || year > time.Now().Year() {
		return nil, fmt.Errorf("BaoStock financial year %d is outside supported range 2007..%d", year, time.Now().Year())
	}
	if quarter < 1 || quarter > 4 {
		return nil, fmt.Errorf("BaoStock financial quarter %d is outside 1..4", quarter)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()
	if err := c.ensureLoggedInLocked(ctx); err != nil {
		return nil, err
	}
	requestBody := strings.Join([]string{
		spec.method, c.userID, "1", strconv.Itoa(quarterlyFinancialPageSize),
		code, strconv.Itoa(year), strconv.Itoa(quarter),
	}, messageSplit)
	messageType, responseBody, err := c.exchangeLocked(ctx, spec.requestType, requestBody)
	if err != nil {
		return nil, fmt.Errorf("BaoStock %s: %w", spec.method, err)
	}
	if messageType == messageException {
		return nil, protocolExceptionError(responseBody)
	}
	if messageType != spec.responseType {
		c.dropConnectionLocked()
		return nil, fmt.Errorf("unexpected BaoStock %s response type %q", spec.method, messageType)
	}
	return parseQuarterlyFinancialPage(responseBody, spec, c.userID, code, year, quarter)
}

func parseQuarterlyFinancialPage(body string, spec financialQuerySpec, userID, code string, year, quarter int) (*QuarterlyFinancialRecord, error) {
	parts := strings.Split(body, messageSplit)
	if len(parts) < 11 {
		return nil, fmt.Errorf("BaoStock %s response has only %d fields", spec.method, len(parts))
	}
	if parts[0] != "0" {
		return nil, fmt.Errorf("BaoStock %s query rejected: code=%s message=%s", spec.method, parts[0], parts[1])
	}
	if parts[2] != spec.method || parts[3] != userID || parts[4] != "1" {
		return nil, fmt.Errorf("BaoStock %s response metadata mismatch", spec.method)
	}
	if pageSize, err := strconv.Atoi(strings.TrimSpace(parts[5])); err != nil || pageSize <= 0 {
		return nil, fmt.Errorf("BaoStock %s response has invalid page size %q", spec.method, parts[5])
	}
	if strings.TrimSpace(parts[7]) != code || strings.TrimSpace(parts[8]) != strconv.Itoa(year) || strings.TrimSpace(parts[9]) != strconv.Itoa(quarter) {
		return nil, fmt.Errorf("BaoStock %s response security/period metadata mismatch", spec.method)
	}
	if got := strings.Split(parts[10], ","); !equalFields(got, spec.fields) {
		return nil, fmt.Errorf("BaoStock %s response fields mismatch: got %q", spec.method, strings.Join(got, ","))
	}
	var payload map[string]json.RawMessage
	if raw := strings.TrimSpace(parts[6]); raw != "" && raw != "null" {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			return nil, fmt.Errorf("BaoStock %s parse record JSON: %w", spec.method, err)
		}
	}
	var rows [][]json.RawMessage
	if payload != nil && len(payload["record"]) > 0 {
		if err := json.Unmarshal(payload["record"], &rows); err != nil {
			return nil, fmt.Errorf("BaoStock %s parse financial rows: %w", spec.method, err)
		}
	}
	if len(rows) == 0 {
		return nil, nil
	}
	if len(rows) != 1 {
		return nil, fmt.Errorf("BaoStock %s returned %d rows for one security/report period", spec.method, len(rows))
	}
	if len(rows[0]) != len(spec.fields) {
		return nil, fmt.Errorf("BaoStock %s row has %d columns; expected %d", spec.method, len(rows[0]), len(spec.fields))
	}
	values := make(map[string]string, len(spec.fields))
	for i, raw := range rows[0] {
		value, err := decodeCell(raw)
		if err != nil {
			return nil, fmt.Errorf("BaoStock %s column %s: %w", spec.method, spec.fields[i], err)
		}
		values[spec.fields[i]] = value
	}
	if strings.TrimSpace(values["code"]) != code {
		return nil, fmt.Errorf("BaoStock %s row security mismatch: requested %s, got %q", spec.method, code, values["code"])
	}
	statDate := strings.TrimSpace(values["statDate"])
	periodEnd := time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	if statDate != periodEnd {
		return nil, fmt.Errorf("BaoStock %s report date %q does not match requested period end %s", spec.method, statDate, periodEnd)
	}
	publicationDate := strings.TrimSpace(values["pubDate"])
	if publicationDate != "" {
		if _, err := time.Parse("2006-01-02", publicationDate); err != nil {
			return nil, fmt.Errorf("BaoStock %s invalid publication date %q", spec.method, publicationDate)
		}
	}
	return &QuarterlyFinancialRecord{Code: code, PublicationDate: publicationDate, ReportDate: statDate, Fields: values}, nil
}

// Client serializes operations over one stateful TCP session so requests and
// responses cannot interleave on the same socket.
type Client struct {
	cfg    Config
	gate   chan struct{}
	conn   net.Conn
	reader *bufio.Reader
	userID string
	logged bool
}

func NewClient(cfg Config) *Client {
	if strings.TrimSpace(cfg.Host) == "" {
		cfg.Host = defaultHost
	}
	if cfg.Port <= 0 {
		cfg.Port = defaultPort
	}
	if strings.TrimSpace(cfg.UserID) == "" {
		cfg.UserID = defaultUserID
	}
	if cfg.Password == "" {
		cfg.Password = defaultPassword
	}
	if cfg.ConnectTimeout <= 0 {
		cfg.ConnectTimeout = 5 * time.Second
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 15 * time.Second
	}
	c := &Client{cfg: cfg, gate: make(chan struct{}, 1), userID: cfg.UserID}
	c.gate <- struct{}{}
	return c
}

// QueryDailyKLines retrieves daily bars using the BaoStock history endpoint.
// BaoStock flags are 1=backward adjusted, 2=forward adjusted, 3=unadjusted.
func (c *Client) QueryDailyKLines(ctx context.Context, code, startDate, endDate, adjustFlag string) ([]DailyKLine, error) {
	if strings.TrimSpace(code) == "" {
		return nil, errors.New("BaoStock code is required")
	}
	if adjustFlag != "1" && adjustFlag != "2" && adjustFlag != "3" {
		return nil, fmt.Errorf("unsupported BaoStock adjustment flag %q", adjustFlag)
	}
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		return nil, fmt.Errorf("invalid BaoStock start date %q", startDate)
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		return nil, fmt.Errorf("invalid BaoStock end date %q", endDate)
	}
	if startDate > endDate {
		return nil, errors.New("BaoStock start date must not be after end date")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()

	if err := c.ensureLoggedInLocked(ctx); err != nil {
		return nil, err
	}

	rows := make([]DailyKLine, 0, 256)
	for pageNumber := 1; pageNumber <= 100; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		requestBody := strings.Join([]string{
			"query_history_k_data_plus",
			c.userID,
			strconv.Itoa(pageNumber),
			strconv.Itoa(defaultPageSize),
			code,
			dailyKLineFields,
			startDate,
			endDate,
			"d",
			adjustFlag,
		}, messageSplit)

		messageType, responseBody, err := c.exchangeLocked(ctx, messageHistoryRequest, requestBody)
		if err != nil {
			return nil, fmt.Errorf("BaoStock daily K-line page %d: %w", pageNumber, err)
		}
		if messageType == messageException {
			return nil, protocolExceptionError(responseBody)
		}
		if messageType != messageCompressed {
			c.dropConnectionLocked()
			return nil, fmt.Errorf("unexpected BaoStock K-line response type %q", messageType)
		}

		page, err := parseKLinePage(responseBody)
		if err != nil {
			c.dropConnectionLocked()
			return nil, fmt.Errorf("decode BaoStock K-line page %d: %w", pageNumber, err)
		}
		if page.errorCode != "0" {
			return nil, fmt.Errorf("BaoStock K-line query rejected: code=%s message=%s", page.errorCode, page.errorMessage)
		}
		if !equalFields(page.fields, strings.Split(dailyKLineFields, ",")) {
			c.dropConnectionLocked()
			return nil, fmt.Errorf("BaoStock K-line fields mismatch: got %q", strings.Join(page.fields, ","))
		}
		if len(rows)+len(page.rows) > maxQueryRows {
			return nil, fmt.Errorf("BaoStock query exceeded safety row limit %d", maxQueryRows)
		}
		rows = append(rows, page.rows...)
		pageSize := page.pageSize
		if pageSize <= 0 {
			pageSize = defaultPageSize
		}
		if len(page.rows) < pageSize {
			return rows, nil
		}
	}
	return nil, errors.New("BaoStock exceeded maximum 100-page K-line query limit")
}

// QueryTradeDates retrieves every calendar date in the inclusive range.
func (c *Client) QueryTradeDates(ctx context.Context, startDate, endDate string) ([]TradeDate, error) {
	if _, err := time.Parse("2006-01-02", startDate); err != nil {
		return nil, fmt.Errorf("invalid BaoStock trading-calendar start date %q", startDate)
	}
	if _, err := time.Parse("2006-01-02", endDate); err != nil {
		return nil, fmt.Errorf("invalid BaoStock trading-calendar end date %q", endDate)
	}
	if startDate > endDate {
		return nil, errors.New("BaoStock trading-calendar start date must not be after end date")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()
	if err := c.ensureLoggedInLocked(ctx); err != nil {
		return nil, err
	}

	rows := make([]TradeDate, 0, 32)
	seen := make(map[string]struct{})
	for pageNumber := 1; pageNumber <= 100; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		requestBody := strings.Join([]string{
			"query_trade_dates", c.userID, strconv.Itoa(pageNumber),
			strconv.Itoa(defaultPageSize), startDate, endDate,
		}, messageSplit)
		messageType, responseBody, err := c.exchangeLocked(ctx, messageTradeDateRequest, requestBody)
		if err != nil {
			return nil, fmt.Errorf("BaoStock trading-calendar page %d: %w", pageNumber, err)
		}
		if messageType == messageException {
			return nil, protocolExceptionError(responseBody)
		}
		if messageType != messageTradeDateResponse {
			c.dropConnectionLocked()
			return nil, fmt.Errorf("unexpected BaoStock trading-calendar response type %q", messageType)
		}
		page, err := parseTradeDatePage(responseBody)
		if err != nil {
			c.dropConnectionLocked()
			return nil, fmt.Errorf("decode BaoStock trading-calendar page %d: %w", pageNumber, err)
		}
		if page.errorCode != "0" {
			return nil, fmt.Errorf("BaoStock trading-calendar query rejected: code=%s message=%s", page.errorCode, page.errorMessage)
		}
		if !equalFields(page.fields, []string{"calendar_date", "is_trading_day"}) {
			c.dropConnectionLocked()
			return nil, fmt.Errorf("BaoStock trading-calendar fields mismatch: got %q", strings.Join(page.fields, ","))
		}
		for _, row := range page.rows {
			if row.Date < startDate || row.Date > endDate {
				return nil, fmt.Errorf("BaoStock trading-calendar date %s is outside requested range %s..%s", row.Date, startDate, endDate)
			}
			if len(rows) > 0 && row.Date <= rows[len(rows)-1].Date {
				return nil, fmt.Errorf("BaoStock trading-calendar rows are duplicate or unsorted at %s", row.Date)
			}
			if _, ok := seen[row.Date]; ok {
				return nil, fmt.Errorf("BaoStock trading-calendar returned duplicate date %s", row.Date)
			}
			seen[row.Date] = struct{}{}
			rows = append(rows, row)
		}
		if len(rows) > maxQueryRows {
			return nil, fmt.Errorf("BaoStock trading-calendar query exceeded safety row limit %d", maxQueryRows)
		}
		pageSize := page.pageSize
		if pageSize <= 0 {
			pageSize = defaultPageSize
		}
		if len(page.rows) < pageSize {
			if len(rows) == 0 {
				return nil, errors.New("BaoStock returned no trading-calendar rows")
			}
			return rows, nil
		}
	}
	return nil, errors.New("BaoStock exceeded maximum 100-page trading-calendar query limit")
}

type tradeDatePage struct {
	errorCode    string
	errorMessage string
	pageSize     int
	fields       []string
	rows         []TradeDate
}

func parseTradeDatePage(body string) (tradeDatePage, error) {
	parts := strings.Split(body, messageSplit)
	if len(parts) < 2 {
		return tradeDatePage{}, errors.New("trading-calendar response has fewer than two status fields")
	}
	page := tradeDatePage{errorCode: parts[0], errorMessage: parts[1]}
	if page.errorCode != "0" {
		return page, nil
	}
	if len(parts) < 10 {
		return tradeDatePage{}, fmt.Errorf("successful trading-calendar response has only %d fields", len(parts))
	}
	page.pageSize, _ = strconv.Atoi(strings.TrimSpace(parts[5]))
	page.fields = strings.Split(parts[9], ",")
	if !equalFields(page.fields, []string{"calendar_date", "is_trading_day"}) {
		return tradeDatePage{}, fmt.Errorf("unexpected trading-calendar field list %q", strings.Join(page.fields, ","))
	}
	var payload map[string]json.RawMessage
	if strings.TrimSpace(parts[6]) != "" {
		decoder := json.NewDecoder(strings.NewReader(parts[6]))
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			return tradeDatePage{}, fmt.Errorf("parse trading-calendar record JSON: %w", err)
		}
	}
	var records [][]json.RawMessage
	if raw := payload["record"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &records); err != nil {
			return tradeDatePage{}, fmt.Errorf("parse trading-calendar rows: %w", err)
		}
	}
	for i, raw := range records {
		if len(raw) != 2 {
			return tradeDatePage{}, fmt.Errorf("trading-calendar row %d has %d columns, expected 2", i, len(raw))
		}
		date, err := decodeCell(raw[0])
		if err != nil {
			return tradeDatePage{}, fmt.Errorf("trading-calendar row %d date: %w", i, err)
		}
		if _, err := time.Parse("2006-01-02", date); err != nil {
			return tradeDatePage{}, fmt.Errorf("trading-calendar row %d has invalid date %q", i, date)
		}
		flagText, err := decodeCell(raw[1])
		if err != nil {
			return tradeDatePage{}, fmt.Errorf("trading-calendar row %d flag: %w", i, err)
		}
		flag, err := strconv.Atoi(strings.TrimSpace(flagText))
		if err != nil || (flag != 0 && flag != 1) {
			return tradeDatePage{}, fmt.Errorf("trading-calendar row %d has invalid is_trading_day %q", i, flagText)
		}
		page.rows = append(page.rows, TradeDate{Date: date, IsTradingDay: flag == 1})
	}
	return page, nil
}

func (c *Client) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := c.acquire(ctx); err != nil {
		return fmt.Errorf("close BaoStock client while a request is active: %w", err)
	}
	defer c.release()

	var logoutErr error
	if c.conn != nil && c.logged {
		body := strings.Join([]string{"logout", c.userID, time.Now().Format("20060102150405")}, messageSplit)
		messageType, responseBody, err := c.exchangeLocked(ctx, messageLogoutRequest, body)
		if err != nil {
			logoutErr = fmt.Errorf("BaoStock logout: %w", err)
		} else if messageType != messageLogoutResponse {
			logoutErr = fmt.Errorf("unexpected BaoStock logout response type %q", messageType)
		} else if err := responseStatusError(responseBody); err != nil {
			logoutErr = fmt.Errorf("BaoStock logout: %w", err)
		}
	}
	c.dropConnectionLocked()
	return logoutErr
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case <-c.gate:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *Client) release() {
	c.gate <- struct{}{}
}

func (c *Client) ensureLoggedInLocked(ctx context.Context) error {
	if c.conn != nil && c.logged {
		return nil
	}
	c.dropConnectionLocked()

	dialCtx, cancel := context.WithTimeout(ctx, c.cfg.ConnectTimeout)
	defer cancel()
	conn, err := (&net.Dialer{Timeout: c.cfg.ConnectTimeout, KeepAlive: 30 * time.Second}).DialContext(
		dialCtx, "tcp", net.JoinHostPort(c.cfg.Host, strconv.Itoa(c.cfg.Port)))
	if err != nil {
		return fmt.Errorf("connect to BaoStock %s:%d: %w", c.cfg.Host, c.cfg.Port, err)
	}
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.userID = c.cfg.UserID

	body := strings.Join([]string{"login", c.cfg.UserID, c.cfg.Password, "0"}, messageSplit)
	messageType, responseBody, err := c.exchangeLocked(ctx, messageLoginRequest, body)
	if err != nil {
		c.dropConnectionLocked()
		return fmt.Errorf("BaoStock login: %w", err)
	}
	if messageType == messageException {
		c.dropConnectionLocked()
		return protocolExceptionError(responseBody)
	}
	if messageType != messageLoginResponse {
		c.dropConnectionLocked()
		return fmt.Errorf("unexpected BaoStock login response type %q", messageType)
	}
	parts := strings.Split(responseBody, messageSplit)
	if len(parts) < 2 {
		c.dropConnectionLocked()
		return errors.New("BaoStock login response is malformed")
	}
	if parts[0] != "0" {
		c.dropConnectionLocked()
		return fmt.Errorf("BaoStock login rejected: code=%s message=%s", parts[0], parts[1])
	}
	c.logged = true
	return nil
}

func (c *Client) exchangeLocked(ctx context.Context, messageType, body string) (string, string, error) {
	if c.conn == nil || c.reader == nil {
		return "", "", errors.New("BaoStock connection is not open")
	}
	if err := ctx.Err(); err != nil {
		return "", "", err
	}
	frame, err := encodeFrame(messageType, body)
	if err != nil {
		return "", "", err
	}
	conn := c.conn
	restoreDeadline := applyDeadline(ctx, conn, c.cfg.RequestTimeout)
	defer restoreDeadline()

	if err := writeAll(conn, append(frame, '\n')); err != nil {
		c.dropConnectionLocked()
		return "", "", fmt.Errorf("send BaoStock request: %w", err)
	}
	responseType, responseBody, err := readResponse(c.reader)
	if err != nil {
		c.dropConnectionLocked()
		return "", "", fmt.Errorf("receive BaoStock response: %w", err)
	}
	return responseType, responseBody, nil
}

func applyDeadline(ctx context.Context, conn net.Conn, timeout time.Duration) func() {
	deadline := time.Now().Add(timeout)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	_ = conn.SetDeadline(deadline)

	callbackDone := make(chan struct{})
	stop := context.AfterFunc(ctx, func() {
		_ = conn.SetDeadline(time.Now())
		close(callbackDone)
	})
	return func() {
		if !stop() {
			<-callbackDone
		}
		_ = conn.SetDeadline(time.Time{})
	}
}

func (c *Client) dropConnectionLocked() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.reader = nil
	c.logged = false
	c.userID = c.cfg.UserID
}

type queryPage struct {
	errorCode    string
	errorMessage string
	pageSize     int
	fields       []string
	rows         []DailyKLine
}

func parseKLinePage(body string) (queryPage, error) {
	parts := strings.Split(body, messageSplit)
	if len(parts) < 2 {
		return queryPage{}, errors.New("response has fewer than two status fields")
	}
	page := queryPage{errorCode: parts[0], errorMessage: parts[1]}
	if page.errorCode != "0" {
		return page, nil
	}
	if len(parts) < 9 {
		return queryPage{}, fmt.Errorf("successful response has only %d fields", len(parts))
	}
	page.pageSize, _ = strconv.Atoi(strings.TrimSpace(parts[5]))
	page.fields = strings.Split(parts[8], ",")

	var payload map[string]json.RawMessage
	if strings.TrimSpace(parts[6]) != "" {
		decoder := json.NewDecoder(strings.NewReader(parts[6]))
		decoder.UseNumber()
		if err := decoder.Decode(&payload); err != nil {
			return queryPage{}, fmt.Errorf("parse record JSON: %w", err)
		}
	}
	var records [][]json.RawMessage
	if raw := payload["record"]; len(raw) > 0 {
		if err := json.Unmarshal(raw, &records); err != nil {
			return queryPage{}, fmt.Errorf("parse record rows: %w", err)
		}
	}
	for rowIndex, rawRow := range records {
		if len(rawRow) != len(page.fields) || len(rawRow) < 14 {
			return queryPage{}, fmt.Errorf("row %d has %d columns, fields specify %d", rowIndex, len(rawRow), len(page.fields))
		}
		values := make([]string, len(rawRow))
		for columnIndex, rawValue := range rawRow {
			value, err := decodeCell(rawValue)
			if err != nil {
				return queryPage{}, fmt.Errorf("row %d column %d: %w", rowIndex, columnIndex, err)
			}
			values[columnIndex] = value
		}
		page.rows = append(page.rows, DailyKLine{
			Date: values[0], Code: values[1], Open: values[2], High: values[3], Low: values[4],
			Close: values[5], PreClose: values[6], Volume: values[7], Amount: values[8],
			AdjustFlag: values[9], Turn: values[10], TradeStatus: values[11], PctChg: values[12], IsST: values[13],
		})
	}
	return page, nil
}

func decodeCell(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return "", err
	}
	switch typed := value.(type) {
	case string:
		return typed, nil
	case json.Number:
		return typed.String(), nil
	case bool:
		return strconv.FormatBool(typed), nil
	default:
		return "", fmt.Errorf("unsupported cell type %T", value)
	}
}

func equalFields(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if strings.TrimSpace(got[index]) != want[index] {
			return false
		}
	}
	return true
}

func responseStatusError(body string) error {
	parts := strings.Split(body, messageSplit)
	if len(parts) < 2 {
		return fmt.Errorf("BaoStock status response is malformed: %s", body)
	}
	if parts[0] == "0" {
		return nil
	}
	return fmt.Errorf("BaoStock server error code=%s message=%s", parts[0], parts[1])
}

func protocolExceptionError(body string) error {
	parts := strings.SplitN(body, messageSplit, 2)
	if len(parts) == 2 {
		return fmt.Errorf("BaoStock protocol exception code=%s message=%s", parts[0], parts[1])
	}
	return fmt.Errorf("BaoStock protocol exception: %s", body)
}
