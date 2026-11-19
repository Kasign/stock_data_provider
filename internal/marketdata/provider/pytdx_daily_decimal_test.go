package provider

import (
	"testing"

	"github.com/injoyai/tdx/protocol"
)

func TestPytdxDailyBarExactValuesPreserveProtocolPrice(t *testing.T) {
	amount := protocol.Price(123456789)
	item := &protocol.Kline{
		Open: protocol.Price(1234000),
		High: protocol.Price(1235000),
		Low: protocol.Price(1233000),
		Close: protocol.Price(1234500),
		Volume: 12345,
		Amount: amount,
	}
	got := pytdxDailyBarExactValues(item)
	if got == nil || got.Open != "1234" || got.High != "1235" || got.Low != "1233" ||
		got.Close != "1234.5" || got.RawVolume != "12345" || got.RawAmount == nil || *got.RawAmount != amount.String() {
		t.Fatalf("unexpected exact TDX values: %+v", got)
	}
}
