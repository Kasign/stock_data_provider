package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
)

// FetchOrderBook 按配置的 Provider 顺序读取真实五档盘口。
// 盘口是日内快照，不走日线缓存，也不从其它数据推导；任一 Provider 返回的
// 数据必须通过自身契约校验并与请求证券一致，否则继续尝试下一个来源。
func (s *DataService) FetchOrderBook(ctx context.Context, rawSymbol string) (*model.OrderBook, error) {
	if s == nil || len(s.orderBookProviders) == 0 {
		return nil, errors.New("no enabled order-book provider")
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var failures []string
	for _, p := range s.orderBookProviders {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		book, fetchErr := p.FetchOrderBook(ctx, symbol)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateOrderBook(symbol, p.Name(), book); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		return book, nil
	}
	if len(failures) == 0 {
		return nil, errors.New("all order-book providers returned no usable data")
	}
	return nil, fmt.Errorf("all order-book providers failed: %s", strings.Join(failures, "; "))
}

func validateOrderBook(symbol, providerName string, book *model.OrderBook) error {
	if book == nil {
		return errors.New("order-book provider returned an empty snapshot")
	}
	returnedSymbol, err := normalize.NormalizeSymbol(book.Symbol)
	if err != nil || returnedSymbol != symbol {
		return fmt.Errorf("order-book identity mismatch: requested %s got %q", symbol, book.Symbol)
	}
	if strings.TrimSpace(book.Source) == "" || book.Source != providerName {
		return errors.New("order-book source is missing or does not match the provider")
	}
	return book.Validate()
}
