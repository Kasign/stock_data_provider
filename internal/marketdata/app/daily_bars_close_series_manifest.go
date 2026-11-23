package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
)

const (
	DailyBarsCloseSeriesManifestSchemaVersion = "2.0.0"
	dailyBarsCloseSeriesSnapshotRefPrefix     = "data_snapshot:sha256:"
)

// DailyBarsCloseSeriesManifest 是 DATA-CTR-008 2.0.0 Manifest。
type DailyBarsCloseSeriesManifest struct {
	SchemaVersion       string                       `json:"schema_version"`
	SnapshotRef         string                       `json:"snapshot_ref"`
	ManifestFingerprint string                       `json:"manifest_fingerprint"`
	CallerRef           string                       `json:"caller_ref"`
	PurposeCode         string                       `json:"purpose_code"`
	Symbol              string                       `json:"symbol"`
	StartDate           string                       `json:"start_date"`
	EndDate             string                       `json:"end_date"`
	AdjustmentBasis     AdjustmentBasis              `json:"adjustment_basis"`
	SystemAsOf          string                       `json:"system_as_of"`
	Sources             []DailyBarsCloseSeriesSource `json:"sources"`
}

// DailyBarsCloseSeriesSnapshot 将 Manifest、Payload 和两者的独立指纹绑定为一个不可变输入。
type DailyBarsCloseSeriesSnapshot struct {
	Manifest           DailyBarsCloseSeriesManifest
	PayloadFingerprint string
	Payload            DailyBarsCloseSeriesPayload
}

type dailyBarsCloseSeriesManifestFingerprintInput struct {
	SchemaVersion   string                       `json:"schema_version"`
	CallerRef       string                       `json:"caller_ref"`
	PurposeCode     string                       `json:"purpose_code"`
	Symbol          string                       `json:"symbol"`
	StartDate       string                       `json:"start_date"`
	EndDate         string                       `json:"end_date"`
	AdjustmentBasis AdjustmentBasis              `json:"adjustment_basis"`
	SystemAsOf      string                       `json:"system_as_of"`
	Sources         []DailyBarsCloseSeriesSource `json:"sources"`
}

type dailyBarsCloseSeriesSnapshotRefInput struct {
	ManifestFingerprint string `json:"manifest_fingerprint"`
	PayloadFingerprint  string `json:"payload_fingerprint"`
}

// BuildDailyBarsCloseSeriesSnapshot 使用可信调用方和用途生成 Manifest、PayloadFingerprint 与组合 SnapshotRef。
// callerRef 必须来自 Server 注入的可信 Principal，不得从 HTTP 请求体或 API Key 推导。
func BuildDailyBarsCloseSeriesSnapshot(input DailyBarsCloseSeriesInput, callerRef, purposeCode string) (DailyBarsCloseSeriesSnapshot, error) {
	if !validDailyBarsCloseSeriesNonBlank(callerRef) {
		return DailyBarsCloseSeriesSnapshot{}, errors.New("daily-bars snapshot caller_ref must come from a trusted principal")
	}
	if !validDailyBarsCloseSeriesPurpose(purposeCode) {
		return DailyBarsCloseSeriesSnapshot{}, fmt.Errorf("unsupported daily-bars snapshot purpose_code %q", purposeCode)
	}
	if input.SystemAsOf.IsZero() {
		return DailyBarsCloseSeriesSnapshot{}, errors.New("daily-bars snapshot system_as_of is required")
	}
	if err := validateDailyBarsCloseSeriesPayload(input.Payload); err != nil {
		return DailyBarsCloseSeriesSnapshot{}, err
	}
	sources, err := canonicalDailyBarsCloseSeriesSources(input.Sources)
	if err != nil {
		return DailyBarsCloseSeriesSnapshot{}, err
	}
	if err := validateDailyBarsCloseSeriesSourceBinding(input.Payload, sources); err != nil {
		return DailyBarsCloseSeriesSnapshot{}, err
	}
	manifest := DailyBarsCloseSeriesManifest{
		SchemaVersion:   DailyBarsCloseSeriesManifestSchemaVersion,
		CallerRef:       callerRef,
		PurposeCode:     purposeCode,
		Symbol:          input.Payload.Symbol,
		StartDate:       input.Payload.StartDate,
		EndDate:         input.Payload.EndDate,
		AdjustmentBasis: input.Payload.AdjustmentBasis,
		SystemAsOf:      input.SystemAsOf.UTC().Format(time.RFC3339Nano),
		Sources:         sources,
	}
	payloadFingerprint, err := fingerprintDailyBarsCloseSeriesPayload(input.Payload)
	if err != nil {
		return DailyBarsCloseSeriesSnapshot{}, err
	}
	manifestFingerprint, err := calculateDailyBarsCloseSeriesManifestFingerprint(manifest)
	if err != nil {
		return DailyBarsCloseSeriesSnapshot{}, err
	}
	snapshotRef, err := calculateDailyBarsCloseSeriesSnapshotRef(manifestFingerprint, payloadFingerprint)
	if err != nil {
		return DailyBarsCloseSeriesSnapshot{}, err
	}
	manifest.ManifestFingerprint = manifestFingerprint
	manifest.SnapshotRef = snapshotRef
	result := DailyBarsCloseSeriesSnapshot{
		Manifest:           manifest,
		PayloadFingerprint:  payloadFingerprint,
		Payload:            cloneDailyBarsCloseSeriesPayload(input.Payload),
	}
	if err := result.Verify(); err != nil {
		return DailyBarsCloseSeriesSnapshot{}, err
	}
	return result, nil
}

