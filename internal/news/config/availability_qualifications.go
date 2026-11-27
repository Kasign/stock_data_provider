package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

const (
	AvailabilityQualificationsEnv = "NEWS_AVAILABILITY_QUALIFICATIONS_JSON"
	maximumQualifications         = 128
)

type availabilityQualificationJSON struct {
	QualificationRef              string                                    `json:"qualification_ref"`
	SourceID                      string                                    `json:"source_id"`
	EndpointID                    string                                    `json:"endpoint_id"`
	ProviderCode                  string                                    `json:"provider_code"`
	SourcePolicyVersionID         string                                    `json:"source_policy_version_id"`
	EndpointPolicyVersionID       string                                    `json:"endpoint_policy_version_id"`
	ProviderOutputPolicyRef       string                                    `json:"provider_output_policy_ref"`
	ProviderOutputPolicyVersionID string                                    `json:"provider_output_policy_version_id"`
	FieldPath                     string                                    `json:"field_path"`
	ParserVersion                 string                                    `json:"parser_version"`
	ReviewRuleVersion             string                                    `json:"review_rule_version"`
	DeclaredTimezoneOrOffset      string                                    `json:"declared_timezone_or_offset"`
	AllowedPrecisions             []domain.AvailabilityTimePrecision        `json:"allowed_precisions"`
	OfficialSemanticsRef          string                                    `json:"official_semantics_ref"`
	DateBoundarySemanticsRef      string                                    `json:"date_boundary_semantics_ref"`
	EndpointAuthorizationRef      string                                    `json:"endpoint_authorization_ref"`
	VersionBindingRef             string                                    `json:"version_binding_ref"`
	RawResponseSampleHash         string                                    `json:"raw_response_sample_hash"`
	Status                        app.AvailabilitySourceQualificationStatus `json:"status"`
	EffectiveAt                   time.Time                                 `json:"effective_at"`
	RevokedAt                     *time.Time                                `json:"revoked_at"`
}

func (c Config) AvailabilityQualifications() []app.AvailabilitySourceQualification {
	out := make([]app.AvailabilitySourceQualification, len(c.availabilityQualifications))
	for i, value := range c.availabilityQualifications {
		out[i] = value.Clone()
	}
	return out
}

func loadAvailabilityQualifications(env platformconfig.Env) ([]app.AvailabilitySourceQualification, error) {
	raw := strings.TrimSpace(env.String(AvailabilityQualificationsEnv, ""))
	if raw == "" {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var entries []availabilityQualificationJSON
	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("news config %s must be a valid JSON array", AvailabilityQualificationsEnv)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("news config %s must contain exactly one JSON value", AvailabilityQualificationsEnv)
	}
	if len(entries) == 0 || len(entries) > maximumQualifications {
		return nil, fmt.Errorf("news config %s must contain 1..%d qualifications", AvailabilityQualificationsEnv, maximumQualifications)
	}
	result := make([]app.AvailabilitySourceQualification, 0, len(entries))
	for _, entry := range entries {
		value := app.AvailabilitySourceQualification{
			QualificationRef:              strings.TrimSpace(entry.QualificationRef),
			SourceID:                      strings.TrimSpace(entry.SourceID),
			EndpointID:                    strings.TrimSpace(entry.EndpointID),
			ProviderCode:                  strings.TrimSpace(entry.ProviderCode),
			SourcePolicyVersionID:         strings.TrimSpace(entry.SourcePolicyVersionID),
			EndpointPolicyVersionID:       strings.TrimSpace(entry.EndpointPolicyVersionID),
			ProviderOutputPolicyRef:       strings.TrimSpace(entry.ProviderOutputPolicyRef),
			ProviderOutputPolicyVersionID: strings.TrimSpace(entry.ProviderOutputPolicyVersionID),
			FieldPath:                     strings.TrimSpace(entry.FieldPath),
			ParserVersion:                 strings.TrimSpace(entry.ParserVersion),
			ReviewRuleVersion:             strings.TrimSpace(entry.ReviewRuleVersion),
			DeclaredTimezoneOrOffset:      strings.TrimSpace(entry.DeclaredTimezoneOrOffset),
			AllowedPrecisions:             append([]domain.AvailabilityTimePrecision(nil), entry.AllowedPrecisions...),
			OfficialSemanticsRef:          strings.TrimSpace(entry.OfficialSemanticsRef),
			DateBoundarySemanticsRef:      strings.TrimSpace(entry.DateBoundarySemanticsRef),
			EndpointAuthorizationRef:      strings.TrimSpace(entry.EndpointAuthorizationRef),
			VersionBindingRef:             strings.TrimSpace(entry.VersionBindingRef),
			RawResponseSampleHash:         strings.TrimSpace(entry.RawResponseSampleHash),
			Status:                        entry.Status,
			EffectiveAt:                   entry.EffectiveAt.UTC(),
			RevokedAt:                     cloneTimeUTC(entry.RevokedAt),
		}
		if err := value.Validate(); err != nil {
			return nil, fmt.Errorf("news config %s contains an invalid qualification: %w", AvailabilityQualificationsEnv, err)
		}
		result = append(result, value.Clone())
	}
	return result, nil
}

func validateQualificationBindings(
	sources []FeedSource,
	policies []ProviderOutputPolicyRegistration,
	qualifications []app.AvailabilitySourceQualification,
) error {
	for _, qualification := range qualifications {
		var matched *FeedSource
		for index := range sources {
			source := &sources[index]
			if source.SourceID == qualification.SourceID &&
				source.EndpointID == qualification.EndpointID &&
				source.SourcePolicyVersionID == qualification.SourcePolicyVersionID &&
				source.EndpointPolicyVersionID == qualification.EndpointPolicyVersionID &&
				source.ProviderCode == qualification.ProviderCode &&
				source.ProviderOutputPolicyRef == qualification.ProviderOutputPolicyRef &&
				source.ProviderOutputPolicyVersionID == qualification.ProviderOutputPolicyVersionID {
				matched = source
				break
			}
		}
		if matched == nil || matched.EndpointPolicy == nil {
			return fmt.Errorf("news config %s qualification %q has no matching registered source/endpoint/provider binding",
				AvailabilityQualificationsEnv, qualification.QualificationRef)
		}
		if !containsMetadataField(matched.SourcePolicy.AllowedMetadataFields, qualification.FieldPath) ||
			!containsMetadataField(matched.EndpointPolicy.AllowedMetadataFields, qualification.FieldPath) {
			return fmt.Errorf("news config %s qualification %q field path is not authorized by source and endpoint policies",
				AvailabilityQualificationsEnv, qualification.QualificationRef)
		}
		if qualification.ProviderCode != "" {
			providerMatched := false
			for _, policy := range policies {
				if policy.ProviderCode == qualification.ProviderCode &&
					policy.PolicyRef == qualification.ProviderOutputPolicyRef &&
					policy.PolicyVersion == qualification.ProviderOutputPolicyVersionID {
					providerMatched = containsMetadataField(policy.Policy.AllowedMetadataFields, qualification.FieldPath)
					break
				}
			}
			if !providerMatched {
				return fmt.Errorf("news config %s qualification %q provider policy does not authorize its field path",
					AvailabilityQualificationsEnv, qualification.QualificationRef)
			}
		}
	}
	return nil
}

func containsMetadataField(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func cloneTimeUTC(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	utc := value.UTC()
	return &utc
}
