package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gowebpki/jcs"
)

const OrderBookFactsManifestSchemaVersion = "1.0.0"

// OrderBookFactsManifest 是日内盘口授权快照的清单。
// observed_at 绑定来源时刻，system_as_of 绑定可信评估截止时刻，二者都不用交易日替代。
type OrderBookFactsManifest struct {
	SchemaVersion       string                       `json:"schema_version"`
	SnapshotRef         string                       `json:"snapshot_ref"`
	ManifestFingerprint string                       `json:"manifest_fingerprint"`
	CallerRef           string                       `json:"caller_ref"`
	PurposeCode         string                       `json:"purpose_code"`
	Symbol              string                       `json:"symbol"`
	ObservedAt          string                       `json:"observed_at"`
	SystemAsOf          string                       `json:"system_as_of"`
	Sources             []DailyBarsCloseSeriesSource `json:"sources"`
}

// OrderBookFactsSnapshot 绑定 Manifest、Payload 与各自指纹。
type OrderBookFactsSnapshot struct {
	Manifest           OrderBookFactsManifest
	PayloadFingerprint string
	Payload            OrderBookFactsPayload
}

type orderBookFactsManifestFingerprintInput struct {
	SchemaVersion string                       `json:"schema_version"`
	CallerRef     string                       `json:"caller_ref"`
	PurposeCode   string                       `json:"purpose_code"`
	Symbol        string                       `json:"symbol"`
	ObservedAt    string                       `json:"observed_at"`
	SystemAsOf    string                       `json:"system_as_of"`
	Sources       []DailyBarsCloseSeriesSource `json:"sources"`
}

type orderBookFactsSnapshotRefInput struct {
	ManifestFingerprint string `json:"manifest_fingerprint"`
	PayloadFingerprint  string `json:"payload_fingerprint"`
}

// BuildOrderBookFactsSnapshot 生成 Manifest、PayloadFingerprint 与组合 SnapshotRef。
// callerRef 必须来自 Server 注入的可信 Principal，systemAsOf 必须是 UTC 且不早于
// 来源系统观察时间，否则会把评估时刻之后才出现的盘口伪装成已可用事实。
func BuildOrderBookFactsSnapshot(
	payload OrderBookFactsPayload,
	sources []DailyBarsCloseSeriesSource,
	callerRef, purposeCode string,
	systemAsOf time.Time,
) (OrderBookFactsSnapshot, error) {
	if err := payload.Validate(); err != nil {
		return OrderBookFactsSnapshot{}, err
	}
	if !validDailyBarsCloseSeriesNonBlank(callerRef) {
		return OrderBookFactsSnapshot{}, errors.New("order-book facts caller_ref must come from a trusted principal")
	}
	if !validDailyBarsCloseSeriesPurpose(purposeCode) {
		return OrderBookFactsSnapshot{}, fmt.Errorf("unsupported order-book facts purpose_code %q", purposeCode)
	}
	if systemAsOf.IsZero() || systemAsOf.Location() != time.UTC {
		return OrderBookFactsSnapshot{}, errors.New("order-book facts system_as_of must be a UTC time")
	}
	if systemAsOf.Before(payload.SystemObservedAt) {
		return OrderBookFactsSnapshot{}, errors.New("order-book facts system_as_of precedes the source observation")
	}
	canonicalSources, err := canonicalOrderBookFactsSources(sources)
	if err != nil {
		return OrderBookFactsSnapshot{}, err
	}
	manifest := OrderBookFactsManifest{
		SchemaVersion: OrderBookFactsManifestSchemaVersion,
		CallerRef:     callerRef,
		PurposeCode:   purposeCode,
		Symbol:        payload.Symbol,
		ObservedAt:    payload.ObservedAt.UTC().Format(time.RFC3339Nano),
		SystemAsOf:    systemAsOf.UTC().Format(time.RFC3339Nano),
		Sources:       canonicalSources,
	}
	payloadFingerprint, err := fingerprintOrderBookFactsPayload(payload)
	if err != nil {
		return OrderBookFactsSnapshot{}, err
	}
	manifestFingerprint, err := fingerprintOrderBookFactsManifest(manifest)
	if err != nil {
		return OrderBookFactsSnapshot{}, err
	}
	snapshotRef, err := fingerprintOrderBookFactsSnapshotRef(manifestFingerprint, payloadFingerprint)
	if err != nil {
		return OrderBookFactsSnapshot{}, err
	}
	manifest.ManifestFingerprint = manifestFingerprint
	manifest.SnapshotRef = snapshotRef
	snapshot := OrderBookFactsSnapshot{
		Manifest: manifest, PayloadFingerprint: payloadFingerprint, Payload: payload,
	}
	if err := snapshot.Verify(); err != nil {
		return OrderBookFactsSnapshot{}, err
	}
	return snapshot, nil
}

