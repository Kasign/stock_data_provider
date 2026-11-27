package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
)

// eastmoneyOrderBookFields 是 push2 单证券详情接口中的五档字段。
// f11-f20 为买一到买五的价/量对，f31-f40 为卖一到卖五的价/量对；
// 数量字段单位为手，由 Adapter 明确归一化为股，不使用任何参与率假设。
var eastmoneyOrderBookFields = []string{
	"f11", "f12", "f13", "f14", "f15", "f16", "f17", "f18", "f19", "f20",
	"f31", "f32", "f33", "f34", "f35", "f36", "f37", "f38", "f39", "f40",
	"f57", "f86",
}

var eastmoneyOrderBookBidPairs = [][2]string{
	{"f19", "f20"}, {"f17", "f18"}, {"f15", "f16"}, {"f13", "f14"}, {"f11", "f12"},
}

var eastmoneyOrderBookAskPairs = [][2]string{
	{"f39", "f40"}, {"f37", "f38"}, {"f35", "f36"}, {"f33", "f34"}, {"f31", "f32"},
}

// FetchOrderBook 从东方财富 push2 单证券详情接口读取真实五档盘口。
// 它是 tradable_volume 的唯一 Provider 事实入口；任何不完整、身份不明或
// 时间不可核验的响应都直接拒绝，不用请求参数或系统时间补写来源事实。
func (p *Eastmoney) FetchOrderBook(ctx context.Context, rawSymbol string) (*model.OrderBook, error) {
	if p == nil {
		return nil, errors.New("Eastmoney order-book provider is nil")
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Eastmoney order book only supports common A-share equities, got %q", rawSymbol)
	}
	secid, err := eastmoneySecID(symbol)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("secid", secid)
	q.Set("fields", strings.Join(eastmoneyOrderBookFields, ","))
	q.Set("fltt", "2")
	q.Set("invt", "2")
	q.Set("_", strconv.FormatInt(providerNowUTC().UnixMilli(), 10))

	var payload struct {
		RC   *int                       `json:"rc"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := p.getJSON(ctx, p.quoteURL, q, &payload); err != nil {
		return nil, err
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney order-book provider error rc=%d", *payload.RC)
	}
	if len(payload.Data) == 0 {
		return nil, errors.New("Eastmoney returned an empty order book")
	}
	reportedSymbol := eastmoneyHotRowString(payload.Data, "f57")
	if reportedSymbol == "" {
		return nil, errors.New("Eastmoney order book response has no source symbol")
	}
	canonical, canonicalErr := normalize.NormalizeSymbol(reportedSymbol)
	if canonicalErr != nil || canonical != symbol {
		return nil, fmt.Errorf("Eastmoney order-book symbol mismatch: requested %s got %q", symbol, reportedSymbol)
	}
	observedAt := parseEastmoneyTime(eastmoneyHotRowString(payload.Data, "f86"))
	if observedAt == nil {
		return nil, errors.New("Eastmoney order book response has no usable provider timestamp")
	}
	bids, err := eastmoneyOrderBookLevels(payload.Data, eastmoneyOrderBookBidPairs)
	if err != nil {
		return nil, fmt.Errorf("Eastmoney order-book bids: %w", err)
	}
	asks, err := eastmoneyOrderBookLevels(payload.Data, eastmoneyOrderBookAskPairs)
	if err != nil {
		return nil, fmt.Errorf("Eastmoney order-book asks: %w", err)
	}
	book := &model.OrderBook{
		Symbol:           symbol,
		Bids:             bids,
		Asks:             asks,
		ObservedAt:       observedAt.UTC(),
		SystemObservedAt: providerNowUTC(),
		Source:           p.Name(),
		QualityStatus:    model.QualityValid,
	}
	if err := book.Validate(); err != nil {
		return nil, fmt.Errorf("Eastmoney order-book data contract: %w", err)
	}
	return book, nil
}

func eastmoneyOrderBookLevels(data map[string]json.RawMessage, pairs [][2]string) ([]model.OrderBookLevel, error) {
	levels := make([]model.OrderBookLevel, 0, len(pairs))
	for _, pair := range pairs {
		price, ok := eastmoneyHotRowFloat(data, pair[0])
		if !ok || price <= 0 {
			return nil, fmt.Errorf("invalid price field %s", pair[0])
		}
		hands, ok := eastmoneyHotRowFloat(data, pair[1])
		if !ok || hands <= 0 {
			return nil, fmt.Errorf("invalid volume field %s", pair[1])
		}
		shares, err := normalize.HandsToShares(hands)
		if err != nil || shares <= 0 {
			return nil, fmt.Errorf("invalid volume field %s: %v", pair[1], err)
		}
		levels = append(levels, model.OrderBookLevel{Price: price, Volume: shares})
	}
	return levels, nil
}
