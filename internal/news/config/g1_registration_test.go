package config

import (
	"strings"
	"testing"
)

const g1FeedSourcesJSON = `[
	{
		"source_id": "cninfo",
		"endpoint_id": "cninfo-his-announcement",
		"url": "http://www.cninfo.com.cn/new/hisAnnouncement/query",
		"publisher_name": "巨潮资讯网",
		"publisher_domain": "cninfo.com.cn",
		"access_method": "official_api",
		"source_policy_version_id": "cninfo-source-policy-v1",
		"endpoint_policy_version_id": "cninfo-endpoint-policy-v1",
		"provider_code": "cninfo",
		"provider_output_policy_ref": "cninfo-provider-output-policy",
		"provider_output_policy_version_id": "cninfo-provider-output-policy-v1",
		"request_params": {
			"pageNum": "1",
			"pageSize": "30",
			"column": "szse",
			"tabName": "fulltext",
			"stock": "600036,gssh0600036",
			"seDate": "2026-10-01~2026-11-27",
			"isHLtitle": "true"
		},
		"source_policy": {
			"status": "approved",
			"allowed_access_methods": ["official_api"],
			"allowed_purposes": ["internal_personal_research"],
			"allowed_content_kinds": ["abstract", "metadata"],
			"allowed_storage_modes": ["transient", "persistent_excerpt", "persistent_metadata"],
			"allowed_metadata_fields": ["announcements.announcementTime"],
			"max_retention_seconds": 86400,
			"effective_at": "2026-11-27T00:00:00Z",
			"evidence_ref": "personal-use:cninfo"
		},
		"endpoint_policy": {
			"status": "approved",
			"allowed_access_methods": ["official_api"],
			"allowed_purposes": ["internal_personal_research"],
			"allowed_content_kinds": ["abstract", "metadata"],
			"allowed_storage_modes": ["transient", "persistent_excerpt", "persistent_metadata"],
			"allowed_metadata_fields": ["announcements.announcementTime"],
			"max_retention_seconds": 86400,
			"effective_at": "2026-11-27T00:00:00Z",
			"evidence_ref": "personal-use:cninfo-his-announcement"
		},
		"url_policy": {
			"allowed_schemes": ["http"],
			"allowed_hosts": ["www.cninfo.com.cn"],
			"allowed_ports": [80]
		},
		"max_response_bytes": 1048576
	}
]`

const g1ProviderPoliciesJSON = `[
	{
		"provider_code": "cninfo",
		"policy_ref": "cninfo-provider-output-policy",
		"policy_version": "cninfo-provider-output-policy-v1",
		"policy": {
			"status": "approved",
			"allowed_purposes": ["internal_personal_research"],
			"allowed_content_kinds": ["abstract", "metadata"],
			"allow_request_processing": true,
			"allow_cross_request_cache": false,
			"max_request_retention_seconds": 86400,
			"max_cache_retention_seconds": 0,
			"allowed_storage_modes": ["persistent_excerpt", "persistent_metadata"],
			"allowed_metadata_fields": ["announcements.announcementTime"],
			"max_persistence_retention_seconds": 86400,
			"effective_at": "2026-11-27T00:00:00Z",
			"evidence_ref": "personal-use:cninfo-provider-output"
		}
	}
]`

const g1QualificationsJSON = `[
	{
		"qualification_ref": "availability-qualification-cninfo-disclosure-v1",
		"source_id": "cninfo",
		"endpoint_id": "cninfo-his-announcement",
		"provider_code": "cninfo",
		"source_policy_version_id": "cninfo-source-policy-v1",
		"endpoint_policy_version_id": "cninfo-endpoint-policy-v1",
		"provider_output_policy_ref": "cninfo-provider-output-policy",
		"provider_output_policy_version_id": "cninfo-provider-output-policy-v1",
		"field_path": "announcements.announcementTime",
		"parser_version": "cninfo-disclosure-date-conservative-v1",
		"review_rule_version": "cninfo-disclosure-review-v1",
		"declared_timezone_or_offset": "+08:00",
		"allowed_precisions": ["disclosure_boundary"],
		"official_semantics_ref": "CSRC指定法定披露渠道：announcementTime为披露日",
		"date_boundary_semantics_ref": "DEC-PLAT-002-073:disclosure-date-end-of-day-v1",
		"endpoint_authorization_ref": "personal-use:cninfo-official-api",
		"version_binding_ref": "NEWS-CTR-001 1.19.1 sample ecd135102d8d1b2726c6348835df76fa4b90f0b1cdaf9e1f56765bbce9c30d01",
		"raw_response_sample_hash": "ecd135102d8d1b2726c6348835df76fa4b90f0b1cdaf9e1f56765bbce9c30d01",
		"status": "qualified",
		"effective_at": "2026-11-27T00:00:00Z"
	}
]`

