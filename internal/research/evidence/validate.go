package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// Validate checks RSC-CTR-001 structure and verifies the content fingerprint.
// It does not resolve source refs or re-authorize source-owned content.
func (e Evidence) Validate() error {
	if e.EvidenceID == "" || e.EvidenceID != strings.TrimSpace(e.EvidenceID) {
		return fmt.Errorf("evidence_id is required without surrounding whitespace")
	}
	if err := validateEvidenceType(e.EvidenceType); err != nil {
		return err
	}
	if err := validateSourceOwner(e.SourceOwner); err != nil {
		return err
	}
	if e.SourceRef == "" || e.SourceRef != strings.TrimSpace(e.SourceRef) {
		return fmt.Errorf("source_ref is required without surrounding whitespace")
	}
	if e.SourceVersion != nil && (*e.SourceVersion == "" || *e.SourceVersion != strings.TrimSpace(*e.SourceVersion)) {
		return fmt.Errorf("source_version cannot be empty or whitespace-padded when provided")
	}
	if e.ClaimRef != nil && (*e.ClaimRef == "" || *e.ClaimRef != strings.TrimSpace(*e.ClaimRef)) {
		return fmt.Errorf("claim_ref cannot be empty or whitespace-padded when provided")
	}
	if e.DerivedFromRefs == nil || e.SubjectRefs == nil || e.Limitations == nil {
		return fmt.Errorf("derived_from_refs, subject_refs and limitations must be non-null arrays")
	}
	if err := validateUniqueStrings("derived_from_refs", e.DerivedFromRefs); err != nil {
		return err
	}
	if err := validateUniqueStrings("subject_refs", e.SubjectRefs); err != nil {
		return err
	}
	if e.EvidenceType == TypeInference || e.EvidenceType == TypeHypothesis {
		if len(e.DerivedFromRefs) == 0 {
			return fmt.Errorf("%s evidence requires derived_from_refs", e.EvidenceType)
		}
	}
	if err := validateRequiredUTC("system_observed_at", e.SystemObservedAt); err != nil {
		return err
	}
	if err := validateRequiredUTC("system_as_of", e.SystemAsOf); err != nil {
		return err
	}
	if e.SystemAsOf.Before(e.SystemObservedAt) {
		return fmt.Errorf("system_as_of must not be earlier than system_observed_at")
	}
	for _, item := range []struct {
		name  string
		value *time.Time
	}{
		{name: "published_at", value: e.PublishedAt},
		{name: "available_at", value: e.AvailableAt},
		{name: "valid_time_start", value: e.ValidTimeStart},
		{name: "valid_time_end", value: e.ValidTimeEnd},
	} {
		if item.value != nil {
			if err := validateRequiredUTC(item.name, *item.value); err != nil {
				return err
			}
		}
	}
	if (e.ValidTimeStart == nil) != (e.ValidTimeEnd == nil) {
		return fmt.Errorf("valid_time_start and valid_time_end must both be null or both be set")
	}
	if e.ValidTimeStart != nil && e.ValidTimeStart.After(*e.ValidTimeEnd) {
		return fmt.Errorf("valid_time_start must not be after valid_time_end")
	}
	if e.Confidence != nil {
		if math.IsNaN(*e.Confidence) || math.IsInf(*e.Confidence, 0) || *e.Confidence < 0 || *e.Confidence > 1 {
			return fmt.Errorf("confidence must be within [0,1]")
		}
		if e.ConfidenceMethod == nil || strings.TrimSpace(*e.ConfidenceMethod) == "" {
			return fmt.Errorf("confidence_method is required when confidence is provided")
		}
		if e.ConfidenceMethodVersion == nil || strings.TrimSpace(*e.ConfidenceMethodVersion) == "" {
			return fmt.Errorf("confidence_method_version is required when confidence is provided")
		}
	}
	if err := validateStance(e.Stance); err != nil {
		return err
	}
	if err := validateQualityStatus(e.QualityStatus); err != nil {
		return err
	}
	if err := validateContentLevel(e.ContentLevel); err != nil {
		return err
	}
	if e.AuthorizationRef == "" || e.AuthorizationRef != strings.TrimSpace(e.AuthorizationRef) {
		return fmt.Errorf("authorization_ref is required without surrounding whitespace")
	}
	if len(e.ContentFingerprint) != 64 {
		return fmt.Errorf("content_fingerprint must be a 64-character lowercase SHA-256 hex string")
	}
	if _, err := hex.DecodeString(e.ContentFingerprint); err != nil || strings.ToLower(e.ContentFingerprint) != e.ContentFingerprint {
		return fmt.Errorf("content_fingerprint must be a 64-character lowercase SHA-256 hex string")
	}
	computed, err := e.ComputeContentFingerprint()
	if err != nil {
		return err
	}
	if computed != e.ContentFingerprint {
		return fmt.Errorf("content_fingerprint does not match canonical evidence content")
	}
	return nil
}

