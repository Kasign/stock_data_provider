package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestSecretTextAndZeroValueContract(t *testing.T) {
	const raw = "text-secret-must-not-leak"
	secret := testEnv(map[string]string{"APP_SECRET": raw}).Secret("APP_SECRET")

	encoded, err := secret.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) != "[REDACTED]" || bytes.Contains(encoded, []byte(raw)) {
		t.Fatalf("unexpected text serialization: %q", encoded)
	}

	var zero Secret
	if zero.Configured() || !zero.IsZero() || zero.Value() != "" {
		t.Fatal("zero Secret must remain unconfigured")
	}
	zeroText, err := zero.MarshalText()
	if err != nil {
		t.Fatal(err)
	}
	if string(zeroText) != "[REDACTED]" || strings.Contains(string(zeroText), raw) {
		t.Fatalf("zero Secret serialization is not redacted: %q", zeroText)
	}
}
