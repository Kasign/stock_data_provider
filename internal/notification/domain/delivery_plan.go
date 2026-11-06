package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gowebpki/jcs"
)

var ErrInvalidDeliveryPlan = errors.New("notification: invalid delivery plan")

type DeliveryPlan struct {
	SourceModule                string   `json:"source_module"`
	EventID                     string   `json:"event_id"`
	SubscriptionID              string   `json:"subscription_id"`
	SubscriptionVersion         int64    `json:"subscription_version"`
	ChannelRef                  string   `json:"channel_ref"`
	ChannelConfigVersion        int64    `json:"channel_config_version"`
	TemplateID                  string   `json:"template_id"`
	TemplateVersion             int64    `json:"template_version"`
	RecipientRef                string   `json:"recipient_ref"`
	PayloadFingerprint          string   `json:"payload_fingerprint"`
	AuthorizationPolicyVersions []string `json:"authorization_policy_versions"`
	AuthorizationRefs           []string `json:"authorization_refs,omitempty"`
	PlanFingerprint             string   `json:"plan_fingerprint"`
	DeliveryID                  string   `json:"delivery_id"`
	DeliveryIdempotencyKey      string   `json:"delivery_idempotency_key"`
}

type deliveryPlanFingerprintInput struct {
	SourceModule                string   `json:"source_module"`
	EventID                     string   `json:"event_id"`
	SubscriptionID              string   `json:"subscription_id"`
	SubscriptionVersion         int64    `json:"subscription_version"`
	ChannelRef                  string   `json:"channel_ref"`
	ChannelConfigVersion        int64    `json:"channel_config_version"`
	TemplateID                  string   `json:"template_id"`
	TemplateVersion             int64    `json:"template_version"`
	RecipientRef                string   `json:"recipient_ref"`
	PayloadFingerprint          string   `json:"payload_fingerprint"`
	AuthorizationPolicyVersions []string `json:"authorization_policy_versions"`
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
	seen := make(map[string]struct{}, len(p.AuthorizationPolicyVersions))
	for _, version := range p.AuthorizationPolicyVersions {
		if version == "" {
			return fmt.Errorf("%w: empty authorization policy version", ErrInvalidDeliveryPlan)
		}
		if _, exists := seen[version]; exists {
			return fmt.Errorf("%w: duplicate authorization policy version", ErrInvalidDeliveryPlan)
		}
		seen[version] = struct{}{}
	}
	for _, ref := range p.AuthorizationRefs {
		if ref == "" {
			return fmt.Errorf("%w: empty authorization reference", ErrInvalidDeliveryPlan)
		}
	}
	return nil
}

func (p DeliveryPlan) CalculateFingerprint() (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	versions := append([]string(nil), p.AuthorizationPolicyVersions...)
	sort.Strings(versions)
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
