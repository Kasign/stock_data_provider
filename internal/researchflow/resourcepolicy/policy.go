// Package resourcepolicy resolves immutable Researchflow resource authorization snapshots.
package resourcepolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

const (
	NewsSearchConfiguredProvidersRef = "news.search.configured-providers@1.0.0"
	newsSearchPolicyVersion          = "1.0.0"
)

var (
	ErrPolicyNotFound = errors.New("Researchflow resource policy not found")
	ErrNoProviders    = errors.New("Researchflow resource policy has no configured providers")
	ErrInvalidInventory = errors.New("Researchflow provider inventory is invalid")
)

var providerCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// Snapshot is the immutable authorization decision made when a Run is created.
// It intentionally contains Provider identities only, never endpoints or credentials.
type Snapshot struct {
	PolicyRef         string   `json:"policy_ref"`
	PolicyFingerprint string   `json:"policy_fingerprint"`
	AllowedProviderCodes []string `json:"allowed_provider_codes"`
}

type definition struct {
	Ref          string `json:"ref"`
	Version      string `json:"version"`
	ResourceType string `json:"resource_type"`
	Selection    string `json:"selection"`
}

type Registry struct {
	definitions map[string]definition
	fingerprints map[string]string
}

// NewRegistry returns a fixed, versioned registry. Definitions are private and
// immutable after construction; callers cannot mutate the registry through inputs.
func NewRegistry() (*Registry, error) {
	definitions := []definition{{
		Ref: NewsSearchConfiguredProvidersRef,
		Version: newsSearchPolicyVersion,
		ResourceType: "news.provider",
		Selection: "server_successfully_assembled_provider_set_at_run_creation",
	}}
	registry := &Registry{
		definitions: make(map[string]definition, len(definitions)),
		fingerprints: make(map[string]string, len(definitions)),
	}
	for _, item := range definitions {
		fingerprint, err := fingerprintDefinition(item)
		if err != nil {
			return nil, err
		}
		registry.definitions[item.Ref] = item
		registry.fingerprints[item.Ref] = fingerprint
	}
	return registry, nil
}

// Resolve snapshots the currently assembled Provider inventory for one exact
// policy version. Empty or malformed inventories fail closed.
func (r *Registry) Resolve(ref string, providerCodes []string) (Snapshot, error) {
	if r == nil {
		return Snapshot{}, ErrPolicyNotFound
	}
	item, ok := r.definitions[ref]
	if !ok {
		return Snapshot{}, fmt.Errorf("%w: %q", ErrPolicyNotFound, ref)
	}
	if len(providerCodes) == 0 {
		return Snapshot{}, ErrNoProviders
	}
	codes := append([]string(nil), providerCodes...)
	for i, code := range codes {
		if code == "" || strings.TrimSpace(code) != code || !providerCodePattern.MatchString(code) {
			return Snapshot{}, fmt.Errorf("%w: invalid Provider Code at index %d", ErrInvalidInventory, i)
		}
	}
	sort.Strings(codes)
	for i := 1; i < len(codes); i++ {
		if codes[i] == codes[i-1] {
			return Snapshot{}, fmt.Errorf("%w: duplicate Provider Code %q", ErrInvalidInventory, codes[i])
		}
	}
	return Snapshot{
		PolicyRef: item.Ref,
		PolicyFingerprint: r.fingerprints[item.Ref],
		AllowedProviderCodes: codes,
	}, nil
}

// ValidateSnapshot checks the immutable policy identity and canonical allowlist.
func (r *Registry) ValidateSnapshot(snapshot Snapshot) error {
	if r == nil {
		return ErrPolicyNotFound
	}
	fingerprint, ok := r.fingerprints[snapshot.PolicyRef]
	if !ok {
		return fmt.Errorf("%w: %q", ErrPolicyNotFound, snapshot.PolicyRef)
	}
	if snapshot.PolicyFingerprint != fingerprint || len(snapshot.AllowedProviderCodes) == 0 {
		return ErrInvalidInventory
	}
	for i, code := range snapshot.AllowedProviderCodes {
		if code == "" || !providerCodePattern.MatchString(code) ||
			(i > 0 && snapshot.AllowedProviderCodes[i-1] >= code) {
			return ErrInvalidInventory
		}
	}
	return nil
}

func fingerprintDefinition(item definition) (string, error) {
	raw, err := json.Marshal(item)
	if err != nil {
		return "", fmt.Errorf("encode Researchflow resource policy: %w", err)
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
