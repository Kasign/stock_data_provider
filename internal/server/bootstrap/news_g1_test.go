package bootstrap

import (
	"context"
	"testing"

	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
)

func g1BootstrapValues() map[string]string {
	return map[string]string{
		newsconfig.FeedSourcesEnv: `[
			{
				"source_id":"cninfo","endpoint_id":"cninfo-his-announcement",
				"url":"http://www.cninfo.com.cn/new/hisAnnouncement/query",
				"publisher_name":"巨潮资讯网","publisher_domain":"cninfo.com.cn",
				"access_method":"official_api",
				"source_policy_version_id":"cninfo-source-policy-v1",
				"endpoint_policy_version_id":"cninfo-endpoint-policy-v1",
				"provider_code":"cninfo",
				"provider_output_policy_ref":"cninfo-provider-output-policy",
				"provider_output_policy_version_id":"cninfo-provider-output-policy-v1",
				"request_params":{"pageNum":"1","stock":"600036,gssh0600036","seDate":"2026-10-01~2026-11-27"},
				"source_policy":{"status":"approved","allowed_access_methods":["official_api"],"allowed_purposes":["internal_personal_research"],"allowed_content_kinds":["abstract","metadata"],"allowed_storage_modes":["transient","persistent_excerpt","persistent_metadata"],"allowed_metadata_fields":["announcements.announcementTime"],"max_retention_seconds":86400,"effective_at":"2026-11-27T00:00:00Z","evidence_ref":"personal-use:cninfo"},
				"endpoint_policy":{"status":"approved","allowed_access_methods":["official_api"],"allowed_purposes":["internal_personal_research"],"allowed_content_kinds":["abstract","metadata"],"allowed_storage_modes":["transient","persistent_excerpt","persistent_metadata"],"allowed_metadata_fields":["announcements.announcementTime"],"max_retention_seconds":86400,"effective_at":"2026-11-27T00:00:00Z","evidence_ref":"personal-use:cninfo-endpoint"},
				"url_policy":{"allowed_schemes":["http"],"allowed_hosts":["www.cninfo.com.cn"],"allowed_ports":[80]},
				"max_response_bytes":1048576
			}
		]`,
		newsconfig.ProviderOutputPoliciesEnv: `[
			{"provider_code":"cninfo","policy_ref":"cninfo-provider-output-policy","policy_version":"cninfo-provider-output-policy-v1","policy":{"status":"approved","allowed_purposes":["internal_personal_research"],"allowed_content_kinds":["abstract","metadata"],"allow_request_processing":true,"allowed_storage_modes":["persistent_excerpt","persistent_metadata"],"allowed_metadata_fields":["announcements.announcementTime"],"max_request_retention_seconds":86400,"max_persistence_retention_seconds":86400,"effective_at":"2026-11-27T00:00:00Z","evidence_ref":"personal-use:cninfo-provider-output"}}
		]`,
		newsconfig.AvailabilityQualificationsEnv: `[
			{"qualification_ref":"availability-qualification-cninfo-disclosure-v1","source_id":"cninfo","endpoint_id":"cninfo-his-announcement","provider_code":"cninfo","source_policy_version_id":"cninfo-source-policy-v1","endpoint_policy_version_id":"cninfo-endpoint-policy-v1","provider_output_policy_ref":"cninfo-provider-output-policy","provider_output_policy_version_id":"cninfo-provider-output-policy-v1","field_path":"announcements.announcementTime","parser_version":"cninfo-disclosure-date-conservative-v1","review_rule_version":"cninfo-disclosure-review-v1","declared_timezone_or_offset":"+08:00","allowed_precisions":["disclosure_boundary"],"official_semantics_ref":"CSRC指定法定披露渠道：announcementTime为披露日","date_boundary_semantics_ref":"DEC-PLAT-002-073:disclosure-date-end-of-day-v1","endpoint_authorization_ref":"personal-use:cninfo-official-api","version_binding_ref":"NEWS-CTR-001 1.19.1 sample","raw_response_sample_hash":"ecd135102d8d1b2726c6348835df76fa4b90f0b1cdaf9e1f56765bbce9c30d01","status":"qualified","effective_at":"2026-11-27T00:00:00Z"}
		]`,
	}
}

func g1BootstrapConfig(t *testing.T) newsconfig.Config {
	t.Helper()
	values := g1BootstrapValues()
	cfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestNewsFeedRegistrationsPropagateG1PolicyBindings(t *testing.T) {
	cfg := g1BootstrapConfig(t)
	registrations := newsFeedRegistrations(cfg.FeedSources())
	if len(registrations) != 1 {
		t.Fatalf("registrations=%d want 1", len(registrations))
	}
	got := registrations[0]
	if got.EndpointPolicy == nil || got.EndpointPolicyVersionID != "cninfo-endpoint-policy-v1" {
		t.Fatalf("endpoint policy not propagated: %+v", got)
	}
	if got.ProviderCode != "cninfo" ||
		got.ProviderOutputPolicyRef != "cninfo-provider-output-policy" ||
		got.ProviderOutputPolicyVersionID != "cninfo-provider-output-policy-v1" {
		t.Fatalf("provider policy binding not propagated: %+v", got)
	}
}

func TestNewsFeedPolicyPublicationsCoverSourceEndpointAndProvider(t *testing.T) {
	cfg := g1BootstrapConfig(t)
	publications, err := newsFeedPolicyPublications(cfg.FeedSources(), cfg.ProviderOutputPolicies())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		newsdomain.NewsPolicyTypeSource + "/cninfo":                                "cninfo-source-policy-v1",
		newsdomain.NewsPolicyTypeEndpoint + "/cninfo-his-announcement":             "cninfo-endpoint-policy-v1",
		newsdomain.NewsPolicyTypeProviderOutput + "/cninfo-provider-output-policy": "cninfo-provider-output-policy-v1",
	}
	got := make(map[string]string, len(publications))
	for _, publication := range publications {
		got[publication.PolicyType+"/"+publication.PolicyRef] = publication.PolicyVersion
	}
	for key, version := range want {
		if got[key] != version {
			t.Fatalf("publications=%v, want %s=%s", got, key, version)
		}
	}
}

func TestNewsQualificationFieldPathsAreStable(t *testing.T) {
	cfg := g1BootstrapConfig(t)
	got := newsQualificationFieldPaths(cfg.AvailabilityQualifications())
	want := []string{"announcements.announcementTime"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("field paths=%v want %v", got, want)
	}
}

func TestNewsFeedCompositionAssemblesG4AvailabilityRecorder(t *testing.T) {
	cfg := g1BootstrapConfig(t)
	composition, err := newNewsFeedStageComposition(
		context.Background(), cfg, &platformhttpclient.Client{}, &newsstorage.MySQLRepository{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if composition == nil || composition.availabilityQualifier == nil || composition.availabilityRecorder == nil {
		t.Fatalf("G3 qualifier/G4 recorder not assembled: %+v", composition)
	}
}

func TestValidateNewsG1BindingsRejectsOrphanQualification(t *testing.T) {
	cfg := g1BootstrapConfig(t)
	registrations := newsFeedRegistrations(cfg.FeedSources())
	bindings := newsProviderPolicyBindings(cfg.ProviderOutputPolicies())
	qualifications := cfg.AvailabilityQualifications()
	if err := validateNewsG1Bindings(registrations, bindings, qualifications); err != nil {
		t.Fatal(err)
	}
	qualifications[0].SourceID = "orphan"
	if err := validateNewsG1Bindings(registrations, bindings, qualifications); err == nil {
		t.Fatal("orphan qualification must be rejected by composition binding validation")
	}
}
