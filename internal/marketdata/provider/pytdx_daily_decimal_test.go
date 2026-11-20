package provider

import (
	"testing"

	"github.com/injoyai/tdx/protocol"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestPytdxDailyBarExactValuesPreserveProtocolPrice(t *testing.T) {
	amount := protocol.Price(123456789)
	item := &protocol.Kline{
		Open:   protocol.Price(1234000),
		High:   protocol.Price(1235000),
		Low:    protocol.Price(1233000),
		Close:  protocol.Price(1234500),
		Volume: 12345,
		Amount: amount,
	}
	got := pytdxDailyBarExactValues(item)
	if got == nil || got.Open != "1234" || got.High != "1235" || got.Low != "1233" ||
		got.Close != "1234.5" || got.RawVolume != "12345" || got.RawAmount == nil || *got.RawAmount != "123456.789" || got.AmountCNY != nil ||
		got.OpenEvidence == nil || got.OpenEvidence.EvidenceKind != model.DailyBarPrecisionEvidenceProtocolFixedScale ||
		got.OpenEvidence.ProtocolScale == nil || *got.OpenEvidence.ProtocolScale != 3 ||
		got.OpenEvidence.SourceToken != "1234000" ||
		got.RawAmountEvidence == nil || got.RawAmountEvidence.EvidenceKind != model.DailyBarPrecisionEvidenceProtocolFixedScale ||
		got.RawAmountEvidence.ProtocolScale == nil || *got.RawAmountEvidence.ProtocolScale != 3 {
		t.Fatalf("unexpected exact TDX values: %+v", got)
	}
}