// Verify 重新计算 PayloadFingerprint、ManifestFingerprint 和组合 SnapshotRef，拒绝任何内容漂移。
func (snapshot DailyBarsCloseSeriesSnapshot) Verify() error {
	manifest := snapshot.Manifest
	if manifest.SchemaVersion != DailyBarsCloseSeriesManifestSchemaVersion {
		return errors.New("daily-bars snapshot manifest schema_version mismatch")
	}
	if !validDailyBarsCloseSeriesNonBlank(manifest.CallerRef) || !validDailyBarsCloseSeriesPurpose(manifest.PurposeCode) {
		return errors.New("daily-bars snapshot manifest caller or purpose is invalid")
	}
	if !validCloseSeriesDate(manifest.StartDate) || !validCloseSeriesDate(manifest.EndDate) || manifest.StartDate > manifest.EndDate {
		return errors.New("daily-bars snapshot manifest date range is invalid")
	}
	if manifest.AdjustmentBasis != AdjustmentNone || !validDailyBarsCloseSeriesNonBlank(manifest.Symbol) {
		return errors.New("daily-bars snapshot manifest identity or adjustment basis is invalid")
	}
	if !validCanonicalCloseSeriesUTC(manifest.SystemAsOf) {
		return errors.New("daily-bars snapshot manifest system_as_of must be canonical UTC RFC3339Nano")
	}
	if err := validateDailyBarsCloseSeriesPayload(snapshot.Payload); err != nil {
		return err
	}
	if manifest.Symbol != snapshot.Payload.Symbol || manifest.StartDate != snapshot.Payload.StartDate ||
		manifest.EndDate != snapshot.Payload.EndDate || manifest.AdjustmentBasis != snapshot.Payload.AdjustmentBasis {
		return errors.New("daily-bars snapshot manifest and payload query identity mismatch")
	}
	if err := validateDailyBarsCloseSeriesSourceBinding(snapshot.Payload, manifest.Sources); err != nil {
		return err
	}
	parsedAsOf, err := time.Parse(time.RFC3339Nano, manifest.SystemAsOf)
	if err != nil {
		return errors.New("daily-bars snapshot manifest system_as_of is invalid")
	}
	if err := validateDailyBarsCloseSeriesAsOf(snapshot.Payload, parsedAsOf); err != nil {
		return err
	}
	canonicalSources, err := canonicalDailyBarsCloseSeriesSources(manifest.Sources)
	if err != nil {
		return err
	}
	if !equalDailyBarsCloseSeriesSources(canonicalSources, manifest.Sources) {
		return errors.New("daily-bars snapshot manifest sources must be canonically sorted")
	}
	payloadFingerprint, err := fingerprintDailyBarsCloseSeriesPayload(snapshot.Payload)
	if err != nil {
		return err
	}
	if !isDailyBarsCloseSeriesFingerprint(snapshot.PayloadFingerprint) || payloadFingerprint != snapshot.PayloadFingerprint {
		return errors.New("daily-bars snapshot payload fingerprint mismatch")
	}
	manifestFingerprint, err := calculateDailyBarsCloseSeriesManifestFingerprint(manifest)
	if err != nil {
		return err
	}
	if !isDailyBarsCloseSeriesFingerprint(manifest.ManifestFingerprint) || manifestFingerprint != manifest.ManifestFingerprint {
		return errors.New("daily-bars snapshot manifest fingerprint mismatch")
	}
	snapshotRef, err := calculateDailyBarsCloseSeriesSnapshotRef(manifestFingerprint, payloadFingerprint)
	if err != nil {
		return err
	}
	if manifest.SnapshotRef != snapshotRef {
		return errors.New("daily-bars snapshot_ref does not bind manifest and payload fingerprints")
	}
	return nil
}

