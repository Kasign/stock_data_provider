package domain

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type AvailabilityDecisionStatus string

const (
	AvailabilityDecisionVerified   AvailabilityDecisionStatus = "verified"
	AvailabilityDecisionConflict   AvailabilityDecisionStatus = "conflict"
	AvailabilityDecisionUnresolved AvailabilityDecisionStatus = "unresolved"
	AvailabilityDecisionRevoked    AvailabilityDecisionStatus = "revoked"
)

// AvailabilityEvidence is an append-only, version-bound record of a source time field.
// Its parsed candidate is not PIT-eligible until a verified decision selects it.
type AvailabilityEvidence struct {
	ID                     string
	NewsID                 string
	VersionID              string
	ObservationID          string
	FieldPath              string
	RawValue               string
	SourceTimezoneOrOffset string
	Precision              AvailabilityTimePrecision
	ParseStatus            AvailabilityTimeParseStatus
	ParseReasonCode        string
	CandidateAvailableAt   *time.Time
	ParserVersion          string
	EvidenceRef            *string
	EvidenceHash           *string
	ObservedAt             time.Time
	RecordedAt             time.Time
	RetentionUntil         time.Time
	PayloadPurgedAt        *time.Time
}

func (e AvailabilityEvidence) Validate() error {
	for name, value := range map[string]string{
		"evidence_id": e.ID,
		"news_id": e.NewsID,
		"version_id": e.VersionID,
		"observation_id": e.ObservationID,
		"field_path": e.FieldPath,
		"parser_version": e.ParserVersion,
	} {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("%s is required without surrounding whitespace", name)
		}
	}
	if len(e.ID) > 128 || len(e.NewsID) > 128 || len(e.VersionID) > 128 ||
		len(e.ObservationID) > 128 || len(e.FieldPath) > 255 || len(e.ParserVersion) > 128 ||
		len(e.SourceTimezoneOrOffset) > 64 || len(e.ParseReasonCode) > 128 {
		return fmt.Errorf("availability evidence field exceeds its storage limit")
	}
	if err := validateAvailabilityPrecision(e.Precision); err != nil {
		return err
	}
	if err := validateAvailabilityParseStatus(e.ParseStatus); err != nil {
		return err
	}
	if err := validateAvailabilityUTC("observed_at", e.ObservedAt); err != nil {
		return err
	}
	if err := validateAvailabilityUTC("recorded_at", e.RecordedAt); err != nil {
		return err
	}
	if err := validateAvailabilityUTC("retention_until", e.RetentionUntil); err != nil {
		return err
	}
	if e.RecordedAt.Before(e.ObservedAt) || !e.RetentionUntil.After(e.RecordedAt) {
		return fmt.Errorf("availability evidence timestamps are not ordered")
	}
	if e.EvidenceRef != nil && (*e.EvidenceRef == "" || *e.EvidenceRef != strings.TrimSpace(*e.EvidenceRef) || len(*e.EvidenceRef) > 512) {
		return fmt.Errorf("evidence_ref is invalid")
	}
	if e.EvidenceHash != nil {
		if len(*e.EvidenceHash) != 64 || strings.ToLower(*e.EvidenceHash) != *e.EvidenceHash {
			return fmt.Errorf("evidence_hash must be a lowercase SHA-256 hex string")
		}
		if _, err := hex.DecodeString(*e.EvidenceHash); err != nil {
			return fmt.Errorf("evidence_hash must be a lowercase SHA-256 hex string")
		}
	}

	if e.PayloadPurgedAt != nil {
		if err := validateAvailabilityUTC("payload_purged_at", *e.PayloadPurgedAt); err != nil {
			return err
		}
		if e.PayloadPurgedAt.Before(e.RecordedAt) || e.RawValue != "" || e.EvidenceRef != nil ||
			e.EvidenceHash != nil || e.CandidateAvailableAt != nil {
			return fmt.Errorf("purged availability evidence retains restricted payload")
		}
		return nil
	}
	if e.RawValue == "" {
		return fmt.Errorf("raw_value is required before payload purge")
	}

	if e.ParseStatus == AvailabilityTimeParsed {
		if e.Precision != AvailabilityTimePrecisionSecond && e.Precision != AvailabilityTimePrecisionMinute {
			return fmt.Errorf("parsed availability time must have second or minute precision")
		}
		if e.CandidateAvailableAt == nil || e.ParseReasonCode != "" || e.SourceTimezoneOrOffset == "" {
			return fmt.Errorf("parsed availability evidence is incomplete")
		}
		if err := validateAvailabilityUTC("candidate_available_at", *e.CandidateAvailableAt); err != nil {
			return err
		}
		if e.CandidateAvailableAt.Nanosecond() != 0 || e.CandidateAvailableAt.After(e.ObservedAt) {
			return fmt.Errorf("candidate_available_at must be second precision and not later than observed_at")
		}
	} else if e.CandidateAvailableAt != nil || e.ParseReasonCode == "" {
		return fmt.Errorf("unparsed availability evidence must have a reason and no candidate")
	}
	return nil
}