// Verify 重新计算三个指纹并校验 Manifest 与 Payload 的身份和时间绑定。
func (snapshot OrderBookFactsSnapshot) Verify() error {
	manifest := snapshot.Manifest
	if manifest.SchemaVersion != OrderBookFactsManifestSchemaVersion {
		return errors.New("order-book facts manifest schema_version mismatch")
	}
	if !validDailyBarsCloseSeriesNonBlank(manifest.CallerRef) || !validDailyBarsCloseSeriesPurpose(manifest.PurposeCode) {
		return errors.New("order-book facts manifest caller or purpose is invalid")
	}
	if err := snapshot.Payload.Validate(); err != nil {
		return err
	}
	manifestObservedAt, err := time.Parse(time.RFC3339Nano, manifest.ObservedAt)
	if err != nil || !manifestObservedAt.Equal(snapshot.Payload.ObservedAt) {
		return errors.New("order-book facts manifest observed_at does not match payload")
	}
	manifestSystemAsOf, err := time.Parse(time.RFC3339Nano, manifest.SystemAsOf)
	if err != nil || manifestSystemAsOf.Before(snapshot.Payload.SystemObservedAt) {
		return errors.New("order-book facts manifest system_as_of is invalid")
	}
	if manifest.Symbol != snapshot.Payload.Symbol {
		return errors.New("order-book facts manifest identity does not match payload")
	}
	if !validDailyBarsCloseSeriesNonBlank(manifest.SnapshotRef) ||
		!validDailyBarsCloseSeriesNonBlank(manifest.ManifestFingerprint) ||
		!validDailyBarsCloseSeriesNonBlank(snapshot.PayloadFingerprint) {
		return errors.New("order-book facts snapshot fingerprints are required")
	}
	payloadFingerprint, err := fingerprintOrderBookFactsPayload(snapshot.Payload)
	if err != nil {
		return err
	}
	if payloadFingerprint != snapshot.PayloadFingerprint {
		return errors.New("order-book facts payload fingerprint mismatch")
	}
	manifestFingerprint, err := fingerprintOrderBookFactsManifest(manifest)
	if err != nil {
		return err
	}
	if manifestFingerprint != manifest.ManifestFingerprint {
		return errors.New("order-book facts manifest fingerprint mismatch")
	}
	snapshotRef, err := fingerprintOrderBookFactsSnapshotRef(manifestFingerprint, payloadFingerprint)
	if err != nil {
		return err
	}
	if snapshotRef != manifest.SnapshotRef {
		return errors.New("order-book facts snapshot_ref mismatch")
	}
	return nil
}

func fingerprintOrderBookFactsPayload(payload OrderBookFactsPayload) (string, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode order-book facts payload: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize order-book facts payload: %w", err)
	}
	return sha256Hex(canonical), nil
}

func fingerprintOrderBookFactsManifest(manifest OrderBookFactsManifest) (string, error) {
	input := orderBookFactsManifestFingerprintInput{
		SchemaVersion: manifest.SchemaVersion,
		CallerRef:     manifest.CallerRef,
		PurposeCode:   manifest.PurposeCode,
		Symbol:        manifest.Symbol,
		ObservedAt:    manifest.ObservedAt,
		SystemAsOf:    manifest.SystemAsOf,
		Sources:       manifest.Sources,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode order-book facts manifest fingerprint input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize order-book facts manifest: %w", err)
	}
	return sha256Hex(canonical), nil
}

func fingerprintOrderBookFactsSnapshotRef(manifestFingerprint, payloadFingerprint string) (string, error) {
	encoded, err := json.Marshal(orderBookFactsSnapshotRefInput{
		ManifestFingerprint: manifestFingerprint, PayloadFingerprint: payloadFingerprint,
	})
	if err != nil {
		return "", fmt.Errorf("encode order-book facts snapshot ref input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize order-book facts snapshot ref: %w", err)
	}
	return dailyTradingStatusFactsSnapshotRefPrefix + sha256Hex(canonical), nil
}

func canonicalOrderBookFactsSources(sources []DailyBarsCloseSeriesSource) ([]DailyBarsCloseSeriesSource, error) {
	if len(sources) == 0 {
		return nil, errors.New("order-book facts snapshot requires at least one source")
	}
	seen := make(map[string]struct{}, len(sources))
	out := make([]DailyBarsCloseSeriesSource, 0, len(sources))
	for _, source := range sources {
		if !validDailyBarsCloseSeriesNonBlank(source.SourceRef) {
			return nil, errors.New("order-book facts snapshot source_ref is required")
		}
		if source.SourceVersion != nil && !validDailyBarsCloseSeriesNonBlank(*source.SourceVersion) {
			return nil, errors.New("order-book facts snapshot source_version must be non-blank when present")
		}
		if _, exists := seen[source.SourceRef]; exists {
			return nil, fmt.Errorf("duplicate order-book facts source %q", source.SourceRef)
		}
		seen[source.SourceRef] = struct{}{}
		out = append(out, source)
	}
	return out, nil
}
