package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

type orderBookProviderStub struct {
	name  string
	book  *model.OrderBook
	err   error
	calls *int
}

func (stub orderBookProviderStub) Name() string { return stub.name }

func (stub orderBookProviderStub) FetchOrderBook(context.Context, string) (*model.OrderBook, error) {
	if stub.calls != nil {
		*stub.calls++
	}
	return stub.book, stub.err
}

func validServiceOrderBook(source string) *model.OrderBook {
	levels := func(price float64) []model.OrderBookLevel {
		return []model.OrderBookLevel{
			{Price: price, Volume: 100}, {Price: price, Volume: 100},
			{Price: price, Volume: 100}, {Price: price, Volume: 100}, {Price: price, Volume: 100},
		}
	}
	return &model.OrderBook{
		Symbol: "600519.SH", Bids: levels(10), Asks: levels(11),
		ObservedAt: time.Now().UTC().Add(-time.Second), SystemObservedAt: time.Now().UTC(),
		Source: source, QualityStatus: model.QualityValid,
	}
}

func TestDataServiceFetchOrderBookUsesConfiguredProviderOrder(t *testing.T) {
	firstCalls, secondCalls := 0, 0
	service := NewWithCapabilities(nil, nil, nil, nil, nil, 0, 0, 120, 1000, ResilienceConfig{}, nil)
	service.SetOrderBookProviders([]provider.OrderBookProvider{
		orderBookProviderStub{name: "first", err: errors.New("unavailable"), calls: &firstCalls},
		orderBookProviderStub{name: "second", book: validServiceOrderBook("second"), calls: &secondCalls},
	})

	book, err := service.FetchOrderBook(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchOrderBook() error = %v", err)
	}
	if firstCalls != 1 || secondCalls != 1 || book.Source != "second" {
		t.Fatalf("provider fallback failed: first=%d second=%d source=%q", firstCalls, secondCalls, book.Source)
	}
}

func TestDataServiceFetchOrderBookFailsClosedWithoutProviders(t *testing.T) {
	service := NewWithCapabilities(nil, nil, nil, nil, nil, 0, 0, 120, 1000, ResilienceConfig{}, nil)
	if _, err := service.FetchOrderBook(context.Background(), "600519.SH"); err == nil {
		t.Fatal("missing order-book provider must fail closed")
	}
}
