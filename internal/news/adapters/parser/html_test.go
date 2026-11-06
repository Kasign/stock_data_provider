package parser

import (
	"strings"
	"testing"
	"time"
)

func TestParseHTMLDocumentExtractsArticleAndMetadata(t *testing.T) {
	payload := []byte(`<!doctype html>
<html><head>
<title>页面标题</title>
<meta property="og:title" content="Open Graph 标题">
<meta name="description" content="页面描述">
<meta property="article:published_time" content="2026-11-04T09:30:00+08:00">
<link rel="canonical" href="https://publisher.example/canonical">
</head><body>
<nav>导航内容</nav>
<main><article><h1>文章标题</h1><p>第一段 &amp; 内容。</p>
<script>不应出现的脚本</script><p>第二段内容。</p>
<p>重复段落</p><p>重复段落</p></article></main>
<footer>页脚内容</footer>
</body></html>`)
	doc, err := ParseHTMLDocument(payload, "text/html; charset=utf-8", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "Open Graph 标题" || doc.Description != "页面描述" {
		t.Fatalf("metadata=%+v", doc)
	}
	if doc.CanonicalURL != "https://publisher.example/canonical" {
		t.Fatalf("canonical_url=%q", doc.CanonicalURL)
	}
	if doc.PublishedAt == nil || doc.PublishedAt.Hour() != 1 {
		t.Fatalf("published_at=%v raw=%q", doc.PublishedAt, doc.PublishedAtRaw)
	}
	if !strings.Contains(doc.Text, "第一段 & 内容。") || !strings.Contains(doc.Text, "第二段内容。") {
		t.Fatalf("text=%q", doc.Text)
	}
	if strings.Count(doc.Text, "重复段落") != 2 {
		t.Fatalf("repeated source paragraphs were lost: %q", doc.Text)
	}
	for _, excluded := range []string{"导航内容", "不应出现的脚本", "页脚内容"} {
		if strings.Contains(doc.Text, excluded) {
			t.Fatalf("unexpected text %q in %q", excluded, doc.Text)
		}
	}
}

func TestHTMLDocumentContentLayersDoNotPromoteDescriptionToBody(t *testing.T) {
	doc, err := ParseHTMLDocument([]byte(`<html><head><meta name="description" content="页面摘要"></head>
	<body><article><p>文章正文</p></article></body></html>`), "text/html", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	layers := doc.ContentLayers("搜索摘要")
	if layers.ProviderSummary != "搜索摘要" || layers.SourceSummary != "页面摘要" || layers.SourceSummaryOrigin != "html_meta" {
		t.Fatalf("layers=%+v", layers)
	}
	if layers.Body == nil || layers.Body.Text != "文章正文" || layers.Body.Origin != "html_article" || layers.Body.Format != "plain_text" {
		t.Fatalf("body=%+v", layers.Body)
	}
}

func TestParseHTMLDocumentConvertsDeclaredLegacyCharset(t *testing.T) {
	payload := []byte("<html><body><article><p>caf\xe9</p></article></body></html>")
	doc, err := ParseHTMLDocument(payload, "text/html; charset=windows-1252", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Text, "café") {
		t.Fatalf("text=%q", doc.Text)
	}
}

func TestParseHTMLDocumentConvertsGBKCharset(t *testing.T) {
	payload := []byte("<html><body><article><p>\xD6\xD0\xCE\xC4</p></article></body></html>")
	doc, err := ParseHTMLDocument(payload, "text/html; charset=gbk", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(doc.Text, "中文") {
		t.Fatalf("text=%q", doc.Text)
	}
}

func TestParseHTMLDocumentFallsBackToBodyAndKeepsUnknownDateRaw(t *testing.T) {
	payload := []byte(`<html><head><meta name="datePublished" content="2026-11-04"></head>
<body><p>正文一</p><p>正文二</p></body></html>`)
	doc, err := ParseHTMLDocument(payload, "text/html", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if doc.PublishedAt != nil || doc.PublishedAtRaw != "2026-11-04" {
		t.Fatalf("published_at=%v raw=%q", doc.PublishedAt, doc.PublishedAtRaw)
	}
	if !strings.Contains(doc.Text, "正文一") || !strings.Contains(doc.Text, "正文二") {
		t.Fatalf("text=%q", doc.Text)
	}
}

func TestParseHTMLDocumentRejectsInvalidInputAndNonHTMLContentType(t *testing.T) {
	if _, err := ParseHTMLDocument(nil, "text/html", time.Now()); err != ErrInvalidHTML {
		t.Fatalf("empty input error=%v", err)
	}
	if _, err := ParseHTMLDocument([]byte("<html/>"), "application/pdf", time.Now()); err != ErrUnsupportedHTML {
		t.Fatalf("content type error=%v", err)
	}
	if _, err := ParseHTMLDocument([]byte("<html/>"), "invalid content type", time.Now()); err == nil {
		t.Fatal("expected invalid content type error")
	}
	if _, err := ParseHTMLDocument([]byte("<html/>"), "text/html", time.Time{}); err != ErrInvalidHTML {
		t.Fatalf("zero observed time error=%v", err)
	}
}
