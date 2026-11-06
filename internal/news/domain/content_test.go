package domain

import "testing"

func TestContentLayersKeepProviderAndSourceSummariesSeparate(t *testing.T) {
	layers := ContentLayersFromCandidate(Candidate{ProviderSummary: " Provider   摘要 "})
	if layers.ProviderSummary != "Provider 摘要" || layers.SourceSummary != "" || layers.Body != nil {
		t.Fatalf("layers=%+v", layers)
	}
}

func TestNormalizeContentLayersPreservesBodyParagraphsAndOrigin(t *testing.T) {
	layers := NormalizeContentLayers(ContentLayers{
		ProviderSummary: " Provider   摘要 ",
		SourceSummary:   " 来源\n摘要 ",
		Body: &ContentPayload{
			Origin: ContentOriginHTMLArticle,
			Format: ContentFormatPlainText,
			Text:   "  第一段\n\n重复段落\n重复段落  ",
		},
	})
	if layers.ProviderSummary != "Provider 摘要" || layers.SourceSummary != "来源 摘要" {
		t.Fatalf("summaries=%+v", layers)
	}
	if layers.Body == nil || layers.Body.Text != "第一段\n\n重复段落\n重复段落" {
		t.Fatalf("body=%+v", layers.Body)
	}
	if layers.Body.Origin != ContentOriginHTMLArticle || layers.Body.Format != ContentFormatPlainText || layers.Body.Kind != "" {
		t.Fatalf("body metadata=%+v", layers.Body)
	}
}

func TestNormalizeContentLayersClonesAttachmentMetadata(t *testing.T) {
	length := int64(10)
	issues := []string{"invalid_media_type"}
	layers := NormalizeContentLayers(ContentLayers{Attachments: []ContentAttachmentMetadata{{RawURL: " raw ", URL: "raw", Length: &length, ValidationIssues: issues}}})
	layers.Attachments[0].ValidationIssues[0] = "changed"
	*layers.Attachments[0].Length = 20
	if issues[0] != "invalid_media_type" || length != 10 {
		t.Fatal("attachment metadata aliases caller-owned memory")
	}
}

func TestNormalizeContentLayersPreservesRawExternalBodyMetadata(t *testing.T) {
	layers := NormalizeContentLayers(ContentLayers{
		BodyDeclaredTypeRaw: " html ", ExternalBodyReference: " https://publisher.example/body ",
	})
	if layers.Body != nil || layers.BodyDeclaredTypeRaw != " html " || layers.ExternalBodyReference != " https://publisher.example/body " {
		t.Fatalf("external reference metadata=%+v", layers)
	}
}

func TestNormalizeContentLayersDoesNotPromoteSummaryToBody(t *testing.T) {
	layers := NormalizeContentLayers(ContentLayers{SourceSummary: "来源摘要"})
	if layers.Body != nil || layers.ProviderSummary != "" {
		t.Fatalf("layers=%+v", layers)
	}
}
func TestNormalizeContentLayersCanonicalizesCRLFWithoutChangingParagraphs(t *testing.T) {
	crlf := NormalizeContentLayers(ContentLayers{
		Body: &ContentPayload{Text: "  第一段\r\n\r\n第二段\r\n  "},
	})
	lf := NormalizeContentLayers(ContentLayers{
		Body: &ContentPayload{Text: "第一段\n\n第二段"},
	})
	if crlf.Body == nil || crlf.Body.Text != "第一段\n\n第二段" {
		t.Fatalf("CRLF body was not canonicalized: %+v", crlf.Body)
	}
	if lf.Body == nil || lf.Body.Text != crlf.Body.Text {
		t.Fatalf("equivalent line endings produced different stored bodies: CRLF=%+v LF=%+v", crlf.Body, lf.Body)
	}
}

func TestRestrictContentLayersHonorsRequestedContentCeiling(t *testing.T) {
	layers := ContentLayers{
		ProviderSummary:       "provider summary",
		SourceSummary:         "source summary",
		SourceSummaryOrigin:   ContentOriginFeedSummary,
		Body:                  &ContentPayload{Origin: ContentOriginFeedBody, Format: ContentFormatPlainText, Text: "unclassified body"},
		BodyDeclaredTypeRaw:   "text/html",
		ExternalBodyReference: "https://publisher.example/body",
		Attachments:           []ContentAttachmentMetadata{{URL: "https://publisher.example/file.pdf"}},
	}

	metadata := RestrictContentLayers(layers, ContentMetadata)
	if metadata.ProviderSummary != "provider summary" || metadata.SourceSummary != "" ||
		metadata.Body != nil || metadata.BodyDeclaredTypeRaw != "" ||
		metadata.ExternalBodyReference != "" || len(metadata.Attachments) != 0 {
		t.Fatalf("metadata request exposed higher-level content: %+v", metadata)
	}

	abstract := RestrictContentLayers(layers, ContentAbstract)
	if abstract.SourceSummary != "source summary" || abstract.Body != nil ||
		abstract.ExternalBodyReference != "" || len(abstract.Attachments) != 0 {
		t.Fatalf("abstract request exposed content above summary: %+v", abstract)
	}

	fullText := RestrictContentLayers(layers, ContentFullText)
	if fullText.SourceSummary != "source summary" || fullText.Body != nil ||
		fullText.ExternalBodyReference != "" || len(fullText.Attachments) != 0 {
		t.Fatalf("unknown body kind must not be promoted to full text: %+v", fullText)
	}

	attachments := RestrictContentLayers(layers, ContentAttachment)
	if len(attachments.Attachments) != 1 || attachments.ProviderSummary != "" || attachments.SourceSummary != "" || attachments.Body != nil {
		t.Fatalf("attachment request must expose attachment metadata only: %+v", attachments)
	}
}

func TestRestrictContentLayersKeepsOnlyClassifiedBodyWithinCeiling(t *testing.T) {
	layers := ContentLayers{
		SourceSummary: "summary",
		Body: &ContentPayload{Kind: ContentSnippet, Text: "snippet"},
	}
	snippet := RestrictContentLayers(layers, ContentSnippet)
	if snippet.Body == nil || snippet.Body.Text != "snippet" {
		t.Fatalf("classified snippet should remain available: %+v", snippet)
	}
	abstract := RestrictContentLayers(layers, ContentAbstract)
	if abstract.Body != nil {
		t.Fatalf("snippet must not be returned for abstract-only request: %+v", abstract)
	}
}
