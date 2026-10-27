package marketdatahttp

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

type wireMarketOverviewIndex struct {
	Symbol        string   `json:"symbol"`
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	LatestPrice   *float64 `json:"latest_price"`
	Change        *float64 `json:"change"`
	ChangePercent *float64 `json:"change_percent"`
	Open          *float64 `json:"open"`
	High          *float64 `json:"high"`
	Low           *float64 `json:"low"`
	PrevClose     *float64 `json:"prev_close"`
	Amount        *float64 `json:"amount"`
	AmountUnit    string   `json:"amount_unit"`
}

type wireMarketOverviewIndexSnapshot struct {
	Items          []wireMarketOverviewIndex `json:"items"`
	Source         string                    `json:"source"`
	FetchedAt      time.Time                 `json:"fetched_at"`
	QualityStatus  string                    `json:"quality_status"`
	QualityReasons []string                  `json:"quality_reasons"`
}

type wireMarketOverviewBreadthSnapshot struct {
	TotalSecurities     int       `json:"total_securities"`
	CoveredSecurities   int       `json:"covered_securities"`
	AdvancingCount      int       `json:"advancing_count"`
	DecliningCount      int       `json:"declining_count"`
	UnchangedCount      int       `json:"unchanged_count"`
	UnclassifiedCount   int       `json:"unclassified_count"`
	LimitUpCount        *int      `json:"limit_up_count"`
	LimitDownCount      *int      `json:"limit_down_count"`
	TotalAmountCNY      *float64  `json:"total_amount_cny"`
	AmountCoverageCount int       `json:"amount_coverage_count"`
	ProviderReportDate  string    `json:"provider_report_date"`
	Source              string    `json:"source"`
	FetchedAt           time.Time `json:"fetched_at"`
	QualityStatus       string    `json:"quality_status"`
	QualityReasons      []string  `json:"quality_reasons"`
}

type wireMarketOverviewSector struct {
	Rank                int      `json:"rank"`
	Code                string   `json:"code"`
	Name                string   `json:"name"`
	Category            string   `json:"category"`
	LatestPrice         *float64 `json:"latest_price"`
	Change              *float64 `json:"change"`
	ChangePercent       *float64 `json:"change_percent"`
	Amount              *float64 `json:"amount"`
	AmountUnit          string   `json:"amount_unit"`
	AdvancingCount      *int     `json:"advancing_count"`
	DecliningCount      *int     `json:"declining_count"`
	LeadingStock        string   `json:"leading_stock"`
	LeaderChangePercent *float64 `json:"leader_change_percent"`
	Source              string   `json:"source"`
	ProviderReportDate  string   `json:"provider_report_date"`
	QualityStatus       string   `json:"quality_status"`
	QualityReasons      []string `json:"quality_reasons"`
}

type wireMarketOverviewSectorSnapshot struct {
	Category       string                     `json:"category"`
	Items          []wireMarketOverviewSector `json:"items"`
	Source         string                     `json:"source"`
	FetchedAt      time.Time                  `json:"fetched_at"`
	QualityStatus  string                     `json:"quality_status"`
	QualityReasons []string                   `json:"quality_reasons"`
}

type marketOverviewFetchResult struct {
	component string
	err       error
}

type wireStockBoard struct {
	Code           string    `json:"code"`
	Name           string    `json:"name"`
	ChangePercent  *float64  `json:"change_percent"`
	Source         string    `json:"source"`
	SourceVersion  string    `json:"source_version"`
	FetchedAt      time.Time `json:"fetched_at"`
	QualityStatus  string    `json:"quality_status"`
	QualityReasons []string  `json:"quality_reasons"`
}

type wireStockBoardSnapshot struct {
	Symbol         string           `json:"symbol"`
	Total          int              `json:"total"`
	HasData        bool             `json:"has_data"`
	Items          []wireStockBoard `json:"items"`
	Source         string           `json:"source"`
	SourceVersion  string           `json:"source_version"`
	FetchedAt      time.Time        `json:"fetched_at"`
	QualityStatus  string           `json:"quality_status"`
	QualityReasons []string         `json:"quality_reasons"`
}

type wireFundamentalMetric struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	SourceField string  `json:"source_field"`
}

type wireFundamentalSection struct {
	Category        string                  `json:"category"`
	ReportDate      string                  `json:"report_date"`
	PublicationDate string                  `json:"publication_date"`
	HasData         bool                    `json:"has_data"`
	Metrics         []wireFundamentalMetric `json:"metrics"`
	QualityStatus   string                  `json:"quality_status"`
	QualityReasons  []string                `json:"quality_reasons"`
}

type wireFundamentalsSnapshot struct {
	Symbol         string                   `json:"symbol"`
	Year           int                      `json:"year"`
	Quarter        int                      `json:"quarter"`
	HasData        bool                     `json:"has_data"`
	Sections       []wireFundamentalSection `json:"sections"`
	Source         string                   `json:"source"`
	SourceVersion  string                   `json:"source_version"`
	FetchedAt      time.Time                `json:"fetched_at"`
	QualityStatus  string                   `json:"quality_status"`
	QualityReasons []string                 `json:"quality_reasons"`
}

type wireChipDistribution struct {
	Symbol          string    `json:"symbol"`
	Date            string    `json:"date"`
	AverageCost     float64   `json:"average_cost"`
	Cost90Low       *float64  `json:"cost_90_low"`
	Cost90High      *float64  `json:"cost_90_high"`
	Cost70Low       *float64  `json:"cost_70_low"`
	Cost70High      *float64  `json:"cost_70_high"`
	SourceVersion   string    `json:"source_version"`
	ProfitRatio     *float64  `json:"profit_ratio"`
	Concentration90 *float64  `json:"concentration_90"`
	Concentration70 *float64  `json:"concentration_70"`
	Source          string    `json:"source"`
	ProviderDate    string    `json:"provider_date"`
	FetchedAt       time.Time `json:"fetched_at"`
	QualityStatus   string    `json:"quality_status"`
	QualityReasons  []string  `json:"quality_reasons"`
}