func validateDailyBarsCloseSeriesPayload(payload DailyBarsCloseSeriesPayload) error {
	if payload.SchemaVersion != DailyBarsCloseSeriesPayloadSchemaVersion || payload.PayloadKind != DailyBarsCloseSeriesPayloadKind {
		return errors.New("daily-bars close series payload schema identity mismatch")
	}
	if !validDailyBarsCloseSeriesNonBlank(payload.Symbol) || payload.Interval != "1d" ||
		!validCloseSeriesDate(payload.StartDate) || !validCloseSeriesDate(payload.EndDate) || payload.StartDate > payload.EndDate ||
		payload.AdjustmentBasis != AdjustmentNone {
		return errors.New("daily-bars close series payload query identity is invalid")
	}
	if !validDailyBarsCloseSeriesNonBlank(payload.TradingCalendarRef) ||
		payload.CoverageEvidence.CalendarQualityStatus != QualityValid ||
		payload.CoverageEvidence.CoverageBasis != "exchange_calendar" ||
		!payload.CoverageEvidence.CoverageSufficient ||
		payload.CoverageEvidence.ExpectedTradingDays <= 0 ||
		payload.CoverageEvidence.MinimumRequiredBars <= 0 ||
		len(payload.Bars) < payload.CoverageEvidence.MinimumRequiredBars {
		return errors.New("daily-bars close series payload lacks valid calendar coverage")
	}
	if !validDailyBarsCloseSeriesQualityStatus(payload.QualityStatus) {
		return errors.New("daily-bars close series payload quality status is invalid")
	}
	if _, err := canonicalDailyBarsCloseSeriesStringSet(payload.Warnings); err != nil {
		return fmt.Errorf("daily-bars close series payload warnings: %w", err)
	}
	if _, err := canonicalDailyBarsCloseSeriesStringSet(payload.CoverageEvidence.Warnings); err != nil {
		return fmt.Errorf("daily-bars close series coverage warnings: %w", err)
	}
	previousDate := ""
	for index, bar := range payload.Bars {
		if !validCloseSeriesDate(bar.TradeDate) || bar.TradeDate < payload.StartDate || bar.TradeDate > payload.EndDate ||
			(previousDate != "" && bar.TradeDate <= previousDate) {
			return fmt.Errorf("daily-bars close series payload bar %d trade_date is invalid or unsorted", index)
		}
		previousDate = bar.TradeDate
		if bar.SourceRef != dailyBarsCloseSeriesSourceEastmoney || !validDailyBarsCloseSeriesNonBlank(bar.NormalizationVersion) {
			return fmt.Errorf("daily-bars close series payload bar %d source or normalization version is invalid", index)
		}
		if !validDailyBarsCloseSeriesQualityStatus(bar.QualityStatus) ||
			!validCanonicalCloseSeriesUTC(bar.FetchedAt) || !validCanonicalCloseSeriesUTC(bar.NormalizedAt) ||
			bar.SystemObservedAt == nil || !validCanonicalCloseSeriesUTC(*bar.SystemObservedAt) {
			return fmt.Errorf("daily-bars close series payload bar %d quality or PIT timestamp is invalid", index)
		}
		if bar.ProviderTime != nil && !validCanonicalCloseSeriesUTC(*bar.ProviderTime) {
			return fmt.Errorf("daily-bars close series payload bar %d provider_time is invalid", index)
		}
		if _, err := canonicalDailyBarsCloseSeriesStringSet(bar.QualityReasons); err != nil {
			return fmt.Errorf("daily-bars close series payload bar %d quality reasons: %w", index, err)
		}
		evidence := DailyBarDecimalEvidence{
			EvidenceKind:         bar.CloseEvidence.EvidenceKind,
			SourceToken:          bar.CloseEvidence.SourceToken,
			SourceFractionDigits: bar.CloseEvidence.SourceFractionDigits,
			ProtocolScale:        bar.CloseEvidence.ProtocolScale,
			UnitConversionFactor: bar.CloseEvidence.UnitConversionFactor,
		}
		if _, err := validateEastmoneyCloseEvidence(bar.Close, &evidence); err != nil {
			return fmt.Errorf("daily-bars close series payload bar %d: %w", index, err)
		}
	}
	return nil
}

