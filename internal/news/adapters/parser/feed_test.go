package parser

import (
	"errors"
	"testing"
	"time"
)

func TestParseRSSFeed(t *testing.T) {
	observedAt := time.Date(2026, 11, 5, 8, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	payload := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:dc="http://purl.org/dc/elements/1.1/">
<channel><title> 市场 新闻 </title><link>https://publisher.example</link>
<item><guid> item-1 </guid><title> 标题   一 </title><link>https://publisher.example/a</link>
<description>摘要 内容</description><content:encoded><![CDATA[<p>正文片段</p>]]></content:encoded>
<dc:creator>作者</dc:creator><pubDate>Wed, 04 Nov 2026 10:30:00 +0800</pubDate></item>
</channel></rss>`)
	doc, err := ParseFeed(payload, observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != FeedRSS || doc.Title != "市场 新闻" || doc.Link != "https://publisher.example" || len(doc.Entries) != 1 {
		t.Fatalf("document=%+v", doc)
	}
	entry := doc.Entries[0]
	if entry.ID != "item-1" || entry.Title != "标题 一" || entry.Summary != "摘要 内容" || entry.Content != "<p>正文片段</p>" || entry.Author != "作者" {
		t.Fatalf("entry=%+v", entry)
	}
	wantPublishedAt := time.Date(2026, 11, 4, 2, 30, 0, 0, time.UTC)
	if entry.PublishedAt == nil || !entry.PublishedAt.Equal(wantPublishedAt) {
		t.Fatalf("published_at=%v", entry.PublishedAt)
	}
	if len(entry.MissingFields) != 0 || doc.ObservedAt.Location() != time.UTC {
		t.Fatalf("entry=%+v observed_at=%v", entry, doc.ObservedAt.Location())
	}
}

func TestParseAtomFeedPrefersAlternateLinkAndPublishedTime(t *testing.T) {
	payload := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
<title>Atom Feed</title><link rel="self" href="https://publisher.example/feed.xml"/>
<link rel="alternate" href="https://publisher.example/"/>
<entry><id>tag:publisher.example,2026:item-2</id><title>Atom 标题</title>
<link rel="enclosure" href="https://publisher.example/file.pdf"/>
<link rel="alternate" href="https://publisher.example/article"/>
<summary>摘要</summary><content>正文片段</content><published>2026-11-04T09:30:00+08:00</published>
<updated>2026-11-04T10:00:00+08:00</updated><author><name>编辑</name></author></entry>
</feed>`)
	doc, err := ParseFeed(payload, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != FeedAtom || doc.Link != "https://publisher.example/" || len(doc.Entries) != 1 {
		t.Fatalf("document=%+v", doc)
	}
	entry := doc.Entries[0]
	if entry.Link != "https://publisher.example/article" || entry.PublishedAtRaw != "2026-11-04T09:30:00+08:00" || entry.Author != "编辑" {
		t.Fatalf("entry=%+v", entry)
	}
	wantPublishedAt := time.Date(2026, 11, 4, 1, 30, 0, 0, time.UTC)
	if entry.PublishedAt == nil || !entry.PublishedAt.Equal(wantPublishedAt) {
		t.Fatalf("published_at=%v", entry.PublishedAt)
	}
}

func TestParseFeedKeepsUnzonedTimeUnparsedAndReportsMissingFields(t *testing.T) {
	payload := []byte(`<rss><channel><item><title>标题</title><pubDate>2026-11-04</pubDate></item></channel></rss>`)
	doc, err := ParseFeed(payload, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	entry := doc.Entries[0]
	if entry.PublishedAt != nil || entry.PublishedAtRaw != "2026-11-04" {
		t.Fatalf("entry=%+v", entry)
	}
	if len(entry.MissingFields) != 4 {
		t.Fatalf("missing_fields=%v", entry.MissingFields)
	}
}

func TestParseAtomFeedDoesNotTreatUpdatedAsPublished(t *testing.T) {
	payload := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry>
		<id>item-3</id><title>更新内容</title><link href="https://publisher.example/a"/>
		<updated>2026-11-04T10:00:00+08:00</updated>
	</entry></feed>`)
	doc, err := ParseFeed(payload, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	entry := doc.Entries[0]
	if entry.PublishedAt != nil || entry.PublishedAtRaw != "" {
		t.Fatalf("published_at must remain absent: %+v", entry)
	}
	if entry.UpdatedAt == nil || entry.UpdatedAt.Hour() != 2 || entry.UpdatedAtRaw == "" {
		t.Fatalf("updated_at=%+v raw=%q", entry.UpdatedAt, entry.UpdatedAtRaw)
	}
	if len(entry.MissingFields) != 2 || entry.MissingFields[0] != "published_at" || entry.MissingFields[1] != "content" {
		t.Fatalf("missing_fields=%v", entry.MissingFields)
	}
}

func TestParseFeedDoesNotInferTimezoneForRFCDate(t *testing.T) {
	for _, raw := range []string{"2026-11-04", "Wed, 04 Nov 2026 10:30:00", "Wed, 04 Nov 2026 10:30:00 CST"} {
		if _, ok := parseFeedTime(raw); ok {
			t.Fatalf("unexpected timezone inference for %q", raw)
		}
	}
}

func TestFeedEntryContentLayersKeepSummaryAndBodySeparate(t *testing.T) {
	doc, err := ParseFeed([]byte(`<rss><channel><item>
		<title>标题</title><description>来源摘要</description>
		<content:encoded xmlns:content="http://purl.org/rss/1.0/modules/content/"><![CDATA[<p>Feed 正文</p>]]></content:encoded>
	</item></channel></rss>`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	layers := doc.Entries[0].ContentLayers("Provider 搜索摘要")
	if layers.ProviderSummary != "Provider 搜索摘要" || layers.SourceSummary != "来源摘要" || layers.SourceSummaryOrigin != "feed_summary" {
		t.Fatalf("layers=%+v", layers)
	}
	if layers.Body == nil || layers.Body.Text != "<p>Feed 正文</p>" || layers.Body.Origin != "feed_body" || layers.Body.Format != "unknown" || layers.Body.Kind != "" {
		t.Fatalf("body=%+v", layers.Body)
	}
}

func TestParseFeedAllowsValidEmptyFeed(t *testing.T) {
	doc, err := ParseFeed([]byte(`<rss version="2.0"><channel><title>Empty</title></channel></rss>`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != FeedRSS || doc.Entries == nil || len(doc.Entries) != 0 {
		t.Fatalf("document=%+v", doc)
	}
}

func TestParseFeedRejectsInvalidAndUnsupportedDocuments(t *testing.T) {
	cases := []struct {
		name string
		data string
		want error
	}{
		{name: "malformed", data: "<rss><channel>", want: ErrInvalidFeed},
		{name: "missing rss channel", data: "<rss/>", want: ErrInvalidFeed},
		{name: "multiple roots", data: "<rss><channel/></rss><rss><channel/></rss>", want: ErrInvalidFeed},
		{name: "unsupported", data: "<html/>", want: ErrUnsupportedFeed},
		{name: "empty", data: "  ", want: ErrInvalidFeed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFeed([]byte(tc.data), time.Now())
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
		})
	}
	if _, err := ParseFeed([]byte("<rss/>"), time.Time{}); !errors.Is(err, ErrInvalidFeed) {
		t.Fatalf("zero observed time error=%v", err)
	}
}

func TestParseFeedAcceptsUTF8BOM(t *testing.T) {
	payload := append([]byte{0xEF, 0xBB, 0xBF}, []byte("<rss><channel/></rss>")...)
	doc, err := ParseFeed(payload, time.Now())
	if err != nil || doc.Format != FeedRSS {
		t.Fatalf("document=%+v error=%v", doc, err)
	}
}

func TestParseRSSFeedExtractsEnclosureMetadata(t *testing.T) {
	payload := []byte(`<rss version="2.0"><channel><item><title>公告</title><enclosure url="https://publisher.example/report.pdf" type="application/pdf; version=1" length="2048"/></item></channel></rss>`)
	doc, err := ParseFeed(payload, time.Now())
	if err != nil { t.Fatal(err) }
	attachments := doc.Entries[0].Attachments
	if len(attachments) != 1 { t.Fatalf("attachments=%+v", attachments) }
	got := attachments[0]
	if got.URL != "https://publisher.example/report.pdf" || got.MediaType != "application/pdf" || got.Length == nil || *got.Length != 2048 { t.Fatalf("attachment=%+v", got) }
}

func TestParseAtomFeedExtractsEnclosureAndIgnoresInvalidLength(t *testing.T) {
	payload := []byte(`<feed xmlns="http://www.w3.org/2005/Atom"><entry><link rel="enclosure" href="https://publisher.example/audio.mp3" type="audio/mpeg" length="-1" title="音频"/></entry></feed>`)
	doc, err := ParseFeed(payload, time.Now())
	if err != nil { t.Fatal(err) }
	attachments := doc.Entries[0].Attachments
	if len(attachments) != 1 { t.Fatalf("attachments=%+v", attachments) }
	got := attachments[0]
	if got.URL != "https://publisher.example/audio.mp3" || got.MediaType != "audio/mpeg" || got.RawMediaType != "audio/mpeg" || got.Length != nil || got.RawLength != "-1" || got.Title != "音频" { t.Fatalf("attachment=%+v", got) }
}