type wireCapitalFlow struct {
	Symbol         string    `json:"symbol"`
	SourceVersion  string    `json:"source_version"`
	MainNetInflow  *float64  `json:"main_net_inflow_yuan"`
	Inflow5D       *float64  `json:"inflow_5d_yuan"`
	Inflow10D      *float64  `json:"inflow_10d_yuan"`
	Observations   int       `json:"observations"`
	Source         string    `json:"source"`
	FetchedAt      time.Time `json:"fetched_at"`
	QualityStatus  string    `json:"quality_status"`
	QualityReasons []string  `json:"quality_reasons"`
}

type wireDragonTigerItem struct {
	Symbol    string `json:"symbol"`
	Code      string `json:"code"`
	TradeDate string `json:"trade_date"`
}

type wireDragonTigerSnapshot struct {
	TradeDate      string                `json:"trade_date"`
	Page           int                   `json:"page"`
	PageSize       int                   `json:"page_size"`
	Total          int                   `json:"total"`
	Pages          int                   `json:"pages"`
	Returned       int                   `json:"returned"`
	SourceReturned int                   `json:"source_returned"`
	HasData        bool                  `json:"has_data"`
	HasMore        bool                  `json:"has_more"`
	Items          []wireDragonTigerItem `json:"items"`
	Source         string                `json:"source"`
	SourceVersion  string                `json:"source_version"`
	FetchedAt      time.Time             `json:"fetched_at"`
	QualityStatus  string                `json:"quality_status"`
	QualityReasons []string              `json:"quality_reasons"`
}

type dragonTigerDateResult struct {
	date           string
	items          []wireDragonTigerItem
	source         string
	sourceVersion  string
	fetchedAt      time.Time
	qualityStatus  string
	qualityReasons []string
	complete       bool
	err            error
}

type wireStockAnnouncement struct {
	ArticleID        string     `json:"article_id"`
	Symbol           string     `json:"symbol"`
	StockName        string     `json:"stock_name"`
	Title            string     `json:"title"`
	Category         string     `json:"category"`
	AnnouncementDate string     `json:"announcement_date"`
	PublishedAt      *time.Time `json:"published_at"`
	SourceURL        string     `json:"source_url"`
	Source           string     `json:"source"`
	SourceVersion    string     `json:"source_version"`
	FetchedAt        time.Time  `json:"fetched_at"`
	QualityStatus    string     `json:"quality_status"`
	QualityReasons   []string   `json:"quality_reasons"`
}

type wireStockAnnouncementSnapshot struct {
	Symbol         string                  `json:"symbol"`
	StartDate      string                  `json:"start_date"`
	EndDate        string                  `json:"end_date"`
	Page           int                     `json:"page"`
	PageSize       int                     `json:"page_size"`
	Total          int                     `json:"total"`
	Pages          int                     `json:"pages"`
	Returned       int                     `json:"returned"`
	HasData        bool                    `json:"has_data"`
	HasMore        bool                    `json:"has_more"`
	Items          []wireStockAnnouncement `json:"items"`
	Source         string                  `json:"source"`
	SourceVersion  string                  `json:"source_version"`
	FetchedAt      time.Time               `json:"fetched_at"`
	QualityStatus  string                  `json:"quality_status"`
	QualityReasons []string                `json:"quality_reasons"`
}

type stockAuxResult struct {
	kind           string
	source         string
	qualityStatus  string
	qualityReasons []string
	warning        string
	err            error
	fundamentals   map[string]any
	boards         []map[string]any
	chip           *domain.ChipDistribution
	capitalFlow    map[string]any
	dragonTiger    map[string]any
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
	if strings.TrimSpace(asOf) == "" {
		if err := c.fetchStockAuxiliary(ctx, code, &out); err != nil {
			return domain.StockBundle{}, err
		}
	} else {
		// Auxiliary endpoints expose current snapshots, not as-of reads.
		// Never attach a later snapshot to an explicitly timestamped analysis.
		out.Warnings = appendUniqueWarning(out.Warnings, "native_auxiliary_snapshots_skipped_for_explicit_as_of")
	}
	return out, nil
}

