package bootstrap

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

	_ "github.com/go-sql-driver/mysql"
)

func newsFeedStartupConfig(t *testing.T, sourceID, policyVersion, expectedCurrent, evidenceRef string) newsconfig.Config {
	t.Helper()
	entry := map[string]any{
		"source_id": sourceID,
		"endpoint_id": "primary",
		"url": "https://feed.example.test/rss.xml",
		"publisher_name": "Feed Integration Publisher",
		"publisher_domain": "feed.example.test",
		"access_method": "rss",
		"source_policy_version_id": policyVersion,
		"source_policy": map[string]any{
			"status": "approved",
			"allowed_access_methods": []string{"rss"},
			"allowed_purposes": []string{"internal_personal_research"},
			"allowed_content_kinds": []string{"abstract"},
			"allowed_storage_modes": []string{"transient", "persistent_excerpt"},
			"allowed_metadata_fields": []string{},
			"max_retention_seconds": 86400,
			"effective_at": time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano),
			"evidence_ref": evidenceRef,
		},
		"url_policy": map[string]any{
			"allowed_schemes": []string{"https"},
			"allowed_hosts": []string{"feed.example.test"},
			"allowed_ports": []int{443},
		},
		"max_response_bytes": 1048576,
	}
	if expectedCurrent != "" {
		entry["expected_current_source_policy_version_id"] = expectedCurrent
	}
	raw, err := json.Marshal([]any{entry})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == newsconfig.FeedSourcesEnv {
			return string(raw), true
		}
		return "", false
	}))
	if err != nil {
		t.Fatalf("load Feed startup configuration: %v", err)
	}
	return cfg
}

func TestNewsFeedStageCompositionMariaDBPublishesOnlyExplicitForwardPolicyTransitions(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News Feed Server composition MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("MariaDB ping: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, newsstorage.MigrationNamespace, newsstorage.Migrations()); err != nil {
		t.Fatalf("apply News migrations: %v", err)
	}
	if err := newsstorage.ValidateSchema(ctx, db); err != nil {
		t.Fatalf("validate News schema: %v", err)
	}
	repository, err := newsstorage.NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	client, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()

	sourceID := fmt.Sprintf("feed-startup-%d", time.Now().UnixNano())
	initial := newsFeedStartupConfig(t, sourceID, "source-policy-v1", "", "operator-authorization:"+sourceID+":v1")
	first, err := newNewsFeedStageComposition(ctx, initial, client, repository)
	if err != nil {
		t.Fatalf("compose first trusted Feed registration: %v", err)
	}
	if first == nil || first.executor == nil || len(first.resourceRefs) != 1 {
		t.Fatalf("first Feed composition is incomplete: %+v", first)
	}

	forward := newsFeedStartupConfig(t, sourceID, "source-policy-v2", "source-policy-v1", "operator-authorization:"+sourceID+":v2")
	second, err := newNewsFeedStageComposition(ctx, forward, client, repository)
	if err != nil {
		t.Fatalf("compose explicitly advanced Source policy: %v", err)
	}
	if second == nil || len(second.resourceRefs) != 1 || second.resourceRefs[0] == first.resourceRefs[0] {
		t.Fatal("policy version transition did not produce a new immutable Run resource identity")
	}

	stale := newsFeedStartupConfig(t, sourceID, "source-policy-v1", "", "operator-authorization:"+sourceID+":v1")
	if _, err := newNewsFeedStageComposition(ctx, stale, client, repository); err == nil {
		t.Fatal("stale startup configuration moved the current Source policy pointer backwards")
	}
	var currentVersion string
	if err := db.QueryRowContext(ctx, "SELECT policy_version FROM news_current_policy_versions WHERE policy_type = ? AND policy_ref = ?", "source", sourceID).Scan(&currentVersion); err != nil {
		t.Fatal(err)
	}
	if currentVersion != "source-policy-v2" {
		t.Fatalf("current Source policy = %q, want source-policy-v2", currentVersion)
	}
}
