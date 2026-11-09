package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
)

var ErrInvalidDeliveryPlan = errors.New("notification: invalid delivery plan")

type AuthorizationPolicyVersion struct {
	PolicyType    string `json:"policy_type"`
	PolicyRef     string `json:"policy_ref"`
	PolicyVersion string `json:"policy_version"`
}

type DeliveryPlan struct {
	SourceModule                string                       `json:"source_module"`
	EventID                     string                       `json:"event_id"`
	SubscriptionID              string                       `json:"subscription_id"`
	SubscriptionVersion         int64                        `json:"subscription_version"`
	ChannelRef                  string                       `json:"channel_ref"`
	ChannelConfigVersion        int64                        `json:"channel_config_version"`
	TemplateID                  string                       `json:"template_id"`
	TemplateVersion             int64                        `json:"template_version"`
	RecipientRef                string                       `json:"recipient_ref"`
	PayloadFingerprint          string                       `json:"payload_fingerprint"`
	AuthorizationPolicyVersions []AuthorizationPolicyVersion `json:"authorization_policy_versions"`
	AuthorizationRefs           []string                     `json:"authorization_refs,omitempty"`
	PlanFingerprint             string                       `json:"plan_fingerprint"`
	DeliveryID                  string                       `json:"delivery_id"`
	DeliveryIdempotencyKey      string                       `json:"delivery_idempotency_key"`
}

type deliveryPlanFingerprintInput struct {
	SourceModule                string                       `json:"source_module"`
	EventID                     string                       `json:"event_id"`
	SubscriptionID              string                       `json:"subscription_id"`
	SubscriptionVersion         int64                        `json:"subscription_version"`
	ChannelRef                  string                       `json:"channel_ref"`
	ChannelConfigVersion        int64                        `json:"channel_config_version"`
	TemplateID                  string                       `json:"template_id"`
	TemplateVersion             int64                        `json:"template_version"`
	RecipientRef                string                       `json:"recipient_ref"`
	PayloadFingerprint          string                       `json:"payload_fingerprint"`
	AuthorizationPolicyVersions []AuthorizationPolicyVersion `json:"authorization_policy_versions"`
}

func (p DeliveryPlan) Validate() error {
	if p.SourceModule == "" || p.EventID == "" || p.SubscriptionID == "" ||
		p.SubscriptionVersion < 1 || p.ChannelRef == "" || p.ChannelConfigVersion < 1 ||
		p.TemplateID == "" || p.TemplateVersion < 1 || p.RecipientRef == "" ||
		p.DeliveryID == "" || p.DeliveryIdempotencyKey == "" {
		return ErrInvalidDeliveryPlan
	}
	if len(p.PayloadFingerprint) != sha256.Size*2 ||
		strings.ToLower(p.PayloadFingerprint) != p.PayloadFingerprint {
		return fmt.Errorf("%w: invalid payload fingerprint", ErrInvalidDeliveryPlan)
	}
	if _, err := hex.DecodeString(p.PayloadFingerprint); err != nil {
		return fmt.Errorf("%w: invalid payload fingerprint", ErrInvalidDeliveryPlan)
	}
	seenPolicies := make(map[AuthorizationPolicyVersion]struct{}, len(p.AuthorizationPolicyVersions))
	for _, policy := range p.AuthorizationPolicyVersions {
		if !validPolicyValue(policy.PolicyType, 128) ||
			!validPolicyValue(policy.PolicyRef, 256) ||
			!validPolicyValue(policy.PolicyVersion, 128) {
			return fmt.Errorf("%w: incomplete authorization policy version tuple", ErrInvalidDeliveryPlan)
		}
		if _, exists := seenPolicies[policy]; exists {
			return fmt.Errorf("%w: duplicate authorization policy version tuple", ErrInvalidDeliveryPlan)
		}
		seenPolicies[policy] = struct{}{}
	}
	if len(p.AuthorizationRefs) > 0 && len(p.AuthorizationPolicyVersions) == 0 {
		return fmt.Errorf("%w: authorization references require evaluated policy versions", ErrInvalidDeliveryPlan)
	}
	seenRefs := make(map[string]struct{}, len(p.AuthorizationRefs))
	for _, ref := range p.AuthorizationRefs {
		if !validPolicyValue(ref, 256) {
			return fmt.Errorf("%w: invalid authorization reference", ErrInvalidDeliveryPlan)
		}
		if _, exists := seenRefs[ref]; exists {
			return fmt.Errorf("%w: duplicate authorization reference", ErrInvalidDeliveryPlan)
		}
		seenRefs[ref] = struct{}{}
	}
	return nil
}

func validPolicyValue(value string, maxBytes int) bool {
	if value == "" || len(value) > maxBytes || value != strings.TrimSpace(value) || !utf8.ValidString(value) {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func (p DeliveryPlan) CalculateFingerprint() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	versions := append([]AuthorizationPolicyVersion(nil), p.AuthorizationPolicyVersions...)
	sort.Slice(versions, func(i, j int) bool {
		if versions[i].PolicyType != versions[j].PolicyType {
			return versions[i].PolicyType < versions[j].PolicyType
		}
		if versions[i].PolicyRef != versions[j].PolicyRef {
			return versions[i].PolicyRef < versions[j].PolicyRef
		}
		return versions[i].PolicyVersion < versions[j].PolicyVersion
	})
	input := deliveryPlanFingerprintInput{
		SourceModule: p.SourceModule, EventID: p.EventID,
		SubscriptionID: p.SubscriptionID, SubscriptionVersion: p.SubscriptionVersion,
		ChannelRef: p.ChannelRef, ChannelConfigVersion: p.ChannelConfigVersion,
		TemplateID: p.TemplateID, TemplateVersion: p.TemplateVersion,
		RecipientRef: p.RecipientRef, PayloadFingerprint: p.PayloadFingerprint,
		AuthorizationPolicyVersions: versions,
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return "", fmt.Errorf("notification: encode delivery plan fingerprint: %w", err)
	}
	canonical, err := jcs.Transform(encoded)
	if err != nil {
		return "", fmt.Errorf("notification: canonicalize delivery plan fingerprint: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func (p DeliveryPlan) VerifyFingerprint() error {
	calculated, err := p.CalculateFingerprint()
	if err != nil {
		return err
	}
	if p.PlanFingerprint != calculated {
		return fmt.Errorf("%w: plan fingerprint mismatch", ErrInvalidDeliveryPlan)
	}
	return nil
}

// SameUniqueKey compares the immutable Delivery uniqueness tuple from the contract.
func (p DeliveryPlan) SameUniqueKey(other DeliveryPlan) bool {
	return p.SourceModule == other.SourceModule &&
		p.EventID == other.EventID &&
		p.SubscriptionID == other.SubscriptionID &&
		p.SubscriptionVersion == other.SubscriptionVersion &&
		p.ChannelRef == other.ChannelRef &&
		p.ChannelConfigVersion == other.ChannelConfigVersion &&
		p.TemplateID == other.TemplateID &&
		p.TemplateVersion == other.TemplateVersion &&
		p.RecipientRef == other.RecipientRef
}

var ErrDeliveryPlanConflict = errors.New("notification: delivery plan conflict")

// ValidateDeliveryPlanReplay accepts a duplicate only when the immutable unique tuple
// and its full plan fingerprint are identical.
func ValidateDeliveryPlanReplay(existing, incoming DeliveryPlan) error {
	if !existing.SameUniqueKey(incoming) || existing.PlanFingerprint != incoming.PlanFingerprint {
		return ErrDeliveryPlanConflict
	}
	return nil
}
