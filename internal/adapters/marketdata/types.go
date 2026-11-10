package marketdata

import (
	"time"
)

const (
	defaultSourceName = "go_market_data_api"
	historyDataset    = "kline.daily"
	quoteDataset      = "quote.realtime"
	indexDataset      = "index.daily"
)

type QuoteResponse struct {
	StockCode       string    `json:"symbol"`
	StockName       string    `json:"name"`
	CurrentPrice    float64   `json:"current_price"`
	Change          *float64  `json:"change"`
	ChangePct       *float64  `json:"change_percent"`
	Open            *float64  `json:"open"`
	High            *float64  `json:"high"`
	Low             *float64  `json:"low"`
	PrevClose       *float64  `json:"prev_close"`
	Volume          *float64  `json:"volume"`
	Amount          *float64  `json:"amount"`
	UpdateTime      string    `json:"quote_time"`
	FetchedAt       time.Time `json:"fetched_at"`
	Source          string    `json:"source"`
	SourceVersion   string    `json:"source_version"`
	QualityStatus   string    `json:"quality_status"`
	QualityReasons  []string  `json:"quality_reasons"`
	VolumeUnit      string    `json:"volume_unit"`
	AmountUnit      string    `json:"amount_unit"`
	AdjustmentBasis string    `json:"adjustment_basis"`
}

type HistoryBar struct {
	Date            string     `json:"trade_date"`
	Open            *float64   `json:"open"`
	High            *float64   `json:"high"`
	Low             *float64   `json:"low"`
	Close           *float64   `json:"close"`
	Volume          *float64   `json:"volume"`
	Amount          *float64   `json:"amount"`
	ChangePercent   *float64   `json:"change_percent"`
	Source          string     `json:"source"`
	SourceVersion   string     `json:"source_version"`
	ProviderTime    *time.Time `json:"provider_timestamp"`
	FetchedAt       time.Time  `json:"fetched_at"`
	AdjustmentBasis string     `json:"adjustment_basis"`
	QualityStatus   string     `json:"quality_status"`
	QualityReasons  []string   `json:"quality_reasons"`
	VolumeUnit      string     `json:"volume_unit"`
	AmountUnit      string     `json:"amount_unit"`
}

type apiHistory struct {
	Symbol          string       `json:"symbol"`
	Period          string       `json:"period"`
	AdjustmentBasis string       `json:"adjustment_basis"`
	VolumeUnit      string       `json:"volume_unit"`
	AmountUnit      string       `json:"amount_unit"`
	QualityStatus   string       `json:"quality_status"`
	Warnings        []string     `json:"warnings"`
	Bars            []HistoryBar `json:"bars"`
}

type HistoryResult struct {
	Symbol        string
	Name          string
	Bars          []HistoryBar
	FetchedAt     time.Time
	Dataset       string
	Limitations   []string
	Warnings      []string
	QualityStatus string
}

type HTTPStatusError struct {
	Method string
	Path   string
	Status int
}

func (e *HTTPStatusError) Error() string {
	return "market-data API request " + e.Method + " " + e.Path + " returned HTTP status " + itoa(e.Status)
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
