package main

import (
	"testing"
	"time"
)

func TestParseCollectOptionsRequiresSymbol(t *testing.T) {
	if _, err := parseCollectOptions([]string{}); err == nil {
		t.Fatal("missing -symbol must be rejected")
	}
}

func TestParseCollectOptionsDefaultsToObservationInstant(t *testing.T) {
	options, err := parseCollectOptions([]string{"-symbol", "600519.SH"})
	if err != nil {
		t.Fatal(err)
	}
	if !options.systemAsOf.IsZero() {
		t.Fatalf("system_as_of = %s, want zero (observation instant)", options.systemAsOf)
	}
	if options.purpose != "risk_review" {
		t.Fatalf("purpose = %q, want the default risk_review", options.purpose)
	}
}

func TestParseCollectOptionsConvertsExplicitSystemAsOfToUTC(t *testing.T) {
	options, err := parseCollectOptions([]string{
		"-symbol", "600519.SH",
		"-system-as-of", "2026-11-27T23:30:00+08:00", "-purpose", "risk_review",
	})
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

func TestParseCollectOptionsRejectsInvalidInput(t *testing.T) {
	for _, args := range [][]string{
		{"-symbol", "600519.SH", "-timeout", "0"},
		{"-symbol", "600519.SH", "-purpose", ""},
		{"-symbol", "600519.SH", "-system-as-of", "not-a-time"},
		{"-symbol", "600519.SH", "extra"},
	} {
		if _, err := parseCollectOptions(args); err == nil {
			t.Fatalf("args %v must be rejected", args)
		}
	}
}
