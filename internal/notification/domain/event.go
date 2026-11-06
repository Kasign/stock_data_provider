package domain

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/gowebpki/jcs"
)

var (
	ErrInvalidPayloadFingerprint  = errors.New("notification: invalid payload fingerprint")
	ErrPayloadFingerprintMismatch = errors.New("notification: payload fingerprint mismatch")
)

// EventEnvelope is the immutable event contract accepted by Notification.
// Payload remains raw JSON so canonicalization sees the producer's exact JSON value.
type EventEnvelope struct {
	EventID            string          `json:"event_id"`
	EventType          string          `json:"event_type"`
	SchemaVersion      string          `json:"schema_version"`
	SourceModule       string          `json:"source_module"`
	OccurredAt         string          `json:"occurred_at"`
	PublishedAt        string          `json:"published_at"`
	SubjectRef         string          `json:"subject_ref"`
	IdempotencyKey     string          `json:"idempotency_key"`
	CorrelationID      string          `json:"correlation_id"`
	PayloadFingerprint string          `json:"payload_fingerprint"`
	Payload            json.RawMessage `json:"payload"`
}

type fingerprintInput struct {
	EventType     string          `json:"event_type"`
	SchemaVersion string          `json:"schema_version"`
	SourceModule  string          `json:"source_module"`
	SubjectRef    string          `json:"subject_ref"`
	OccurredAt    string          `json:"occurred_at"`
	Payload       json.RawMessage `json:"payload"`
}

// CalculatePayloadFingerprint returns the RFC 8785 JCS SHA-256 digest.
// Transport identity and publication metadata are intentionally excluded.
func (e EventEnvelope) CalculatePayloadFingerprint() (string, error) {
	if len(e.Payload) == 0 || !json.Valid(e.Payload) {
		return "", fmt.Errorf("notification: invalid event payload")
	}

	input := fingerprintInput{
		EventType: e.EventType, SchemaVersion: e.SchemaVersion,
		SourceModule: e.SourceModule, SubjectRef: e.SubjectRef,
		OccurredAt: e.OccurredAt, Payload: e.Payload,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("notification: encode fingerprint input: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("notification: canonicalize fingerprint input: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// VerifyPayloadFingerprint rejects absent, malformed, uppercase, or mismatched digests.
func (e EventEnvelope) VerifyPayloadFingerprint() error {
	if len(e.PayloadFingerprint) != sha256.Size*2 ||
		strings.ToLower(e.PayloadFingerprint) != e.PayloadFingerprint {
		return ErrInvalidPayloadFingerprint
	}
	if _, err := hex.DecodeString(e.PayloadFingerprint); err != nil {
		return ErrInvalidPayloadFingerprint
	}
	calculated, err := e.CalculatePayloadFingerprint()
	if err != nil {
		return err
	}
	if subtle.ConstantTimeCompare([]byte(calculated), []byte(e.PayloadFingerprint)) != 1 {
		return ErrPayloadFingerprintMismatch
	}
	return nil
}