// AvailabilityDecision is a complete append-only visibility state for one version.
type AvailabilityDecision struct {
	ID                    string
	VersionID             string
	Sequence              uint64
	Status                AvailabilityDecisionStatus
	SelectedEvidenceID    *string
	ConsideredEvidenceIDs []string
	ReasonCode            string
	ReviewRuleVersion     string
	SupersedesDecisionID  *string
	RecordedAt            time.Time
}

func (d AvailabilityDecision) Validate() error {
	if d.ID == "" || d.ID != strings.TrimSpace(d.ID) || len(d.ID) > 128 ||
		d.VersionID == "" || d.VersionID != strings.TrimSpace(d.VersionID) || len(d.VersionID) > 128 ||
		d.Sequence == 0 || d.ReasonCode == "" || d.ReasonCode != strings.TrimSpace(d.ReasonCode) || len(d.ReasonCode) > 128 ||
		d.ReviewRuleVersion == "" || d.ReviewRuleVersion != strings.TrimSpace(d.ReviewRuleVersion) || len(d.ReviewRuleVersion) > 128 {
		return fmt.Errorf("availability decision identity and review metadata are required")
	}
	if err := validateAvailabilityUTC("recorded_at", d.RecordedAt); err != nil {
		return err
	}
	if d.Sequence == 1 && d.SupersedesDecisionID != nil {
		return fmt.Errorf("first availability decision cannot supersede another decision")
	}
	if d.Sequence > 1 && (d.SupersedesDecisionID == nil || *d.SupersedesDecisionID == "" ||
		*d.SupersedesDecisionID != strings.TrimSpace(*d.SupersedesDecisionID) || len(*d.SupersedesDecisionID) > 128) {
		return fmt.Errorf("availability decision after the first must identify its predecessor")
	}
	if d.ConsideredEvidenceIDs == nil {
		return fmt.Errorf("considered_evidence_ids must be an explicit array")
	}
	if !sort.StringsAreSorted(d.ConsideredEvidenceIDs) {
		return fmt.Errorf("considered_evidence_ids must be sorted")
	}
	seen := make(map[string]struct{}, len(d.ConsideredEvidenceIDs))
	for _, id := range d.ConsideredEvidenceIDs {
		if id == "" || id != strings.TrimSpace(id) || len(id) > 128 {
			return fmt.Errorf("considered_evidence_ids contains an invalid id")
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("considered_evidence_ids contains a duplicate id")
		}
		seen[id] = struct{}{}
	}

	switch d.Status {
	case AvailabilityDecisionVerified:
		if d.SelectedEvidenceID == nil || *d.SelectedEvidenceID == "" || len(*d.SelectedEvidenceID) > 128 {
			return fmt.Errorf("verified decision requires selected_evidence_id")
		}
		if _, exists := seen[*d.SelectedEvidenceID]; !exists {
			return fmt.Errorf("selected_evidence_id must be included in considered_evidence_ids")
		}
	case AvailabilityDecisionConflict:
		if d.SelectedEvidenceID != nil || len(d.ConsideredEvidenceIDs) < 2 {
			return fmt.Errorf("conflict decision requires at least two evidence ids and no selection")
		}
	case AvailabilityDecisionUnresolved, AvailabilityDecisionRevoked:
		if d.SelectedEvidenceID != nil {
			return fmt.Errorf("unresolved or revoked decision cannot select evidence")
		}
	default:
		return fmt.Errorf("availability decision status is invalid")
	}
	return nil
}

func validateAvailabilityPrecision(value AvailabilityTimePrecision) error {
	switch value {
	case AvailabilityTimePrecisionSecond, AvailabilityTimePrecisionMinute,
		AvailabilityTimePrecisionDate, AvailabilityTimePrecisionMonth,
		AvailabilityTimePrecisionYear, AvailabilityTimePrecisionSubsecond,
		AvailabilityTimePrecisionUnknown:
		return nil
	default:
		return fmt.Errorf("availability time precision is invalid")
	}
}

func validateAvailabilityParseStatus(value AvailabilityTimeParseStatus) error {
	switch value {
	case AvailabilityTimeParsed, AvailabilityTimeUnparseable,
		AvailabilityTimeAmbiguous, AvailabilityTimeUnsupportedPrecision:
		return nil
	default:
		return fmt.Errorf("availability time parse status is invalid")
	}
}

func validateAvailabilityUTC(name string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s is required", name)
	}
	_, offset := value.Zone()
	if offset != 0 || value.Location() != time.UTC {
		return fmt.Errorf("%s must be UTC", name)
	}
	return nil
}
