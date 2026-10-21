package provider

import (
	"bytes"
	"strings"
	"testing"
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
