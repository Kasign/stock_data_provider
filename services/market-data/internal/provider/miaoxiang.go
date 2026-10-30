package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const (
	defaultMiaoXiangURL       = "https://mkapi2.dfcfs.com/finskillshub/api/claw/query"
	miaoXiangVersion          = "mx-api-chip-flow-v1"
	miaoXiangCacheTTL         = 5 * time.Minute
	maxMiaoXiangResponseBytes = 2 << 20
)

// MX 查询按全局实例串行执行，避免并发请求或过于频繁地调用按配额计费的自然语言接口。
var miaoXiangRequestLock sync.Mutex
var miaoXiangRateMu sync.Mutex
var miaoXiangLastRequest time.Time

type MiaoXiang struct {
	apiKey   string
	endpoint string
	client   *http.Client
	logger   *slog.Logger
	now      func() time.Time
	cacheMu  sync.Mutex
	cache    map[string]cacheEntry
	cacheTTL time.Duration
}

type mxFlowObservation struct {
	date   string
	amount *float64
}

type cacheEntry struct {
	storedAt time.Time
	value    any
}

type mxEnvelope struct {
	Status  json.RawMessage `json:"status"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

type mxDataNode struct {
	Data   json.RawMessage `json:"data"`
	Tables json.RawMessage `json:"dataTableDTOList"`
}

type mxSearchNode struct {
	SearchDataResultDTO struct {
		Tables []mxTable `json:"dataTableDTOList"`
	} `json:"searchDataResultDTO"`
}

type mxTable struct {
	NameMap any            `json:"nameMap"`
	Table   map[string]any `json:"table"`
}

var mxNumberPattern = regexp.MustCompile(`[-+]?[0-9]+(?:\.[0-9]+)?`)

func NewMiaoXiang(apiKey, endpoint string, timeout time.Duration) (*MiaoXiang, error) {
	apiKey = strings.TrimSpace(apiKey)
	if !validMXAPIKey(apiKey) {
		return nil, errors.New("MX API key is missing or looks like a placeholder")
	}
	if strings.TrimSpace(endpoint) == "" {
		endpoint = defaultMiaoXiangURL
	}
	u, err := url.Parse(endpoint)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("MX API endpoint must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &MiaoXiang{
		apiKey: apiKey, endpoint: u.String(),
		client: &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }},
		logger: slog.Default(),
		now:    func() time.Time { return time.Now().UTC() },
		cache:  make(map[string]cacheEntry), cacheTTL: miaoXiangCacheTTL,
	}, nil
}

func (p *MiaoXiang) Name() string { return "miaoxiang" }

// FetchChipDistribution 仅用于显式启用的补充数据能力，不实现实时行情或日 K Provider 接口。
func (p *MiaoXiang) FetchChipDistribution(ctx context.Context, rawSymbol string) (*model.ChipDistribution, error) {
	symbol, err := mxCanonicalAShare(rawSymbol)
	if err != nil {
		return nil, err
	}
	tables, err := p.queryTables(ctx, "chip:"+symbol, symbol+" 获利比例 平均成本 90%筹码集中度 70%筹码集中度")
	if err != nil {
		return nil, err
	}
	target := findChipTable(tables)
	if target == nil {
		return nil, errors.New("MX API chip response has no usable average-cost table")
	}
	rows := tableRows(*target)
	if len(rows) == 0 {
		return nil, errors.New("MX API chip response table is empty")
	}
	row := rows[0]
	avg, ok := parseNumber(row["平均成本"])
	if !ok || avg <= 0 || !finiteNumber(avg) {
		return nil, errors.New("MX API chip average cost is missing or invalid")
	}

	out := &model.ChipDistribution{
		Symbol: symbol, Date: normalizeMXDate(row["_date"]), AverageCost: avg,
		Source: p.Name(), SourceVersion: miaoXiangVersion, ProviderDate: normalizeMXDate(row["_date"]), FetchedAt: p.now(),
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"natural_language_supplemental_source", "publication_availability_timestamp_unavailable"},
	}
	out.ProfitRatio = parseRatioPtr(row["获利比例"])
	out.Concentration90 = parseRatioPtr(row["90%筹码集中度"])
	out.Concentration70 = parseRatioPtr(row["70%筹码集中度"])
	if out.Date == "" {
		out.QualityStatus = model.QualityDegraded
		out.QualityReasons = append(out.QualityReasons, "provider_date_missing")
	}
	if out.ProfitRatio == nil {
		out.QualityStatus = model.QualityDegraded
		out.QualityReasons = append(out.QualityReasons, "profit_ratio_missing")
	}
	if out.Concentration90 == nil {
		out.QualityStatus = model.QualityDegraded
		out.QualityReasons = append(out.QualityReasons, "concentration_90_missing")
	}
	if out.Concentration70 == nil {
		out.QualityStatus = model.QualityDegraded
		out.QualityReasons = append(out.QualityReasons, "concentration_70_missing")
	}
	return out, nil
}

// FetchCapitalFlow 返回以人民币元计量的主力净流入。只有 5/10 个交易日的观测值完整时，才会返回对应区间合计。
func (p *MiaoXiang) FetchCapitalFlow(ctx context.Context, rawSymbol string) (*model.CapitalFlow, error) {
	symbol, err := mxCanonicalAShare(rawSymbol)
	if err != nil {
		return nil, err
	}
	tables, err := p.queryTables(ctx, "flow:"+symbol, symbol+" 近10日每日主力净流入资金")
	if err != nil {
		return nil, err
	}

	amountLabels := map[string]bool{"主力净流入": true, "主力净流入资金": true, "主力净流入金额": true}
	var target *mxTable
	bestRows := -1
	for i := range tables {
		labels := tableLabels(tables[i])
		hasAmount := false
		for _, label := range labels {
			if amountLabels[label] {
				hasAmount = true
				break
			}
		}
		if !hasAmount {
			continue
		}
		rowCount := len(valueArray(tables[i].Table["headName"]))
		if rowCount > bestRows {
			target = &tables[i]
			bestRows = rowCount
		}
	}
	if target == nil {
		return nil, errors.New("MX API capital-flow response has no explicit monetary metric table")
	}
	rows := tableRows(*target)
	series := make([]mxFlowObservation, 0, len(rows))
	seenDates := make(map[string]struct{}, len(rows))
	loc := time.FixedZone("CST", 8*60*60)
	today := p.now().In(loc).Format("2006-01-02")
	for index, row := range rows {
		date := normalizeMXDate(row["_date"])
		parsedDate, dateErr := time.Parse("2006-01-02", date)
		if date == "" || dateErr != nil || parsedDate.Format("2006-01-02") != date {
			return nil, fmt.Errorf("MX API capital-flow row %d has an invalid observation date", index)
		}
		if date > today {
			return nil, fmt.Errorf("MX API capital-flow row %d date %s is in the future", index, date)
		}
		if _, exists := seenDates[date]; exists {
			return nil, fmt.Errorf("MX API capital-flow response contains duplicate date %s", date)
		}
		seenDates[date] = struct{}{}
		label := ""
		for _, candidate := range []string{"主力净流入", "主力净流入资金", "主力净流入金额"} {
			if _, ok := row[candidate]; ok {
				label = candidate
				break
			}
		}
		series = append(series, mxFlowObservation{date: date, amount: parseMoneyYuanPtr(row[label])})
	}
	sort.SliceStable(series, func(i, j int) bool { return series[i].date > series[j].date })
	if len(series) == 0 {
		return nil, errors.New("MX API capital-flow table is empty")
	}
	flow := &model.CapitalFlow{
		Symbol: symbol, ProviderDate: series[0].date, Source: p.Name(), SourceVersion: miaoXiangVersion, FetchedAt: p.now(), Observations: len(series),
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"natural_language_supplemental_source", "publication_availability_timestamp_unavailable"},
	}
	flow.MainNetInflow = series[0].amount
	if flow.MainNetInflow == nil {
		flow.QualityReasons = append(flow.QualityReasons, "latest_main_net_inflow_missing_or_not_monetary")
	}
	flow.Inflow5D = completeWindowSum(series, 5)
	flow.Inflow10D = completeWindowSum(series, 10)
	if flow.Inflow5D == nil {
		flow.QualityReasons = append(flow.QualityReasons, "inflow_5d_requires_5_complete_observations")
	}
	if flow.Inflow10D == nil {
		flow.QualityReasons = append(flow.QualityReasons, "inflow_10d_requires_10_complete_observations")
	}
	return flow, nil
}

func completeWindowSum(series []mxFlowObservation, size int) *float64 {
	if len(series) < size {
		return nil
	}
	sum := 0.0
	for _, item := range series[:size] {
		if item.amount == nil || !finiteNumber(*item.amount) {
			return nil
		}
		sum += *item.amount
	}
	if !finiteNumber(sum) {
		return nil
	}
	return &sum
}

func (p *MiaoXiang) queryTables(ctx context.Context, key, query string) ([]mxTable, error) {
	if ctx == nil {
		return nil, errors.New("MX API context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if value, ok := p.cacheGet(key); ok {
		tables, valid := value.([]mxTable)
		if valid {
			return tables, nil
		}
	}
	// 不要把新的 API 请求排在仍在执行或已超时的请求后面；此处沿用上游 Python 管理器的非排队准入策略。
	if !miaoXiangRequestLock.TryLock() {
		return nil, errors.New("MX API request already in progress; supplemental query skipped")
	}
	defer miaoXiangRequestLock.Unlock()
	miaoXiangRateMu.Lock()
	wait := 300*time.Millisecond - p.now().Sub(miaoXiangLastRequest)
	miaoXiangRateMu.Unlock()
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
	body, _ := json.Marshal(map[string]string{"toolQuery": query})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build MX API request: %w", err)
	}
	req.Header.Set("apikey", p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "stock-market-data/1.0")
	started := time.Now()
	resp, err := p.client.Do(req)
	miaoXiangRateMu.Lock()
	miaoXiangLastRequest = p.now()
	miaoXiangRateMu.Unlock()
	if err != nil {
		p.logger.Warn("补充数据源请求失败", "service", "market-data", "provider", p.Name(), "dataset", mxDataset(key), "symbol", mxSymbol(key), "duration_ms", time.Since(started).Milliseconds(), "error", err.Error())
		return nil, fmt.Errorf("MX API request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		p.logger.Warn("补充数据源返回 HTTP 错误", "service", "market-data", "provider", p.Name(), "dataset", mxDataset(key), "symbol", mxSymbol(key), "status_code", resp.StatusCode, "duration_ms", time.Since(started).Milliseconds())
		return nil, fmt.Errorf("MX API returned HTTP status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxMiaoXiangResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read MX API response: %w", err)
	}
	if len(raw) > maxMiaoXiangResponseBytes {
		return nil, errors.New("MX API response exceeds 2 MiB")
	}
	var envelope mxEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode MX API response: %w", err)
	}
	var status int
	if len(envelope.Status) == 0 || json.Unmarshal(envelope.Status, &status) != nil || status != 0 {
		return nil, fmt.Errorf("MX API reported a non-success status (message=%q)", boundedText(envelope.Message, 120))
	}
	tables, err := extractMXTables(envelope.Data)
	if err != nil {
		return nil, err
	}
	p.cacheSet(key, tables)
	p.logger.Info("补充数据源请求完成", "service", "market-data", "provider", p.Name(), "dataset", mxDataset(key), "symbol", mxSymbol(key), "status", "success", "http_status", resp.StatusCode, "response_bytes", len(raw), "table_count", len(tables), "duration_ms", time.Since(started).Milliseconds())
	return tables, nil
}

func extractMXTables(data json.RawMessage) ([]mxTable, error) {
	var node map[string]json.RawMessage
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, errors.New("MX API data payload is not an object")
	}
	candidates := []json.RawMessage{node["dataTableDTOList"]}
	if nestedRaw := node["data"]; len(nestedRaw) > 0 {
		var nested map[string]json.RawMessage
		if json.Unmarshal(nestedRaw, &nested) == nil {
			if searchRaw := nested["searchDataResultDTO"]; len(searchRaw) > 0 {
				var search map[string]json.RawMessage
				if json.Unmarshal(searchRaw, &search) == nil {
					candidates = append([]json.RawMessage{search["dataTableDTOList"]}, candidates...)
				}
			}
		}
	}
	for _, candidate := range candidates {
		if len(candidate) == 0 || string(candidate) == "null" {
			continue
		}
		var tables []mxTable
		if err := json.Unmarshal(candidate, &tables); err == nil && len(tables) > 0 {
			return tables, nil
		}
	}
	return nil, errors.New("MX API did not return a dataTableDTOList in a supported response envelope")
}

func tableLabels(table mxTable) []string {
	nameMap := normalizeNameMap(table.NameMap)
	keys := make([]string, 0, len(table.Table))
	for key := range table.Table {
		if key != "headName" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	labels := make([]string, 0, len(keys))
	for _, key := range keys {
		if label := nameMap[key]; label != "" {
			labels = append(labels, label)
		} else {
			labels = append(labels, key)
		}
	}
	return labels
}
func normalizeNameMap(value any) map[string]string {
	out := map[string]string{}
	switch x := value.(type) {
	case map[string]any:
		for key, v := range x {
			out[key] = strings.TrimSpace(fmt.Sprint(v))
		}
	case []any:
		for i, v := range x {
			out[strconv.Itoa(i)] = strings.TrimSpace(fmt.Sprint(v))
		}
	}
	return out
}
func tableRows(table mxTable) []map[string]string {
	nameMap := normalizeNameMap(table.NameMap)
	heads := valueArray(table.Table["headName"])
	rows := make([]map[string]string, len(heads))
	for i := range rows {
		rows[i] = map[string]string{"_date": heads[i]}
	}
	for key, raw := range table.Table {
		if key == "headName" {
			continue
		}
		values := valueArray(raw)
		label := nameMap[key]
		if label == "" {
			label = key
		}
		for i, value := range values {
			for len(rows) <= i {
				rows = append(rows, map[string]string{})
			}
			rows[i][label] = value
		}
	}
	return rows
}
func valueArray(value any) []string {
	raw, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, len(raw))
	for i, item := range raw {
		if item != nil {
			out[i] = strings.TrimSpace(fmt.Sprint(item))
		}
	}
	return out
}
func findChipTable(tables []mxTable) *mxTable {
	bestScore, bestRows := -1, -1
	var target *mxTable
	for i := range tables {
		labels := tableLabels(tables[i])
		hasAvg, score := false, 0
		for _, label := range labels {
			if strings.Contains(label, "平均成本") {
				hasAvg = true
			}
			for _, keyword := range []string{"获利比例", "90%筹码集中度", "70%筹码集中度"} {
				if strings.Contains(label, keyword) {
					score++
				}
			}
		}
		rows := len(valueArray(tables[i].Table["headName"]))
		if hasAvg && (score > bestScore || (score == bestScore && rows < bestRows)) {
			target = &tables[i]
			bestScore = score
			bestRows = rows
		}
	}
	if target != nil {
		return target
	}
	for i := range tables {
		for _, label := range tableLabels(tables[i]) {
			if strings.Contains(label, "筹码") {
				return &tables[i]
			}
		}
	}
	return nil
}
func parseNumber(value string) (float64, bool) {
	value = strings.ReplaceAll(strings.TrimSpace(value), ",", "")
	match := mxNumberPattern.FindString(value)
	if match == "" {
		return 0, false
	}
	number, err := strconv.ParseFloat(match, 64)
	if err != nil || !finiteNumber(number) {
		return 0, false
	}
	return number, true
}
func parseRatioPtr(value string) *float64 {
	n, ok := parseNumber(value)
	if !ok {
		return nil
	}
	if strings.ContainsAny(value, "%％") || n > 1 {
		n /= 100
	}
	if n < 0 || n > 1 || !finiteNumber(n) {
		return nil
	}
	return &n
}
func parseMoneyYuanPtr(value string) *float64 {
	if strings.ContainsAny(value, "%％") {
		return nil
	}
	n, ok := parseNumber(value)
	if !ok {
		return nil
	}
	switch {
	case strings.Contains(value, "亿"):
		n *= 1e8
	case strings.Contains(value, "万"):
		n *= 1e4
	}
	if !finiteNumber(n) {
		return nil
	}
	return &n
}

func normalizeMXDate(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.ReplaceAll(value, "/", "-")
	if len(value) >= 10 {
		for _, layout := range []string{"2006-01-02", "2006-1-2", "20060102"} {
			if len(value) >= len(layout) {
				if t, err := time.Parse(layout, value[:len(layout)]); err == nil {
					return t.Format("2006-01-02")
				}
			}
		}
	}
	if t, err := time.Parse("20060102", value); err == nil {
		return t.Format("2006-01-02")
	}
	return value
}

func mxDataset(key string) string {
	if i := strings.IndexByte(key, ':'); i > 0 {
		return key[:i]
	}
	return "unknown"
}
func mxSymbol(key string) string {
	if i := strings.IndexByte(key, ':'); i >= 0 && i+1 < len(key) {
		return key[i+1:]
	}
	return ""
}

func mxCanonicalAShare(raw string) (string, error) {
	symbol, err := normalize.NormalizeSymbol(raw)
	if err != nil {
		return "", err
	}
	code := strings.Split(symbol, ".")[0]
	// 与上游 MX 适配器保持一致：ETF 不支持筹码分布和个股资金流查询。
	if strings.HasPrefix(code, "15") || strings.HasPrefix(code, "16") || strings.HasPrefix(code, "18") ||
		strings.HasPrefix(code, "50") || strings.HasPrefix(code, "51") || strings.HasPrefix(code, "52") ||
		strings.HasPrefix(code, "56") || strings.HasPrefix(code, "58") {
		return "", fmt.Errorf("MX supplemental data does not support ETF symbol %q", symbol)
	}
	return symbol, nil
}
func validMXAPIKey(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 8 {
		return false
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"replace-with", "change-me", "changeme", "your-key", "example-key", "placeholder"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

func boundedText(value string, max int) string {
	value = strings.Map(func(r rune) rune {
		if r < ' ' && r != '\t' {
			return -1
		}
		return r
	}, value)
	if len(value) > max {
		return value[:max]
	}
	return value
}
func (p *MiaoXiang) cacheGet(key string) (any, bool) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	item, ok := p.cache[key]
	if !ok {
		return nil, false
	}
	if p.now().Sub(item.storedAt) >= p.cacheTTL {
		delete(p.cache, key)
		return nil, false
	}
	return item.value, true
}
func (p *MiaoXiang) cacheSet(key string, value any) {
	p.cacheMu.Lock()
	defer p.cacheMu.Unlock()
	p.cache[key] = cacheEntry{storedAt: p.now(), value: value}
}
