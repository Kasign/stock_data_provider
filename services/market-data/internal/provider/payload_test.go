package provider

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestReadBoundedBodyRejectsOversizedPayload(t *testing.T) {
	if _, err := readBoundedBody(strings.NewReader("12345"), 4); err == nil {
		t.Fatal("expected oversized payload rejection")
	}
	body, err := readBoundedBody(strings.NewReader("1234"), 4)
	if err != nil || string(body) != "1234" {
		t.Fatalf("exact-limit payload should pass: body=%q err=%v", body, err)
	}
}

func TestDecodeSingleJSONRejectsTrailingValuesAndJunk(t *testing.T) {
	var target map[string]any
	for _, payload := range []string{`{"ok":true}{"other":true}`, `{"ok":true}junk`} {
		if err := decodeSingleJSON([]byte(payload), &target); err == nil {
			t.Fatalf("expected strict JSON rejection for %q", payload)
		}
	}
	if err := decodeSingleJSON(bytes.TrimSpace([]byte(" {\"ok\":true} ")), &target); err != nil {
		t.Fatalf("valid single JSON should pass: %v", err)
	}
}

func TestValidProviderSnapshotTimeRejectsZeroAndFutureValues(t *testing.T) {
	reference := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	withinSkew := reference.Add(5 * time.Minute)
	past := reference.Add(-24 * time.Hour)
	future := reference.Add(5*time.Minute + time.Nanosecond)

	if got := validProviderSnapshotTime(nil, reference); got != nil {
		t.Fatalf("nil timestamp should remain nil, got %v", got)
	}
	zero := time.Time{}
	if got := validProviderSnapshotTime(&zero, reference); got != nil {
		t.Fatalf("zero timestamp should be rejected, got %v", got)
	}
	if got := validProviderSnapshotTime(&future, reference); got != nil {
		t.Fatalf("timestamp beyond the allowed future skew should be rejected, got %v", got)
	}
	if got := validProviderSnapshotTime(&withinSkew, reference); got == nil || !got.Equal(withinSkew) {
		t.Fatalf("timestamp exactly at the allowed skew should be accepted, got %v", got)
	}
	if got := validProviderSnapshotTime(&past, reference); got == nil || !got.Equal(past) {
		t.Fatalf("older timestamp should remain valid for freshness handling, got %v", got)
	}
}
