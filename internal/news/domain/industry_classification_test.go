package domain

import (
	"errors"
	"testing"
	"time"
)

func industryClassificationRecord(version, code, name, parent string, from, to time.Time) IndustryClassificationRecord {
	record := IndustryClassificationRecord{
		ClassificationSystem: "CSRC",
		ClassificationVersion: version,
		IndustryCode: code,
		IndustryName: name,
		ParentCode: parent,
		ValidFrom: from,
		ObservedAt: time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC),
		Source: "official-classification",
		EvidenceRef: "classification-snapshot-2026",
		Status: IndustryClassificationActive,
	}
	if !to.IsZero() {
		record.ValidTo = &to
	}
	return record
}

func TestResolveIndustryClassificationSeparatesVersionsWithSameCode(t *testing.T) {
	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	oldVersion := industryClassificationRecord("2021", "A01", "旧行业含义", "", at, time.Time{})
	newVersion := industryClassificationRecord("2025", "A01", "新行业含义", "", at, time.Time{})

	got, err := ResolveIndustryClassification([]IndustryClassificationRecord{oldVersion, newVersion}, "CSRC", "2021", "A01", at)
	if err != nil || got.Status != IndustryClassificationResolved || got.Record.IndustryName != "旧行业含义" {
		t.Fatalf("old version=%+v err=%v", got, err)
	}
	got, err = ResolveIndustryClassification([]IndustryClassificationRecord{oldVersion, newVersion}, "CSRC", "2025", "A01", at)
	if err != nil || got.Status != IndustryClassificationResolved || got.Record.IndustryName != "新行业含义" {
		t.Fatalf("new version=%+v err=%v", got, err)
	}
}

func TestResolveIndustryClassificationSupportsNameAndParentHistory(t *testing.T) {
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	change := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	before := industryClassificationRecord("2021", "A01", "旧名称", "A", start, change)
	after := industryClassificationRecord("2021", "A01", "新名称", "B", change, time.Time{})

	oldResult, err := ResolveIndustryClassification([]IndustryClassificationRecord{before, after}, "CSRC", "2021", "A01", change.Add(-time.Second))
	if err != nil || oldResult.Status != IndustryClassificationResolved ||
		oldResult.Record.IndustryName != "旧名称" || oldResult.Record.ParentCode != "A" {
		t.Fatalf("old snapshot=%+v err=%v", oldResult, err)
	}
	newResult, err := ResolveIndustryClassification([]IndustryClassificationRecord{before, after}, "CSRC", "2021", "A01", change)
	if err != nil || newResult.Status != IndustryClassificationResolved ||
		newResult.Record.IndustryName != "新名称" || newResult.Record.ParentCode != "B" {
		t.Fatalf("new snapshot=%+v err=%v", newResult, err)
	}
}

func TestResolveIndustryClassificationRejectsOverlappingRecords(t *testing.T) {
	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	first := industryClassificationRecord("2025", "A01", "甲", "", at, time.Time{})
	second := industryClassificationRecord("2025", "A01", "乙", "", at, time.Time{})
	got, err := ResolveIndustryClassification([]IndustryClassificationRecord{first, second}, "CSRC", "2025", "A01", at)
	if err != nil || got.Status != IndustryClassificationAmbiguous {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestValidateIndustryClassificationRecordRejectsInvalidPeriodAndMissingEvidence(t *testing.T) {
	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	record := industryClassificationRecord("2025", "A01", "行业", "", at, at)
	if !errors.Is(ValidateIndustryClassificationRecord(record), ErrIndustryClassificationInvalid) {
		t.Fatal("invalid effective period must be rejected")
	}
	record = industryClassificationRecord("2025", "A01", "行业", "", at, time.Time{})
	record.EvidenceRef = ""
	if !errors.Is(ValidateIndustryClassificationRecord(record), ErrIndustryClassificationInvalid) {
		t.Fatal("missing evidence must be rejected")
	}
	record.EvidenceRef = "snapshot"
	record.ClassificationVersion = ""
	if _, err := ResolveIndustryClassification([]IndustryClassificationRecord{record}, "CSRC", "", "A01", at); !errors.Is(err, ErrIndustryClassificationInvalid) {
		t.Fatalf("missing version err=%v", err)
	}
}

func TestResolveIndustryClassificationDoesNotCrossVersions(t *testing.T) {
	at := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	record := industryClassificationRecord("2021", "A01", "行业", "", at, time.Time{})
	got, err := ResolveIndustryClassification([]IndustryClassificationRecord{record}, "CSRC", "2025", "A01", at)
	if err != nil || got.Status != IndustryClassificationNotFound {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