func (c *Client) fetchStockAuxiliary(ctx context.Context, code string, out *domain.StockBundle) error {
	if ctx == nil {
		return errors.New("stock auxiliary context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	results := make(chan stockAuxResult, 5)
	go func() { results <- c.fetchFundamentals(ctx, code) }()
	go func() { results <- c.fetchStockBoards(ctx, code) }()
	go func() { results <- c.fetchChipDistribution(ctx, code) }()
	go func() { results <- c.fetchCapitalFlow(ctx, code) }()
	go func() { results <- c.fetchDragonTigerSummary(ctx, code, out.DailyBars) }()

	for i := 0; i < 5; i++ {
		result := <-results
		if err := ctx.Err(); err != nil {
			return err
		}
		if result.err != nil {
			out.Warnings = appendUniqueWarning(out.Warnings, "go_market_data_"+result.kind+"_unavailable")
			continue
		}
		if result.warning != "" {
			out.Warnings = appendUniqueWarning(out.Warnings, result.warning)
		}
		if strings.TrimSpace(result.source) != "" {
			if out.Sources == nil {
				out.Sources = make(map[string]string)
			}
			out.Sources[result.kind] = result.source
		}
		if result.qualityStatus != "" && result.qualityStatus != "valid" {
			out.Warnings = appendUniqueWarning(out.Warnings, "go_market_data_"+result.kind+"_quality_"+result.qualityStatus)
		}
		for _, reason := range result.qualityReasons {
			reason = strings.TrimSpace(reason)
			if reason != "" {
				out.Warnings = appendUniqueWarning(out.Warnings, "go_market_data_"+result.kind+"_quality_reason:"+reason)
			}
		}
		switch result.kind {
		case "fundamentals":
			out.Fundamentals = result.fundamentals
		case "stock_boards":
			out.Boards = result.boards
		case "chip_distribution":
			out.Chip = result.chip
		case "capital_flow":
			out.CapitalFlow = result.capitalFlow
		case "dragon_tiger":
			out.DragonTiger = result.dragonTiger
		}
	}
	return nil
}

func (c *Client) fetchFundamentals(ctx context.Context, code string) stockAuxResult {
	result := stockAuxResult{kind: "fundamentals"}
	year, quarter := latestPublicReportPeriod(time.Now())
	query := url.Values{}
	query.Set("year", fmt.Sprintf("%d", year))
	query.Set("quarter", fmt.Sprintf("%d", quarter))
	var snapshot wireFundamentalsSnapshot
	path := "/api/v1/stocks/" + url.PathEscape(strings.TrimSpace(code)) + "/fundamentals?" + query.Encode()
	if err := c.get(ctx, path, &snapshot); err != nil {
		result.err = err
		return result
	}
	if !sameStockCode(snapshot.Symbol, code) || snapshot.Year != year || snapshot.Quarter != quarter {
		result.err = errors.New("fundamentals response identity/period mismatch")
		return result
	}
	result.source, result.qualityStatus, result.qualityReasons = snapshot.Source, snapshot.QualityStatus, snapshot.QualityReasons
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		result.err = errors.New("fundamentals response is missing source or fetched_at")
		return result
	}
	if !snapshot.HasData {
		result.warning = fmt.Sprintf("go_market_data_fundamentals_no_data_for_%d_q%d", year, quarter)
		return result
	}
	if snapshot.QualityStatus != "valid" && snapshot.QualityStatus != "degraded" {
		result.err = errors.New("fundamentals quality status is not usable")
		return result
	}
	if len(snapshot.Sections) == 0 {
		result.err = errors.New("fundamentals response has no sections")
		return result
	}
	result.fundamentals = fundamentalsMap(snapshot)
	return result
}

func (c *Client) fetchStockBoards(ctx context.Context, code string) stockAuxResult {
	result := stockAuxResult{kind: "stock_boards"}
	var snapshot wireStockBoardSnapshot
	path := "/api/v1/stocks/" + url.PathEscape(strings.TrimSpace(code)) + "/boards"
	if err := c.get(ctx, path, &snapshot); err != nil {
		result.err = err
		return result
	}
	if !sameStockCode(snapshot.Symbol, code) {
		result.err = errors.New("stock boards response symbol mismatch")
		return result
	}
	result.source, result.qualityStatus, result.qualityReasons = snapshot.Source, snapshot.QualityStatus, snapshot.QualityReasons
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		result.err = errors.New("stock boards response is missing source or fetched_at")
		return result
	}
	if !snapshot.HasData {
		result.warning = "go_market_data_stock_boards_empty_result_is_ambiguous"
		return result
	}
	if snapshot.QualityStatus != "valid" && snapshot.QualityStatus != "degraded" {
		result.err = errors.New("stock boards quality status is not usable")
		return result
	}
	if snapshot.Total < len(snapshot.Items) {
		result.err = errors.New("stock boards total is smaller than returned item count")
		return result
	}
	result.boards = make([]map[string]any, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" {
			result.err = errors.New("stock board item is missing code or name")
			result.boards = nil
			return result
		}
		result.boards = append(result.boards, jsonObjectMap(item))
	}
	return result
}

func (c *Client) fetchChipDistribution(ctx context.Context, code string) stockAuxResult {
	result := stockAuxResult{kind: "chip_distribution"}
	var snapshot wireChipDistribution
	path := "/api/v1/stocks/" + url.PathEscape(strings.TrimSpace(code)) + "/chip-distribution"
	if err := c.get(ctx, path, &snapshot); err != nil {
		result.err = err
		return result
	}
	if !sameStockCode(snapshot.Symbol, code) {
		result.err = errors.New("chip distribution response symbol mismatch")
		return result
	}
	result.source, result.qualityStatus, result.qualityReasons = snapshot.Source, snapshot.QualityStatus, snapshot.QualityReasons
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		result.err = errors.New("chip distribution response is missing source or fetched_at")
		return result
	}
	if snapshot.QualityStatus != "valid" && snapshot.QualityStatus != "degraded" {
		result.err = errors.New("chip distribution quality status is not usable")
		return result
	}
	if snapshot.AverageCost <= 0 || math.IsNaN(snapshot.AverageCost) || math.IsInf(snapshot.AverageCost, 0) {
		result.warning = "go_market_data_chip_distribution_average_cost_unavailable"
	} else {
		average := snapshot.AverageCost
		result.chip = &domain.ChipDistribution{CostAverage: &average}
	}
	if result.chip == nil {
		result.chip = &domain.ChipDistribution{}
	}
	result.chip.ProfitRatio = usableFiniteValue(snapshot.ProfitRatio)
	result.chip.Concentration90 = usableFiniteValue(snapshot.Concentration90)
	result.chip.Concentration70 = usableFiniteValue(snapshot.Concentration70)
	result.chip.Raw = jsonObjectMap(snapshot)
	if result.chip.CostAverage == nil && result.chip.ProfitRatio == nil &&
		result.chip.Concentration90 == nil && result.chip.Concentration70 == nil {
		result.chip = nil
		result.warning = "go_market_data_chip_distribution_has_no_usable_metrics"
	}
	return result
}

func (c *Client) fetchCapitalFlow(ctx context.Context, code string) stockAuxResult {
	result := stockAuxResult{kind: "capital_flow"}
	var snapshot wireCapitalFlow
	path := "/api/v1/stocks/" + url.PathEscape(strings.TrimSpace(code)) + "/capital-flow"
	if err := c.get(ctx, path, &snapshot); err != nil {
		result.err = err
		return result
	}
	if !sameStockCode(snapshot.Symbol, code) {
		result.err = errors.New("capital flow response symbol mismatch")
		return result
	}
	result.source, result.qualityStatus, result.qualityReasons = snapshot.Source, snapshot.QualityStatus, snapshot.QualityReasons
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() || snapshot.Observations < 0 {
		result.err = errors.New("capital flow response metadata or observation count is invalid")
		return result
	}
	if snapshot.QualityStatus != "valid" && snapshot.QualityStatus != "degraded" {
		result.err = errors.New("capital flow quality status is not usable")
		return result
	}
	if usableFiniteValue(snapshot.MainNetInflow) == nil && usableFiniteValue(snapshot.Inflow5D) == nil &&
		usableFiniteValue(snapshot.Inflow10D) == nil {
		result.warning = "go_market_data_capital_flow_has_no_usable_metrics"
		return result
	}
	result.capitalFlow = jsonObjectMap(snapshot)
	return result
}

