package pythonapi

import (
	"encoding/json"
	"time"
)

const (
	defaultSourceName = "daily_stock_analysis_api"
	quotePath         = "/api/v1/stocks/"
	historyDataset    = "kline.daily"
	quoteDataset      = "quote.realtime"
	indexDataset      = "index.daily"
)

type QuoteResponse struct {
	StockCode    string   `json:"stock_code"`
	StockName    string   `json:"stock_name"`
	CurrentPrice float64  `json:"current_price"`
	Change       *float64 `json:"change"`
	ChangePct    *float64 `json:"change_percent"`
	Open         *float64 `json:"open"`
	High         *float64 `json:"high"`
	Low          *float64 `json:"low"`
	PrevClose    *float64 `json:"prev_close"`
	Volume       *float64 `json:"volume"`
	Amount       *float64 `json:"amount"`
	UpdateTime   string   `json:"update_time"`
}

type HistoryBar struct {
	Date          string   `json:"date"`
	Open          *float64 `json:"open"`
	High          *float64 `json:"high"`
	Low           *float64 `json:"low"`
	Close         *float64 `json:"close"`
	Volume        *float64 `json:"volume"`
	Amount        *float64 `json:"amount"`
	ChangePercent *float64 `json:"change_percent"`
}

type apiHistory struct {
	StockCode string       `json:"stock_code"`
	StockName string       `json:"stock_name"`
	Period    string       `json:"period"`
	Data      []HistoryBar `json:"data"`
}

type ProfileResponse struct {
	RequestedCode string          `json:"requested_code"`
	CanonicalCode string          `json:"canonical_code"`
	Market        string          `json:"market"`
	AsOf          string          `json:"as_of"`
	Quote         json.RawMessage `json:"quote"`
	History       json.RawMessage `json:"history"`
	Research      json.RawMessage `json:"research"`
	Intelligence  json.RawMessage `json:"intelligence"`
	Portfolio     json.RawMessage `json:"portfolio"`
	Monitors      json.RawMessage `json:"monitors"`
	Evidence      json.RawMessage `json:"evidence_quality"`
}

type HistoryResult struct {
	Symbol      string
	Name        string
	Bars        []HistoryBar
	FetchedAt   time.Time
	Dataset     string
	Limitations []string
}

type HTTPStatusError struct {
	Method string
	Path   string
	Status int
}

func (e *HTTPStatusError) Error() string {
	return "python API request " + e.Method + " " + e.Path + " returned HTTP status " + itoa(e.Status)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
