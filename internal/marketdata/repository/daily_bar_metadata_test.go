package repository

import (
	"encoding/json"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestDailyBarMetadataPreservesExactValuesAndLegacyShape(t *testing.T) {
	rawAmount := "123456.7890"
	bar := model.DailyBar{
		Symbol: "600519.SH", TradeDate: "2026-11-18", Period: "1d",
		Open: 1500, High: 1510, Low: 1490, Close: 1505,
		Source: "pytdx", SourceVersion: "pytdx-adapter-v1",
		ExactValues: &model.DailyBarExactValues{
			Open: "1500.000", High: "1510.000", Low: "1490.000", Close: "1505.000",
			RawVolume: "12345", RawAmount: &rawAmount, AmountCNY: &rawAmount,
		},
	}
	publicJSON, err := json.Marshal(bar)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicJSON), "exact_values") || strings.Contains(string(publicJSON), "1500.000") {
		t.Fatalf("exact sidecar must not leak into legacy JSON: %s", publicJSON)
	}

	metadata, err := marshalDailyBarMetadata(bar)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(metadata), "\"exact_values\"") || !strings.Contains(string(metadata), "\"1500.000\"") {
		t.Fatalf("revision metadata must persist exact values: %s", metadata)
	}
	var loaded model.DailyBar
	if err := unmarshalDailyBarMetadata(metadata, &loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.ExactValues == nil || loaded.ExactValues.Open != "1500.000" ||
		loaded.ExactValues.RawVolume != "12345" || loaded.ExactValues.RawAmount == nil || loaded.ExactValues.AmountCNY == nil ||
		*loaded.ExactValues.RawAmount != rawAmount || *loaded.ExactValues.AmountCNY != rawAmount {
		t.Fatalf("exact sidecar did not round-trip: %+v", loaded.ExactValues)
	}

	legacyJSON, err := json.Marshal(bar)
	if err != nil {
		t.Fatal(err)
	}
	var legacyLoaded model.DailyBar
	if err := unmarshalDailyBarMetadata(legacyJSON, &legacyLoaded); err != nil {
		t.Fatal(err)
	}
	if legacyLoaded.ExactValues != nil || legacyLoaded.Symbol != bar.Symbol {
		t.Fatalf("legacy flat metadata must remain readable: %+v", legacyLoaded)
	}
}

func TestDailyBarRevisionHashBindsExactValues(t *testing.T) {
	bar := model.DailyBar{Symbol: "600519.SH", TradeDate: "2026-11-18", Period: "1d", Source: "pytdx", SourceVersion: "adapter-v1"}
	bar.ExactValues = &model.DailyBarExactValues{Open: "1500.000", High: "1510.000", Low: "1490.000", Close: "1505.000", RawVolume: "12345"}
	first, err := dailyBarRevisionHash(bar)
	if err != nil {
		t.Fatal(err)
	}
	bar.ExactValues.Close = "1505.001"
	second, err := dailyBarRevisionHash(bar)
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("changing exact decimal content must create a new daily-bar revision")
	}
}