// ValidateForStrictPIT checks that the Evidence was knowable at its own cutoff
// and at the consuming Run cutoff. Unknown available_at is not strict-PIT safe.
func (e Evidence) ValidateForStrictPIT(runAsOf time.Time) error {
	if err := e.Validate(); err != nil {
		return err
	}
	if err := validateRequiredUTC("run system_as_of", runAsOf); err != nil {
		return err
	}
	if e.SystemAsOf.After(runAsOf) {
		return fmt.Errorf("evidence system_as_of is later than Run system_as_of")
	}
	if e.SystemObservedAt.After(e.SystemAsOf) {
		return fmt.Errorf("system_observed_at is later than evidence system_as_of")
	}
	if e.AvailableAt == nil {
		return fmt.Errorf("available_at is unknown; evidence is not strict-PIT eligible")
	}
	if e.AvailableAt.After(e.SystemAsOf) {
		return fmt.Errorf("available_at is later than evidence system_as_of")
	}
	return nil
}

// ComputeContentFingerprint applies the RSC-CTR-001 fingerprint input rules:
// JCS over all semantic fields except evidence_id, authorization_ref and
// content_fingerprint, followed by SHA-256.
func (e Evidence) ComputeContentFingerprint() (string, error) {
	fields := map[string]any{
		"available_at":              nullableTime(e.AvailableAt),
		"claim_ref":                 nullableString(e.ClaimRef),
		"confidence":                nullableFloat(e.Confidence),
		"confidence_method":         nullableString(e.ConfidenceMethod),
		"confidence_method_version": nullableString(e.ConfidenceMethodVersion),
		"content_level":             string(e.ContentLevel),
		"derived_from_refs":         e.DerivedFromRefs,
		"evidence_type":             string(e.EvidenceType),
		"limitations":               e.Limitations,
		"published_at":              nullableTime(e.PublishedAt),
		"quality_status":            string(e.QualityStatus),
		"source_locator":            nullableString(e.SourceLocator),
		"source_owner":              string(e.SourceOwner),
		"source_ref":                e.SourceRef,
		"source_version":            nullableString(e.SourceVersion),
		"stance":                    string(e.Stance),
		"subject_refs":              e.SubjectRefs,
		"system_as_of":              e.SystemAsOf.UTC().Format(time.RFC3339Nano),
		"system_observed_at":        e.SystemObservedAt.UTC().Format(time.RFC3339Nano),
		"valid_time_end":            nullableTime(e.ValidTimeEnd),
		"valid_time_start":          nullableTime(e.ValidTimeStart),
	}
	canonical, err := canonicalObject(fields)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:]), nil
}

func canonicalObject(fields map[string]any) (string, error) {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		if !utf8.ValidString(key) {
			return "", fmt.Errorf("canonical JSON object key is not valid UTF-8")
		}
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return lessUTF16(keys[i], keys[j])
	})
	var builder strings.Builder
	builder.WriteByte('{')
	for index, key := range keys {
		if index > 0 {
			builder.WriteByte(',')
		}
		encodedKey, err := canonicalString(key)
		if err != nil {
			return "", err
		}
		encodedValue, err := canonicalValue(fields[key])
		if err != nil {
			return "", fmt.Errorf("canonicalize %s: %w", key, err)
		}
		builder.WriteString(encodedKey)
		builder.WriteByte(':')
		builder.WriteString(encodedValue)
	}
	builder.WriteByte('}')
	return builder.String(), nil
}

