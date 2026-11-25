package domain

import "time"

// NewsItem 保存当前新闻主体；历史内容快照单独保存在 NewsItemVersion。
type NewsItem struct {
	ID                string
	CanonicalURL      string
	CanonicalURLHash  string
	Title             string
	PublisherName     string
	PublisherDomain   string
	Author             string
	PublishedAt        *time.Time
	Content            ContentLayers
	ContentFingerprint string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}
