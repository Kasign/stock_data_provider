package config

import (
	"encoding/json"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func validFeedSourceJSON() string {
	return `[
		{
			"source_id": "trusted-finance-feed",
			"endpoint_id": "primary",
			"url": "https://feed.example.com/rss.xml",
			"publisher_name": "示例财经",
			"publisher_domain": "example.com",
			"access_method": "rss",
			"source_policy_version_id": "source-policy-v1",
			"source_policy": {
				"status": "approved",
				"allowed_access_methods": ["rss"],
				"allowed_purposes": ["internal_personal_research"],
				"allowed_content_kinds": ["abstract"],
				"allowed_storage_modes": ["transient", "persistent_excerpt"],
				"allowed_metadata_fields": [],
				"max_retention_seconds": 86400,
				"effective_at": "2026-01-01T00:00:00Z",
				"evidence_ref": "operator-authorization:trusted-finance-feed"
			},
			"url_policy": {
				"allowed_schemes": ["https"],
				"allowed_hosts": ["feed.example.com"],
				"allowed_ports": [443]
			},
			"max_response_bytes": 1048576
		}
	]`
}

func TestLoadFeedSourcesIsOptionalAndDefensivelyCopied(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := cfg.FeedSources(); len(got) != 0 {
		t.Fatalf("FeedSources() = %d, want 0 when unset", len(got))
	}

	cfg, err = Load(env(map[string]string{FeedSourcesEnv: validFeedSourceJSON()}))
	if err != nil {
		t.Fatalf("Load() configured feed error = %v", err)
	}
	sources := cfg.FeedSources()
	if len(sources) != 1 {
		t.Fatalf("FeedSources() = %d, want 1", len(sources))
	}
	got := sources[0]
	if got.SourceID != "trusted-finance-feed" || got.EndpointID != "primary" ||
		got.AccessMethod != domain.AccessRSS || got.SourcePolicyVersionID != "source-policy-v1" ||
		got.SourcePolicy.MaxRetention.Seconds() != 86400 || got.MaxResponseBytes != 1048576 {
		t.Fatalf("unexpected feed source snapshot: %+v", got)
	}

	sources[0].SourcePolicy.AllowedPurposes[0] = domain.PurposeRedistribution
	sources[0].URLPolicy.AllowedHosts[0] = "attacker.example"
	if again := cfg.FeedSources(); again[0].SourcePolicy.AllowedPurposes[0] != domain.PurposeInternalPersonalResearch ||
		again[0].URLPolicy.AllowedHosts[0] != "feed.example.com" {
		t.Fatal("caller mutation changed the immutable Feed source config snapshot")
	}
}

func TestLoadFeedSourcesRejectsInvalidOrAmbiguousConfiguration(t *testing.T) {
	tests := []struct {
		name string
		mutate func(map[string]any)
	}{
		{name: "empty array", mutate: func(v map[string]any) { v["entries"] = []any{} }},
		{name: "duplicate endpoint", mutate: func(v map[string]any) {
			entries := v["entries"].([]any)
			v["entries"] = append(entries, entries[0])
		}},
		{name: "unsupported access method", mutate: func(v map[string]any) {
			v["entries"].([]any)[0].(map[string]any)["access_method"] = "scripted_web"
		}},
		{name: "URL outside allowlist", mutate: func(v map[string]any) {
			v["entries"].([]any)[0].(map[string]any)["url"] = "https://other.example/rss.xml"
		}},
		{name: "policy denies persistent excerpt", mutate: func(v map[string]any) {
			policy := v["entries"].([]any)[0].(map[string]any)["source_policy"].(map[string]any)
			policy["allowed_storage_modes"] = []any{"transient"}
		}},
		{name: "missing authorization evidence", mutate: func(v map[string]any) {
			policy := v["entries"].([]any)[0].(map[string]any)["source_policy"].(map[string]any)
			policy["evidence_ref"] = ""
		}},
		{name: "negative response limit", mutate: func(v map[string]any) {
			v["entries"].([]any)[0].(map[string]any)["max_response_bytes"] = -1
		}},
		{name: "unknown field", mutate: func(v map[string]any) {
			v["entries"].([]any)[0].(map[string]any)["arbitrary_url"] = "https://attacker.example/"
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var document map[string]any
			if err := json.Unmarshal([]byte(`{"entries":`+validFeedSourceJSON()+`}`), &document); err != nil {
				t.Fatal(err)
			}
			test.mutate(document)
			raw, err := json.Marshal(document["entries"])
			if err != nil {
				t.Fatal(err)
			}
			_, err = Load(env(map[string]string{FeedSourcesEnv: string(raw)}))
			if err == nil {
				t.Fatal("Load() error = nil, want invalid Feed configuration")
			}
			if strings.Contains(err.Error(), "attacker.example") || strings.Contains(err.Error(), "feed.example.com") {
				t.Fatalf("configuration error leaked endpoint value: %v", err)
			}
		})
	}
}

func TestLoadFeedSourcesRejectsConflictingPolicySnapshotsForSameSource(t *testing.T) {
	var entries []map[string]any
	if err := json.Unmarshal([]byte(validFeedSourceJSON()), &entries); err != nil {
		t.Fatal(err)
	}
	second := map[string]any{}
	for key, value := range entries[0] {
		second[key] = value
	}
	second["endpoint_id"] = "secondary"
	policy := map[string]any{}
	for key, value := range entries[0]["source_policy"].(map[string]any) {
		policy[key] = value
	}
	policy["evidence_ref"] = "different-policy-evidence"
	second["source_policy"] = policy
	entries = append(entries, second)
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(env(map[string]string{FeedSourcesEnv: string(raw)})); err == nil {
		t.Fatal("Load() accepted conflicting policy payloads for one source")
	}
}

func TestLoadFeedSourcesRejectsTrailingJSON(t *testing.T) {
	raw := validFeedSourceJSON() + `{}`
	if _, err := Load(env(map[string]string{FeedSourcesEnv: raw})); err == nil {
		t.Fatal("Load() accepted trailing JSON")
	}
}


func TestLoadFeedSourcesRejectsConflictingExpectedCurrentPolicyAcrossEndpoints(t *testing.T) {
	var entries []map[string]any
	if err := json.Unmarshal([]byte(validFeedSourceJSON()), &entries); err != nil {
		t.Fatal(err)
	}
	second := map[string]any{}
	for key, value := range entries[0] {
		second[key] = value
	}
	second["endpoint_id"] = "secondary"
	second["expected_current_source_policy_version_id"] = "source-policy-v0"
	entries = append(entries, second)
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(env(map[string]string{FeedSourcesEnv: string(raw)})); err == nil {
		t.Fatal("Load() accepted inconsistent expected current policy versions for one Source")
	}
}
