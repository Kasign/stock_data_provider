package app

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

const maximumEvidenceCursorKeys = 8

var (
	ErrEvidenceCursorInvalid = errors.New("news.evidence.cursor_invalid")
	ErrEvidenceCursorExpired = errors.New("news.evidence.cursor_expired")
)

// EvidenceCursorCodec signs immutable query continuation claims. Previous keys
// must remain in Keys until every cursor they signed has passed its 15-minute TTL.
type EvidenceCursorCodec struct {
	activeKeyID string
	keys        map[string]platformconfig.Secret
}

type EvidenceCursorPosition struct {
	SnapshotRef          string
	LastSystemObservedAt time.Time
	LastNewsID           string
	LastVersionID        string
	ExpiresAt            time.Time
}

type evidenceCursorClaims struct {
	TokenVersion          int                  `json:"token_version"`
	KeyID                 string               `json:"key_id"`
	CallerRef             string               `json:"caller_ref"`
	PurposeCode           EvidencePurposeCode  `json:"purpose_code"`
	RequestedContentLevel EvidenceContentLevel `json:"requested_content_level"`
	FiltersFingerprint    string               `json:"filters_fingerprint"`
	PageSize              int                  `json:"page_size"`
	SystemAsOf            time.Time            `json:"system_as_of"`
	SnapshotRef           string               `json:"snapshot_ref"`
	SortVersion           string               `json:"sort_version"`
	LastSystemObservedAt  time.Time            `json:"last_system_observed_at"`
	LastNewsID            string               `json:"last_news_id"`
	LastVersionID         string               `json:"last_version_id"`
	IssuedAt              time.Time            `json:"issued_at"`
	ExpiresAt             time.Time            `json:"expires_at"`
}

type evidenceFilterFingerprintInput struct {
	NewsIDs          []string                `json:"news_ids"`
	SourceRefs       []string                `json:"source_refs"`
	CanonicalSymbols []string                `json:"canonical_symbols"`
	PublishedAtFrom  *time.Time              `json:"published_at_from"`
	PublishedAtTo    *time.Time              `json:"published_at_to"`
	QualityStatuses  []EvidenceQualityStatus `json:"quality_statuses"`
	BusinessTimeFrom *time.Time              `json:"business_time_from"`
	BusinessTimeTo   *time.Time              `json:"business_time_to"`
}

// NewEvidenceCursorCodec copies the key ring. Key bytes are kept in Platform
// Secret values and are accessed only for HMAC operations.
func NewEvidenceCursorCodec(activeKeyID string, keys map[string]platformconfig.Secret) (*EvidenceCursorCodec, error) {
	if !validEvidenceCursorKeyID(activeKeyID) || len(keys) == 0 || len(keys) > maximumEvidenceCursorKeys {
		return nil, ErrEvidenceCursorInvalid
	}
	copied := make(map[string]platformconfig.Secret, len(keys))
	for keyID, secret := range keys {
		if !validEvidenceCursorKeyID(keyID) || !secret.Configured() ||
			len(secret.Value()) < 32 || len(secret.Value()) > 256 {
			return nil, ErrEvidenceCursorInvalid
		}
		copied[keyID] = secret
	}
	if _, exists := copied[activeKeyID]; !exists {
		return nil, ErrEvidenceCursorInvalid
	}
	return &EvidenceCursorCodec{activeKeyID: activeKeyID, keys: copied}, nil
}

func (c *EvidenceCursorCodec) EncodeNextCursor(
	request ListEvidenceRequest,
	snapshotRef string,
	last ResearchEvidenceRecord,
	issuedAt time.Time,
) (string, error) {
	if c == nil || !validEvidenceOpaqueRef(snapshotRef) || issuedAt.IsZero() ||
		!validEvidenceUTC(issuedAt) || request.Validate(issuedAt) != nil ||
		!validEvidenceOpaqueRef(last.NewsID) || !validEvidenceOpaqueRef(last.VersionID) ||
		last.SystemObservedAt.IsZero() || !validEvidenceUTC(last.SystemObservedAt) ||
		last.SystemObservedAt.After(request.SystemAsOf) ||
		!last.SystemAsOf.Equal(request.SystemAsOf) {
		return "", ErrEvidenceCursorInvalid
	}
	secret, exists := c.keys[c.activeKeyID]
	if !exists || !secret.Configured() {
		return "", ErrEvidenceCursorInvalid
	}
	fingerprint, err := evidenceFiltersFingerprint(request)
	if err != nil {
		return "", ErrEvidenceCursorInvalid
	}
	claims := evidenceCursorClaims{
		TokenVersion:          1,
		KeyID:                 c.activeKeyID,
		CallerRef:             request.CallerRef,
		PurposeCode:           request.PurposeCode,
		RequestedContentLevel: request.RequestedContentLevel,
		FiltersFingerprint:    fingerprint,
		PageSize:              request.PageSize,
		SystemAsOf:            request.SystemAsOf.UTC(),
		SnapshotRef:           snapshotRef,
		SortVersion:           evidenceSortVersion,
		LastSystemObservedAt:  last.SystemObservedAt.UTC(),
		LastNewsID:            last.NewsID,
		LastVersionID:         last.VersionID,
		IssuedAt:              issuedAt.UTC(),
		ExpiresAt:             issuedAt.UTC().Add(evidenceCursorTTL),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", ErrEvidenceCursorInvalid
	}
	mac := hmac.New(sha256.New, []byte(secret.Value()))
	_, _ = mac.Write(payload)
	token := base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if len(token) > 4096 {
		return "", ErrEvidenceCursorInvalid
	}
	return token, nil
}

