package main

import (
	"testing"
	"time"
)

func TestParseCollectOptionsRequiresSymbolAndTradeDate(t *testing.T) {
	now := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	if _, err := parseCollectOptions([]string{"-trade-date", "2026-11-27"}, now); err == nil {
		t.Fatal("missing -symbol must be rejected")
	}
	if _, err := parseCollectOptions([]string{"-symbol", "600519.SH"}, now); err == nil {
		t.Fatal("missing -trade-date must be rejected")
	}
}

func TestParseCollectOptionsRejectsMalformedTradeDate(t *testing.T) {
	now := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	for _, bad := range []string{"2026/11/27", "20261127", "2026-13-01"} {
		if _, err := parseCollectOptions([]string{"-symbol", "600519.SH", "-trade-date", bad}, now); err == nil {
			t.Fatalf("trade date %q must be rejected", bad)
		}
	}
}

func TestParseCollectOptionsDefaultsToObservationInstant(t *testing.T) {
	now := time.Date(2026, 11, 27, 15, 0, 0, 123456789, time.UTC)
	options, err := parseCollectOptions([]string{"-symbol", "600519.SH", "-trade-date", "2026-11-27"}, now)
	if err != nil {
		t.Fatal(err)
	}
	// 零值表示「以实际观测时刻为准」：预采集不能预先声明一个比观测更早的时点。
	if !options.systemAsOf.IsZero() {
		t.Fatalf("system_as_of = %s, want zero (observation instant)", options.systemAsOf)
	}
	if options.purpose != "risk_review" {
		t.Fatalf("purpose = %q, want the default risk_review", options.purpose)
	}
}

func TestParseCollectOptionsConvertsExplicitSystemAsOfToUTC(t *testing.T) {
	now := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	options, err := parseCollectOptions([]string{
		"-symbol", "600519.SH", "-trade-date", "2026-11-27",
		"-system-as-of", "2026-11-27T23:30:00+08:00", "-purpose", "risk_review",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 27, 15, 30, 0, 0, time.UTC)
	if !options.systemAsOf.Equal(want) {
		t.Fatalf("system_as_of = %s, want %s", options.systemAsOf, want)
	}
	if options.systemAsOf.Location() != time.UTC {
		t.Fatalf("system_as_of location = %s, want UTC", options.systemAsOf.Location())
	}
}

func TestParseCollectOptionsRejectsPositionalArguments(t *testing.T) {
	now := time.Date(2026, 11, 27, 15, 0, 0, 0, time.UTC)
	if _, err := parseCollectOptions([]string{"-symbol", "600519.SH", "-trade-date", "2026-11-27", "extra"}, now); err == nil {
		t.Fatal("positional arguments must be rejected")
	}
}