func canonicalValue(value any) (string, error) {
	switch typed := value.(type) {
	case nil:
		return "null", nil
	case string:
		return canonicalString(typed)
	case bool:
		if typed {
			return "true", nil
		}
		return "false", nil
	case []string:
		values := make([]any, len(typed))
		for i := range typed {
			values[i] = typed[i]
		}
		return canonicalValue(values)
	case []any:
		var builder strings.Builder
		builder.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				builder.WriteByte(',')
			}
			encoded, err := canonicalValue(item)
			if err != nil {
				return "", fmt.Errorf("array item %d: %w", index, err)
			}
			builder.WriteString(encoded)
		}
		builder.WriteByte(']')
		return builder.String(), nil
	case map[string]any:
		return canonicalObject(typed)
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return "", fmt.Errorf("JCS does not permit NaN or Infinity")
		}
		return canonicalNumber(typed), nil
	default:
		return "", fmt.Errorf("unsupported canonical JSON type %T", value)
	}
}

func lessUTF16(left, right string) bool {
	leftUnits := utf16.Encode([]rune(left))
	rightUnits := utf16.Encode([]rune(right))
	limit := len(leftUnits)
	if len(rightUnits) < limit {
		limit = len(rightUnits)
	}
	for i := 0; i < limit; i++ {
		if leftUnits[i] != rightUnits[i] {
			return leftUnits[i] < rightUnits[i]
		}
	}
	return len(leftUnits) < len(rightUnits)
}

func canonicalString(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", fmt.Errorf("canonical JSON string is not valid UTF-8")
	}
	var builder strings.Builder
	encoder := json.NewEncoder(&builder)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return "", err
	}
	encoded := strings.TrimSuffix(builder.String(), "\n")
	encoded = strings.ReplaceAll(encoded, `\u2028`, "\u2028")
	encoded = strings.ReplaceAll(encoded, `\u2029`, "\u2029")
	return encoded, nil
}

func canonicalNumber(value float64) string {
	if value == 0 {
		return "0"
	}
	absolute := math.Abs(value)
	if absolute >= 1e-6 && absolute < 1e21 {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	formatted := strconv.FormatFloat(value, 'e', -1, 64)
	parts := strings.SplitN(formatted, "e", 2)
	exponent, _ := strconv.Atoi(parts[1])
	sign := ""
	if exponent >= 0 {
		sign = "+"
	}
	return parts[0] + "e" + sign + strconv.Itoa(exponent)
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func validateRequiredUTC(name string, value time.Time) error {
	if value.IsZero() {
		return fmt.Errorf("%s is required", name)
	}
	_, offset := value.Zone()
	if offset != 0 || value.Location() != time.UTC {
		return fmt.Errorf("%s must be UTC", name)
	}
	return nil
}

func validateUniqueStrings(name string, values []string) error {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return fmt.Errorf("%s cannot contain empty or whitespace-padded values", name)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%s contains duplicate value %q", name, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateEvidenceType(value EvidenceType) error {
	switch value {
	case TypeFact, TypeReportedFact, TypeInference, TypeHypothesis, TypeUnknown:
		return nil
	default:
		return fmt.Errorf("evidence_type is invalid")
	}
}

func validateSourceOwner(value SourceOwner) error {
	switch value {
	case SourceData, SourceNews, SourceQuant, SourceResearch:
		return nil
	default:
		return fmt.Errorf("source_owner is invalid")
	}
}

func validateStance(value Stance) error {
	switch value {
	case StanceSupports, StanceContradicts, StanceNeutral:
		return nil
	default:
		return fmt.Errorf("stance is invalid")
	}
}

func validateQualityStatus(value QualityStatus) error {
	switch value {
	case QualityValid, QualityDegraded, QualityInvalid, QualityUnknown:
		return nil
	default:
		return fmt.Errorf("quality_status is invalid")
	}
}

func validateContentLevel(value ContentLevel) error {
	switch value {
	case ContentMetadata, ContentSummary, ContentSnippet, ContentFull:
		return nil
	default:
		return fmt.Errorf("content_level is invalid")
	}
}
