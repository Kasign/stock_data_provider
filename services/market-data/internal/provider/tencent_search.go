package provider

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/simplifiedchinese"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const (
	defaultTencentStockSearchURL = "https://smartbox.gtimg.cn/s3/"
	tencentStockSearchVersion    = "tencent-smartbox-stock-search-v1"
)

// SearchStocks 从腾讯 Smartbox 关键词接口获取候选证券，不扫描全市场目录。
// 若上游没有匹配的 A 股结果，返回错误，让服务层继续尝试东方财富等后备源。
func (p *Tencent) SearchStocks(ctx context.Context, rawQuery string, limit int) (*model.StockSearchSnapshot, error) {
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
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	searchTerm := tencentStockSearchTerm(query)
	endpoint := strings.TrimSpace(p.searchURL)
	if endpoint == "" {
		endpoint = defaultTencentStockSearchURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Tencent stock-search URL: %w", err)
	}
	params := parsed.Query()
	params.Set("v", "2")
	params.Set("t", "all")
	params.Set("c", "1")
	params.Set("q", searchTerm)
	parsed.RawQuery = params.Encode()

	body, err := p.getBytes(ctx, parsed.String())
	if err != nil {
		return nil, fmt.Errorf("Tencent stock-search request: %w", err)
	}
	candidates, err := parseTencentSmartboxStocks(body)
	if err != nil {
		return nil, fmt.Errorf("parse Tencent stock-search response: %w", err)
	}

	needle := strings.ToLower(searchTerm)
	type rankedItem struct {
		item model.StockSearchItem
		rank int
	}
	matches := make([]rankedItem, 0, len(candidates))
	for _, item := range candidates {
		name := strings.ToLower(item.Name)
		code := strings.ToLower(item.Code)
		if !strings.Contains(name, needle) && !strings.Contains(code, needle) {
			continue
		}
		rank := 3
		switch {
		case strings.EqualFold(item.Name, searchTerm), strings.EqualFold(item.Code, searchTerm):
			rank = 0
		case strings.HasPrefix(name, needle), strings.HasPrefix(code, needle):
			rank = 1
		default:
			rank = 2
		}
		matches = append(matches, rankedItem{item: item, rank: rank})
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].rank < matches[j].rank })
	if len(matches) == 0 {
		return nil, errors.New("Tencent stock search returned no matching A-share records")
	}

	items := make([]model.StockSearchItem, 0, min(limit, len(matches)))
	for _, match := range matches {
		if len(items) >= limit {
			break
		}
		items = append(items, match.item)
	}
	return &model.StockSearchSnapshot{
		Query:          query,
		Total:          len(matches),
		Items:          items,
		Source:         p.Name(),
		SourceVersion:  tencentStockSearchVersion,
		FetchedAt:      time.Now().UTC(),
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_search_total_not_reported"},
	}, nil
}

func tencentStockSearchTerm(query string) string {
	parts := strings.Split(strings.TrimSpace(query), ".")
	if len(parts) == 2 {
		switch strings.ToUpper(parts[1]) {
		case "SH", "SZ", "BJ":
			return parts[0]
		}
	}
	return strings.TrimSpace(query)
}

// isTencentAshareStockCode 过滤 Smartbox 的 ETF、指数、可转债等非普通 A 股证券代码。
 // Smartbox 的 t=all 结果仅给出交易所标记与代码，交易所标记本身不能证明证券类型。
func isTencentAshareStockCode(code, exchange string) bool {
	if len(code) != 6 {
		return false
	}
	var prefixes []string
	switch exchange {
	case "SH":
		prefixes = []string{"600", "601", "603", "605", "688", "689"}
	case "SZ":
		prefixes = []string{"000", "001", "002", "003", "300", "301"}
	case "BJ":
		return strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") || strings.HasPrefix(code, "92")
	default:
		return false
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(code, prefix) {
			return true
		}
	}
	return false
}

func parseTencentSmartboxStocks(body []byte) ([]model.StockSearchItem, error) {
	text := string(body)
	if !utf8.Valid(body) {
		decoded, err := simplifiedchinese.GBK.NewDecoder().Bytes(body)
		if err != nil {
			return nil, fmt.Errorf("decode GBK response: %w", err)
		}
		text = string(decoded)
	}
	const marker = "v_hint=\""
	start := strings.Index(text, marker)
	if start < 0 {
		return nil, errors.New("response has no v_hint payload")
	}
	start += len(marker)
	end := strings.Index(text[start:], "\"")
	if end < 0 {
		return nil, errors.New("v_hint payload is unterminated")
	}
	payload := text[start : start+end]
	// 腾讯 Smartbox 在部分响应中会把中文名称写成 JavaScript Unicode 转义。
	if decoded, decodeErr := strconv.Unquote("\"" + payload + "\""); decodeErr == nil {
		payload = decoded
	}
	if strings.TrimSpace(payload) == "" {
		return nil, errors.New("v_hint payload is empty")
	}

	items := make([]model.StockSearchItem, 0)
	seen := make(map[string]struct{})
	for _, record := range strings.Split(payload, "^") {
		fields := strings.Split(strings.TrimSpace(record), "~")
		if len(fields) < 3 {
			continue
		}
		market := strings.ToUpper(strings.TrimSpace(fields[0]))
		code := strings.TrimSpace(fields[1])
		name := strings.TrimSpace(fields[2])
		if name == "" || len(code) != 6 {
			continue
		}
		exchange := ""
		switch market {
		case "SH":
			exchange = "SH"
		case "SZ":
			exchange = "SZ"
		case "BJ":
			exchange = "BJ"
		default:
			// Smartbox may include funds, indices, overseas markets and other securities.
			continue
		}
		if !isTencentAshareStockCode(code, exchange) {
			continue
		}
		symbol, err := normalize.NormalizeSymbol(code + "." + exchange)
		if err != nil {
			continue
		}
		parts := strings.Split(symbol, ".")
		if len(parts) != 2 || parts[0] != code || parts[1] != exchange {
			continue
		}
		if _, duplicate := seen[symbol]; duplicate {
			continue
		}
		seen[symbol] = struct{}{}
		items = append(items, model.StockSearchItem{
			Symbol:   symbol,
			Code:     code,
			Name:     name,
			Exchange: exchange,
		})
	}
	if len(items) == 0 {
		return nil, errors.New("v_hint payload contained no supported A-share records")
	}
	return items, nil
}
