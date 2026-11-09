package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"
)

func TestPayloadDigestUsesCompletePayloadBytes(t *testing.T) {
	payload := []byte(`{"id":"ctx-1","version":1}`)
	sum := sha256.Sum256(payload)
	if got, want := payloadDigest(payload), hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("payloadDigest() = %q, want %q", got, want)
	}
	if payloadDigest(payload) == payloadDigest([]byte(`{"id":"ctx-1","version":2}`)) {
		t.Fatal("different complete payloads must have different digests")
	}
}

func TestFormatPITUsesFixedNineDigitUTCPrecision(t *testing.T) {
	value := time.Date(2026, 11, 6, 2, 3, 4, 120000000, time.FixedZone("UTC+8", 8*60*60))
	got := formatPIT(value)
	want := "2026-11-05T18:03:04.120000000Z"
	if got != want {
		t.Fatalf("formatPIT() = %q, want %q", got, want)
	}
	if len(got) != 30 {
		t.Fatalf("formatPIT() length = %d, want 30", len(got))
	}
	earlier := formatPIT(time.Date(2026, 1, 1, 0, 0, 0, 100000000, time.UTC))
	later := formatPIT(time.Date(2026, 1, 1, 0, 0, 0, 110000000, time.UTC))
	if earlier >= later {
		t.Fatalf("fixed-width PIT values must sort chronologically: %q >= %q", earlier, later)
	}
}

func TestNewRepositoryBaseRequiresDatabase(t *testing.T) {
	_, err := newRepositoryBase(nil, nil)
	if !errors.Is(err, ErrInvalidRepository) {
		t.Fatalf("newRepositoryBase(nil) error = %v, want ErrInvalidRepository", err)
	}
}


func TestVerifyStoredPayloadRejectsDigestMismatch(t *testing.T) {
	payload := []byte(`{"context_id":"ctx-1","context_version":1}`)
	validDigest := payloadDigest(payload)
	if err := verifyStoredPayload(payload, validDigest); err != nil {
		t.Fatalf("valid stored payload rejected: %v", err)
	}
	if err := verifyStoredPayload([]byte(`{"context_id":"ctx-1","context_version":2}`), validDigest); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered stored payload error = %v, want ErrCorruptRecord", err)
	}
	if err := verifyStoredPayload(payload, ""); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("missing stored digest error = %v, want ErrCorruptRecord", err)
	}
}
