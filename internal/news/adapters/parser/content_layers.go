package parser

import "example.com/stock-analysis-flow/internal/news/domain"

// ContentLayers 将 Feed 的 Provider 摘要、来源摘要和正文映射到独立层。
// Feed 内容格式尚未可靠识别时标记 unknown，不擅自按全文或 HTML 处理。
func (entry FeedEntry) ContentLayers(providerSummary string) domain.ContentLayers {
	layers := domain.ContentLayers{ProviderSummary: providerSummary}
	if entry.Summary != "" {
		layers.SourceSummary = entry.Summary
		layers.SourceSummaryOrigin = domain.ContentOriginFeedSummary
	}
	if entry.Content != "" {
		layers.Body = &domain.ContentPayload{
			Origin: domain.ContentOriginFeedBody,
			Format: domain.ContentFormatUnknown,
			Text:   entry.Content,
		}
	}
	return domain.NormalizeContentLayers(layers)
}

// ContentLayers 将 HTML 元信息描述与提取的文章文本分开，不把 description 当作正文。
func (doc HTMLDocument) ContentLayers(providerSummary string) domain.ContentLayers {
	layers := domain.ContentLayers{ProviderSummary: providerSummary}
	if doc.Description != "" {
		layers.SourceSummary = doc.Description
		layers.SourceSummaryOrigin = domain.ContentOriginHTMLMeta
	}
	if doc.Text != "" {
		layers.Body = &domain.ContentPayload{
			Origin: domain.ContentOriginHTMLArticle,
			Format: domain.ContentFormatPlainText,
			Text:   doc.Text,
		}
	}
	return domain.NormalizeContentLayers(layers)
}