func g1Env() map[string]string {
	return map[string]string{
		FeedSourcesEnv:                g1FeedSourcesJSON,
		ProviderOutputPoliciesEnv:     g1ProviderPoliciesJSON,
		AvailabilityQualificationsEnv: g1QualificationsJSON,
	}
}

func TestLoadG1RegistrationsFromTrustedConfig(t *testing.T) {
	cfg, err := Load(env(g1Env()))
	if err != nil {
		t.Fatal(err)
	}
	sources := cfg.FeedSources()
	if len(sources) != 1 {
		t.Fatalf("feed sources=%d want 1", len(sources))
	}
	source := sources[0]
	if source.EndpointID != "cninfo-his-announcement" || source.EndpointPolicy == nil ||
		source.EndpointPolicyVersionID != "cninfo-endpoint-policy-v1" {
		t.Fatalf("endpoint policy registration missing: %+v", source)
	}
	if source.ProviderCode != "cninfo" ||
		source.ProviderOutputPolicyRef != "cninfo-provider-output-policy" ||
		source.ProviderOutputPolicyVersionID != "cninfo-provider-output-policy-v1" {
		t.Fatalf("provider policy binding missing: %+v", source)
	}
	policies := cfg.ProviderOutputPolicies()
	if len(policies) != 1 || policies[0].PolicyRef != "cninfo-provider-output-policy" {
		t.Fatalf("provider policies=%+v", policies)
	}
	qualifications := cfg.AvailabilityQualifications()
	if len(qualifications) != 1 || qualifications[0].QualificationRef != "availability-qualification-cninfo-disclosure-v1" {
		t.Fatalf("qualifications=%+v", qualifications)
	}
	if err := qualifications[0].Validate(); err != nil {
		t.Fatalf("loaded qualification invalid: %v", err)
	}
}

func TestLoadG1RejectsQualificationWithoutMatchingSource(t *testing.T) {
	values := g1Env()
	values[AvailabilityQualificationsEnv] = strings.Replace(
		g1QualificationsJSON, `"source_id": "cninfo"`, `"source_id": "unknown-source"`, 1,
	)
	if _, err := Load(env(values)); err == nil {
		t.Fatal("qualification without a matching registered source must be rejected")
	}
}

func TestLoadG1RejectsProviderPolicyBindingMismatch(t *testing.T) {
	values := g1Env()
	values[ProviderOutputPoliciesEnv] = strings.Replace(
		g1ProviderPoliciesJSON, `"policy_version": "cninfo-provider-output-policy-v1"`,
		`"policy_version": "cninfo-provider-output-policy-v2"`, 1,
	)
	if _, err := Load(env(values)); err == nil {
		t.Fatal("provider policy version mismatch must be rejected")
	}
}

func TestLoadG1RequiresEndpointPolicyVersion(t *testing.T) {
	values := g1Env()
	values[FeedSourcesEnv] = strings.Replace(
		g1FeedSourcesJSON, `"endpoint_policy_version_id": "cninfo-endpoint-policy-v1",`, "", 1,
	)
	if _, err := Load(env(values)); err == nil {
		t.Fatal("endpoint policy without a version must be rejected")
	}
}

func TestLoadG1ReturnsDefensiveQualificationCopy(t *testing.T) {
	cfg, err := Load(env(g1Env()))
	if err != nil {
		t.Fatal(err)
	}
	qualifications := cfg.AvailabilityQualifications()
	qualifications[0].AllowedPrecisions[0] = "second"
	again := cfg.AvailabilityQualifications()
	if again[0].AllowedPrecisions[0] != "disclosure_boundary" {
		t.Fatal("caller mutation changed immutable qualification snapshot")
	}
}
