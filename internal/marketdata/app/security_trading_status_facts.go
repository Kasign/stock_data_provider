package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/gowebpki/jcs"
)

// 交易状态事实授权快照（执行清单 1.4 前置②，DEC-PLAT-002-060 草案）。
//
// 口径（2026-11-26 决议）：只承载 Data 真实拥有的事实，逐项表达 available/unavailable。
// 不包含涨跌停状态与可成交量——Provider 从未提供这两类事实，Quant 侧也没有
// 权威的涨跌幅制度表或流动性模型；把它们写成"推导值"会把猜测伪装成事实。
// Quant 的 tradability 门禁据此把任一项 unavailable 视为不可验证并阻断。

const (
	SecurityTradingStatusFactsPayloadKind           = "security_trading_status_facts"
	SecurityTradingStatusFactsPayloadSchemaVersion  = "1.0.0"
	SecurityTradingStatusFactsManifestSchemaVersion = "1.0.0"
	dailyTradingStatusFactsSnapshotRefPrefix        = "data_snapshot:sha256:"
)

var (
	ErrInvalidSecurityTradingStatusFacts = errors.New("invalid security trading status facts")
)

// TradingFactKey 是可交易性事实的封闭枚举。
// 新增事实必须同时更新 tradingFactOrder、校验映射与 Quant 侧门禁，否则拒绝。
type TradingFactKey string

const (
	TradingFactRiskWarning TradingFactKey = "risk_warning_status"
	TradingFactSuspension  TradingFactKey = "suspension_status"
	TradingFactDelisting   TradingFactKey = "delisting_status"
	TradingFactListingDate TradingFactKey = "listing_date"
	TradingFactTradingDay  TradingFactKey = "is_trading_day"
)

// tradingFactOrder 固定事实顺序，使 Golden 与指纹稳定，不依赖 map 迭代顺序。
var tradingFactOrder = []TradingFactKey{
	TradingFactRiskWarning,
	TradingFactSuspension,
	TradingFactDelisting,
	TradingFactListingDate,
	TradingFactTradingDay,
}

// 事实可用性是两值枚举：没有"部分可用"。部分可用的表达方式是拆成更细的事实项。
const (
	TradingFactAvailable   = "available"
	TradingFactUnavailable = "unavailable"
)

// SecurityTradingStatusFact 是单项事实。
// available 必须有 value 且不得带原因；unavailable 必须带原因且 value 为空。
type SecurityTradingStatusFact struct {
	Key               string `json:"key"`
	Availability      string `json:"availability"`
	Value             string `json:"value,omitempty"`
	UnavailableReason string `json:"unavailable_reason,omitempty"`
}

// SecurityTradingStatusFactsPayload 是 1.0.0 Payload。
// QualityStatus 与事实可用性严格对应：全部 available → valid，任一 unavailable → degraded。
type SecurityTradingStatusFactsPayload struct {
	SchemaVersion string                      `json:"schema_version"`
	PayloadKind   string                      `json:"payload_kind"`
	Symbol        string                      `json:"symbol"`
	TradeDate     string                      `json:"trade_date"`
	QualityStatus QualityStatus               `json:"quality_status"`
	Facts         []SecurityTradingStatusFact `json:"facts"`
}

