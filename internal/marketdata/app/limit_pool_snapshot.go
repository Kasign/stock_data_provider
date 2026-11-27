package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gowebpki/jcs"
)

const LimitPoolFactsManifestSchemaVersion = "1.0.0"

// LimitPoolFactsManifest 是涨跌停事实授权快照的清单。
// trade_date 是该池所属交易日，observed_at 是池数据的观察时刻（收盘后），
// system_as_of 是可信评估截止时刻；三者都不互相替代。
type LimitPoolFactsManifest struct {
	SchemaVersion       string                       `json:"schema_version"`
	SnapshotRef         string                       `json:"snapshot_ref"`
	ManifestFingerprint string                       `json:"manifest_fingerprint"`
	CallerRef           string                       `json:"caller_ref"`
	PurposeCode         string                       `json:"purpose_code"`
	Symbol              string                       `json:"symbol"`
	TradeDate           string                       `json:"trade_date"`
	ObservedAt          string                       `json:"observed_at"`
	SystemAsOf          string                       `json:"system_as_of"`
	Sources             []DailyBarsCloseSeriesSource `json:"sources"`
}

// LimitPoolFactsSnapshot 绑定 Manifest、Payload 与各自指纹。
type LimitPoolFactsSnapshot struct {
	Manifest           LimitPoolFactsManifest
	PayloadFingerprint string
	Payload            LimitPoolFactsPayload
}

type limitPoolFactsManifestFingerprintInput struct {
	SchemaVersion string                       `json:"schema_version"`
	CallerRef     string                       `json:"caller_ref"`
	PurposeCode   string                       `json:"purpose_code"`
	Symbol        string                       `json:"symbol"`
	TradeDate     string                       `json:"trade_date"`
	ObservedAt    string                       `json:"observed_at"`
	SystemAsOf    string                       `json:"system_as_of"`
	Sources       []DailyBarsCloseSeriesSource `json:"sources"`
}

type limitPoolFactsSnapshotRefInput struct {
	ManifestFingerprint string `json:"manifest_fingerprint"`
	PayloadFingerprint  string `json:"payload_fingerprint"`
}

// BuildLimitPoolFactsSnapshot 生成 Manifest、PayloadFingerprint 与组合 SnapshotRef。
func BuildLimitPoolFactsSnapshot(
	payload LimitPoolFactsPayload,
	sources []DailyBarsCloseSeriesSource,
	callerRef, purposeCode string,
	systemAsOf time.Time,
) (LimitPoolFactsSnapshot, error) {
	if err := payload.Validate(); err != nil {
		return LimitPoolFactsSnapshot{}, err
	}
	if !validDailyBarsCloseSeriesNonBlank(callerRef) {
		return LimitPoolFactsSnapshot{}, errors.New("limit-pool facts caller_ref must come from a trusted principal")
	}
	if !validDailyBarsCloseSeriesPurpose(purposeCode) {
		return LimitPoolFactsSnapshot{}, fmt.Errorf("unsupported limit-pool facts purpose_code %q", purposeCode)
	}
	if systemAsOf.IsZero() || systemAsOf.Location() != time.UTC {
		return LimitPoolFactsSnapshot{}, errors.New("limit-pool facts system_as_of must be a UTC time")
	}
	if systemAsOf.Before(payload.SystemObservedAt) {
		return LimitPoolFactsSnapshot{}, errors.New("limit-pool facts system_as_of precedes the source observation")
	}
	canonicalSources, err := canonicalLimitPoolFactsSources(sources)
	if err != nil {
		return LimitPoolFactsSnapshot{}, err
	}
	manifest := LimitPoolFactsManifest{
		SchemaVersion: LimitPoolFactsManifestSchemaVersion,
		CallerRef:     callerRef, PurposeCode: purposeCode,
		Symbol: payload.Symbol, TradeDate: payload.TradeDate,
		ObservedAt: payload.ObservedAt.UTC().Format(time.RFC3339Nano),
		SystemAsOf: systemAsOf.UTC().Format(time.RFC3339Nano),
		Sources:    canonicalSources,
	}
	payloadFingerprint, err := fingerprintLimitPoolFactsPayload(payload)
	if err != nil {
		return LimitPoolFactsSnapshot{}, err
	}
	manifestFingerprint, err := fingerprintLimitPoolFactsManifest(manifest)
	if err != nil {
		return LimitPoolFactsSnapshot{}, err
	}
	snapshotRef, err := fingerprintLimitPoolFactsSnapshotRef(manifestFingerprint, payloadFingerprint)
	if err != nil {
		return LimitPoolFactsSnapshot{}, err
	}
	manifest.ManifestFingerprint = manifestFingerprint
	manifest.SnapshotRef = snapshotRef
	snapshot := LimitPoolFactsSnapshot{
		Manifest: manifest, PayloadFingerprint: payloadFingerprint, Payload: payload,
	}
	if err := snapshot.Verify(); err != nil {
		return LimitPoolFactsSnapshot{}, err
	}
	return snapshot, nil
}

