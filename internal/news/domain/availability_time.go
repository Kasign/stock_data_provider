package domain

import (
	"strings"
	"time"
)

// AvailabilityTimePrecision describes the precision declared by the source value.
// It is evidence metadata; it must never be inferred from the database column type.
type AvailabilityTimePrecision string

const (
	AvailabilityTimePrecisionSecond    AvailabilityTimePrecision = "second"
	AvailabilityTimePrecisionMinute    AvailabilityTimePrecision = "minute"
	AvailabilityTimePrecisionDate      AvailabilityTimePrecision = "date"
	AvailabilityTimePrecisionMonth     AvailabilityTimePrecision = "month"
	AvailabilityTimePrecisionYear      AvailabilityTimePrecision = "year"
	AvailabilityTimePrecisionSubsecond AvailabilityTimePrecision = "subsecond"
	AvailabilityTimePrecisionUnknown   AvailabilityTimePrecision = "unknown"
)

// AvailabilityTimeParseStatus describes syntactic normalization only.
// A parsed value is still an untrusted candidate and is not approved for PIT use.
type AvailabilityTimeParseStatus string

const (
	AvailabilityTimeParsed               AvailabilityTimeParseStatus = "parsed"
	AvailabilityTimeUnparseable          AvailabilityTimeParseStatus = "unparseable"
	AvailabilityTimeAmbiguous            AvailabilityTimeParseStatus = "ambiguous"
	AvailabilityTimeUnsupportedPrecision AvailabilityTimeParseStatus = "unsupported_precision"
)

// AvailabilityTimeCandidate preserves the source representation and its declared precision.
// CandidateAvailableAt is populated only after an unambiguous timezone-aware parse.
type AvailabilityTimeCandidate struct {
	RawValue               string
	SourceTimezoneOrOffset string
	Precision              AvailabilityTimePrecision
	Status                 AvailabilityTimeParseStatus
	ReasonCode             string
	CandidateAvailableAt   *time.Time
}

// ParseAvailabilityTime parses a source timestamp without asserting its business meaning.
//
// Second-precision values are converted to UTC without changing their instant.
// Minute-precision values use the final second of the declared minute as a conservative
// candidate boundary. Date/month/year values and subsecond values do not produce a candidate.
// A parsed candidate still requires source-semantic, authorization and version-binding review.
func ParseAvailabilityTime(raw string) AvailabilityTimeCandidate {
	result := AvailabilityTimeCandidate{
		RawValue:   raw,
		Precision:  AvailabilityTimePrecisionUnknown,
		Status:     AvailabilityTimeUnparseable,
		ReasonCode: "news.available_at.invalid_format",
	}
	value := strings.TrimSpace(raw)
	if value == "" {
		result.ReasonCode = "news.available_at.empty"
		return result
	}

	switch {
	case len(value) == 4 && allDigits(value):
		result.Precision = AvailabilityTimePrecisionYear
		result.Status = AvailabilityTimeUnsupportedPrecision
		result.ReasonCode = "news.available_at.year_only"
		return result
	case len(value) == 7 && value[4] == '-' && allDigits(value[:4]) && allDigits(value[5:]):
		result.Precision = AvailabilityTimePrecisionMonth
		result.Status = AvailabilityTimeUnsupportedPrecision
		result.ReasonCode = "news.available_at.month_only"
		return result
	case len(value) == 10:
		if _, err := time.Parse("2006-01-02", value); err == nil {
			result.Precision = AvailabilityTimePrecisionDate
			result.Status = AvailabilityTimeUnsupportedPrecision
			result.ReasonCode = "news.available_at.date_only"
			return result
		}
	}

	tIndex := strings.IndexByte(value, 'T')
	if tIndex < 0 {
		result.ReasonCode = "news.available_at.timezone_or_datetime_missing"
		return result
	}
	timeAndZone := value[tIndex+1:]
	zoneIndex := strings.IndexAny(timeAndZone, "Z+-")
	if zoneIndex < 0 {
		result.Status = AvailabilityTimeAmbiguous
		result.ReasonCode = "news.available_at.timezone_missing"
		return result
	}
	clock := timeAndZone[:zoneIndex]
	result.SourceTimezoneOrOffset = timeAndZone[zoneIndex:]
	if strings.ContainsAny(clock, ".,") {
		result.Precision = AvailabilityTimePrecisionSubsecond
		result.Status = AvailabilityTimeUnsupportedPrecision
		result.ReasonCode = "news.available_at.subsecond_precision_unsupported"
		return result
	}

	switch strings.Count(clock, ":") {
	case 2:
		result.Precision = AvailabilityTimePrecisionSecond
		parsed, err := time.Parse(time.RFC3339, value)
		if err != nil {
			result.ReasonCode = "news.available_at.invalid_second_timestamp"
			return result
		}
		candidate := parsed.UTC()
		result.CandidateAvailableAt = &candidate
	case 1:
		result.Precision = AvailabilityTimePrecisionMinute
		parsed, err := time.Parse("2006-01-02T15:04Z07:00", value)
		if err != nil {
			result.ReasonCode = "news.available_at.invalid_minute_timestamp"
			return result
		}
		candidate := parsed.Add(59 * time.Second).UTC()
		result.CandidateAvailableAt = &candidate
	default:
		result.ReasonCode = "news.available_at.invalid_time_precision"
		return result
	}

	result.Status = AvailabilityTimeParsed
	result.ReasonCode = ""
	return result
}

func allDigits(value string) bool {
	if value == "" {
		return false
	}
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}
