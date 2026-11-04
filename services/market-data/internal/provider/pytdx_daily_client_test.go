package provider

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/injoyai/tdx/protocol"
)

type fakePytdxDailyClient struct {
	klineResponse *protocol.KlineResp
	klineErr      error
	gbbqResponse  *protocol.GbbqResp
	gbbqErr       error
}

func (f *fakePytdxDailyClient) Close() error { return nil }
func (f *fakePytdxDailyClient) GetKlineDay(string, uint16, uint16) (*protocol.KlineResp, error) {
	return f.klineResponse, f.klineErr
}
func (f *fakePytdxDailyClient) GetGbbq(string) (*protocol.GbbqResp, error) {
	return f.gbbqResponse, f.gbbqErr
}

func TestPytdxDailyBarsGbbqFailureRemainsProviderError(t *testing.T) {
	networkErr := errors.New("simulated GBBQ network timeout")
	client := &fakePytdxDailyClient{
		klineResponse: &protocol.KlineResp{List: []*protocol.Kline{{
			Time: time.Date(2026, 10, 21, 15, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
			Open: protocol.Price(10000), High: protocol.Price(10200),
			Low: protocol.Price(9800), Close: protocol.Price(10100),
			Last: protocol.Price(10000), Volume: 100, Amount: protocol.Price(101000000),
		}}},
		gbbqErr: networkErr,
	}
	p := NewPytdx(time.Second, "127.0.0.1")
	p.dailyClientFactory = func(context.Context) (pytdxDailyClient, error) { return client, nil }

	_, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-21", "2026-10-21", 10, model.AdjustmentForward)
	if err == nil || !errors.Is(err, networkErr) {
		t.Fatalf("expected original GBBQ transport error, got %v", err)
	}
	if got := err.Error(); got == "" || !containsPytdxError(got, "ex-rights/ex-dividend data required") {
		t.Fatalf("unexpected error classification context: %v", err)
	}
}

func TestPytdxDailyBarsEmptyGbbqResponseIsNotReportedAsUnsupported(t *testing.T) {
	client := &fakePytdxDailyClient{
		klineResponse: &protocol.KlineResp{List: []*protocol.Kline{{
			Time: time.Date(2026, 10, 21, 15, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
			Open: protocol.Price(10000), High: protocol.Price(10200),
			Low: protocol.Price(9800), Close: protocol.Price(10100),
			Last: protocol.Price(10000), Volume: 100, Amount: protocol.Price(101000000),
		}}},
		gbbqResponse: &protocol.GbbqResp{List: []*protocol.Gbbq{}},
	}
	p := NewPytdx(time.Second, "127.0.0.1")
	p.dailyClientFactory = func(context.Context) (pytdxDailyClient, error) { return client, nil }

	bars, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-21", "2026-10-21", 10, model.AdjustmentForward)
	if err != nil {
		t.Fatalf("empty corporate-action list should be treated as a valid empty event set, got %v", err)
	}
	if len(bars) != 1 || bars[0].AdjustmentBasis != model.AdjustmentForward || bars[0].AdjustmentMethod != "pytdx_qfq" {
		t.Fatalf("unexpected adjusted bars: %+v", bars)
	}
}

func containsPytdxError(value, fragment string) bool {
	return len(value) >= len(fragment) && strings.Contains(value, fragment)
}

func TestPytdxDailyBarsGbbqConnectionFailureIsPreserved(t *testing.T) {
	connectionErr := errors.New("dial tcp 127.0.0.1:7709: connect: connection refused")
	client := &fakePytdxDailyClient{
		klineResponse: &protocol.KlineResp{List: []*protocol.Kline{{
			Time: time.Date(2026, 10, 21, 15, 0, 0, 0, time.FixedZone("CST", 8*60*60)),
			Open: protocol.Price(10000), High: protocol.Price(10200),
			Low: protocol.Price(9800), Close: protocol.Price(10100),
			Last: protocol.Price(10000), Volume: 100, Amount: protocol.Price(101000000),
		}}},
		gbbqErr: connectionErr,
	}
	p := NewPytdx(time.Second, "127.0.0.1")
	p.dailyClientFactory = func(context.Context) (pytdxDailyClient, error) { return client, nil }

	_, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-21", "2026-10-21", 10, model.AdjustmentBackward)
	if err == nil || !errors.Is(err, connectionErr) {
		t.Fatalf("expected original GBBQ connection error, got %v", err)
	}
}