// latestPublicReportPeriod selects the newest quarter whose standard A-share
// disclosure deadline has passed. This avoids treating an in-progress Q3/Q4
// filing window as if every listed company had already published that report.

func (c *Client) fetchDragonTigerSummary(ctx context.Context, code string, bars []domain.DailyBar) stockAuxResult {
	result := stockAuxResult{kind: "dragon_tiger"}
	dates := recentDragonTigerDates(bars, 10)
	if len(dates) == 0 {
		result.err = errors.New("no completed daily-bar dates available for dragon-tiger summary")
		return result
	}
	results := make(chan dragonTigerDateResult, len(dates))
	semaphore := make(chan struct{}, 4)
	for _, date := range dates {
		date := date
		go func() {
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				results <- dragonTigerDateResult{date: date, err: ctx.Err()}
				return
			}
			dateResult := c.fetchDragonTigerDate(ctx, date)
			<-semaphore
			results <- dateResult
		}()
	}
	summaryReasons := []string{}
	source, sourceVersion := "", ""
	var newestFetch time.Time
	matchedDates := make(map[string]struct{})
	coveredDates := 0
	hasIncompleteCoverage := false
	var latestIsOnList *bool
	latestReferenceDate := dates[0]
	for range dates {
		dateResult := <-results
		if dateResult.err != nil {
			hasIncompleteCoverage = true
			summaryReasons = appendUniqueWarning(summaryReasons, "dragon_tiger_date_fetch_failed:"+dateResult.date)
			continue
		}
		if source == "" {
			source, sourceVersion = dateResult.source, dateResult.sourceVersion
		} else if dateResult.source != "" && dateResult.source != source {
			hasIncompleteCoverage = true
			summaryReasons = appendUniqueWarning(summaryReasons, "dragon_tiger_mixed_provider_sources")
		}
		if dateResult.fetchedAt.After(newestFetch) {
			newestFetch = dateResult.fetchedAt
		}
		for _, reason := range dateResult.qualityReasons {
			summaryReasons = appendUniqueWarning(summaryReasons, reason)
		}
		matched := false
		for _, item := range dateResult.items {
			symbol := item.Symbol
			if strings.TrimSpace(symbol) == "" {
				symbol = item.Code
			}
			if sameStockCode(code, symbol) {
				matched = true
				break
			}
		}
		if matched {
			matchedDates[dateResult.date] = struct{}{}
		}
		if dateResult.complete && len(dateResult.items) > 0 {
			coveredDates++
		} else {
			hasIncompleteCoverage = true
		}
		if dateResult.date == latestReferenceDate {
			if matched {
				onList := true
				latestIsOnList = &onList
			} else if dateResult.complete && len(dateResult.items) > 0 {
				onList := false
				latestIsOnList = &onList
			}
		}
	}
	if len(matchedDates) == 0 {
		summaryReasons = appendUniqueWarning(summaryReasons, "dragon_tiger_recent_coverage_has_no_confirmed_stock_match")
	}
	if coveredDates < len(dates) {
		summaryReasons = appendUniqueWarning(summaryReasons, "dragon_tiger_lookback_coverage_incomplete")
	}
	latestMatchDate := ""
	for date := range matchedDates {
		if date > latestMatchDate {
			latestMatchDate = date
		}
	}
	qualityStatus := "unknown"
	if coveredDates > 0 || len(matchedDates) > 0 {
		qualityStatus = "degraded"
		summaryReasons = appendUniqueWarning(summaryReasons, "provider_snapshot_timestamp_not_exposed")
	}
	if hasIncompleteCoverage && qualityStatus != "unknown" {
		summaryReasons = appendUniqueWarning(summaryReasons, "dragon_tiger_summary_is_partial")
	}
	summary := map[string]any{
		"is_on_list": nil, "recent_count": len(matchedDates),
		"recent_count_is_lower_bound": hasIncompleteCoverage,
		"reference_trade_date": latestReferenceDate,
		"lookback_trading_days": len(dates), "covered_trading_days": coveredDates,
		"quality_status": qualityStatus, "quality_reasons": summaryReasons,
		"source": source, "source_version": sourceVersion,
	}
	if !newestFetch.IsZero() {
		summary["fetched_at"] = newestFetch
	}
	if latestIsOnList != nil {
		summary["is_on_list"] = *latestIsOnList
	}
	if latestMatchDate != "" {
		summary["latest_date"] = latestMatchDate
	}
	result.dragonTiger = summary
	result.source, result.qualityStatus, result.qualityReasons = source, qualityStatus, summaryReasons
	if qualityStatus == "unknown" {
		result.warning = "go_market_data_dragon_tiger_no_confirmed_coverage"
	} else if hasIncompleteCoverage {
		result.warning = "go_market_data_dragon_tiger_summary_partial_coverage"
	}
	return result
}

func recentDragonTigerDates(bars []domain.DailyBar, limit int) []string {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	seen := make(map[string]struct{}, len(bars))
	dates := make([]string, 0, len(bars))
	for _, bar := range bars {
		if bar.IsPartial || len(bar.Date) != 10 || bar.Date > today {
			continue
		}
		if _, err := time.Parse("2006-01-02", bar.Date); err != nil {
			continue
		}
		if _, exists := seen[bar.Date]; exists {
			continue
		}
		seen[bar.Date] = struct{}{}
		dates = append(dates, bar.Date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))
	if limit > 0 && len(dates) > limit {
		dates = dates[:limit]
	}
	return dates
}