// Verify 重新计算三个指纹并校验身份、交易日与时间绑定。
func (snapshot LimitPoolFactsSnapshot) Verify() error {
	manifest := snapshot.Manifest
	if manifest.SchemaVersion != LimitPoolFactsManifestSchemaVersion {
		return errors.New("limit-pool facts manifest schema_version mismatch")
	}
	if !validDailyBarsCloseSeriesNonBlank(manifest.CallerRef) || !validDailyBarsCloseSeriesPurpose(manifest.PurposeCode) {
		return errors.New("limit-pool facts manifest caller or purpose is invalid")
	}
	if err := snapshot.Payload.Validate(); err != nil {
		return err
	}
	manifestObservedAt, err := time.Parse(time.RFC3339Nano, manifest.ObservedAt)
	if err != nil || !manifestObservedAt.Equal(snapshot.Payload.ObservedAt) {
		return errors.New("limit-pool facts manifest observed_at does not match payload")
	}
	manifestSystemAsOf, err := time.Parse(time.RFC3339Nano, manifest.SystemAsOf)
	if err != nil || manifestSystemAsOf.Before(snapshot.Payload.SystemObservedAt) {
		return errors.New("limit-pool facts manifest system_as_of is invalid")
	}
	if manifest.Symbol != snapshot.Payload.Symbol || manifest.TradeDate != snapshot.Payload.TradeDate {
		return errors.New("limit-pool facts manifest identity does not match payload")
	}
	if !validDailyBarsCloseSeriesNonBlank(manifest.SnapshotRef) ||
		!validDailyBarsCloseSeriesNonBlank(manifest.ManifestFingerprint) ||
		!validDailyBarsCloseSeriesNonBlank(snapshot.PayloadFingerprint) {
		return errors.New("limit-pool facts snapshot fingerprints are required")
	}
	payloadFingerprint, err := fingerprintLimitPoolFactsPayload(snapshot.Payload)
	if err != nil {
		return err
	}
	if payloadFingerprint != snapshot.PayloadFingerprint {
		return errors.New("limit-pool facts payload fingerprint mismatch")
	}
	manifestFingerprint, err := fingerprintLimitPoolFactsManifest(manifest)
	if err != nil {
		return err
	}
	if manifestFingerprint != manifest.ManifestFingerprint {
		return errors.New("limit-pool facts manifest fingerprint mismatch")
	}
	snapshotRef, err := fingerprintLimitPoolFactsSnapshotRef(manifestFingerprint, payloadFingerprint)
	if err != nil {
		return err
	}
	if snapshotRef != manifest.SnapshotRef {
		return errors.New("limit-pool facts snapshot_ref mismatch")
	}
	return nil
}

func fingerprintLimitPoolFactsPayload(payload LimitPoolFactsPayload) (string, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode limit-pool facts payload: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize limit-pool facts payload: %w", err)
	}
	return sha256Hex(canonical), nil
}

func fingerprintLimitPoolFactsManifest(manifest LimitPoolFactsManifest) (string, error) {
	input := limitPoolFactsManifestFingerprintInput{
		SchemaVersion: manifest.SchemaVersion,
		CallerRef:     manifest.CallerRef, PurposeCode: manifest.PurposeCode,
		Symbol: manifest.Symbol, TradeDate: manifest.TradeDate,
		ObservedAt: manifest.ObservedAt, SystemAsOf: manifest.SystemAsOf,
		Sources: manifest.Sources,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode limit-pool facts manifest fingerprint input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize limit-pool facts manifest: %w", err)
	}
	return sha256Hex(canonical), nil
}

func fingerprintLimitPoolFactsSnapshotRef(manifestFingerprint, payloadFingerprint string) (string, error) {
	encoded, err := json.Marshal(limitPoolFactsSnapshotRefInput{
		ManifestFingerprint: manifestFingerprint, PayloadFingerprint: payloadFingerprint,
	})
	if err != nil {
		return "", fmt.Errorf("encode limit-pool facts snapshot ref input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize limit-pool facts snapshot ref: %w", err)
	}
	return dailyTradingStatusFactsSnapshotRefPrefix + sha256Hex(canonical), nil
}

func canonicalLimitPoolFactsSources(sources []DailyBarsCloseSeriesSource) ([]DailyBarsCloseSeriesSource, error) {
	if len(sources) == 0 {
		return nil, errors.New("limit-pool facts snapshot requires at least one source")
	}
	seen := make(map[string]struct{}, len(sources))
	out := make([]DailyBarsCloseSeriesSource, 0, len(sources))
	for _, source := range sources {
		if !validDailyBarsCloseSeriesNonBlank(source.SourceRef) {
			return nil, errors.New("limit-pool facts snapshot source_ref is required")
		}
		if source.SourceVersion != nil && !validDailyBarsCloseSeriesNonBlank(*source.SourceVersion) {
			return nil, errors.New("limit-pool facts snapshot source_version must be non-blank when present")
		}
		if _, exists := seen[source.SourceRef]; exists {
			return nil, fmt.Errorf("duplicate limit-pool facts source %q", source.SourceRef)
		}
		seen[source.SourceRef] = struct{}{}
		out = append(out, source)
	}
	return out, nil
}
