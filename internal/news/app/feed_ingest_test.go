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

	identityRepository := &memoryIdentityRepository{}
	ingest := NewIngestCandidate(identityRepository, func() string { return "feed-observation-1" })
	observation, err := ingest.Execute(context.Background(), "source-1", "feed-poll-1/source-guid-1", candidate,
		domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"publisher.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationNew || observation.NewsID == "" {
		t.Fatalf("identity result=%+v", observation)
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

	researchRepository := &memoryResearchRepository{
		observations: map[string]domain.Observation{observation.ID: observation},
		versions:     map[string][]domain.NewsItemVersion{},
	}
	research := NewApplyResearchUpdate(researchRepository, func() string { return "feed-version-1" })
	research.now = func() time.Time { return observedAt }
	result, err := research.Execute(context.Background(), ResearchUpdateRequest{
		NewsID: observation.NewsID, Candidate: observation.Candidate, Content: content, ObservationID: observation.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.VersionCreated || result.Version.VersionNo != 1 || result.Version.Author != "作者甲" {
		t.Fatalf("version metadata=%+v", result.Version)
	}
	if result.Version.Content.Body == nil || len(result.Version.Content.Attachments) != 1 {
		t.Fatalf("version content=%+v", result.Version.Content)
	}
}