func (c *Client) fetchDragonTigerDate(ctx context.Context, date string) dragonTigerDateResult {
	result := dragonTigerDateResult{date: date}
	const pageSize = 500
	const maxPages = 10
	var total, pages int
	var source, sourceVersion string
	allItems := make([]wireDragonTigerItem, 0)
	for page := 1; page <= maxPages; page++ {
		query := url.Values{}
		query.Set("date", date)
		query.Set("page", fmt.Sprintf("%d", page))
		query.Set("page_size", fmt.Sprintf("%d", pageSize))
		var snapshot wireDragonTigerSnapshot
		if err := c.get(ctx, "/api/v1/market/dragon-tiger?"+query.Encode(), &snapshot); err != nil {
			result.err = err
			return result
		}
		if snapshot.TradeDate != date || snapshot.Page != page || snapshot.PageSize != pageSize ||
			strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
			result.err = errors.New("dragon-tiger response identity/source/fetched_at mismatch")
			return result
		}
		if snapshot.Total < 0 || snapshot.Pages < 0 || snapshot.Returned != len(snapshot.Items) || snapshot.Returned > pageSize {
			result.err = errors.New("dragon-tiger response pagination counts are invalid")
			return result
		}
		if page == 1 {
			total, pages, source, sourceVersion = snapshot.Total, snapshot.Pages, snapshot.Source, snapshot.SourceVersion
			result.source, result.sourceVersion = source, sourceVersion
		} else if snapshot.Total != total || snapshot.Pages != pages || snapshot.Source != source {
			result.err = errors.New("dragon-tiger pagination identity/source changed between pages")
			return result
		}
		if snapshot.FetchedAt.After(result.fetchedAt) { result.fetchedAt = snapshot.FetchedAt }
		for _, item := range snapshot.Items {
			if item.TradeDate != date {
				result.err = errors.New("dragon-tiger item trade date mismatch")
				return result
			}
			allItems = append(allItems, item)
		}
		for _, reason := range snapshot.QualityReasons {
			result.qualityReasons = appendUniqueWarning(result.qualityReasons, strings.TrimSpace(reason))
		}
		if snapshot.Total == 0 {
			if snapshot.Pages != 0 || snapshot.Returned != 0 || snapshot.HasData || snapshot.HasMore || snapshot.QualityStatus != "unknown" {
				result.err = errors.New("empty dragon-tiger snapshot must remain unknown")
				return result
			}
			result.qualityStatus = "unknown"
			result.qualityReasons = appendUniqueWarning(result.qualityReasons, "dragon_tiger_source_returned_unknown_empty_snapshot")
			return result
		}
		if snapshot.Pages != (snapshot.Total+pageSize-1)/pageSize || page > snapshot.Pages ||
			snapshot.HasMore != (page < snapshot.Pages) {
			result.err = errors.New("dragon-tiger pagination counters are inconsistent")
			return result
		}
		if snapshot.HasData && snapshot.QualityStatus != "valid" && snapshot.QualityStatus != "degraded" {
			result.err = errors.New("dragon-tiger non-empty data has unusable quality status")
			return result
		}
		if !snapshot.HasData && snapshot.Returned > 0 {
			result.err = errors.New("dragon-tiger returned rows while has_data is false")
			return result
		}
		if page == snapshot.Pages {
			if snapshot.HasMore {
				result.err = errors.New("dragon-tiger final page still reports more data")
				return result
			}
			result.complete = true
			result.qualityStatus = snapshot.QualityStatus
			break
		}
	}
	if pages > maxPages {
		result.complete = false
		result.qualityReasons = appendUniqueWarning(result.qualityReasons, "dragon_tiger_pagination_capped_at_ten_pages")
	}
	result.items = allItems
	if len(allItems) == 0 {
		result.complete = false
		result.qualityStatus = "unknown"
		result.qualityReasons = appendUniqueWarning(result.qualityReasons, "dragon_tiger_source_has_no_usable_a_share_rows")
	}
	return result
}
func latestPublicReportPeriod(now time.Time) (int, int) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	now = now.In(location)
	switch {
	case !now.Before(time.Date(now.Year(), time.November, 1, 0, 0, 0, 0, location)):
		return now.Year(), 3
	case !now.Before(time.Date(now.Year(), time.September, 1, 0, 0, 0, 0, location)):
		return now.Year(), 2
	case !now.Before(time.Date(now.Year(), time.May, 1, 0, 0, 0, 0, location)):
		return now.Year(), 1
	default:
		return now.Year() - 1, 3
	}
}

func sameStockCode(left, right string) bool {
	canonical := func(value string) string {
		value = strings.ToUpper(strings.TrimSpace(value))
		if index := strings.IndexByte(value, '.'); index >= 0 {
			value = value[:index]
		}
		if len(value) == 8 && (strings.HasPrefix(value, "SH") || strings.HasPrefix(value, "SZ")) {
			value = value[2:]
		}
		return value
	}
	return canonical(left) != "" && canonical(left) == canonical(right)
}

func usableFiniteValue(value *float64) *float64 {
	if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
		return nil
	}
	copy := *value
	return &copy
}

func jsonObjectMap(input any) map[string]any {
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return nil
	}
	return result
}

func appendUniqueWarning(warnings []string, warning string) []string {
	warning = strings.TrimSpace(warning)
	if warning == "" {
		return warnings
	}
	for _, existing := range warnings {
		if existing == warning {
			return warnings
		}
	}
	return append(warnings, warning)
}

func fundamentalsMap(snapshot wireFundamentalsSnapshot) map[string]any {
	sections := make([]map[string]any, 0, len(snapshot.Sections))
	metrics := make(map[string]any)
	seen := make(map[string]bool)
	duplicates := make(map[string]bool)
	for _, section := range snapshot.Sections {
		sectionMetrics := make([]wireFundamentalMetric, 0, len(section.Metrics))
		for _, metric := range section.Metrics {
			if strings.TrimSpace(metric.Key) == "" || math.IsNaN(metric.Value) || math.IsInf(metric.Value, 0) {
				continue
			}
			sectionMetrics = append(sectionMetrics, metric)
			metrics[section.Category+"."+metric.Key] = metric.Value
			if seen[metric.Key] {
				duplicates[metric.Key] = true
				delete(metrics, metric.Key)
			} else if !duplicates[metric.Key] {
				seen[metric.Key] = true
				metrics[metric.Key] = metric.Value
			}
		}
		sections = append(sections, map[string]any{
			"category": section.Category, "report_date": section.ReportDate,
			"publication_date": section.PublicationDate, "has_data": section.HasData,
			"metrics": sectionMetrics, "quality_status": section.QualityStatus,
			"quality_reasons": section.QualityReasons,
		})
	}
	result := map[string]any{
		"symbol": snapshot.Symbol, "year": snapshot.Year, "quarter": snapshot.Quarter,
		"has_data": snapshot.HasData, "sections": sections, "metrics": metrics,
		"source": snapshot.Source, "source_version": snapshot.SourceVersion,
		"fetched_at": snapshot.FetchedAt, "quality_status": snapshot.QualityStatus,
		"quality_reasons": snapshot.QualityReasons,
	}
	for key, value := range metrics {
		if !strings.Contains(key, ".") {
			result[key] = value
		}
	}
	return result
}