// Validate 校验身份、枚举、顺序与可用性一致性。
func (payload SecurityTradingStatusFactsPayload) Validate() error {
	if payload.SchemaVersion != SecurityTradingStatusFactsPayloadSchemaVersion ||
		payload.PayloadKind != SecurityTradingStatusFactsPayloadKind {
		return fmt.Errorf("%w: schema_version or payload_kind mismatch", ErrInvalidSecurityTradingStatusFacts)
	}
	if !validDailyBarsCloseSeriesNonBlank(payload.Symbol) || !validCloseSeriesDate(payload.TradeDate) {
		return fmt.Errorf("%w: symbol or trade_date is invalid", ErrInvalidSecurityTradingStatusFacts)
	}
	if len(payload.Facts) != len(tradingFactOrder) {
		return fmt.Errorf("%w: expected %d facts, got %d", ErrInvalidSecurityTradingStatusFacts, len(tradingFactOrder), len(payload.Facts))
	}
	hasUnavailable := false
	for index, expected := range tradingFactOrder {
		fact := payload.Facts[index]
		if fact.Key != string(expected) {
			return fmt.Errorf("%w: fact %d key = %q, want %q", ErrInvalidSecurityTradingStatusFacts, index, fact.Key, expected)
		}
		switch fact.Availability {
		case TradingFactAvailable:
			if !validDailyBarsCloseSeriesNonBlank(fact.Value) || fact.UnavailableReason != "" {
				return fmt.Errorf("%w: available fact %q must carry value and no reason", ErrInvalidSecurityTradingStatusFacts, fact.Key)
			}
		case TradingFactUnavailable:
			hasUnavailable = true
			if fact.Value != "" || !validDailyBarsCloseSeriesNonBlank(fact.UnavailableReason) {
				return fmt.Errorf("%w: unavailable fact %q must carry reason and no value", ErrInvalidSecurityTradingStatusFacts, fact.Key)
			}
		default:
			return fmt.Errorf("%w: fact %q has unknown availability %q", ErrInvalidSecurityTradingStatusFacts, fact.Key, fact.Availability)
		}
	}
	expectedQuality := QualityValid
	if hasUnavailable {
		expectedQuality = QualityDegraded
	}
	if payload.QualityStatus != expectedQuality {
		return fmt.Errorf("%w: quality_status = %q, want %q", ErrInvalidSecurityTradingStatusFacts, payload.QualityStatus, expectedQuality)
	}
	return nil
}

// SecurityTradingStatusFactsManifest 是交易状态事实快照的清单。
// 与 close series Manifest 同族但字段不同：没有区间与复权基准确认，只有单证券单日。
type SecurityTradingStatusFactsManifest struct {
	SchemaVersion       string                       `json:"schema_version"`
	SnapshotRef         string                       `json:"snapshot_ref"`
	ManifestFingerprint string                       `json:"manifest_fingerprint"`
	CallerRef           string                       `json:"caller_ref"`
	PurposeCode         string                       `json:"purpose_code"`
	Symbol              string                       `json:"symbol"`
	TradeDate           string                       `json:"trade_date"`
	SystemAsOf          string                       `json:"system_as_of"`
	Sources             []DailyBarsCloseSeriesSource `json:"sources"`
}

// SecurityTradingStatusFactsSnapshot 绑定 Manifest、Payload 与各自指纹。
type SecurityTradingStatusFactsSnapshot struct {
	Manifest           SecurityTradingStatusFactsManifest
	PayloadFingerprint string
	Payload            SecurityTradingStatusFactsPayload
}

type tradingStatusFactsManifestFingerprintInput struct {
	SchemaVersion string                       `json:"schema_version"`
	CallerRef     string                       `json:"caller_ref"`
	PurposeCode   string                       `json:"purpose_code"`
	Symbol        string                       `json:"symbol"`
	TradeDate     string                       `json:"trade_date"`
	SystemAsOf    string                       `json:"system_as_of"`
	Sources       []DailyBarsCloseSeriesSource `json:"sources"`
}

type tradingStatusFactsSnapshotRefInput struct {
	ManifestFingerprint string `json:"manifest_fingerprint"`
	PayloadFingerprint  string `json:"payload_fingerprint"`
}

