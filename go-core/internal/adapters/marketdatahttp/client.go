package marketdatahttp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	token = strings.TrimSpace(token)
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("MARKET_DATA_BASE_URL must be an absolute http(s) URL without credentials, query or fragment")
	}
	if token == "" {
		return nil, errors.New("MARKET_DATA_API_KEY is required when market-data service is enabled")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		baseURL: baseURL,
		token:   token,
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

type wireQuote struct {
	Symbol               string     `json:"symbol"`
	Name                 string     `json:"name"`
	CurrentPrice         *float64   `json:"current_price"`
	Change               *float64   `json:"change"`
	ChangePercent        *float64   `json:"change_percent"`
	Open                 *float64   `json:"open"`
	High                 *float64   `json:"high"`
	Low                  *float64   `json:"low"`
	PrevClose            *float64   `json:"prev_close"`
	Volume               *int64     `json:"volume"`
	Amount               *float64   `json:"amount"`
	VolumeRatio          *float64   `json:"volume_ratio"`
	TurnoverRate         *float64   `json:"turnover_rate"`
	QuoteTime            *time.Time `json:"quote_time"`
	FetchedAt            time.Time  `json:"fetched_at"`
	NormalizedAt         time.Time  `json:"normalized_at"`
	Source               string     `json:"source"`
	SourceVersion        string     `json:"source_version"`
	RequestID            string     `json:"request_id"`
	NormalizationVersion string     `json:"normalization_version"`
	RawVolume            *float64   `json:"raw_volume"`
	RawVolumeUnit        string     `json:"raw_volume_unit"`
	RawAmount            *float64   `json:"raw_amount"`
	RawAmountUnit        string     `json:"raw_amount_unit"`
	VolumeUnit           string     `json:"volume_unit"`
	AmountUnit           string     `json:"amount_unit"`
	AdjustmentBasis      string     `json:"adjustment_basis"`
	QualityStatus        string     `json:"quality_status"`
	QualityReasons       []string   `json:"quality_reasons"`
	FallbackFrom         string     `json:"fallback_from"`
	FallbackReason       string     `json:"fallback_reason"`
}

type wireBar struct {
	TradeDate               string     `json:"trade_date"`
	IsPartial               bool       `json:"is_partial"`
	Open                    float64    `json:"open"`
	High                    float64    `json:"high"`
	Low                     float64    `json:"low"`
	Close                   float64    `json:"close"`
	Volume                  *int64     `json:"volume"`
	Amount                  *float64   `json:"amount"`
	ChangePercent           *float64   `json:"change_percent"`
	AdjustmentBasis         string     `json:"adjustment_basis"`
	AdjustmentReferenceDate string     `json:"adjustment_reference_date"`
	Source                  string     `json:"source"`
	SourceVersion           string     `json:"source_version"`
	RequestID               string     `json:"request_id"`
	NormalizationVersion    string     `json:"normalization_version"`
	ProviderTimestamp       *time.Time `json:"provider_timestamp"`
	RawVolume               *float64   `json:"raw_volume"`
	RawVolumeUnit           string     `json:"raw_volume_unit"`
	RawAmount               *float64   `json:"raw_amount"`
	RawAmountUnit           string     `json:"raw_amount_unit"`
	VolumeUnit              string     `json:"volume_unit"`
	AmountUnit              string     `json:"amount_unit"`
	QualityStatus           string     `json:"quality_status"`
	QualityReasons          []string   `json:"quality_reasons"`
	FallbackFrom            string     `json:"fallback_from"`
	FallbackReason          string     `json:"fallback_reason"`
	FetchedAt               time.Time  `json:"fetched_at"`
	NormalizedAt            time.Time  `json:"normalized_at"`
}

type wireBundle struct {
	Symbol                string            `json:"symbol"`
	Name                  string            `json:"name"`
	Quote                 *wireQuote        `json:"quote"`
	DailyBars             []wireBar         `json:"daily_bars"`
	Sources               map[string]string `json:"sources"`
	Warnings              []string          `json:"warnings"`
	SnapshotID            string            `json:"snapshot_id"`
	AnalysisReferenceTime time.Time         `json:"analysis_reference_time"`
	FetchedAt             time.Time         `json:"fetched_at"`
	QualityStatus         string            `json:"quality_status"`
}

func (c *Client) FetchStockBundle(ctx context.Context, code string, lookback int, asOf string) (domain.StockBundle, error) {
	var wire wireBundle
	path := "/api/v1/bundles/" + url.PathEscape(strings.TrimSpace(code))
	q := url.Values{}
	q.Set("lookback_days", fmt.Sprintf("%d", lookback))
	q.Set("adjustment", "forward_adjusted")
	if strings.TrimSpace(asOf) != "" {
		q.Set("as_of", asOf)
	}
	if err := c.get(ctx, path+"?"+q.Encode(), &wire); err != nil {
		return domain.StockBundle{}, err
	}
	out := domain.StockBundle{
		Code: strings.ToUpper(strings.TrimSpace(code)), Name: wire.Name,
		Sources: wire.Sources, Warnings: append([]string(nil), wire.Warnings...),
		SnapshotID: wire.SnapshotID, AnalysisReferenceTime: wire.AnalysisReferenceTime,
		FetchedAt: wire.FetchedAt, QualityStatus: wire.QualityStatus,
		DailyBars: make([]domain.DailyBar, 0, len(wire.DailyBars)),
	}
	if wire.Quote != nil {
		w := wire.Quote
		out.Quote = &domain.Quote{
			Code: out.Code, Name: w.Name, Price: w.CurrentPrice, Change: w.Change, ChangePct: w.ChangePercent,
			Open: w.Open, High: w.High, Low: w.Low, PrevClose: w.PrevClose, Source: w.Source,
			SourceVersion: w.SourceVersion, RequestID: w.RequestID, NormalizationVersion: w.NormalizationVersion,
			VolumeRatio: w.VolumeRatio, TurnoverRate: w.TurnoverRate,
			RawVolume: w.RawVolume, RawVolumeUnit: w.RawVolumeUnit, RawAmount: w.RawAmount, RawAmountUnit: w.RawAmountUnit,
			VolumeUnit: w.VolumeUnit, AmountUnit: w.AmountUnit, AdjustmentBasis: w.AdjustmentBasis,
			QualityStatus: w.QualityStatus, QualityReasons: append([]string(nil), w.QualityReasons...),
			FallbackFrom: w.FallbackFrom, FallbackReason: w.FallbackReason,
			FetchedAt: w.FetchedAt, NormalizedAt: w.NormalizedAt,
		}
		if w.QuoteTime != nil {
			out.Quote.QuoteTime = w.QuoteTime.Format(time.RFC3339)
			out.Quote.Timestamp = out.Quote.QuoteTime
			quoteTime := *w.QuoteTime
			out.Quote.ProviderTimestamp = &quoteTime
		}
		if w.Volume != nil {
			v := float64(*w.Volume)
			out.Quote.Volume = &v
		}
		if w.Amount != nil {
			v := *w.Amount
			out.Quote.Amount = &v
		}
	}
	for _, w := range wire.DailyBars {
		bar := domain.DailyBar{
			Date: w.TradeDate, IsPartial: w.IsPartial, Open: w.Open, High: w.High, Low: w.Low, Close: w.Close,
			Source: w.Source, SourceVersion: w.SourceVersion, RequestID: w.RequestID,
			NormalizationVersion: w.NormalizationVersion, RawVolume: w.RawVolume, RawVolumeUnit: w.RawVolumeUnit,
			RawAmount: w.RawAmount, RawAmountUnit: w.RawAmountUnit, ProviderTimestamp: w.ProviderTimestamp,
			FetchedAt: w.FetchedAt, NormalizedAt: w.NormalizedAt, AdjustmentReferenceDate: w.AdjustmentReferenceDate,
			AdjustmentBasis: w.AdjustmentBasis, VolumeUnit: w.VolumeUnit, AmountUnit: w.AmountUnit,
			QualityStatus: w.QualityStatus, QualityReasons: append([]string(nil), w.QualityReasons...),
			FallbackFrom: w.FallbackFrom, FallbackReason: w.FallbackReason, MissingFields: []string{},
		}
		if w.Volume != nil {
			bar.Volume = float64(*w.Volume)
		} else {
			bar.MissingFields = append(bar.MissingFields, "volume")
		}
		if w.Amount != nil {
			bar.Amount = *w.Amount
		} else {
			bar.MissingFields = append(bar.MissingFields, "amount")
		}
		if w.ChangePercent != nil {
			bar.PctChg = *w.ChangePercent
		} else {
			bar.MissingFields = append(bar.MissingFields, "pct_chg")
		}
		out.DailyBars = append(out.DailyBars, bar)
	}
	return out, nil
}

func (c *Client) Health(ctx context.Context) error {
	var payload struct {
		Status string `json:"status"`
	}
	if err := c.get(ctx, "/health/ready", &payload); err != nil {
		return err
	}
	if payload.Status != "ready" {
		return fmt.Errorf("market-data readiness status is %q", payload.Status)
	}
	return nil
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("market-data request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("market-data HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxMarketDataResponseBytes+1))
	if err != nil {
		return fmt.Errorf("read market-data response: %w", err)
	}
	if int64(len(body)) > maxMarketDataResponseBytes {
		return fmt.Errorf("market-data response exceeds %d byte limit", maxMarketDataResponseBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("decode market-data response: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("market-data response contains multiple JSON values")
		}
		return fmt.Errorf("market-data response has trailing non-JSON content: %w", err)
	}
	return nil
}

const maxMarketDataResponseBytes int64 = 16 << 20