// FetchStockAnnouncements validates date, identity, pagination and source quality.
// FetchedAt remains ingestion time and is never substituted for a source publication timestamp.
func (c *Client) FetchStockAnnouncements(ctx context.Context, code, startDate, endDate string, page, pageSize int) (domain.StockAnnouncementSnapshot, error) {
	symbol := strings.TrimSpace(code)
	if symbol == "" {
		return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement symbol is required")
	}
	start, startErr := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	end, endErr := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if startErr != nil || endErr != nil || start.Format("2006-01-02") != startDate || end.Format("2006-01-02") != endDate || start.After(end) {
		return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement dates must be valid YYYY-MM-DD values with start_date <= end_date")
	}
	if page < 1 || page > 1000 || pageSize < 1 || pageSize > 100 {
		return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement page/page_size is outside the supported range")
	}
	query := url.Values{}
	query.Set("start_date", startDate)
	query.Set("end_date", endDate)
	query.Set("page", fmt.Sprintf("%d", page))
	query.Set("page_size", fmt.Sprintf("%d", pageSize))
	var wire wireStockAnnouncementSnapshot
	path := "/api/v1/stocks/" + url.PathEscape(symbol) + "/announcements?" + query.Encode()
	if err := c.get(ctx, path, &wire); err != nil {
		return domain.StockAnnouncementSnapshot{}, err
	}
	if !sameStockCode(symbol, wire.Symbol) || wire.StartDate != startDate || wire.EndDate != endDate || wire.Page != page || wire.PageSize != pageSize {
		return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement response identity does not match the request")
	}
	if strings.TrimSpace(wire.Source) == "" || wire.FetchedAt.IsZero() {
		return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement response is missing source or fetched_at")
	}
	if wire.Total < 0 || wire.Pages < 0 || wire.Returned != len(wire.Items) || wire.Returned > pageSize {
		return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement response counts are inconsistent")
	}
	if wire.Total == 0 {
		if wire.Pages != 0 || wire.Returned != 0 || len(wire.Items) != 0 || wire.HasData || wire.HasMore || wire.QualityStatus != "unknown" {
			return domain.StockAnnouncementSnapshot{}, errors.New("empty stock announcement response must remain unknown, not a confirmed absence")
		}
	} else {
		if wire.Pages != (wire.Total+pageSize-1)/pageSize || page > wire.Pages || wire.Returned == 0 || wire.Total < wire.Returned || !wire.HasData || wire.HasMore != (page < wire.Pages) {
			return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement pagination counters are inconsistent")
		}
		if wire.QualityStatus != "valid" && wire.QualityStatus != "degraded" {
			return domain.StockAnnouncementSnapshot{}, fmt.Errorf("unsupported stock announcement quality status %q", wire.QualityStatus)
		}
	}
	items := make([]domain.StockAnnouncement, 0, len(wire.Items))
	seenIDs := make(map[string]struct{}, len(wire.Items))
	for _, item := range wire.Items {
		if strings.TrimSpace(item.ArticleID) == "" || strings.TrimSpace(item.Title) == "" || !sameStockCode(symbol, item.Symbol) ||
			strings.TrimSpace(item.Source) != strings.TrimSpace(wire.Source) || item.FetchedAt.IsZero() {
			return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement item has missing identity/title/source metadata")
		}
		if _, ok := seenIDs[item.ArticleID]; ok {
			return domain.StockAnnouncementSnapshot{}, fmt.Errorf("stock announcement page has duplicate article ID %q", item.ArticleID)
		}
		seenIDs[item.ArticleID] = struct{}{}
		date, err := time.Parse("2006-01-02", item.AnnouncementDate)
		if err != nil || date.Format("2006-01-02") != item.AnnouncementDate || item.AnnouncementDate < startDate || item.AnnouncementDate > endDate {
			return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement item date is invalid or outside the requested range")
		}
		parsedURL, err := url.Parse(strings.TrimSpace(item.SourceURL))
		if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement item has an invalid source_url")
		}
		if item.QualityStatus != "valid" && item.QualityStatus != "degraded" {
			return domain.StockAnnouncementSnapshot{}, fmt.Errorf("unsupported stock announcement item quality status %q", item.QualityStatus)
		}
		if item.PublishedAt == nil && item.QualityStatus == "valid" {
			return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement without a source publication timestamp must not be marked valid")
		}
		if item.PublishedAt != nil && item.PublishedAt.IsZero() {
			return domain.StockAnnouncementSnapshot{}, errors.New("stock announcement source publication timestamp is zero")
		}
		items = append(items, domain.StockAnnouncement{
			ArticleID: item.ArticleID, Symbol: item.Symbol, StockName: item.StockName, Title: item.Title, Category: item.Category,
			AnnouncementDate: item.AnnouncementDate, PublishedAt: item.PublishedAt, SourceURL: item.SourceURL, Source: item.Source,
			SourceVersion: item.SourceVersion, FetchedAt: item.FetchedAt, QualityStatus: item.QualityStatus,
			QualityReasons: append([]string(nil), item.QualityReasons...),
		})
	}
	return domain.StockAnnouncementSnapshot{
		Symbol: wire.Symbol, StartDate: wire.StartDate, EndDate: wire.EndDate, Page: wire.Page, PageSize: wire.PageSize,
		Total: wire.Total, Pages: wire.Pages, Returned: wire.Returned, HasData: wire.HasData, HasMore: wire.HasMore,
		Items: items, Source: wire.Source, SourceVersion: wire.SourceVersion, FetchedAt: wire.FetchedAt,
		QualityStatus: wire.QualityStatus, QualityReasons: append([]string(nil), wire.QualityReasons...),
	}, nil
}