// BuildSecurityTradingStatusFactsSnapshot 生成 Manifest、PayloadFingerprint 与组合 SnapshotRef。
// callerRef 必须来自 Server 注入的可信 Principal，不得从请求体推导。
func BuildSecurityTradingStatusFactsSnapshot(
	payload SecurityTradingStatusFactsPayload,
	sources []DailyBarsCloseSeriesSource,
	callerRef, purposeCode string,
	systemAsOf time.Time,
) (SecurityTradingStatusFactsSnapshot, error) {
	if err := payload.Validate(); err != nil {
		return SecurityTradingStatusFactsSnapshot{}, err
	}
	if !validDailyBarsCloseSeriesNonBlank(callerRef) {
		return SecurityTradingStatusFactsSnapshot{}, errors.New("trading-status facts caller_ref must come from a trusted principal")
	}
	if !validDailyBarsCloseSeriesPurpose(purposeCode) {
		return SecurityTradingStatusFactsSnapshot{}, fmt.Errorf("unsupported trading-status facts purpose_code %q", purposeCode)
	}
	if systemAsOf.IsZero() || systemAsOf.Location() != time.UTC {
		return SecurityTradingStatusFactsSnapshot{}, errors.New("trading-status facts system_as_of must be a UTC time")
	}
	canonicalSources, err := canonicalTradingStatusFactsSources(sources)
	if err != nil {
		return SecurityTradingStatusFactsSnapshot{}, err
	}

	manifest := SecurityTradingStatusFactsManifest{
		SchemaVersion: SecurityTradingStatusFactsManifestSchemaVersion,
		CallerRef:     callerRef,
		PurposeCode:   purposeCode,
		Symbol:        payload.Symbol,
		TradeDate:     payload.TradeDate,
		SystemAsOf:    systemAsOf.UTC().Format(time.RFC3339Nano),
		Sources:       canonicalSources,
	}
	payloadFingerprint, err := fingerprintTradingStatusFactsPayload(payload)
	if err != nil {
		return SecurityTradingStatusFactsSnapshot{}, err
	}
	manifestFingerprint, err := fingerprintTradingStatusFactsManifest(manifest)
	if err != nil {
		return SecurityTradingStatusFactsSnapshot{}, err
	}
	snapshotRef, err := fingerprintTradingStatusFactsSnapshotRef(manifestFingerprint, payloadFingerprint)
	if err != nil {
		return SecurityTradingStatusFactsSnapshot{}, err
	}
	manifest.ManifestFingerprint = manifestFingerprint
	manifest.SnapshotRef = snapshotRef

	snapshot := SecurityTradingStatusFactsSnapshot{
		Manifest:           manifest,
		PayloadFingerprint: payloadFingerprint,
		Payload:            cloneTradingStatusFactsPayload(payload),
	}
	if err := snapshot.Verify(); err != nil {
		return SecurityTradingStatusFactsSnapshot{}, err
	}
	return snapshot, nil
}

// Verify 重新计算三个指纹并校验 Manifest 与 Payload 的身份绑定。
func (snapshot SecurityTradingStatusFactsSnapshot) Verify() error {
	manifest := snapshot.Manifest
	if manifest.SchemaVersion != SecurityTradingStatusFactsManifestSchemaVersion {
		return errors.New("trading-status facts manifest schema_version mismatch")
	}
	if !validDailyBarsCloseSeriesNonBlank(manifest.CallerRef) || !validDailyBarsCloseSeriesPurpose(manifest.PurposeCode) {
		return errors.New("trading-status facts manifest caller or purpose is invalid")
	}
	if err := snapshot.Payload.Validate(); err != nil {
		return err
	}
	if manifest.Symbol != snapshot.Payload.Symbol || manifest.TradeDate != snapshot.Payload.TradeDate {
		return errors.New("trading-status facts manifest identity does not match payload")
	}
	if manifest.SystemAsOf == "" {
		return errors.New("trading-status facts manifest system_as_of is required")
	}
	if !validDailyBarsCloseSeriesNonBlank(manifest.SnapshotRef) ||
		!validDailyBarsCloseSeriesNonBlank(manifest.ManifestFingerprint) ||
		!validDailyBarsCloseSeriesNonBlank(snapshot.PayloadFingerprint) {
		return errors.New("trading-status facts snapshot fingerprints are required")
	}
	payloadFingerprint, err := fingerprintTradingStatusFactsPayload(snapshot.Payload)
	if err != nil {
		return err
	}
	if payloadFingerprint != snapshot.PayloadFingerprint {
		return errors.New("trading-status facts payload fingerprint mismatch")
	}
	manifestFingerprint, err := fingerprintTradingStatusFactsManifest(manifest)
	if err != nil {
		return err
	}
	if manifestFingerprint != manifest.ManifestFingerprint {
		return errors.New("trading-status facts manifest fingerprint mismatch")
	}
	snapshotRef, err := fingerprintTradingStatusFactsSnapshotRef(manifestFingerprint, payloadFingerprint)
	if err != nil {
		return err
	}
	if snapshotRef != manifest.SnapshotRef {
		return errors.New("trading-status facts snapshot_ref mismatch")
	}
	return nil
}

