package app

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/adapters/parser"
	"example.com/stock-analysis-flow/internal/news/domain"
)

func TestFeedIngestionMappingToVersionOffline(t *testing.T) {
	observedAt := time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC)
	payload := []byte(`<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
		<channel><item>
			<guid>source-guid-1</guid><title>公告标题</title>
			<link>https://publisher.example/article</link>
			<description>来源摘要</description>
			<content:encoded><![CDATA[<p>正文内容</p>]]></content:encoded>
			<author>作者甲</author><pubDate>Wed, 04 Nov 2026 10:30:00 +0800</pubDate>
			<enclosure url="" type="bad mime" length="-1"/>
		</item></channel>
	</rss>`)
	feed, err := parser.ParseFeed(payload, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	entry := feed.Entries[0]
	candidate := domain.NormalizeCandidate(entry.ToCandidate("登记媒体", "publisher.example"), observedAt, "")
	content := entry.ContentLayers("Provider 搜索摘要")
	if candidate.SourceItemID != "source-guid-1" || candidate.ProviderItemID != "" || candidate.Author != "作者甲" {
		t.Fatalf("candidate mapping=%+v", candidate)
	}
	if content.Body == nil || content.Body.Format != domain.ContentFormatHTMLFragment || content.SourceSummary != "来源摘要" || len(content.Attachments) != 1 {
		t.Fatalf("content mapping=%+v", content)
	}

	quality := domain.AssessBusinessQuality(candidate, content, domain.SourceIdentityResolved)
	if quality.Status != domain.BusinessQualityDegraded {
		t.Fatalf("attachment metadata should degrade content quality: %+v", quality)
	}
	foundAttachmentIssue := false
	for _, issue := range quality.Issues {
		if issue.Code == "attachment_missing_url" || issue.Code == "attachment_invalid_media_type" || issue.Code == "attachment_invalid_length" {
			foundAttachmentIssue = true
		}
	}
	if !foundAttachmentIssue {
		t.Fatalf("attachment validation issue missing: %+v", quality.Issues)
	}

	// Feed 内容通过统一正式入库用例持久化；当前 Source Policy 只授权摘要，
	// 因此正文和附件元数据不能因为 Parser 能解析就越权写入版本。
	registration := sourcePersistenceRegistration()
	registration.URLPolicy = domain.SourceURLPolicy{
		AllowedSchemes: []string{"https"}, AllowedHosts: []string{"publisher.example"},
	}
	repository := newSourcePersistenceMemoryRepository(registration)
	ingester, err := NewIngestCollectedSource(
		repository, func() string { return "feed-observation-1" },
		providerIngestCatalogStub{registration: registration},
	)
	if err != nil {
		t.Fatal(err)
	}
	collection := CollectSourceResult{
		CollectionPurpose: domain.PurposeInternalPersonalResearch,
		SourceID:              registration.SourceID,
		EndpointID:            registration.EndpointID,
		SourcePolicyVersionID: registration.SourcePolicyVersionID,
		PolicyEvaluatedAt:     observedAt,
		Entries:               []CollectedSourceEntry{{Candidate: candidate, Content: content}},
	}
	if err := sealCollectSourceResult(&collection); err != nil {
		t.Fatal(err)
	}
	observations, err := ingester.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: collection, IdempotencyPrefix: "feed-poll-1",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || observations[0].IdentityStatus != domain.ObservationNew || observations[0].NewsID == "" {
		t.Fatalf("formal Feed ingest result=%+v", observations)
	}
	if len(repository.newsItems) != 1 || len(repository.versions) != 1 {
		t.Fatalf("formal Feed ingest did not atomically create current item and first version: %+v", repository)
	}
	stored := repository.newsItems[0]
	version := repository.versions[0]
	if stored.Author != "作者甲" || version.Author != "作者甲" || version.VersionNo != 1 {
		t.Fatalf("persisted Feed version metadata: item=%+v version=%+v", stored, version)
	}
	if stored.Content.SourceSummary != "来源摘要" || stored.Content.SourceSummaryOrigin != domain.ContentOriginFeedSummary ||
		version.Content.SourceSummary != "来源摘要" || version.Content.SourceSummaryOrigin != domain.ContentOriginFeedSummary {
		t.Fatalf("authorized Feed summary was not persisted to item and version: item=%+v version=%+v", stored.Content, version.Content)
	}
	if stored.Content.Body != nil || len(stored.Content.Attachments) != 0 ||
		version.Content.Body != nil || len(version.Content.Attachments) != 0 {
		t.Fatalf("unapproved Feed body or attachment metadata was persisted: item=%+v version=%+v", stored.Content, version.Content)
	}
}
