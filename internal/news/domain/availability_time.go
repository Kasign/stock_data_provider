package domain

import (
	"strconv"
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
	// AvailabilityTimePrecisionDisclosureBoundary marks a PIT-safe conservative
	// boundary derived from a source field that encodes only a disclosure DATE.
	// It is not an exact first-public instant; the value is the end of the
	// declared disclosure day, so it is never earlier than the real disclosure.
	AvailabilityTimePrecisionDisclosureBoundary AvailabilityTimePrecision = "disclosure_boundary"
	AvailabilityTimePrecisionUnknown            AvailabilityTimePrecision = "unknown"
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

// ParseDisclosureDateConservativeBoundary parses a source field that encodes a
// disclosure DATE (e.g. CNINFO announcementTime, epoch milliseconds at local
// midnight) into a conservative PIT boundary. The candidate is the final second
// of the declared disclosure day in the declared fixed offset, converted to UTC.
//
// Semantics: this is deliberately NOT an exact first-public instant. The source
// only proves "disclosed on date D"; using end-of-day D guarantees the candidate
// is never earlier than the true disclosure, so strict PIT never adopts the
// information before it was actually available (no look-ahead). The trade-off,
// accepted by the project owner as v1 provisional, is day-level granularity.
//
// This parser is intentionally separate from ParseAvailabilityTime: the general
// parser keeps rejecting date-only values, and this boundary is only reachable
// for a source explicitly qualified with disclosure-date semantics.
func ParseDisclosureDateConservativeBoundary(rawMillis, declaredOffset string) AvailabilityTimeCandidate {
	result := AvailabilityTimeCandidate{
		RawValue:               rawMillis,
		SourceTimezoneOrOffset: declaredOffset,
		Precision:              AvailabilityTimePrecisionDisclosureBoundary,
		Status:                 AvailabilityTimeUnparseable,
	}
	if rawMillis == "" || !allDigits(rawMillis) {
		result.ReasonCode = "news.available_at.disclosure_date.invalid_millis"
		return result
	}
	millis, err := strconv.ParseInt(rawMillis, 10, 64)
	if err != nil || millis <= 0 {
		result.ReasonCode = "news.available_at.disclosure_date.invalid_millis"
		return result
	}
	zone, err := parseFixedOffset(declaredOffset)
	if err != nil {
		result.ReasonCode = "news.available_at.disclosure_date.invalid_offset"
		return result
	}
	local := time.UnixMilli(millis).UTC().In(zone)
	// End of the declared disclosure day: 23:59:59 local, zero nanoseconds.
	boundary := time.Date(local.Year(), local.Month(), local.Day(), 23, 59, 59, 0, zone).UTC()
	result.Status = AvailabilityTimeParsed
	result.CandidateAvailableAt = &boundary
	return result
}

// parseFixedOffset accepts only explicit numeric offsets (+HH:MM / -HH:MM) with
// whole-minute precision. "Z" is intentionally rejected: a disclosure-date
// source must declare its local calendar offset to define the disclosure day.
func parseFixedOffset(value string) (*time.Location, error) {
	if len(value) != 6 || (value[0] != '+' && value[0] != '-') || value[3] != ':' {
		return nil, strconv.ErrSyntax
	}
	hours, err := strconv.Atoi(value[1:3])
	if err != nil || hours > 23 {
		return nil, strconv.ErrSyntax
	}
	minutes, err := strconv.Atoi(value[4:6])
	if err != nil || minutes > 59 {
		return nil, strconv.ErrSyntax
	}
	offset := hours*3600 + minutes*60
	if value[0] == '-' {
		offset = -offset
	}
	return time.FixedZone(value, offset), nil
}

// IsPITSafeAvailabilityPrecision reports whether a parsed candidate at this
// precision may be adopted by strict PIT. Second and minute are exact/upper-
// bounded instants; disclosure_boundary is a conservative end-of-day boundary
// whose value is a UTC whole second that is never earlier than the real
// disclosure. Low precisions (date/month/year), subsecond and unknown are not
// PIT-safe and must stay fail-closed.
func IsPITSafeAvailabilityPrecision(value AvailabilityTimePrecision) bool {
	return value == AvailabilityTimePrecisionSecond ||
		value == AvailabilityTimePrecisionMinute ||
		value == AvailabilityTimePrecisionDisclosureBoundary
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