func canonicalDailyBarsCloseSeriesSources(values []DailyBarsCloseSeriesSource) ([]DailyBarsCloseSeriesSource, error) {
	if len(values) == 0 {
		return nil, errors.New("daily-bars snapshot manifest requires at least one source")
	}
	sources := make([]DailyBarsCloseSeriesSource, len(values))
	for i, source := range values {
		if !validDailyBarsCloseSeriesNonBlank(source.SourceRef) {
			return nil, fmt.Errorf("daily-bars snapshot source %d has an invalid source_ref", i)
		}
		if source.SourceVersion != nil && !validDailyBarsCloseSeriesNonBlank(*source.SourceVersion) {
			return nil, fmt.Errorf("daily-bars snapshot source %d has an invalid source_version", i)
		}
		if source.ProviderInterfaceVersion != "" && !validDailyBarsCloseSeriesNonBlank(source.ProviderInterfaceVersion) {
			return nil, fmt.Errorf("daily-bars snapshot source %d has an invalid provider_interface_version", i)
		}
		sources[i] = DailyBarsCloseSeriesSource{
			SourceRef:                source.SourceRef,
			SourceVersion:            cloneDailyBarsCloseSeriesString(source.SourceVersion),
			ProviderInterfaceVersion: source.ProviderInterfaceVersion,
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].SourceRef != sources[j].SourceRef {
			return sources[i].SourceRef < sources[j].SourceRef
		}
		if sources[i].ProviderInterfaceVersion != sources[j].ProviderInterfaceVersion {
			return sources[i].ProviderInterfaceVersion < sources[j].ProviderInterfaceVersion
		}
		if sources[i].SourceVersion == nil {
			return sources[j].SourceVersion != nil
		}
		if sources[j].SourceVersion == nil {
			return false
		}
		return *sources[i].SourceVersion < *sources[j].SourceVersion
	})
	for i := 1; i < len(sources); i++ {
		if sources[i].SourceRef == sources[i-1].SourceRef {
			return nil, errors.New("daily-bars snapshot sources must have one entry per source_ref")
		}
	}
	return sources, nil
}

func validateDailyBarsCloseSeriesSourceBinding(payload DailyBarsCloseSeriesPayload, sources []DailyBarsCloseSeriesSource) error {
	if len(sources) != 1 || sources[0].SourceRef != dailyBarsCloseSeriesSourceEastmoney ||
		sources[0].ProviderInterfaceVersion != dailyBarsCloseSeriesEastmoneyAPIVersion || sources[0].SourceVersion != nil {
		return errors.New("first daily-bars close series snapshot requires the verified Eastmoney interface and explicit missing Provider data version")
	}
	for index, bar := range payload.Bars {
		if bar.SourceRef != sources[0].SourceRef {
			return fmt.Errorf("daily-bars close series payload bar %d source_ref is not bound by Manifest", index)
		}
	}
	return nil
}

