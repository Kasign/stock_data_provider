package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/baostock"
	"example.com/stock-analysis-flow/internal/marketdata/model"
)

const (
	// baostockUniverseVersion 标识证券目录的来源契约：query_all_stock 单页全量 + 交易日定档。
	baostockUniverseVersion = "baostock-query-all-stock-v1"
	// baostockUniverseTTL 是目录在进程内的复用窗口；目录只在上市/退市时变化，按日刷新足够。
	baostockUniverseTTL = 6 * time.Hour
	// baostockUniverseLookbackDays 是向前回找最近交易日的窗口，覆盖长假。
	baostockUniverseLookbackDays = 30
	// baostockUniverseMinItems 是完整性下限：低于该数量说明分页或过滤出错，必须失败关闭。
	baostockUniverseMinItems = 1000
)

// baostockASharePrefixes 只保留 A 股主板/科创板/创业板代码段。
// BaoStock 的 query_all_stock 同时返回指数、B 股与场内基金，不能直接当作 A 股目录。
var baostockASharePrefixes = map[string]map[string]struct{}{
	"sh": {"600": {}, "601": {}, "603": {}, "605": {}, "688": {}, "689": {}},
	"sz": {"000": {}, "001": {}, "002": {}, "003": {}, "300": {}, "301": {}, "302": {}},
}

type baostockUniverseClient interface {
	QueryTradeDates(ctx context.Context, startDate, endDate string) ([]baostock.TradeDate, error)
	QueryAllStocks(ctx context.Context, tradeDate string) ([]baostock.AllStockRecord, error)
}

// ListStockUniverse 使用 BaoStock 全量证券目录提供 A 股证券池。
//
// 它是 Eastmoney clist 目录之外的第二个证券池来源：两者访问完全不同的网络端点，
// Eastmoney 行情域名不可达时仍能给出完整（不含北交所）的 A 股目录。
func (p *Baostock) ListStockUniverse(ctx context.Context) (*model.StockUniverseSnapshot, error) {
	if p == nil {
		return nil, errors.New("BaoStock provider is not configured")
	}
	client, ok := p.client.(baostockUniverseClient)
	if !ok {
		return nil, errors.New("BaoStock client does not implement security-directory queries")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	p.universeMu.Lock()
	defer p.universeMu.Unlock()
	if p.universe != nil && providerNowUTC().Sub(p.universeFetchedAt) < baostockUniverseTTL {
		return cloneStockUniverse(p.universe), nil
	}
	tradeDate, err := latestBaostockTradingDate(ctx, client)
	if err != nil {
		return nil, err
	}
	records, err := client.QueryAllStocks(ctx, tradeDate)
	if err != nil {
		return nil, err
	}
	snapshot, err := buildBaostockStockUniverse(records, providerNowUTC())
	if err != nil {
		return nil, err
	}
	p.universe = cloneStockUniverse(snapshot)
	p.universeFetchedAt = snapshot.FetchedAt
	return snapshot, nil
}

// latestBaostockTradingDate 返回最近一个不晚于今日（Asia/Shanghai）的交易日。
// 目录按交易日定档，避免用非交易日日期向 BaoStock 请求空结果。
func latestBaostockTradingDate(ctx context.Context, client baostockUniverseClient) (string, error) {
	now := providerNowUTC().In(providerMarketLocation())
	end := now.Format("2006-01-02")
	start := now.AddDate(0, 0, -baostockUniverseLookbackDays).Format("2006-01-02")
	rows, err := client.QueryTradeDates(ctx, start, end)
	if err != nil {
		return "", fmt.Errorf("BaoStock security-directory trading calendar: %w", err)
	}
	for i := len(rows) - 1; i >= 0; i-- {
		if rows[i].IsTradingDay {
			return rows[i].Date, nil
		}
	}
	return "", fmt.Errorf("BaoStock returned no trading day between %s and %s", start, end)
}

// buildBaostockStockUniverse 把原始目录行裁剪为 A 股证券池。
// 原始响应含指数、B 股与场内基金；名字缺失的行无法用于展示与检索，一并排除并记录原因。
func buildBaostockStockUniverse(records []baostock.AllStockRecord, fetchedAt time.Time) (*model.StockUniverseSnapshot, error) {
	items := make([]model.StockSearchItem, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	unnamed := 0
	for _, record := range records {
		symbol, code, exchange, ok := baostockAShareSymbol(record.Code)
		if !ok {
			continue
		}
		name := strings.TrimSpace(record.Name)
		if name == "" {
			unnamed++
			continue
		}
		if _, duplicate := seen[symbol]; duplicate {
			return nil, fmt.Errorf("BaoStock security directory contains duplicate symbol %s", symbol)
		}
		seen[symbol] = struct{}{}
		items = append(items, model.StockSearchItem{Symbol: symbol, Code: code, Name: name, Exchange: exchange})
	}
	if len(items) < baostockUniverseMinItems {
		return nil, fmt.Errorf("BaoStock security directory is incomplete: %d A-share symbols, want at least %d", len(items), baostockUniverseMinItems)
	}
	reasons := []string{"provider_universe_excludes_beijing_exchange"}
	if unnamed > 0 {
		reasons = append(reasons, "provider_universe_rows_without_name_excluded")
	}
	return &model.StockUniverseSnapshot{
		Items: items, Total: len(items), TotalIsLowerBound: true,
		Source: baostockProviderName, SourceVersion: baostockUniverseVersion,
		FetchedAt: fetchedAt.UTC(), QualityStatus: model.QualityDegraded, QualityReasons: reasons,
	}, nil
}

// baostockAShareSymbol 把 sh.600519 转换为 (600519.SH, 600519, SH, true)；
// 非 A 股代码段返回 ok=false，由调用方跳过而不是报错。
func baostockAShareSymbol(raw string) (symbol, code, exchange string, ok bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) != 9 || value[2] != '.' {
		return "", "", "", false
	}
	market, digits := value[:2], value[3:]
	prefixes, known := baostockASharePrefixes[market]
	if !known {
		return "", "", "", false
	}
	if _, allowed := prefixes[digits[:3]]; !allowed {
		return "", "", "", false
	}
	return digits + "." + strings.ToUpper(market), digits, strings.ToUpper(market), true
}

// cloneStockUniverse 复制快照外壳，避免调用方修改缓存中的质量字段。
// Items 在进程内视为只读，不做逐条复制。
func cloneStockUniverse(snapshot *model.StockUniverseSnapshot) *model.StockUniverseSnapshot {
	if snapshot == nil {
		return nil
	}
	clone := *snapshot
	clone.QualityReasons = append([]string(nil), snapshot.QualityReasons...)
	return &clone
}