func (c *EvidenceCursorCodec) DecodeCursor(
	token string,
	request ListEvidenceRequest,
	now time.Time,
) (EvidenceCursorPosition, error) {
	if c == nil || token == "" || len(token) > 4096 || now.IsZero() || !validEvidenceUTC(now) {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	request.Cursor = &token
	if request.Validate(now) != nil {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) == 0 || len(payload) > 3072 {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	var header struct {
		KeyID string `json:"key_id"`
	}
	if err := json.Unmarshal(payload, &header); err != nil || !validEvidenceCursorKeyID(header.KeyID) {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	secret, exists := c.keys[header.KeyID]
	if !exists || !secret.Configured() {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(signature) != sha256.Size {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	mac := hmac.New(sha256.New, []byte(secret.Value()))
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}

	var claims evidenceCursorClaims
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&claims); err != nil {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	if claims.TokenVersion != 1 || claims.KeyID != header.KeyID ||
		claims.SortVersion != evidenceSortVersion ||
		!validEvidenceOpaqueRef(claims.SnapshotRef) ||
		!validEvidenceUTC(claims.SystemAsOf) ||
		claims.SystemAsOf.IsZero() ||
		!validEvidenceUTC(claims.LastSystemObservedAt) ||
		claims.LastSystemObservedAt.IsZero() ||
		claims.LastSystemObservedAt.After(claims.SystemAsOf) ||
		!validEvidenceOpaqueRef(claims.LastNewsID) ||
		!validEvidenceOpaqueRef(claims.LastVersionID) ||
		!validEvidenceUTC(claims.IssuedAt) ||
		!validEvidenceUTC(claims.ExpiresAt) ||
		claims.IssuedAt.IsZero() ||
		!claims.ExpiresAt.Equal(claims.IssuedAt.Add(evidenceCursorTTL)) {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	if now.Before(claims.IssuedAt) {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	if !now.Before(claims.ExpiresAt) {
		return EvidenceCursorPosition{}, ErrEvidenceCursorExpired
	}
	fingerprint, err := evidenceFiltersFingerprint(request)
	if err != nil ||
		claims.CallerRef != request.CallerRef ||
		claims.PurposeCode != request.PurposeCode ||
		claims.RequestedContentLevel != request.RequestedContentLevel ||
		claims.PageSize != request.PageSize ||
		!claims.SystemAsOf.Equal(request.SystemAsOf) ||
		claims.FiltersFingerprint != fingerprint {
		return EvidenceCursorPosition{}, ErrEvidenceCursorInvalid
	}
	return EvidenceCursorPosition{
		SnapshotRef:          claims.SnapshotRef,
		LastSystemObservedAt: claims.LastSystemObservedAt,
		LastNewsID:           claims.LastNewsID,
		LastVersionID:        claims.LastVersionID,
		ExpiresAt:            claims.ExpiresAt,
	}, nil
}

func evidenceFiltersFingerprint(request ListEvidenceRequest) (string, error) {
	if request.Filters == nil {
		return "", ErrInvalidEvidenceQuery
	}
	filters := request.Filters
	input := evidenceFilterFingerprintInput{
		NewsIDs:          sortedEvidenceStrings(filters.NewsIDs),
		SourceRefs:       sortedEvidenceStrings(filters.SourceRefs),
		CanonicalSymbols: sortedEvidenceStrings(filters.CanonicalSymbols),
		PublishedAtFrom:  utcEvidenceTime(filters.PublishedAtFrom),
		PublishedAtTo:    utcEvidenceTime(filters.PublishedAtTo),
		QualityStatuses:  append([]EvidenceQualityStatus{}, filters.QualityStatuses...),
		BusinessTimeFrom: utcEvidenceTime(request.BusinessTimeFrom),
		BusinessTimeTo:   utcEvidenceTime(request.BusinessTimeTo),
	}
	sort.Slice(input.QualityStatuses, func(i, j int) bool {
		return input.QualityStatuses[i] < input.QualityStatuses[j]
	})
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func sortedEvidenceStrings(values []string) []string {
	result := append([]string{}, values...)
	sort.Strings(result)
	return result
}

func utcEvidenceTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	normalized := value.UTC()
	return &normalized
}

func validEvidenceCursorKeyID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return true
}
