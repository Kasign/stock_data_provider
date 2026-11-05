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