func calculateDailyBarsCloseSeriesManifestFingerprint(manifest DailyBarsCloseSeriesManifest) (string, error) {
	input := dailyBarsCloseSeriesManifestFingerprintInput{
		SchemaVersion:   manifest.SchemaVersion,
		CallerRef:       manifest.CallerRef,
		PurposeCode:     manifest.PurposeCode,
		Symbol:          manifest.Symbol,
		StartDate:       manifest.StartDate,
		EndDate:         manifest.EndDate,
		AdjustmentBasis: manifest.AdjustmentBasis,
		SystemAsOf:      manifest.SystemAsOf,
		Sources:         manifest.Sources,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode daily-bars manifest fingerprint input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize daily-bars manifest fingerprint input: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func fingerprintDailyBarsCloseSeriesPayload(payload DailyBarsCloseSeriesPayload) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode daily-bars close series payload: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize daily-bars close series payload: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func calculateDailyBarsCloseSeriesSnapshotRef(manifestFingerprint, payloadFingerprint string) (string, error) {
	if !isDailyBarsCloseSeriesFingerprint(manifestFingerprint) || !isDailyBarsCloseSeriesFingerprint(payloadFingerprint) {
		return "", errors.New("daily-bars snapshot fingerprints must be lowercase SHA-256 hex")
	}
	encoded, err := json.Marshal(dailyBarsCloseSeriesSnapshotRefInput{
		ManifestFingerprint: manifestFingerprint,
		PayloadFingerprint: payloadFingerprint,
	})
	if err != nil {
		return "", fmt.Errorf("encode daily-bars snapshot reference input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize daily-bars snapshot reference input: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return dailyBarsCloseSeriesSnapshotRefPrefix + hex.EncodeToString(sum[:]), nil
}

func validateDailyBarsCloseSeriesAsOf(payload DailyBarsCloseSeriesPayload, asOf time.Time) error {
	for index, bar := range payload.Bars {
		for _, value := range []string{bar.FetchedAt, bar.NormalizedAt, *bar.SystemObservedAt} {
			observedAt, err := time.Parse(time.RFC3339Nano, value)
			if err != nil || observedAt.After(asOf) {
				return fmt.Errorf("daily-bars snapshot payload bar %d contains data newer than system_as_of", index)
			}
		}
	}
	return nil
}

func cloneDailyBarsCloseSeriesPayload(payload DailyBarsCloseSeriesPayload) DailyBarsCloseSeriesPayload {
	cloned := payload
	cloned.Warnings = append([]string{}, payload.Warnings...)
	cloned.CoverageEvidence.Warnings = append([]string{}, payload.CoverageEvidence.Warnings...)
	cloned.Bars = make([]DailyBarsCloseSeriesBar, len(payload.Bars))
	for index, bar := range payload.Bars {
		cloned.Bars[index] = bar
		cloned.Bars[index].CloseEvidence.SourceFractionDigits = cloneDailyBarsCloseSeriesInt(bar.CloseEvidence.SourceFractionDigits)
		cloned.Bars[index].CloseEvidence.ProtocolScale = cloneDailyBarsCloseSeriesInt(bar.CloseEvidence.ProtocolScale)
		cloned.Bars[index].ProviderTime = cloneDailyBarsCloseSeriesString(bar.ProviderTime)
		cloned.Bars[index].SystemObservedAt = cloneDailyBarsCloseSeriesString(bar.SystemObservedAt)
		cloned.Bars[index].QualityReasons = append([]string{}, bar.QualityReasons...)
	}
	return cloned
}

func equalDailyBarsCloseSeriesSources(left, right []DailyBarsCloseSeriesSource) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].SourceRef != right[index].SourceRef ||
			left[index].ProviderInterfaceVersion != right[index].ProviderInterfaceVersion {
			return false
		}
		if left[index].SourceVersion == nil || right[index].SourceVersion == nil {
			if left[index].SourceVersion != nil || right[index].SourceVersion != nil {
				return false
			}
			continue
		}
		if *left[index].SourceVersion != *right[index].SourceVersion {
			return false
		}
	}
	return true
}

func validDailyBarsCloseSeriesPurpose(value string) bool {
	switch value {
	case "quant_analysis", "research_analysis", "risk_review":
		return true
	default:
		return false
	}
}

func validDailyBarsCloseSeriesNonBlank(value string) bool {
	return utf8.ValidString(value) && value != "" && strings.TrimSpace(value) == value
}

func validCanonicalCloseSeriesUTC(value string) bool {
	if !strings.HasSuffix(value, "Z") {
		return false
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.UTC().Format(time.RFC3339Nano) == value
}

func isDailyBarsCloseSeriesFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, char := range value {
		if !(char >= '0' && char <= '9') && !(char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

func cloneDailyBarsCloseSeriesString(value *string) *string {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
