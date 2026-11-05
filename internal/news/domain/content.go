package domain

import "strings"

// ContentOrigin 标识内容片段来自哪个采集层，不代表来源授权状态。
type ContentOrigin string

const (
	ContentOriginFeedSummary ContentOrigin = "feed_summary"
	ContentOriginFeedBody    ContentOrigin = "feed_body"
	ContentOriginHTMLMeta    ContentOrigin = "html_meta"
	ContentOriginHTMLArticle ContentOrigin = "html_article"
)

// ContentFormat 描述载荷的实际编码形式。
type ContentFormat string

const (
	ContentFormatUnknown      ContentFormat = "unknown"
	ContentFormatPlainText    ContentFormat = "plain_text"
	ContentFormatHTMLFragment ContentFormat = "html_fragment"
)

// ContentAttachmentMetadata 保存 Feed 声明的附件元数据，不代表附件已下载或获准保存。
type ContentAttachmentMetadata struct {
	URL              string
	RawURL           string
	MediaType        string
	RawMediaType     string
	RawLength        string
	Length           *int64
	Title            string
	ValidationIssues []string
}

// ContentPayload 保存来源正文/片段。Kind 未确定时保持空值，不推断为全文。
type ContentPayload struct {
	Origin ContentOrigin
	Format ContentFormat
	Kind   ContentKind
	Text   string
}

// ContentLayers 将搜索 Provider 摘要、发布来源摘要和正文载荷分开保存。
type ContentLayers struct {
	ProviderSummary     string
	SourceSummary       string
	SourceSummaryOrigin ContentOrigin
	Body                *ContentPayload
	Attachments         []ContentAttachmentMetadata
}

// ContentLayersFromCandidate 只把 Provider 的搜索摘要放入 Provider 层，不提升为来源摘要或正文。
func ContentLayersFromCandidate(candidate Candidate) ContentLayers {
	return NormalizeContentLayers(ContentLayers{ProviderSummary: candidate.ProviderSummary})
}

// NormalizeContentLayers 规范化摘要空白并仅裁剪正文首尾空白，保留正文段落和重复内容。
func NormalizeContentLayers(layers ContentLayers) ContentLayers {
	layers.ProviderSummary = normalizeCandidateText(layers.ProviderSummary)
	layers.SourceSummary = normalizeCandidateText(layers.SourceSummary)
	if len(layers.Attachments) > 0 {
		attachments := make([]ContentAttachmentMetadata, len(layers.Attachments))
		for i, attachment := range layers.Attachments {
			attachments[i] = attachment
			attachments[i].ValidationIssues = append([]string(nil), attachment.ValidationIssues...)
			if attachment.Length != nil {
				length := *attachment.Length
				attachments[i].Length = &length
			}
		}
		layers.Attachments = attachments
	}
	if layers.Body != nil {
		body := *layers.Body
		body.Text = strings.TrimSpace(body.Text)
		if body.Text == "" {
			layers.Body = nil
		} else {
			layers.Body = &body
		}
	}
	return layers
}
