package domain

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gowebpki/jcs"
)

var (
	ErrInvalidCommand      = errors.New("invalid create run command")
	ErrIdempotencyConflict = errors.New("idempotency key conflicts with a different request")
)

func ValidateCreateRunCommand(cmd CreateRunCommand, allowEmptyInputs bool) error {
	if strings.TrimSpace(cmd.SchemaVersion) == "" || strings.TrimSpace(cmd.IdempotencyKey) == "" ||
		len(cmd.IdempotencyKey) > 128 || strings.TrimSpace(cmd.TemplateRef) == "" ||
		strings.TrimSpace(cmd.TemplateVersion) == "" {
		return fmt.Errorf("%w: schema, idempotency key and exact template identity are required", ErrInvalidCommand)
	}
	if _, err := canonicalInputSnapshotRefs(cmd.InputSnapshotRefs, allowEmptyInputs); err != nil {
		return err
	}
	parameters := bytes.TrimSpace(cmd.Parameters)
	if len(parameters) == 0 || !json.Valid(parameters) || parameters[0] != '{' {
		return fmt.Errorf("%w: parameters must be a JSON object; use {} when empty", ErrInvalidCommand)
	}
	return nil
}

// canonicalInputSnapshotRefs validates opaque references and returns a stable set order.
func canonicalInputSnapshotRefs(refs []string, allowEmpty bool) ([]string, error) {
	if refs == nil {
		return nil, fmt.Errorf("%w: input_snapshot_refs must be present", ErrInvalidCommand)
	}
	if len(refs) == 0 && !allowEmpty {
		return nil, fmt.Errorf("%w: template does not allow empty input snapshots", ErrInvalidCommand)
	}
	canonical := append([]string(nil), refs...)
	seen := make(map[string]struct{}, len(canonical))
	for _, ref := range canonical {
		if strings.TrimSpace(ref) == "" {
			return nil, fmt.Errorf("%w: input snapshot reference is empty", ErrInvalidCommand)
		}
		if strings.TrimSpace(ref) != ref {
			return nil, fmt.Errorf("%w: input snapshot reference must not contain surrounding whitespace", ErrInvalidCommand)
		}
		if _, exists := seen[ref]; exists {
			return nil, fmt.Errorf("%w: duplicate input snapshot reference %q", ErrInvalidCommand, ref)
		}
		seen[ref] = struct{}{}
	}
	sort.Strings(canonical)
	return canonical, nil
}

// RequestFingerprint scopes semantic request intent to the authenticated caller.
// Server-generated PIT/Deadline timestamps and tracing identifiers are deliberately excluded.
func RequestFingerprint(callerRef string, cmd CreateRunCommand) (string, error) {
	if strings.TrimSpace(callerRef) == "" {
		return "", fmt.Errorf("%w: authenticated caller is required", ErrInvalidCommand)
	}
	if err := ValidateCreateRunCommand(cmd, true); err != nil {
		return "", err
	}
	refs, err := canonicalInputSnapshotRefs(cmd.InputSnapshotRefs, true)
	if err != nil {
		return "", err
	}
	payload := struct {
		CallerRef         string          `json:"caller_ref"`
		Operation         string          `json:"operation"`
		SchemaVersion     string          `json:"schema_version"`
		TemplateRef       string          `json:"template_ref"`
		TemplateVersion   string          `json:"template_version"`
		InputSnapshotRefs []string        `json:"input_snapshot_refs"`
		Parameters        json.RawMessage `json:"parameters"`
	}{callerRef, "CreateRun", cmd.SchemaVersion, cmd.TemplateRef, cmd.TemplateVersion, refs, bytes.TrimSpace(cmd.Parameters)}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode create run fingerprint: %w", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", fmt.Errorf("%w: canonicalize create run fingerprint: %v", ErrInvalidCommand, err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func ValidateIdempotentReplay(existingFingerprint, incomingFingerprint string) error {
	if existingFingerprint == "" || incomingFingerprint == "" {
		return fmt.Errorf("%w: request fingerprint is required", ErrInvalidCommand)
	}
	if existingFingerprint != incomingFingerprint {
		return ErrIdempotencyConflict
	}
	return nil
}
