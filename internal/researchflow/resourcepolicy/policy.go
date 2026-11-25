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
	NewsFeedConfiguredSourcesRef    = "news.feed.configured-sources@1.0.0"
	QuantDailyBarsProvidersRef      = "quant.daily-bars.configured-providers@1.0.0"
	newsSearchPolicyVersion         = "1.0.0"
	newsFeedPolicyVersion           = "1.0.0"
	quantDailyBarsPolicyVersion     = "1.0.0"
)

var (
	ErrPolicyNotFound   = errors.New("Researchflow resource policy not found")
	ErrNoProviders      = errors.New("Researchflow resource policy has no configured providers")
	ErrInvalidInventory = errors.New("Researchflow resource policy inventory is invalid")
)

var (
	providerCodePattern       = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	sourceEndpointRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}/[A-Za-z0-9][A-Za-z0-9._-]{0,127}@[a-f0-9]{64}// Package resourcepolicy resolves immutable Researchflow resource authorization snapshots.
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
	NewsFeedConfiguredSourcesRef    = "news.feed.configured-sources@1.0.0"
	QuantDailyBarsProvidersRef      = "quant.daily-bars.configured-providers@1.0.0"
	newsSearchPolicyVersion         = "1.0.0"
	newsFeedPolicyVersion           = "1.0.0"
	quantDailyBarsPolicyVersion     = "1.0.0"
)

var (
	ErrPolicyNotFound   = errors.New("Researchflow resource policy not found")
	ErrNoProviders      = errors.New("Researchflow resource policy has no configured providers")
	ErrInvalidInventory = errors.New("Researchflow resource policy inventory is invalid")
)

var (
	providerCodePattern       = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
	)
)

// Snapshot is the immutable resource authorization decision made when a Run is created.
// Provider and Source/Endpoint identities are stored separately; credentials and URLs are never included.
type Snapshot struct {
	PolicyRef                 string   `json:"policy_ref"`
	PolicyFingerprint         string   `json:"policy_fingerprint"`
	AllowedProviderCodes      []string `json:"allowed_provider_codes,omitempty"`
	AllowedSourceEndpointRefs []string `json:"allowed_source_endpoint_refs,omitempty"`
}

type definition struct {
	Ref          string `json:"ref"`
	Version      string `json:"version"`
	ResourceType string `json:"resource_type"`
	Selection    string `json:"selection"`
}

type Registry struct {
	definitions  map[string]definition
	fingerprints map[string]string
}

func NewRegistry() (*Registry, error) {
	definitions := []definition{
		{
			Ref: NewsSearchConfiguredProvidersRef, Version: newsSearchPolicyVersion,
			ResourceType: "news.provider",
			Selection:    "server_successfully_assembled_provider_set_at_run_creation",
		},
		{
			Ref: NewsFeedConfiguredSourcesRef, Version: newsFeedPolicyVersion,
			ResourceType: "news.feed_catalog",
			Selection:    "server_successfully_assembled_trusted_feed_source_catalog_at_run_creation",
		},
		{
			Ref: QuantDailyBarsProvidersRef, Version: quantDailyBarsPolicyVersion,
			ResourceType: "marketdata.daily_bars_provider",
			Selection:    "server_successfully_assembled_daily_bars_provider_set_at_run_creation",
		},
	}
	registry := &Registry{
		definitions:  make(map[string]definition, len(definitions)),
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

// Resolve snapshots the successfully assembled Provider inventory for one exact policy version.
func (r *Registry) Resolve(ref string, providerCodes []string) (Snapshot, error) {
	if r == nil {
		return Snapshot{}, ErrPolicyNotFound
	}
	item, ok := r.definitions[ref]
	if !ok {
		return Snapshot{}, fmt.Errorf("%w: %q", ErrPolicyNotFound, ref)
	}
	if ref == NewsFeedConfiguredSourcesRef {
		return Snapshot{}, fmt.Errorf("%w: Feed policy requires Source/Endpoint identities", ErrInvalidInventory)
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
		PolicyRef: item.Ref, PolicyFingerprint: r.fingerprints[item.Ref],
		AllowedProviderCodes: codes,
	}, nil
}

// ResolveSourceEndpoints snapshots the exact trusted Feed endpoints assembled by Server.
func (r *Registry) ResolveSourceEndpoints(ref string, sourceEndpointRefs []string) (Snapshot, error) {
	if r == nil {
		return Snapshot{}, ErrPolicyNotFound
	}
	item, ok := r.definitions[ref]
	if !ok || ref != NewsFeedConfiguredSourcesRef {
		return Snapshot{}, fmt.Errorf("%w: %q", ErrPolicyNotFound, ref)
	}
	if len(sourceEndpointRefs) == 0 {
		return Snapshot{}, ErrNoProviders
	}
	refs := append([]string(nil), sourceEndpointRefs...)
	for i, value := range refs {
		if value != strings.TrimSpace(value) || !sourceEndpointRefPattern.MatchString(value) {
			return Snapshot{}, fmt.Errorf("%w: invalid Source/Endpoint reference at index %d", ErrInvalidInventory, i)
		}
	}
	sort.Strings(refs)
	for i := 1; i < len(refs); i++ {
		if refs[i] == refs[i-1] {
			return Snapshot{}, fmt.Errorf("%w: duplicate Source/Endpoint reference %q", ErrInvalidInventory, refs[i])
		}
	}
	return Snapshot{
		PolicyRef: item.Ref, PolicyFingerprint: r.fingerprints[item.Ref],
		AllowedSourceEndpointRefs: refs,
	}, nil
}

// ValidateSnapshotInventory validates the type-specific dynamic inventory without
// needing a Registry instance. The Registry separately verifies the policy fingerprint.
func ValidateSnapshotInventory(snapshot Snapshot) error {
	if snapshot.PolicyRef == NewsFeedConfiguredSourcesRef {
		if len(snapshot.AllowedProviderCodes) != 0 || len(snapshot.AllowedSourceEndpointRefs) == 0 {
			return ErrInvalidInventory
		}
		for i, value := range snapshot.AllowedSourceEndpointRefs {
			if value != strings.TrimSpace(value) || !sourceEndpointRefPattern.MatchString(value) ||
				(i > 0 && snapshot.AllowedSourceEndpointRefs[i-1] >= value) {
				return ErrInvalidInventory
			}
		}
		return nil
	}
	if len(snapshot.AllowedSourceEndpointRefs) != 0 || len(snapshot.AllowedProviderCodes) == 0 {
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

func (r *Registry) ValidateSnapshot(snapshot Snapshot) error {
	if r == nil {
		return ErrPolicyNotFound
	}
	fingerprint, ok := r.fingerprints[snapshot.PolicyRef]
	if !ok {
		return fmt.Errorf("%w: %q", ErrPolicyNotFound, snapshot.PolicyRef)
	}
	if snapshot.PolicyFingerprint != fingerprint || ValidateSnapshotInventory(snapshot) != nil {
		return ErrInvalidInventory
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
