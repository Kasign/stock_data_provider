package indicator

import (
	"errors"
	"testing"
)

func TestATRUsesPreviousCloseBeforeInputWindow(t *testing.T) {
	highs := []float64{12, 14}
	lows := []float64{9, 11}
	closes := []float64{11, 13}
	params := ATRParams{Window: 2, SmoothingMethod: "wilder"}

	got, err := ATR(highs, lows, closes, 10, params)
	if err != nil {
		t.Fatalf("ATR with preceding close: %v", err)
	}
	if got.Status != StatusAvailable || got.SampleCount != 2 || got.Value != 3 {
		t.Fatalf("unexpected ATR result: %+v", got)
	}

	// 改变窗口前一交易日收盘价，只影响首个 TR，不应被输入窗口首日 close 替代。
	got, err = ATR(highs, lows, closes, 1, params)
	if err != nil {
		t.Fatalf("ATR with alternate preceding close: %v", err)
	}
	if got.Value != 7 {
		t.Fatalf("ATR did not use previous close for first TR: got %v, want 7", got.Value)
	}
}

func TestATRRequiresWindowBarsAndValidPreviousClose(t *testing.T) {
	params := ATRParams{Window: 3, SmoothingMethod: "wilder"}
	highs := []float64{12, 14}
	lows := []float64{9, 11}
	closes := []float64{11, 13}

	got, err := ATR(highs, lows, closes, 10, params)
	if err != nil {
		t.Fatalf("ATR with insufficient bars: %v", err)
	}
	if got.Status != StatusInsufficientData || got.SampleCount != 2 || got.Value != 0 {
		t.Fatalf("unexpected insufficient-data result: %+v", got)
	}

	if _, err := ATR(highs, lows, closes, 0, params); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("zero previous close error: got %v, want %v", err, ErrInvalidInput)
	}
}
