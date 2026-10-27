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
	Symbol         string   `json:"symbol"`
	Code           string   `json:"code"`
	Name           string   `json:"name"`
	LatestPrice    *float64 `json:"latest_price"`
	Change         *float64 `json:"change"`
	ChangePercent  *float64 `json:"change_percent"`
	Open           *float64 `json:"open"`
	High           *float64 `json:"high"`
	Low            *float64 `json:"low"`
	PrevClose      *float64 `json:"prev_close"`
	Amount         *float64 `json:"amount"`
	AmountUnit     string   `json:"amount_unit"`
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
	Rank                int       `json:"rank"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	Category            string    `json:"category"`
	LatestPrice         *float64  `json:"latest_price"`
	Change              *float64  `json:"change"`
	ChangePercent       *float64  `json:"change_percent"`
	Amount              *float64  `json:"amount"`
	AmountUnit          string    `json:"amount_unit"`
	AdvancingCount      *int      `json:"advancing_count"`
	DecliningCount      *int      `json:"declining_count"`
	LeadingStock        string    `json:"leading_stock"`
	LeaderChangePercent *float64  `json:"leader_change_percent"`
	Source              string    `json:"source"`
	ProviderReportDate  string    `json:"provider_report_date"`
	QualityStatus       string    `json:"quality_status"`
	QualityReasons      []string  `json:"quality_reasons"`
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
