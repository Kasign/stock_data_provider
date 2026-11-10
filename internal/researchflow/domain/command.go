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
	"time"

	"github.com/gowebpki/jcs"
)

var (
	ErrInvalidCommand      = errors.New("invalid create run command")
	ErrIdempotencyConflict = errors.New("idempotency key conflicts with a different request")
)

func ValidateCreateRunCommand(cmd CreateRunCommand, now time.Time, allowEmptyInputs bool) error {
	if strings.TrimSpace(cmd.SchemaVersion) == "" || strings.TrimSpace(cmd.IdempotencyKey) == "" ||
		len(cmd.IdempotencyKey) > 128 || strings.TrimSpace(cmd.TemplateRef) == "" ||
		strings.TrimSpace(cmd.TemplateVersion) == "" || strings.TrimSpace(cmd.CorrelationID) == "" {
		return fmt.Errorf("%w: required field missing or idempotency key exceeds 128 characters", ErrInvalidCommand)
	}
	if cmd.SystemAsOf.IsZero() || cmd.DeadlineAt.IsZero() || !isUTC(cmd.SystemAsOf) || !isUTC(cmd.DeadlineAt) {
		return fmt.Errorf("%w: system_as_of and deadline_at must be UTC timestamps", ErrInvalidCommand)
	}
	now = now.UTC()
	if cmd.SystemAsOf.After(now) {
		return fmt.Errorf("%w: system_as_of must not be in the future", ErrInvalidCommand)
	}
	if !cmd.DeadlineAt.After(now) {
		return fmt.Errorf("%w: deadline_at must be later than database UTC now", ErrInvalidCommand)
	}
	if _, err := canonicalInputSnapshotRefs(cmd.InputSnapshotRefs, allowEmptyInputs); err != nil {
		return err
	}
	parameters := bytes.TrimSpace(cmd.Parameters)
	if len(parameters) == 0 || !json.Valid(parameters) || parameters[0] != '{' {
		return fmt.Errorf("%w: parameters must be a JSON object; use {} when empty", ErrInvalidCommand)
	}
	if cmd.OriginatingRequestID != nil && strings.TrimSpace(*cmd.OriginatingRequestID) == "" {
		return fmt.Errorf("%w: originating_request_id must be null or non-empty", ErrInvalidCommand)
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

func isUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}

// RequestFingerprint scopes idempotency to the authenticated caller and CreateRun operation.
// Tracing IDs are deliberately excluded. callerRef must come from trusted authentication context.
func RequestFingerprint(callerRef string, cmd CreateRunCommand) (string, error) {
	if strings.TrimSpace(callerRef) == "" {
		return "", fmt.Errorf("%w: authenticated caller is required", ErrInvalidCommand)
	}
	if cmd.SystemAsOf.IsZero() || cmd.DeadlineAt.IsZero() || !isUTC(cmd.SystemAsOf) || !isUTC(cmd.DeadlineAt) {
		return "", fmt.Errorf("%w: system_as_of and deadline_at must be non-zero UTC timestamps", ErrInvalidCommand)
	}
	refs, err := canonicalInputSnapshotRefs(cmd.InputSnapshotRefs, true)
	if err != nil {
		return "", err
	}
	parameters := bytes.TrimSpace(cmd.Parameters)
	if len(parameters) == 0 || !json.Valid(parameters) || parameters[0] != '{' {
		return "", fmt.Errorf("%w: parameters must be a valid JSON object", ErrInvalidCommand)
	}
	payload := struct {
		CallerRef         string          `json:"caller_ref"`
		Operation         string          `json:"operation"`
		TemplateRef       string          `json:"template_ref"`
		TemplateVersion   string          `json:"template_version"`
		SystemAsOf        time.Time       `json:"system_as_of"`
		DeadlineAt        time.Time       `json:"deadline_at"`
		InputSnapshotRefs []string        `json:"input_snapshot_refs"`
		Parameters        json.RawMessage `json:"parameters"`
	}{callerRef, "CreateRun", cmd.TemplateRef, cmd.TemplateVersion, cmd.SystemAsOf.UTC(), cmd.DeadlineAt.UTC(), refs, parameters}
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
