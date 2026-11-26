package stage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sourceadapter "example.com/stock-analysis-flow/internal/news/adapters/source"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

	_ "github.com/go-sql-driver/mysql"
)

type feedStageMariaDBTransport struct {
	body string
}

func (t feedStageMariaDBTransport) Do(_ context.Context, _ string, _ string, request *http.Request) (*http.Response, error) {
	if request == nil || request.URL == nil || request.URL.Host != "feed.example" {
		return nil, errors.New("unexpected Feed Stage integration request")
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/rss+xml; charset=utf-8"}},
		Body:       io.NopCloser(strings.NewReader(t.body)),
		Request:    request,
	}, nil
}

func TestNewsFeedCollectionAdapterMariaDBIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News Feed Stage MariaDB integration")
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

	sourceID := fmt.Sprintf("feed-stage-%d", time.Now().UnixNano())
	endpointID := "rss-primary"
	now := time.Now().UTC()
	policy := domain.SourcePolicy{
		Status:               domain.AuthorizationApproved,
		AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
		AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:  []domain.ContentKind{domain.ContentAbstract},
		AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient, domain.StoragePersistentExcerpt},
		MaxRetention:         24 * time.Hour,
		EffectiveAt:          now.Add(-time.Minute),
		EvidenceRef:          "operator-authorization:" + sourceID,
	}
	policyPayload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := newsapp.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.ExecuteIfCurrent(ctx, domain.NewsPolicyTypeSource, sourceID, "source-policy-v1", "", policyPayload); err != nil {
		t.Fatalf("publish initial Source policy: %v", err)
	}

	registration := newsapp.SourceEndpointRegistration{
		SourceID: sourceID, EndpointID: endpointID, Enabled: true,
		URL: "https://feed.example/rss", PublisherName: "Integration Publisher",
		PublisherDomain: "publisher.example", AccessMethod: domain.AccessRSS,
		SourcePolicy: policy, SourcePolicyVersionID: "source-policy-v1",
		URLPolicy: domain.SourceURLPolicy{
			AllowedSchemes: []string{"https"},
			AllowedHosts:   []string{"feed.example", "news.example"},
			AllowedPorts:   []uint16{443},
		},
		MaxResponseBytes: 1 << 20,
	}
	catalog, err := sourceadapter.NewStaticSourceCatalog([]newsapp.SourceEndpointRegistration{registration})
	if err != nil {
		t.Fatalf("create StaticSourceCatalog: %v", err)
	}
	feedBody := `<?xml version="1.0"?>
		<rss version="2.0"><channel><title>Integration Feed</title>
		<item><guid isPermaLink="false">native-article-1</guid>
		<title>真实 Feed 入库验证</title>
		<link>https://news.example/article/1</link>
		<description>仅授权摘要应被持久化</description>
		<pubDate>Wed, 25 Nov 2026 10:00:00 GMT</pubDate></item>
		</channel></rss>`
	collector, err := newsapp.NewCollectSourceWithPolicyResolver(
		catalog, feedStageMariaDBTransport{body: feedBody}, repository,
	)
	if err != nil {
		t.Fatalf("create CollectSource: %v", err)
	}
	var sequence atomic.Uint64
	ingester, err := newsapp.NewIngestCollectedSource(repository, func() string {
		return fmt.Sprintf("feed-stage-id-%d-%d", now.UnixNano(), sequence.Add(1))
	}, catalog)
	if err != nil {
		t.Fatalf("create IngestCollectedSource: %v", err)
	}
	adapter, err := NewFeedCollectionAdapter([]newsapp.SourceEndpointRegistration{registration}, collector, ingester)
	if err != nil {
		t.Fatalf("create FeedCollectionAdapter: %v", err)
	}
	resourceRef := SourceEndpointSnapshotRef(registration)
	result, err := adapter.Execute(ctx, "run-feed-stage", CollectFeedsStageCode, []string{resourceRef})
	if err != nil {
		t.Fatalf("execute formal Feed collection Stage: %v", err)
	}
	if result.OutputType != CollectFeedsOutputType || result.SchemaVersion != CollectFeedsSchemaVersion ||
		result.Retryable || len(result.ReasonCodes) != 0 {
		t.Fatalf("Feed Stage result contract = %+v", result)
	}
	if err := ValidateFeedCollectionPayload(result.Payload); err != nil {
		t.Fatalf("validate Feed Stage output: %v", err)
	}
	if err := ValidateFeedCollectionQualitySummary(result.QualitySummary); err != nil {
		t.Fatalf("validate Feed Stage quality summary: %v", err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "success" || output.SourceCount != 1 || output.CollectedEntryCount != 1 ||
		output.IngestedCount != 1 || len(output.Sources) != 1 || len(output.Sources[0].NewsIDs) != 1 {
		t.Fatalf("unexpected Feed Stage output: %+v", output)
	}
	if strings.Contains(string(result.Payload), "https://") || strings.Contains(string(result.Payload), "仅授权摘要") {
		t.Fatalf("Feed StageOutput leaked URL or article content: %s", result.Payload)
	}

	newsID := output.Sources[0].NewsIDs[0]
	var itemCount, versionCount, observationCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_items WHERE news_id = ? AND canonical_url = ?",
		newsID, "https://news.example/article/1").Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_item_versions WHERE news_id = ?", newsID).Scan(&versionCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_observations WHERE news_id = ? AND source_id = ?", newsID, sourceID).Scan(&observationCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 1 || versionCount != 1 || observationCount != 1 {
		t.Fatalf("Feed Stage persisted rows: items=%d versions=%d observations=%d", itemCount, versionCount, observationCount)
	}

	var currentContentJSON, versionContentJSON []byte
	if err := db.QueryRowContext(ctx, "SELECT content_json FROM news_items WHERE news_id = ?", newsID).Scan(&currentContentJSON); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, "SELECT content_json FROM news_item_versions WHERE news_id = ? ORDER BY version_no DESC LIMIT 1", newsID).Scan(&versionContentJSON); err != nil {
		t.Fatal(err)
	}
	for label, raw := range map[string][]byte{"current item": currentContentJSON, "latest version": versionContentJSON} {
		var content domain.ContentLayers
		if err := json.Unmarshal(raw, &content); err != nil {
			t.Fatalf("decode %s content: %v", label, err)
		}
		if content.SourceSummary != "仅授权摘要应被持久化" || content.Body != nil || len(content.Attachments) != 0 {
			t.Fatalf("%s content violated abstract-only Source Policy: %+v", label, content)
		}
	}
}
