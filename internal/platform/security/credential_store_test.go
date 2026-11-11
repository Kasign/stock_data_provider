package security

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

func TestOwnerCredentialRedactsPasswordHashFromSerializationAndLogging(t *testing.T) {
	const secretHash = "$argon2id$v=19$m=8192,t=1,p=1$YWJjZGVmZ2hpamtsbW5vcA$YWJjZGVmZ2hpamtsbW5vcHFyc3R1dnd4eXowMTIzNDU2"
	credential := OwnerCredential{
		Subject: "owner", PasswordHash: secretHash, CredentialVersion: 7,
	}

	encoded, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), secretHash) {
		t.Fatal("JSON serialization exposed the password hash")
	}
	if strings.Contains(fmt.Sprintf("%+v", credential), secretHash) {
		t.Fatal("formatted logging exposed the password hash")
	}

	var output bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&output, nil))
	logger.Info("credential loaded", "credential", credential)
	if strings.Contains(output.String(), secretHash) {
		t.Fatal("structured logging exposed the password hash")
	}
}