func fingerprintTradingStatusFactsPayload(payload SecurityTradingStatusFactsPayload) (string, error) {
	if err := payload.Validate(); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode trading-status facts payload: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize trading-status facts payload: %w", err)
	}
	return sha256Hex(canonical), nil
}

func fingerprintTradingStatusFactsManifest(manifest SecurityTradingStatusFactsManifest) (string, error) {
	input := tradingStatusFactsManifestFingerprintInput{
		SchemaVersion: manifest.SchemaVersion,
		CallerRef:     manifest.CallerRef,
		PurposeCode:   manifest.PurposeCode,
		Symbol:        manifest.Symbol,
		TradeDate:     manifest.TradeDate,
		SystemAsOf:    manifest.SystemAsOf,
		Sources:       manifest.Sources,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("encode trading-status facts manifest fingerprint input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize trading-status facts manifest: %w", err)
	}
	return sha256Hex(canonical), nil
}

func fingerprintTradingStatusFactsSnapshotRef(manifestFingerprint, payloadFingerprint string) (string, error) {
	encoded, err := json.Marshal(tradingStatusFactsSnapshotRefInput{
		ManifestFingerprint: manifestFingerprint,
		PayloadFingerprint:  payloadFingerprint,
	})
	if err != nil {
		return "", fmt.Errorf("encode trading-status facts snapshot ref input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("canonicalize trading-status facts snapshot ref: %w", err)
	}
	return dailyTradingStatusFactsSnapshotRefPrefix + sha256Hex(canonical), nil
}

func canonicalTradingStatusFactsSources(sources []DailyBarsCloseSeriesSource) ([]DailyBarsCloseSeriesSource, error) {
	if len(sources) == 0 {
		return nil, errors.New("trading-status facts snapshot requires at least one source")
	}
	seen := make(map[string]struct{}, len(sources))
	out := make([]DailyBarsCloseSeriesSource, 0, len(sources))
	for _, source := range sources {
		if !validDailyBarsCloseSeriesNonBlank(source.SourceRef) {
			return nil, errors.New("trading-status facts snapshot source_ref is required")
		}
		if source.SourceVersion != nil && !validDailyBarsCloseSeriesNonBlank(*source.SourceVersion) {
			return nil, errors.New("trading-status facts snapshot source_version must be non-blank when present")
		}
		if _, exists := seen[source.SourceRef]; exists {
			return nil, fmt.Errorf("duplicate trading-status facts source %q", source.SourceRef)
		}
		seen[source.SourceRef] = struct{}{}
		out = append(out, source)
	}
	return out, nil
}

func cloneTradingStatusFactsPayload(payload SecurityTradingStatusFactsPayload) SecurityTradingStatusFactsPayload {
	cloned := payload
	cloned.Facts = append([]SecurityTradingStatusFact{}, payload.Facts...)
	return cloned
}

func sha256Hex(document []byte) string {
	sum := sha256.Sum256(document)
	return hex.EncodeToString(sum[:])
}