func (c *Client) FetchMarketOverview(ctx context.Context, region, asOf string) (domain.MarketOverview, error) {
	region = strings.ToLower(strings.TrimSpace(region))
	if region == "" {
		region = "cn"
	}
	if region != "cn" {
		return domain.MarketOverview{}, fmt.Errorf("native market overview does not support region %q", region)
	}
	if ctx == nil {
		return domain.MarketOverview{}, errors.New("market overview context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return domain.MarketOverview{}, err
	}

	now := time.Now().UTC()
	if raw := strings.TrimSpace(asOf); raw != "" {
		referenceTime, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return domain.MarketOverview{}, errors.New("native market overview as_of must be an RFC3339 timestamp")
		}
		age := now.Sub(referenceTime)
		if age < 0 || age > 90*time.Second {
			return domain.MarketOverview{}, errors.New("native market overview only supports a live snapshot; as_of must be within 90 seconds before execution")
		}
	}

	industryQuery := url.Values{"type": {"industry"}, "limit": {"20"}}
	conceptQuery := url.Values{"type": {"concept"}, "limit": {"20"}}
	indices := &wireMarketOverviewIndexSnapshot{}
	breadth := &wireMarketOverviewBreadthSnapshot{}
	industry := &wireMarketOverviewSectorSnapshot{}
	concept := &wireMarketOverviewSectorSnapshot{}
	requests := []struct {
		component string
		path      string
		output    any
	}{
		{component: "indices", path: "/api/v1/market/indices", output: indices},
		{component: "breadth", path: "/api/v1/market/breadth", output: breadth},
		{component: "industry", path: "/api/v1/market/sectors?" + industryQuery.Encode(), output: industry},
		{component: "concept", path: "/api/v1/market/sectors?" + conceptQuery.Encode(), output: concept},
	}
	results := make(chan marketOverviewFetchResult, len(requests))
	for _, request := range requests {
		request := request
		go func() {
			results <- marketOverviewFetchResult{
				component: request.component,
				err:       c.get(ctx, request.path, request.output),
			}
		}()
	}

	failed := make([]string, 0, len(requests))
	for range requests {
		result := <-results
		if result.err != nil {
			failed = append(failed, result.component)
		}
	}
	if err := ctx.Err(); err != nil {
		return domain.MarketOverview{}, err
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	marketNow := time.Now().In(loc)
	out := domain.MarketOverview{
		Region:   region,
		Date:     marketNow.Format("2006-01-02"),
		Indices:  make([]domain.MarketIndex, 0, len(indices.Items)),
		Warnings: []string{},
	}
	sources := make([]string, 0, 4)
	for _, part := range failed {
		appendOverviewWarning(&out.Warnings, "native_market_overview_component_unavailable_"+part)
	}

	if !containsOverviewString(failed, "indices") {
		appendOverviewSource(&sources, "indices", indices.Source)
		appendOverviewQualityWarnings(&out.Warnings, "indices", indices.QualityStatus, indices.QualityReasons)
		appendOverviewFreshnessWarning(&out.Warnings, "indices", indices.FetchedAt, marketNow)
		for _, item := range indices.Items {
			code := strings.TrimSpace(item.Code)
			if code == "" {
				code = strings.TrimSpace(strings.Split(item.Symbol, ".")[0])
			}
			if code == "" || strings.TrimSpace(item.Name) == "" ||
				!finiteOverviewNumber(item.LatestPrice) || *item.LatestPrice <= 0 ||
				!finiteOverviewNumber(item.Change) || !finiteOverviewNumber(item.ChangePercent) {
				appendOverviewWarning(&out.Warnings, "native_market_overview_invalid_index_item_skipped")
				continue
			}
			index := domain.MarketIndex{
				Code: code, Name: strings.TrimSpace(item.Name),
				Current: *item.LatestPrice, Change: *item.Change, ChangePct: *item.ChangePercent,
			}
			if finiteOverviewNumber(item.Open) {
				index.Open = *item.Open
			}
			if finiteOverviewNumber(item.High) {
				index.High = *item.High
			}
			if finiteOverviewNumber(item.Low) {
				index.Low = *item.Low
			}
			if finiteOverviewNumber(item.PrevClose) {
				index.PrevClose = *item.PrevClose
			}
			if finiteOverviewNumber(item.Amount) && *item.Amount >= 0 {
				index.Amount = *item.Amount
			}
			out.Indices = append(out.Indices, index)
		}
	}

	if !containsOverviewString(failed, "breadth") {
		appendOverviewSource(&sources, "breadth", breadth.Source)
		appendOverviewQualityWarnings(&out.Warnings, "breadth", breadth.QualityStatus, breadth.QualityReasons)
		appendOverviewFreshnessWarning(&out.Warnings, "breadth", breadth.FetchedAt, marketNow)
		if date, ok := strictOverviewDate(breadth.ProviderReportDate); ok {
			out.Date = date
		} else {
			appendOverviewWarning(&out.Warnings, "native_market_overview_provider_report_date_missing_using_collection_date")
		}
		out.UpCount, out.DownCount, out.FlatCount = breadth.AdvancingCount, breadth.DecliningCount, breadth.UnchangedCount
		if breadth.LimitUpCount != nil && *breadth.LimitUpCount >= 0 {
			out.LimitUpCount = *breadth.LimitUpCount
		} else {
			appendOverviewWarning(&out.Warnings, "native_market_overview_limit_up_count_unavailable")
		}
		if breadth.LimitDownCount != nil && *breadth.LimitDownCount >= 0 {
			out.LimitDownCount = *breadth.LimitDownCount
		} else {
			appendOverviewWarning(&out.Warnings, "native_market_overview_limit_down_count_unavailable")
		}
		if finiteOverviewNumber(breadth.TotalAmountCNY) && *breadth.TotalAmountCNY >= 0 {
			out.TotalAmount = *breadth.TotalAmountCNY
		} else {
			appendOverviewWarning(&out.Warnings, "native_market_overview_total_amount_unavailable")
		}
		if breadth.UnclassifiedCount > 0 {
			appendOverviewWarning(&out.Warnings, "native_market_overview_unclassified_securities_present")
		}
		if breadth.TotalSecurities > 0 && breadth.CoveredSecurities < breadth.TotalSecurities {
			appendOverviewWarning(&out.Warnings, "native_market_overview_market_breadth_partial_coverage")
		}
		if breadth.AmountCoverageCount > 0 && breadth.CoveredSecurities > 0 &&
			breadth.AmountCoverageCount < breadth.CoveredSecurities {
			appendOverviewWarning(&out.Warnings, "native_market_overview_total_amount_partial_coverage")
		}
	}

	if !containsOverviewString(failed, "industry") {
		appendOverviewSource(&sources, "industry", industry.Source)
		appendOverviewQualityWarnings(&out.Warnings, "industry", industry.QualityStatus, industry.QualityReasons)
		appendOverviewFreshnessWarning(&out.Warnings, "industry", industry.FetchedAt, marketNow)
		out.TopSectors = overviewSectorMaps(industry.Items, 5)
		if len(out.TopSectors) == 0 {
			appendOverviewWarning(&out.Warnings, "native_market_overview_industry_rankings_empty")
		}
	}
	if !containsOverviewString(failed, "concept") {
		appendOverviewSource(&sources, "concept", concept.Source)
		appendOverviewQualityWarnings(&out.Warnings, "concept", concept.QualityStatus, concept.QualityReasons)
		appendOverviewFreshnessWarning(&out.Warnings, "concept", concept.FetchedAt, marketNow)
		out.TopConcepts = overviewSectorMaps(concept.Items, 5)
		if len(out.TopConcepts) == 0 {
			appendOverviewWarning(&out.Warnings, "native_market_overview_concept_rankings_empty")
		}
	}
	if len(out.TopSectors) > 0 || len(out.TopConcepts) > 0 {
		appendOverviewWarning(&out.Warnings, "native_market_overview_bottom_sector_rankings_not_exposed_by_current_api")
	}
	if len(sources) > 0 {
		appendOverviewWarning(&out.Warnings, "native_market_overview_sources="+strings.Join(sources, ","))
	}

	if len(out.Indices) == 0 &&
		(containsOverviewString(failed, "breadth") || breadth.TotalSecurities <= 0) &&
		len(out.TopSectors) == 0 && len(out.TopConcepts) == 0 {
		if len(failed) > 0 {
			return domain.MarketOverview{}, fmt.Errorf("native market overview has no usable components; unavailable: %s", strings.Join(failed, ","))
		}
		return domain.MarketOverview{}, errors.New("native market overview returned no usable index, breadth or sector data")
	}
	return out, nil
}

func finiteOverviewNumber(value *float64) bool {
	return value != nil && !math.IsNaN(*value) && !math.IsInf(*value, 0)
}

func strictOverviewDate(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	parsed, err := time.Parse("2006-01-02", raw)
	if err != nil || parsed.Format("2006-01-02") != raw {
		return "", false
	}
	return parsed.Format("2006-01-02"), true
}

func containsOverviewString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func appendOverviewWarning(warnings *[]string, value string) {
	for _, current := range *warnings {
		if current == value {
			return
		}
	}
	*warnings = append(*warnings, value)
}

func appendOverviewSource(sources *[]string, key, value string) {
	value = strings.TrimSpace(value)
	if value != "" {
		*sources = append(*sources, key+"="+value)
	}
}

func appendOverviewQualityWarnings(warnings *[]string, component, quality string, reasons []string) {
	quality = strings.ToLower(strings.TrimSpace(quality))
	if quality != "" && quality != "valid" {
		appendOverviewWarning(warnings, "native_market_overview_"+component+"_quality_"+quality)
	}
	for _, reason := range reasons {
		reason = strings.TrimSpace(reason)
		if reason != "" {
			appendOverviewWarning(warnings, "native_market_overview_"+component+"_quality_reason:"+reason)
		}
	}
}

func appendOverviewFreshnessWarning(warnings *[]string, component string, fetchedAt, now time.Time) {
	if fetchedAt.IsZero() {
		appendOverviewWarning(warnings, "native_market_overview_"+component+"_fetched_at_missing")
		return
	}
	age := now.Sub(fetchedAt)
	if age < -time.Minute {
		appendOverviewWarning(warnings, "native_market_overview_"+component+"_fetched_at_in_future")
	} else if age > 5*time.Minute {
		appendOverviewWarning(warnings, "native_market_overview_"+component+"_snapshot_stale")
	}
}

func overviewSectorMaps(items []wireMarketOverviewSector, limit int) []map[string]any {
	capacity := len(items)
	if capacity > limit {
		capacity = limit
	}
	result := make([]map[string]any, 0, capacity)
	for _, item := range items {
		if len(result) >= limit {
			break
		}
		name := strings.TrimSpace(item.Name)
		if name == "" || !finiteOverviewNumber(item.ChangePercent) {
			continue
		}
		entry := map[string]any{
			"rank": item.Rank, "code": strings.TrimSpace(item.Code), "name": name,
			"change_percent": *item.ChangePercent, "source": strings.TrimSpace(item.Source),
			"quality_status": strings.TrimSpace(item.QualityStatus),
		}
		if item.Category != "" {
			entry["category"] = item.Category
		}
		if finiteOverviewNumber(item.LatestPrice) {
			entry["latest_price"] = *item.LatestPrice
		}
		if finiteOverviewNumber(item.Change) {
			entry["change"] = *item.Change
		}
		if finiteOverviewNumber(item.Amount) && *item.Amount >= 0 {
			entry["amount"] = *item.Amount
		}
		if item.AmountUnit != "" {
			entry["amount_unit"] = item.AmountUnit
		}
		if item.AdvancingCount != nil && *item.AdvancingCount >= 0 {
			entry["advancing_count"] = *item.AdvancingCount
		}
		if item.DecliningCount != nil && *item.DecliningCount >= 0 {
			entry["declining_count"] = *item.DecliningCount
		}
		if item.LeadingStock != "" {
			entry["leading_stock"] = item.LeadingStock
		}
		if finiteOverviewNumber(item.LeaderChangePercent) {
			entry["leader_change_percent"] = *item.LeaderChangePercent
		}
		if item.ProviderReportDate != "" {
			entry["provider_report_date"] = item.ProviderReportDate
		}
		result = append(result, entry)
	}
	return result
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
