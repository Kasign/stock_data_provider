package domain

import (
	"errors"
	"testing"
	"time"
)

func securityIdentityRecord(symbol, name, symbolStatus string, from, to time.Time) SecurityIdentityRecord {
	record := SecurityIdentityRecord{
		IdentityNamespace: "exchange-security-master",
		IdentityValue:     "instrument-600000",
		Market:            "XSHG",
		Symbol:            symbol,
		Name:              name,
		SymbolStatus:      SecuritySymbolStatus(symbolStatus),
		LifecycleStatus:   SecurityLifecycleListed,
		ValidFrom:         from,
		ObservedAt:        time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC),
		Source:            "official-security-master",
		EvidenceRef:       "snapshot-2026-11-05",
	}
	if !to.IsZero() {
		record.ValidTo = &to
	}
	return record
}

func TestResolveSecurityIdentitySupportsCodeAndNameHistory(t *testing.T) {
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	change := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	oldCode := securityIdentityRecord("600000", "旧简称", string(SecuritySymbolHistorical), start, change)
	newCode := securityIdentityRecord("601000", "新简称", string(SecuritySymbolActive), change, time.Time{})

	before, err := ResolveSecurityIdentity([]SecurityIdentityRecord{oldCode, newCode}, "XSHG", "600000", change.Add(-time.Second))
	if err != nil || before.Status != SecurityIdentityResolved || before.Record.Name != "旧简称" {
		t.Fatalf("before=%+v err=%v", before, err)
	}
	after, err := ResolveSecurityIdentity([]SecurityIdentityRecord{oldCode, newCode}, "XSHG", "601000", change)
	if err != nil || after.Status != SecurityIdentityResolved || after.Record.Name != "新简称" {
		t.Fatalf("after=%+v err=%v", after, err)
	}
}

func TestResolveSecurityIdentityDoesNotMergeSameCodeAcrossMarkets(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	shanghai := securityIdentityRecord("000001", "甲证券", string(SecuritySymbolActive), at, time.Time{})
	shenzhen := securityIdentityRecord("000001", "乙证券", string(SecuritySymbolActive), at, time.Time{})
	shenzhen.Market = "XSHE"
	shenzhen.IdentityValue = "instrument-sz-000001"

	got, err := ResolveSecurityIdentity([]SecurityIdentityRecord{shanghai, shenzhen}, "XSHE", "000001", at)
	if err != nil || got.Status != SecurityIdentityResolved || got.Record.IdentityValue != "instrument-sz-000001" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestResolveSecurityIdentityReturnsAmbiguousForOverlappingHistory(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	first := securityIdentityRecord("600000", "甲", string(SecuritySymbolActive), at, time.Time{})
	second := securityIdentityRecord("600000", "乙", string(SecuritySymbolActive), at, time.Time{})
	second.IdentityValue = "another-instrument"

	got, err := ResolveSecurityIdentity([]SecurityIdentityRecord{first, second}, "XSHG", "600000", at)
	if err != nil || got.Status != SecurityIdentityAmbiguous {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestValidateSecurityIdentityRequiresProvenanceAndValidPeriod(t *testing.T) {
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	record := securityIdentityRecord("600000", "浦发银行", string(SecuritySymbolActive), at, at.Add(-time.Second))
	if !errors.Is(ValidateSecurityIdentityRecord(record), ErrSecurityIdentityInvalid) {
		t.Fatal("invalid effective period must be rejected")
	}
	record = securityIdentityRecord("600000", "浦发银行", string(SecuritySymbolActive), at, time.Time{})
	record.EvidenceRef = ""
	if !errors.Is(ValidateSecurityIdentityRecord(record), ErrSecurityIdentityInvalid) {
		t.Fatal("missing provenance must be rejected")
	}
}

func TestResolveSecurityIdentityReturnsDelistedSnapshotForHistoricalLookup(t *testing.T) {
	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	record := securityIdentityRecord("600000", "退市证券", string(SecuritySymbolHistorical), at, time.Time{})
	record.LifecycleStatus = SecurityLifecycleDelisted

	got, err := ResolveSecurityIdentity([]SecurityIdentityRecord{record}, "XSHG", "600000", at.Add(time.Hour))
	if err != nil || got.Status != SecurityIdentityResolved || got.Record.LifecycleStatus != SecurityLifecycleDelisted {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
