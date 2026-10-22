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
