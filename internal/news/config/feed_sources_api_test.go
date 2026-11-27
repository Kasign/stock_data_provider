package config

import (
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func validOfficialAPISourceJSON() string {
	return `[
		{
			"source_id": "cninfo",
			"endpoint_id": "announcements",
			"url": "https://www.cninfo.com.cn/new/hisAnnouncement/query",
			"publisher_name": "巨潮资讯网",
			"publisher_domain": "cninfo.com.cn",
			"access_method": "official_api",
			"source_policy_version_id": "cninfo-source-policy-v1",
			"request_params": {
				"stock": "600036,gssh0600036",
				"seDate": "2026-10-01~2026-11-27"
			},
			"source_policy": {
				"status": "approved",
				"allowed_access_methods": ["official_api"],
				"allowed_purposes": ["internal_personal_research"],
				"allowed_content_kinds": ["abstract"],
				"allowed_storage_modes": ["transient", "persistent_excerpt"],
				"allowed_metadata_fields": [],
				"max_retention_seconds": 86400,
				"effective_at": "2026-01-01T00:00:00Z",
				"evidence_ref": "personal-use:cninfo"
			},
			"url_policy": {
				"allowed_schemes": ["https"],
				"allowed_hosts": ["www.cninfo.com.cn"],
				"allowed_ports": [443]
			},
			"max_response_bytes": 1048576
		}
	]`
}

func TestLoadFeedSourcesAcceptsOfficialAPIWithFixedParams(t *testing.T) {
	cfg, err := Load(env(map[string]string{FeedSourcesEnv: validOfficialAPISourceJSON()}))
	if err != nil {
		t.Fatal(err)
	}
	sources := cfg.FeedSources()
	if len(sources) != 1 {
		t.Fatalf("sources=%d want 1", len(sources))
	}
	got := sources[0]
	if got.AccessMethod != domain.AccessOfficialAPI {
		t.Fatalf("access_method=%q", got.AccessMethod)
	}
	if got.RequestParams["stock"] != "600036,gssh0600036" || got.RequestParams["seDate"] != "2026-10-01~2026-11-27" {
		t.Fatalf("request_params=%v", got.RequestParams)
	}
	// defensive copy：调用方改动不得影响内部快照。
	got.RequestParams["stock"] = "injected"
	if again := cfg.FeedSources(); again[0].RequestParams["stock"] != "600036,gssh0600036" {
		t.Fatal("caller mutation changed immutable request_params")
	}
}

func TestLoadFeedSourcesRejectsOfficialAPIWithoutParams(t *testing.T) {
	payload := strings.Replace(validOfficialAPISourceJSON(),
		`"request_params": {
				"stock": "600036,gssh0600036",
				"seDate": "2026-10-01~2026-11-27"
			},`, "", 1)
	_, err := Load(env(map[string]string{FeedSourcesEnv: payload}))
	if err == nil {
		t.Fatal("official_api source without request_params must be rejected")
	}
}
